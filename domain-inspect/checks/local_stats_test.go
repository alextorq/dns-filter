package checks

import (
	"context"
	"os"
	"testing"

	allow_db "github.com/alextorq/dns-filter/allow-domain/db"
	blocked_db "github.com/alextorq/dns-filter/blocked-domain/db"
	app_db "github.com/alextorq/dns-filter/db"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// TestMain wires up an isolated SQLite DB for every db-backed test in this
// package. We chdir to a temp dir to redirect the default ./filter.sqlite
// path of the config singleton.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "domain-inspect-checks-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	conn := app_db.GetConnection()
	if err := conn.AutoMigrate(
		&blocked_db.BlockList{},
		&blocked_db.BlockDomainEvent{},
		&allow_db.AllowDomainEvent{},
	); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func resetTables(t *testing.T) {
	t.Helper()
	conn := app_db.GetConnection()
	conn.Exec("DELETE FROM block_lists")
	conn.Exec("DELETE FROM block_domain_events")
	conn.Exec("DELETE FROM allow_domain_events")
}

func TestLocalStats_UnknownDomain(t *testing.T) {
	resetTables(t)

	res := LocalStats(context.Background(), "unknown.example")

	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("expected OK, got %s", res.Status)
	}
	if got, _ := res.Details["in_block_list"].(bool); got {
		t.Error("unknown domain must not be in block list")
	}
	if got, _ := res.Details["in_allow_list"].(bool); got {
		t.Error("unknown domain must not be in allow list")
	}
}

func TestLocalStats_BlockedDomainWithEvents(t *testing.T) {
	resetTables(t)
	conn := app_db.GetConnection()

	const domain = "blocked.example"
	blocklist := blocked_db.BlockList{Url: domain, Active: true, Source: "test"}
	if err := conn.Create(&blocklist).Error; err != nil {
		t.Fatalf("seed blocklist: %v", err)
	}
	for range 3 {
		if err := conn.Create(&blocked_db.BlockDomainEvent{DomainId: blocklist.ID}).Error; err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	res := LocalStats(context.Background(), domain)

	if got, _ := res.Details["in_block_list"].(bool); !got {
		t.Error("expected in_block_list=true")
	}
	if got, _ := res.Details["block_list_source"].(string); got != "test" {
		t.Errorf("source: got %q, want %q", got, "test")
	}
	if got, _ := res.Details["block_events_total"].(int64); got != 3 {
		t.Errorf("event count: got %d, want 3", got)
	}
}

func TestLocalStats_AllowedDomain(t *testing.T) {
	resetTables(t)
	conn := app_db.GetConnection()

	const domain = "allowed.example"
	if err := conn.Create(&allow_db.AllowDomainEvent{Domain: domain, Active: true}).Error; err != nil {
		t.Fatalf("seed allow: %v", err)
	}

	res := LocalStats(context.Background(), domain)

	if got, _ := res.Details["in_allow_list"].(bool); !got {
		t.Error("expected in_allow_list=true")
	}
	if got, _ := res.Details["allow_list_active"].(bool); !got {
		t.Error("expected allow_list_active=true")
	}
}
