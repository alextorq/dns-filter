package change_status

import (
	"os"
	"sync"
	"testing"

	clients "github.com/alextorq/dns-filter/clients/client"
	"github.com/alextorq/dns-filter/clients/db"
	app_db "github.com/alextorq/dns-filter/db"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "clients-change-status-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	conn := app_db.GetConnection()
	if err := conn.AutoMigrate(&db.ExcludeClient{}); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func cleanup(t *testing.T, ip string) {
	t.Helper()
	conn := app_db.GetConnection()
	conn.Unscoped().Where("user_id = ?", ip).Delete(&db.ExcludeClient{})
	clients.GetClients().RemoveClient(ip)
}

func seed(t *testing.T, ip string) uint {
	t.Helper()
	if err := db.AddClient(ip); err != nil {
		t.Fatalf("seed db: %v", err)
	}
	conn := app_db.GetConnection()
	var c db.ExcludeClient
	if err := conn.Where("user_id = ?", ip).First(&c).Error; err != nil {
		t.Fatalf("seed lookup: %v", err)
	}
	clients.GetClients().AddClient(ip)
	return c.ID
}

// Locks in #27: deactivating must drop the IP from the in-memory exclusion
// dictionary so the DNS hot path immediately starts filtering it.
func TestChangeClientStatus_DeactivateRemovesFromMemory(t *testing.T) {
	const ip = "10.0.27.1"
	t.Cleanup(func() { cleanup(t, ip) })

	id := seed(t, ip)
	if !clients.GetClients().ClientExist(ip) {
		t.Fatal("seed: client must be present in memory")
	}

	if err := ChangeClientStatus(id, false); err != nil {
		t.Fatalf("change status: %v", err)
	}
	if clients.GetClients().ClientExist(ip) {
		t.Fatal("after deactivate, client must be gone from memory")
	}

	stored, err := db.GetClientById(id)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored.Active {
		t.Fatal("DB Active flag must be false after deactivate")
	}
}

// Reactivation must put the IP back into the dictionary without a restart.
func TestChangeClientStatus_ReactivateAddsToMemory(t *testing.T) {
	const ip = "10.0.27.2"
	t.Cleanup(func() { cleanup(t, ip) })

	id := seed(t, ip)
	if err := ChangeClientStatus(id, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if clients.GetClients().ClientExist(ip) {
		t.Fatal("after deactivate, client must be gone from memory")
	}

	if err := ChangeClientStatus(id, true); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if !clients.GetClients().ClientExist(ip) {
		t.Fatal("after reactivate, client must be present in memory")
	}
}

func TestChangeClientStatus_MissingIDReturnsError(t *testing.T) {
	if err := ChangeClientStatus(99999, false); err == nil {
		t.Fatal("expected error for unknown id, got nil")
	}
}

// Locks in the P2 review fix: concurrent toggles on the same id must leave
// DB and the in-memory dict in agreement. Without the package-level mutex the
// loser's dict mutation could land after the winner's — DB says active=true
// but the IP is missing from the dict (or vice versa).
func TestChangeClientStatus_ConcurrentTogglesAgree(t *testing.T) {
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
				active := (seed+i)%2 == 0
				if err := ChangeClientStatus(id, active); err != nil {
					t.Errorf("toggle: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	stored, err := db.GetClientById(id)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	inMem := clients.GetClients().ClientExist(ip)
	if stored.Active != inMem {
		t.Fatalf("DB Active=%v but in-memory presence=%v after concurrent toggles", stored.Active, inMem)
	}
}
