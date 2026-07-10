package sync

import (
	"context"
	"fmt"
	"reflect"

	"github.com/alextorq/dns-filter/source/db"
)

type Logger interface {
	Debug(args ...any)
	Error(err error)
}

// SourceLister is the narrow read port over the sources table.
type SourceLister interface {
	GetAllActive() ([]db.Source, error)
}

// Loader is the per-source input port. Concrete AdBlock/hosts adapters own
// their HTTP client, endpoint and parser; the sync use-case only selects one
// by source identity.
type Loader interface {
	Load(context.Context) ([]string, error)
}

type LoaderRegistry map[db.BlockListSource]Loader

var remoteSources = [...]db.BlockListSource{
	db.SourceEasyList,
	db.SourceRuAdList,
	db.SourceAdGuardRussian,
	db.SourceStevenBlack,
	db.SourceHaGeZiMulti,
}

// NewLoaderRegistry validates the complete remote-source set and copies it so
// callers cannot change a running Module by mutating their input map.
func NewLoaderRegistry(loaders LoaderRegistry) (LoaderRegistry, error) {
	for _, source := range remoteSources {
		loader, ok := loaders[source]
		if !ok || isNilLoader(loader) {
			return nil, fmt.Errorf("source sync: loader for %s is required", source)
		}
	}
	result := make(LoaderRegistry, len(loaders))
	for source, loader := range loaders {
		result[source] = loader
	}
	return result, nil
}

func isRemoteSource(source db.BlockListSource) bool {
	for _, candidate := range remoteSources {
		if source == candidate {
			return true
		}
	}
	return false
}

func isNilLoader(loader Loader) bool {
	if loader == nil {
		return true
	}
	v := reflect.ValueOf(loader)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// BlockWriter is the narrow write port over the blocklist. CreateDNSRecordsByDomains
// adds the freshly pulled domains; DeleteDNSRecordsBySourceNotIn prunes the ones
// that vanished from every source (see pruneVanishedDomains).
type BlockWriter interface {
	CreateDNSRecordsByDomainsContext(ctx context.Context, urls []string, source string) error
	DeleteDNSRecordsBySourceNotInContext(ctx context.Context, source string, keep []string) error
}

type DomainBySource struct {
	Source  db.BlockListSource
	Domains []string
}

// LoadAndParseActiveSources downloads + parses every enabled source. Network /
// parser errors are logged and the source skipped so a single bad source does
// not abort the whole batch. complete reports whether every attempted source
// loaded cleanly — when false the prune phase must be skipped, since the union
// of fresh domains is incomplete and would delete domains a failed source
// still lists.
func LoadAndParseActiveSources(ctx context.Context, repo SourceLister, loaders LoaderRegistry, log Logger) (result []DomainBySource, complete bool) {
	result = make([]DomainBySource, 0)
	if ctx.Err() != nil {
		return result, false
	}

	items, err := repo.GetAllActive()
	if err != nil {
		log.Error(err)
		return result, false
	}

	complete = true

	for _, item := range items {
		if ctx.Err() != nil {
			return result, false
		}
		loader, ok := loaders[item.Name]
		if !ok || isNilLoader(loader) {
			if isRemoteSource(item.Name) {
				log.Error(fmt.Errorf("failed to load %s: loader is not configured", item.Name))
				complete = false
			}
			continue
		}
		partial, err := loader.Load(ctx)
		if err != nil {
			if ctx.Err() != nil {
				complete = false
				continue
			}
			log.Error(fmt.Errorf("failed to load %s: %w", item.Name, err))
			complete = false
			continue
		}
		log.Debug(fmt.Sprintf("Loaded %s domains: %d", item.Name, len(partial)))
		result = append(result, DomainBySource{Source: item.Name, Domains: partial})
	}

	return result, complete
}

// Sync downloads every active source, adds the successfully pulled domains,
// then prunes vanished rows only when every source loaded cleanly. A source
// download failure skips prune; a DB write error or cancellation aborts.
func Sync(ctx context.Context, repo SourceLister, blockRepo BlockWriter, loaders LoaderRegistry, log Logger) error {
	list, complete := LoadAndParseActiveSources(ctx, repo, loaders, log)
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, item := range list {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := blockRepo.CreateDNSRecordsByDomainsContext(ctx, item.Domains, item.Source.String()); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return pruneVanishedDomains(ctx, list, complete, blockRepo, log)
}

// pruneVanishedDomains drops, per source, every block_lists row whose domain is
// gone from *all* freshly synced sources — the deletion half of Sync, split out
// so the union/gate logic is testable without network I/O.
//
// A domain is kept if any synced list still carries it, so the prune diffs each
// source against the union of every fresh set rather than its own: without that
// a domain shared by two lists and dropped by the one that "owns" its row would
// be deleted even though the other list still blocks it.
//
// The prune is skipped entirely when complete is false: a source that failed to
// download is absent from list, so the union would be missing its domains and
// the prune could delete them. A source that parsed to an empty set is left
// untouched too — an empty parse is more likely a garbage response than a list
// that genuinely emptied.
func pruneVanishedDomains(ctx context.Context, list []DomainBySource, complete bool, blockRepo BlockWriter, log Logger) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !complete {
		log.Debug("source sync incomplete — skipping prune of vanished domains")
		return nil
	}

	union := make([]string, 0)
	for _, item := range list {
		union = append(union, item.Domains...)
	}

	for _, item := range list {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(item.Domains) == 0 {
			continue
		}
		if err := blockRepo.DeleteDNSRecordsBySourceNotInContext(ctx, item.Source.String(), union); err != nil {
			return err
		}
	}
	return nil
}
