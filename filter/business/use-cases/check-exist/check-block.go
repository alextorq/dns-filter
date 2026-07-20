package check_exist_domain

import (
	"fmt"
	"time"
)

// BlockChecker is the output port for the authoritative DB verdict.
// *blocked-domain/db.Repo satisfies it via structural typing.
type BlockChecker interface {
	IsActivelyBlocked(domain string) (bool, error)
}

// Cache is the verdict cache port. Production uses the LRU in filter/cache;
// tests inject a map-backed fake.
type Cache interface {
	Get(key string) (bool, bool)
	Add(key string, val bool)
}

// Bloom is the membership-test port (the in-memory bloom filter).
type Bloom interface {
	DomainExist(domain string) bool
}

type Logger interface {
	Debug(args ...any)
	Error(err error)
}

type RuntimeState interface {
	Enabled() bool
	PausedUntil() int64
}

// Deps groups the narrow ports CheckBlock / CheckCacheOrDb need. Constructed
// once at the composition root and reused across DNS queries.
type Deps struct {
	Repo  BlockChecker
	Cache Cache
	Bloom Bloom
	State RuntimeState
	Log   Logger
}

// CheckCacheOrDb returns the cached verdict if present, otherwise consults the
// repository and caches a positive answer. On a DB error the call fails OPEN
// (returns false) and DOES NOT cache — so a transient DB blip cannot lock in a
// "not blocked" verdict for the next ~1500 lookups (#25).
func CheckCacheOrDb(d Deps, domain string) bool {
	if val, found := d.Cache.Get(domain); found {
		d.Log.Debug("get block domain check from cache domain: ", domain, "value: ", found)
		return val
	}
	d.Log.Debug("get block domain check from db: ", domain)
	exist, err := d.Repo.IsActivelyBlocked(domain)
	if err != nil {
		d.Log.Error(fmt.Errorf("IsActivelyBlocked(%s): %w", domain, err))
		return false
	}
	d.Cache.Add(domain, exist)
	return exist
}

// CheckBlock is the hot-path entry: respects the global Enabled flag and any
// active pause, then consults bloom → cache → DB. Bloom miss short-circuits
// without touching the DB; bloom hit defers to CheckCacheOrDb.
func CheckBlock(d Deps, domain string, now time.Time) bool {
	if !d.State.Enabled() {
		return false
	}
	if until := d.State.PausedUntil(); until > now.Unix() {
		return false
	}
	if d.Bloom.DomainExist(domain) {
		return CheckCacheOrDb(d, domain)
	}
	return false
}
