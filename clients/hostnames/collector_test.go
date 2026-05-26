package hostnames

import (
	"context"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/alextorq/dns-filter/clients/discovery"
)

// --- fakes ---

type fakeStore struct {
	mu       sync.Mutex
	upserts  map[string]string // mac -> hostname (last write wins)
	pruned   []time.Duration
	upsertErr error
	pruneErr  error
}

func newFakeStore() *fakeStore { return &fakeStore{upserts: map[string]string{}} }

func (s *fakeStore) Upsert(mac, hostname string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.upserts[mac] = hostname
	return nil
}

func (s *fakeStore) PruneOlderThan(window time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruned = append(s.pruned, window)
	return s.pruneErr
}

func (s *fakeStore) snapshot() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.upserts))
	maps.Copy(out, s.upserts)
	return out
}

type fakeMACs map[string]string // ip -> mac

func (f fakeMACs) MAC(ip string) (string, bool) {
	mac, ok := f[ip]
	return mac, ok
}

type fakeLog struct {
	mu   sync.Mutex
	errs []error
}

func (l *fakeLog) Info(_ ...any) {}
func (l *fakeLog) Error(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, err)
}
func (l *fakeLog) errorCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.errs)
}

func browseReturning(hosts []discovery.MDNSHost, err error) Browser {
	return func(_ context.Context) ([]discovery.MDNSHost, error) { return hosts, err }
}

// --- tests ---

func TestSweep_RecordsResolvedMAC(t *testing.T) {
	store := newFakeStore()
	c := &Collector{
		Browse: browseReturning([]discovery.MDNSHost{{IP: "192.168.1.40", Hostname: "iPhone-Ivan"}}, nil),
		MACs:   fakeMACs{"192.168.1.40": "aa:bb:cc:dd:ee:ff"},
		Store:  store,
		Log:    &fakeLog{},
		TTL:    time.Hour,
	}

	c.sweep(context.Background())

	got := store.snapshot()
	if got["aa:bb:cc:dd:ee:ff"] != "iPhone-Ivan" {
		t.Fatalf("expected hostname keyed by MAC, got %v", got)
	}
	if _, ok := got["192.168.1.40"]; ok {
		t.Fatal("must never key by IP")
	}
}

func TestSweep_SkipsUnresolvedMAC(t *testing.T) {
	store := newFakeStore()
	c := &Collector{
		Browse: browseReturning([]discovery.MDNSHost{{IP: "192.168.1.99", Hostname: "Mystery"}}, nil),
		MACs:   fakeMACs{}, // arpwatcher doesn't know this IP yet
		Store:  store,
		Log:    &fakeLog{},
	}

	c.sweep(context.Background())

	if len(store.snapshot()) != 0 {
		t.Fatalf("host with unknown MAC must be skipped, got %v", store.snapshot())
	}
}

func TestSweep_SkipsEmptyHostname(t *testing.T) {
	store := newFakeStore()
	c := &Collector{
		Browse: browseReturning([]discovery.MDNSHost{{IP: "192.168.1.40", Hostname: ""}}, nil),
		MACs:   fakeMACs{"192.168.1.40": "aa:bb:cc:dd:ee:ff"},
		Store:  store,
		Log:    &fakeLog{},
	}

	c.sweep(context.Background())

	if len(store.snapshot()) != 0 {
		t.Fatalf("empty hostname must be skipped, got %v", store.snapshot())
	}
}

func TestSweep_NilMACLookupSkipsAll(t *testing.T) {
	store := newFakeStore()
	c := &Collector{
		Browse: browseReturning([]discovery.MDNSHost{{IP: "192.168.1.40", Hostname: "TV"}}, nil),
		MACs:   nil,
		Store:  store,
		Log:    &fakeLog{},
	}

	c.sweep(context.Background()) // must not panic

	if len(store.snapshot()) != 0 {
		t.Fatalf("nil MAC lookup must record nothing, got %v", store.snapshot())
	}
}

func TestSweep_PartialBrowseErrorStillRecords(t *testing.T) {
	store := newFakeStore()
	log := &fakeLog{}
	c := &Collector{
		// Browse returned one usable host AND a (partial) error.
		Browse: browseReturning(
			[]discovery.MDNSHost{{IP: "192.168.1.40", Hostname: "Printer"}},
			errors.New("mdns browse _smb._tcp: timeout"),
		),
		MACs:  fakeMACs{"192.168.1.40": "aa:bb:cc:dd:ee:ff"},
		Store: store,
		Log:   log,
	}

	c.sweep(context.Background())

	if store.snapshot()["aa:bb:cc:dd:ee:ff"] != "Printer" {
		t.Fatalf("partial error must not abort the sweep, got %v", store.snapshot())
	}
	if log.errorCount() == 0 {
		t.Fatal("partial browse error should be logged")
	}
}

func TestSweep_UpsertErrorIsLoggedAndContinues(t *testing.T) {
	store := newFakeStore()
	store.upsertErr = errors.New("db locked")
	log := &fakeLog{}
	c := &Collector{
		Browse: browseReturning([]discovery.MDNSHost{
			{IP: "192.168.1.40", Hostname: "A"},
			{IP: "192.168.1.41", Hostname: "B"},
		}, nil),
		MACs:  fakeMACs{"192.168.1.40": "aa:bb:cc:dd:ee:ff", "192.168.1.41": "11:22:33:44:55:66"},
		Store: store,
		Log:   log,
	}

	c.sweep(context.Background()) // must not panic despite both upserts failing

	if log.errorCount() < 2 {
		t.Fatalf("each failed upsert should be logged, got %d", log.errorCount())
	}
}

func TestSweep_PrunesAfterRecording(t *testing.T) {
	store := newFakeStore()
	c := &Collector{
		Browse: browseReturning(nil, nil),
		MACs:   fakeMACs{},
		Store:  store,
		Log:    &fakeLog{},
		TTL:    7 * 24 * time.Hour,
	}

	c.sweep(context.Background())

	if len(store.pruned) != 1 || store.pruned[0] != 7*24*time.Hour {
		t.Fatalf("expected a single prune with the configured TTL, got %v", store.pruned)
	}
}

func TestRun_ImmediateSweepThenStopsOnCancel(t *testing.T) {
	store := newFakeStore()
	c := &Collector{
		Browse:   browseReturning([]discovery.MDNSHost{{IP: "192.168.1.40", Hostname: "TV"}}, nil),
		MACs:     fakeMACs{"192.168.1.40": "aa:bb:cc:dd:ee:ff"},
		Store:    store,
		Log:      &fakeLog{},
		Interval: time.Hour, // long, so only the immediate sweep runs
		TTL:      time.Hour,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}

	if store.snapshot()["aa:bb:cc:dd:ee:ff"] != "TV" {
		t.Fatalf("immediate sweep should have recorded before cancel, got %v", store.snapshot())
	}
}
