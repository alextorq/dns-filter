package change_filter

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/identifier"
	"github.com/alextorq/dns-filter/clients/store"
	app_db "github.com/alextorq/dns-filter/db"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "clients-change-filter-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	conn := app_db.GetConnection()
	if err := conn.AutoMigrate(&db.Client{}); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func ipLookup(ip string) identifier.Lookup {
	return identifier.Lookup{Kind: identifier.KindIP, Value: ip}
}

func cleanup(t *testing.T, ip string) {
	t.Helper()
	conn := app_db.GetConnection()
	conn.Unscoped().Where("ip = ?", ip).Delete(&db.Client{})
	store.Get().Remove(ipLookup(ip))
}

// seed inserts a client that starts as Filtered=false (i.e., excluded), and
// mirrors that into the in-memory store. Returns the row id.
func seed(t *testing.T, ip string) uint {
	t.Helper()
	c := &db.Client{IP: ip, Filtered: false}
	if err := db.CreateClient(c); err != nil {
		t.Fatalf("seed db: %v", err)
	}
	store.Get().Add(ipLookup(ip))
	return c.ID
}

// Locks in #27 behavior, ported to the new schema: turning the filter ON for
// an excluded client must drop its IP from the in-memory exclusion set so the
// DNS hot path immediately starts filtering it.
func TestChangeFilter_EnableRemovesFromMemory(t *testing.T) {
	const ip = "10.0.27.1"
	t.Cleanup(func() { cleanup(t, ip) })

	id := seed(t, ip)
	if !store.Get().IsExcluded(ipLookup(ip)) {
		t.Fatal("seed: client must be in exclusion set")
	}

	if _, err := ChangeFilter(id, true); err != nil {
		t.Fatalf("change filter: %v", err)
	}
	if store.Get().IsExcluded(ipLookup(ip)) {
		t.Fatal("after enabling filter, client must be gone from exclusion set")
	}

	stored, err := db.GetClientByID(id)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !stored.Filtered {
		t.Fatal("DB Filtered flag must be true after enable")
	}
}

// Disabling the filter again must put the IP back into the exclusion set
// without a service restart.
func TestChangeFilter_DisableAddsToMemory(t *testing.T) {
	const ip = "10.0.27.2"
	t.Cleanup(func() { cleanup(t, ip) })

	id := seed(t, ip)
	if _, err := ChangeFilter(id, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if store.Get().IsExcluded(ipLookup(ip)) {
		t.Fatal("after enabling filter, client must be gone from exclusion set")
	}

	if _, err := ChangeFilter(id, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !store.Get().IsExcluded(ipLookup(ip)) {
		t.Fatal("after disabling filter, client must be in exclusion set")
	}
}

func TestChangeFilter_MissingIDReturnsErrNotFound(t *testing.T) {
	_, err := ChangeFilter(99999, true)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown id, got %v", err)
	}
}

// Concurrent toggles on the same id must leave DB and the in-memory store in
// agreement. Without the package-level mutex the loser's store mutation could
// land after the winner's, leaving DB Filtered=true with the IP still in the
// exclusion set (or vice versa).
func TestChangeFilter_ConcurrentTogglesAgree(t *testing.T) {
	const ip = "10.0.27.99"
	t.Cleanup(func() { cleanup(t, ip) })

	id := seed(t, ip)

	const goroutines = 16
	const iters = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(seed int) {
			defer wg.Done()
			for i := range iters {
				filtered := (seed+i)%2 == 0
				if _, err := ChangeFilter(id, filtered); err != nil {
					t.Errorf("toggle: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	stored, err := db.GetClientByID(id)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	excluded := store.Get().IsExcluded(ipLookup(ip))
	// DB Filtered=true ↔ NOT in exclusion set.
	if stored.Filtered == excluded {
		t.Fatalf("DB Filtered=%v but in-memory excluded=%v after concurrent toggles", stored.Filtered, excluded)
	}
}
