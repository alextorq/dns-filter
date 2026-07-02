package clients

import (
	"sync"
	"testing"
	"time"

	clientdb "github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/discovery"
	"github.com/alextorq/dns-filter/clients/identifier"
	"github.com/alextorq/dns-filter/clients/store"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestModule(t *testing.T) (*Module, *clientdb.Repo, *store.Store) {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := conn.AutoMigrate(&clientdb.Client{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := clientdb.NewRepo(conn)
	exclusions := store.New()
	return NewModule(repo, exclusions), repo, exclusions
}

func TestModule_SyncUsesCanonicalMACLookup(t *testing.T) {
	m, repo, exclusions := newTestModule(t)
	c := &clientdb.Client{IP: "10.0.0.10", MAC: "aa:bb:cc:dd:ee:ff", Filtered: false}
	if err := repo.Create(c); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := m.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !exclusions.IsExcluded(identifier.Lookup{Kind: identifier.KindMAC, Value: c.MAC}) {
		t.Fatal("MAC exclusion missing")
	}
	if exclusions.IsExcluded(identifier.Lookup{Kind: identifier.KindIP, Value: c.IP}) {
		t.Fatal("IP exclusion must not coexist once a MAC is known")
	}
}

func TestModule_ConcurrentTogglesKeepDBAndStoreConsistent(t *testing.T) {
	m, repo, exclusions := newTestModule(t)
	c := &clientdb.Client{IP: "10.0.0.11", Filtered: false}
	if err := repo.Create(c); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := m.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	const goroutines = 16
	const iterations = 50
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := range iterations {
				if _, err := m.ChangeFilter(c.ID, (seed+i)%2 == 0); err != nil {
					t.Errorf("ChangeFilter: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	stored, err := repo.GetByID(c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	excluded := exclusions.IsExcluded(identifier.Lookup{Kind: identifier.KindIP, Value: c.IP})
	if stored.Filtered == excluded {
		t.Fatalf("DB/store diverged: filtered=%v excluded=%v", stored.Filtered, excluded)
	}
}

type blockingSyncRepo struct {
	mu          sync.Mutex
	client      clientdb.Client
	syncStarted chan struct{}
	releaseSync chan struct{}
}

func (r *blockingSyncRepo) GetAll() ([]clientdb.Client, error) { return nil, nil }
func (r *blockingSyncRepo) GetExcluded() ([]clientdb.Client, error) {
	r.mu.Lock()
	snapshot := r.client
	r.mu.Unlock()
	close(r.syncStarted)
	<-r.releaseSync
	return []clientdb.Client{snapshot}, nil
}
func (r *blockingSyncRepo) GetByID(uint) (*clientdb.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.client
	return &c, nil
}
func (*blockingSyncRepo) Create(*clientdb.Client) error { return nil }
func (r *blockingSyncRepo) UpdateFields(_ uint, fields map[string]any) error {
	r.mu.Lock()
	r.client.Filtered = fields["filtered"].(bool)
	r.mu.Unlock()
	return nil
}
func (*blockingSyncRepo) Delete(uint) error { return nil }

// A snapshot loaded before ChangeFilter must not be installed after the
// toggle's incremental store mutation. Module.mutationMu makes the toggle wait
// for Sync's Replace, then apply the final DB/store state.
func TestModule_SyncSerializesWithChangeFilter(t *testing.T) {
	repo := &blockingSyncRepo{
		client:      clientdb.Client{ID: 1, IP: "10.0.0.12", Filtered: false},
		syncStarted: make(chan struct{}),
		releaseSync: make(chan struct{}),
	}
	exclusions := store.New()
	m := NewModule(repo, exclusions)

	syncDone := make(chan error, 1)
	go func() { syncDone <- m.Sync() }()
	<-repo.syncStarted

	toggleDone := make(chan error, 1)
	go func() {
		_, err := m.ChangeFilter(1, true)
		toggleDone <- err
	}()

	select {
	case err := <-toggleDone:
		close(repo.releaseSync)
		t.Fatalf("ChangeFilter bypassed in-flight Sync: %v", err)
	case <-time.After(50 * time.Millisecond):
		// Expected: ChangeFilter is blocked on mutationMu.
	}

	close(repo.releaseSync)
	if err := <-syncDone; err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := <-toggleDone; err != nil {
		t.Fatalf("ChangeFilter: %v", err)
	}
	if exclusions.IsExcluded(identifier.Lookup{Kind: identifier.KindIP, Value: repo.client.IP}) {
		t.Fatal("stale Sync snapshot overwrote the final filtered=true state")
	}
}

func TestAnnotateRegisteredMatchesIPAndNormalizedMAC(t *testing.T) {
	devices := []discovery.Device{
		{IP: "10.0.0.20"},
		{IP: "10.0.0.21", MAC: "AA-BB-CC-DD-EE-FF"},
		{IP: "10.0.0.22", MAC: "00:11:22:33:44:55"},
	}
	rows := []clientdb.Client{
		{IP: "10.0.0.20"},
		{MAC: "aa:bb:cc:dd:ee:ff"},
	}
	annotateRegistered(devices, rows)
	if !devices[0].AlreadyRegistered || !devices[1].AlreadyRegistered || devices[2].AlreadyRegistered {
		t.Fatalf("unexpected annotations: %+v", devices)
	}
}
