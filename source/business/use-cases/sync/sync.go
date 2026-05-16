package sync

import (
	"fmt"

	"github.com/alextorq/dns-filter/source/business/use-cases/sync/easy-list"
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

// BlockWriter is the narrow write port over the blocklist. CreateDNSRecordsByDomains
// adds the freshly pulled domains; DeleteDNSRecordsBySourceNotIn prunes the ones
// that vanished from every source (see pruneVanishedDomains).
type BlockWriter interface {
	CreateDNSRecordsByDomains(urls []string, source string) error
	DeleteDNSRecordsBySourceNotIn(source string, keep []string) error
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
func LoadAndParseActiveSources(repo SourceLister, log Logger) (result []DomainBySource, complete bool) {
	result = make([]DomainBySource, 0)

	items, err := repo.GetAllActive()
	if err != nil {
		log.Error(err)
		return result, false
	}

	complete = true

	loadAdBlock := func(source db.BlockListSource, url string) {
		partial, err := easy_list.LoadFromURL(url)
		if err != nil {
			log.Error(fmt.Errorf("failed to load %s: %w", source, err))
			complete = false
			return
		}
		log.Debug(fmt.Sprintf("Loaded %s domains: %d", source, len(partial)))
		result = append(result, DomainBySource{Source: source, Domains: partial})
	}

	loadHosts := func(source db.BlockListSource, url string) {
		partial, err := LoadHostsFromURL(url)
		if err != nil {
			log.Error(fmt.Errorf("failed to load %s: %w", source, err))
			complete = false
			return
		}
		log.Debug(fmt.Sprintf("Loaded %s domains: %d", source, len(partial)))
		result = append(result, DomainBySource{Source: source, Domains: partial})
	}

	for _, item := range items {
		switch item.Name {
		case db.SourceEasyList:
			loadAdBlock(db.SourceEasyList, easy_list.EasyListURL)
		case db.SourceRuAdList:
			loadAdBlock(db.SourceRuAdList, easy_list.RuAdListURL)
		case db.SourceAdGuardRussian:
			loadAdBlock(db.SourceAdGuardRussian, easy_list.AdGuardRussianURL)
		case db.SourceStevenBlack:
			loadHosts(db.SourceStevenBlack, StevenBlackURL)
		case db.SourceHaGeZiMulti:
			loadHosts(db.SourceHaGeZiMulti, HaGeZiMultiURL)
		}
	}

	return result, complete
}

// Sync downloads every active source, adds the freshly pulled domains, then
// prunes the ones that vanished. Errors from a single source abort the whole
// batch (a half-imported source would leave the bloom out-of-sync with the DB).
func Sync(repo SourceLister, blockRepo BlockWriter, log Logger) error {
	list, complete := LoadAndParseActiveSources(repo, log)
	for _, item := range list {
		if err := blockRepo.CreateDNSRecordsByDomains(item.Domains, item.Source.String()); err != nil {
			return err
		}
	}
	return pruneVanishedDomains(list, complete, blockRepo, log)
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
func pruneVanishedDomains(list []DomainBySource, complete bool, blockRepo BlockWriter, log Logger) error {
	if !complete {
		log.Debug("source sync incomplete — skipping prune of vanished domains")
		return nil
	}

	union := make([]string, 0)
	for _, item := range list {
		union = append(union, item.Domains...)
	}

	for _, item := range list {
		if len(item.Domains) == 0 {
			continue
		}
		if err := blockRepo.DeleteDNSRecordsBySourceNotIn(item.Source.String(), union); err != nil {
			return err
		}
	}
	return nil
}
