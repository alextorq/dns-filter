// Package source is the composition root for the block-list source feature.
// Module wires the source repository, the blocklist writer, and the logger;
// main constructs one and calls Seed + Sync at startup, then hands the
// reference to web.Handlers.
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

// BlockWriter is the narrow port over the blocklist (used to upsert pulled
// source domains).
type BlockWriter interface {
	CreateDNSRecordsByDomains(urls []string, source string) error
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

// Sync downloads + parses every active source and upserts the parsed domains
// into the blocklist. Failure aborts main at startup — partial sync would
// leave the bloom out-of-sync with the DB.
func (m *Module) Sync() error {
	return syncRec.Sync(m.repo, m.blockRepo, m.log)
}
