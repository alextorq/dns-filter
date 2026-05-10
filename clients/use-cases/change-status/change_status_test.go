package change_status

import (
	"os"
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
