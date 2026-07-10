// Package source is the composition root for the block-list source feature.
// Module wires the source repository, the blocklist writer, and the logger;
// main constructs one, calls Seed synchronously at startup and Sync from a
// background goroutine (see main.backgroundSync), then hands the reference to
// web.Handlers.
package source

import (
	"context"
	"fmt"
	"reflect"

	syncRec "github.com/alextorq/dns-filter/source/business/use-cases/sync"
)

type Logger interface {
	Info(args ...any)
	Debug(args ...any)
	Error(err error)
}

// BlockWriter is the narrow port over the blocklist. The context-aware methods
// apply a freshly pulled source (add new domains, drop the ones gone upstream)
// and let application shutdown interrupt long SQLite batches.
type BlockWriter interface {
	CreateDNSRecordsByDomainsContext(ctx context.Context, urls []string, source string) error
	DeleteDNSRecordsBySourceNotInContext(ctx context.Context, source string, keep []string) error
}

// SourceRepo is the consumer-owned persistence port used by Module. The DB
// adapter satisfies it structurally; tests can supply an in-memory fake.
type SourceRepo interface {
	syncRec.SourceLister
	Seed()
}

type Module struct {
	repo      SourceRepo
	blockRepo BlockWriter
	loaders   syncRec.LoaderRegistry
	log       Logger
}

func NewModule(repo SourceRepo, blockRepo BlockWriter, loaders syncRec.LoaderRegistry, log Logger) (*Module, error) {
	if isNilDependency(repo) {
		return nil, fmt.Errorf("source module: source repo is required")
	}
	if isNilDependency(blockRepo) {
		return nil, fmt.Errorf("source module: block writer is required")
	}
	if isNilDependency(log) {
		return nil, fmt.Errorf("source module: logger is required")
	}
	validatedLoaders, err := syncRec.NewLoaderRegistry(loaders)
	if err != nil {
		return nil, fmt.Errorf("source module: %w", err)
	}
	return &Module{repo: repo, blockRepo: blockRepo, loaders: validatedLoaders, log: log}, nil
}

func isNilDependency(dependency any) bool {
	if dependency == nil {
		return true
	}
	v := reflect.ValueOf(dependency)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// Seed inserts the default catalog of known sources if missing. Idempotent.
func (m *Module) Seed() {
	m.repo.Seed()
}

// Sync downloads + parses every active source and applies it to the blocklist
// (new domains added, vanished ones dropped). At startup it runs inside the
// backgroundSync goroutine (see main.go) so the DNS server can serve traffic
// immediately; the caller refreshes the in-memory filter once Sync returns.
func (m *Module) Sync(ctx context.Context) error {
	return syncRec.Sync(ctx, m.repo, m.blockRepo, m.loaders, m.log)
}
