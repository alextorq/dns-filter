package check_exist_domain

import (
	"errors"
	"sync"
	"testing"
	"time"

	runtime_state "github.com/alextorq/dns-filter/filter/runtime-state"
)

type fakeRepo struct {
	verdict map[string]bool
	err     error
	calls   int
}

func (f *fakeRepo) IsActivelyBlocked(domain string) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	v, ok := f.verdict[domain]
	if !ok {
		return false, nil
	}
	return v, nil
}

type mapCache struct {
	mu sync.Mutex
	m  map[string]bool
}

func newMapCache() *mapCache { return &mapCache{m: map[string]bool{}} }

func (c *mapCache) Get(key string) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	return v, ok
}

func (c *mapCache) Add(key string, val bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = val
}

type setBloom struct {
	known map[string]struct{}
}

func (b *setBloom) DomainExist(domain string) bool {
	_, ok := b.known[domain]
	return ok
}

type silentLog struct {
	errs []error
}

func (l *silentLog) Debug(args ...any) {}
func (l *silentLog) Error(err error)   { l.errs = append(l.errs, err) }

func freshState(t *testing.T) *runtime_state.State {
	t.Helper()
	return runtime_state.New(true)
}

var checkTestNow = time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

func newDeps(repo *fakeRepo, cache *mapCache, bloom *setBloom, state *runtime_state.State) (Deps, *silentLog) {
	log := &silentLog{}
	return Deps{Repo: repo, Cache: cache, Bloom: bloom, State: state, Log: log}, log
}

// CheckBlock must not consult bloom or DB when the global toggle is off.
func TestCheckBlock_DisabledShortCircuits(t *testing.T) {
	state := freshState(t)
	state.SetEnabled(false)

	repo := &fakeRepo{verdict: map[string]bool{"x.example": true}}
	bloom := &setBloom{known: map[string]struct{}{"x.example": {}}}
	d, _ := newDeps(repo, newMapCache(), bloom, state)

	if got := CheckBlock(d, "x.example", checkTestNow); got {
		t.Fatal("disabled filter must return false")
	}
	if repo.calls != 0 {
		t.Fatalf("DB must not be called when disabled, got %d calls", repo.calls)
	}
}

// CheckBlock must respect an active pause even when the domain is in bloom + DB.
func TestCheckBlock_PauseSuppressesBlocking(t *testing.T) {
	state := freshState(t)
	state.SetPausedUntil(checkTestNow.Add(5 * time.Minute).Unix())

	repo := &fakeRepo{verdict: map[string]bool{"x.example": true}}
	bloom := &setBloom{known: map[string]struct{}{"x.example": {}}}
	d, _ := newDeps(repo, newMapCache(), bloom, state)

	if got := CheckBlock(d, "x.example", checkTestNow); got {
		t.Fatal("paused filter must return false even for an actively blocked domain")
	}

	state.SetPausedUntil(0)
	if got := CheckBlock(d, "x.example", checkTestNow); !got {
		t.Fatal("after clearing pause, blocked domain must be reported as blocked")
	}
}

func TestCheckBlock_PauseEndingAtNowIsExpired(t *testing.T) {
	state := freshState(t)
	state.SetPausedUntil(checkTestNow.Unix())
	repo := &fakeRepo{verdict: map[string]bool{"x.example": true}}
	bloom := &setBloom{known: map[string]struct{}{"x.example": {}}}
	d, _ := newDeps(repo, newMapCache(), bloom, state)

	if got := CheckBlock(d, "x.example", checkTestNow); !got {
		t.Fatal("pause ending exactly at now must no longer suppress blocking")
	}
}

// Bloom miss must short-circuit without consulting the cache or DB.
func TestCheckBlock_BloomMissSkipsDB(t *testing.T) {
	state := freshState(t)
	repo := &fakeRepo{verdict: map[string]bool{"x.example": true}}
	bloom := &setBloom{known: map[string]struct{}{}} // empty
	d, _ := newDeps(repo, newMapCache(), bloom, state)

	if got := CheckBlock(d, "x.example", checkTestNow); got {
		t.Fatal("bloom miss must return false")
	}
	if repo.calls != 0 {
		t.Fatalf("DB must not be called on bloom miss, got %d calls", repo.calls)
	}
}

// Bloom hit + DB-confirmed active → blocked, and the verdict must land in cache
// so a second call hits the cache path.
func TestCheckBlock_BloomHitConsultsDBAndCachesVerdict(t *testing.T) {
	state := freshState(t)
	repo := &fakeRepo{verdict: map[string]bool{"x.example": true}}
	bloom := &setBloom{known: map[string]struct{}{"x.example": {}}}
	cache := newMapCache()
	d, _ := newDeps(repo, cache, bloom, state)

	if !CheckBlock(d, "x.example", checkTestNow) {
		t.Fatal("expected blocked verdict")
	}
	if !CheckBlock(d, "x.example", checkTestNow) {
		t.Fatal("second call must still report blocked")
	}
	if repo.calls != 1 {
		t.Fatalf("expected DB hit only once, got %d calls", repo.calls)
	}
}

// Locks in #25: bloom hit but DB says inactive → not blocked, and the negative
// verdict is cached so we don't re-hit the DB.
func TestCheckBlock_DeactivatedDomainNotBlocked(t *testing.T) {
	state := freshState(t)
	repo := &fakeRepo{verdict: map[string]bool{}} // domain not in DB == inactive
	bloom := &setBloom{known: map[string]struct{}{"deactivated.example": {}}}
	cache := newMapCache()
	d, _ := newDeps(repo, cache, bloom, state)

	if CheckBlock(d, "deactivated.example", checkTestNow) {
		t.Fatal("deactivated domain must not be reported as blocked (issue #25)")
	}
	// Negative verdict is allowed in the cache (the regression was about
	// stale-positive after deactivation, not about caching negatives).
	if _, ok := cache.Get("deactivated.example"); !ok {
		t.Fatal("expected negative verdict to be cached")
	}
}

// Fail-open contract: a DB error returns false AND must NOT cache, so a
// transient blip can't lock in a "not blocked" verdict for the LRU window.
func TestCheckCacheOrDb_DBErrorFailsOpenWithoutCaching(t *testing.T) {
	state := freshState(t)
	repo := &fakeRepo{err: errors.New("db down")}
	cache := newMapCache()
	d, log := newDeps(repo, cache, &setBloom{}, state)

	if got := CheckCacheOrDb(d, "x.example"); got {
		t.Fatal("DB error must fail open (return false)")
	}
	if _, ok := cache.Get("x.example"); ok {
		t.Fatal("error verdict must NOT be cached, otherwise the LRU pins fail-open across the next ~1500 lookups")
	}
	if len(log.errs) != 1 {
		t.Fatalf("expected one logged error, got %d", len(log.errs))
	}
}
