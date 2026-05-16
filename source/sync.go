// Package source is the composition root for the block-list source feature.
// Module wires the source repository, the blocklist writer, and the logger;
// main constructs one, calls Seed synchronously at startup and Sync from a
// background goroutine (see main.backgroundSync), then hands the reference to
// web.Handlers.
package source

import (
	syncRec "github.com/alextorq/dns-filter/source/business/use-cases/sync"
	"github.com/alextorq/dns-filter/source/db"
)

type Logger interface {
	Info(args ...any)
	Debug(args ...any)
	Error(err error)
}

// BlockWriter is the narrow port over the blocklist. CreateDNSRecordsByDomains
// and DeleteDNSRecordsBySourceNotIn together apply a freshly pulled source
// (add new domains, drop the ones gone upstream); ChangeRecordStatusBySource
// backs the source enable/disable kill-switch.
type BlockWriter interface {
	CreateDNSRecordsByDomains(urls []string, source string) error
	DeleteDNSRecordsBySourceNotIn(source string, keep []string) error
	ChangeRecordStatusBySource(source string, active bool) error
}

type Module struct {
	repo      *db.Repo
	blockRepo BlockWriter
	log       Logger
}

func NewModule(repo *db.Repo, blockRepo BlockWriter, log Logger) *Module {
	return &Module{repo: repo, blockRepo: blockRepo, log: log}
}

// Seed inserts the default catalog of known sources if missing. Idempotent.
func (m *Module) Seed() {
	m.repo.Seed()
}

// Sync downloads + parses every active source and applies it to the blocklist
// (new domains added, vanished ones dropped). At startup it runs inside the
// backgroundSync goroutine (see main.go) so the DNS server can serve traffic
// immediately; the caller refreshes the in-memory filter once Sync returns.
func (m *Module) Sync() error {
	return syncRec.Sync(m.repo, m.blockRepo, m.log)
}
