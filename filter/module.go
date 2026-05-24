// Package filter is the composition root for the DNS filter feature.
//
// Module owns the in-memory bloom + LRU cache and the BlockChecker repository,
// and exposes the narrow API the rest of the app needs: CheckExist (hot path),
// UpdateFromDb (cache invalidation), and the runtime toggles (status, pause,
// resume). Construct it once in main.
package filter

import (
	"fmt"
	"strconv"

	"github.com/alextorq/dns-filter/config"
	changefilter "github.com/alextorq/dns-filter/filter/business/use-cases/change-filter-dns-records"
	checkexist "github.com/alextorq/dns-filter/filter/business/use-cases/check-exist"
	pausefilter "github.com/alextorq/dns-filter/filter/business/use-cases/pause-filter"
	"github.com/alextorq/dns-filter/utils"
)

// BlockChecker is the repository port the module needs. *blocked-domain/db.Repo
// satisfies it via structural typing.
type BlockChecker interface {
	GetAllActiveURLs() ([]string, error)
	IsActivelyBlocked(domain string) (bool, error)
}

// Bloom is the in-memory membership-test port (the real *filter/filter.Filter
// satisfies it; tests inject a stub).
type Bloom interface {
	DomainExist(domain string) bool
	UpdateFilter(rows []string) // return type intentionally absent — caller doesn't use it
}

// Cache is the verdict LRU port.
type Cache interface {
	Get(key string) (bool, bool)
	Add(key string, val bool)
	Clear()
}

// Logger covers all severities the module's use-cases need.
type Logger interface {
	Info(args ...any)
	Debug(args ...any)
	Error(err error)
}

// Module is the wired-up filter feature.
type Module struct {
	repo  BlockChecker
	bloom Bloom
	cache Cache
	conf  *config.Config
	log   Logger
	// persist, when set, is invoked after every successful toggle with the new
	// (enabled, pausedUntil) state so it survives a restart. nil disables
	// persistence (the default — tests and the hot path don't need it).
	persist func(enabled bool, pausedUntil int64)
}

func NewModule(repo BlockChecker, bloom Bloom, cache Cache, conf *config.Config, log Logger) *Module {
	return &Module{repo: repo, bloom: bloom, cache: cache, conf: conf, log: log}
}

// SetStateSink installs the persistence hook. Wire it at the composition root
// to filter.PersistHook so on/off and pause survive restarts; leave unset in
// tests.
func (m *Module) SetStateSink(persist func(enabled bool, pausedUntil int64)) {
	m.persist = persist
}

// persistState snapshots the current toggle state through the sink, if one is
// installed. Called after a mutation; reads the atomics so it always reflects
// what the use-case just stored.
func (m *Module) persistState() {
	if m.persist != nil {
		m.persist(m.conf.Enabled.Load(), m.conf.PausedUntilUnix.Load())
	}
}

// CheckExist is the hot-path entry — see check-exist/check-block.go.
//
// The query name arrives straight from miekg/dns (q.Name): FQDN-form but with
// no guaranteed letter case (DNS 0x20 encoding). Canonicalizing here keeps the
// bloom filter, the LRU verdict cache and the SQLite lookup all keyed on the
// same form the block list is stored in (#30).
func (m *Module) CheckExist(domain string) bool {
	return checkexist.CheckBlock(checkexist.Deps{
		Repo:  m.repo,
		Cache: m.cache,
		Bloom: m.bloom,
		Conf:  m.conf,
		Log:   m.log,
	}, utils.CanonicalDomain(domain))
}

// UpdateFromDb rebuilds the bloom from the active block list and discards the
// LRU verdict cache. Both must move together: a stale cache after a mutation
// would otherwise serve the old verdict for ~1500 lookups (#26).
func (m *Module) UpdateFromDb() error {
	list, err := m.repo.GetAllActiveURLs()
	if err != nil {
		return fmt.Errorf("ошибка получения данных из БД: %w", err)
	}
	m.log.Info("Фильтр обновлён из БД, записей: " + strconv.Itoa(len(list)))
	m.bloom.UpdateFilter(list)
	m.cache.Clear()
	return nil
}

// ChangeStatus toggles the global filter on/off, returning the new value.
func (m *Module) ChangeStatus() bool {
	v := changefilter.ChangeFilterDnsRecords(m.conf, m.log)
	m.persistState()
	return v
}

// Pause pauses filtering for the given number of minutes.
func (m *Module) Pause(minutes int) (int64, error) {
	until, err := pausefilter.PauseFilter(m.conf, m.log, minutes)
	if err != nil {
		return until, err
	}
	m.persistState()
	return until, nil
}

// Resume clears any active pause.
func (m *Module) Resume() {
	pausefilter.ResumeFilter(m.conf, m.log)
	m.persistState()
}

// PausedUntil returns the active pause deadline (unix seconds), or 0.
func (m *Module) PausedUntil() int64 {
	return pausefilter.GetPausedUntil(m.conf)
}

// Enabled returns the current global toggle.
func (m *Module) Enabled() bool {
	return m.conf.Enabled.Load()
}
