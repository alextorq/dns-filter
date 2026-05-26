package db

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlConn, err := conn.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(&HostName{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepo(conn)
}

func TestUpsert_InsertThenLookup(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.Upsert("aa:bb:cc:dd:ee:ff", "Kitchen-TV"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	m, err := repo.AllAsMap()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if got := m["aa:bb:cc:dd:ee:ff"]; got != "Kitchen-TV" {
		t.Fatalf("hostname = %q, want Kitchen-TV", got)
	}
}

func TestUpsert_UpdatesHostnameAndLastSeen(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.Upsert("aa:bb:cc:dd:ee:ff", "old-name"); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	var first HostName
	if err := repo.db.First(&first).Error; err != nil {
		t.Fatalf("read first: %v", err)
	}

	// Force a measurable gap so the LastSeen bump is observable.
	if err := repo.db.Model(&HostName{}).Where("mac = ?", "aa:bb:cc:dd:ee:ff").
		Update("last_seen", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := repo.Upsert("aa:bb:cc:dd:ee:ff", "new-name"); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	var rows []HostName
	if err := repo.db.Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected a single row after re-upsert, got %d", len(rows))
	}
	if rows[0].Hostname != "new-name" {
		t.Fatalf("hostname = %q, want new-name", rows[0].Hostname)
	}
	if !rows[0].LastSeen.After(time.Now().Add(-time.Minute)) {
		t.Fatalf("LastSeen was not refreshed on re-upsert: %v", rows[0].LastSeen)
	}
}

func TestUpsert_NormalizesMAC(t *testing.T) {
	repo := newTestRepo(t)

	// Uppercase + dash form should collapse onto the same canonical row as the
	// lowercase colon form traffic uses.
	if err := repo.Upsert("AA-BB-CC-DD-EE-FF", "phone"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.Upsert("aa:bb:cc:dd:ee:ff", "phone-renamed"); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	m, err := repo.AllAsMap()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(m) != 1 {
		t.Fatalf("expected 1 normalized entry, got %d: %v", len(m), m)
	}
	if m["aa:bb:cc:dd:ee:ff"] != "phone-renamed" {
		t.Fatalf("normalized lookup failed: %v", m)
	}
}

func TestUpsert_EmptyInputsAreNoOps(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.Upsert("", "ghost"); err != nil {
		t.Fatalf("upsert empty mac: %v", err)
	}
	if err := repo.Upsert("aa:bb:cc:dd:ee:ff", ""); err != nil {
		t.Fatalf("upsert empty hostname: %v", err)
	}
	if err := repo.Upsert("   ", "  "); err != nil {
		t.Fatalf("upsert blank: %v", err)
	}

	m, err := repo.AllAsMap()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty table, got %v", m)
	}
}

func TestAllAsMap_MissingKeyAbsent(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.Upsert("aa:bb:cc:dd:ee:ff", "known"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	m, err := repo.AllAsMap()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if _, ok := m["11:22:33:44:55:66"]; ok {
		t.Fatal("unknown MAC should be absent from the map")
	}
}

func TestPruneOlderThan(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.Upsert("aa:bb:cc:dd:ee:ff", "fresh"); err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}
	if err := repo.Upsert("11:22:33:44:55:66", "stale"); err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	// Backdate the stale row well past the retention window.
	if err := repo.db.Model(&HostName{}).Where("mac = ?", "11:22:33:44:55:66").
		Update("last_seen", time.Now().Add(-48*time.Hour)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := repo.PruneOlderThan(24 * time.Hour); err != nil {
		t.Fatalf("prune: %v", err)
	}

	m, err := repo.AllAsMap()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if _, ok := m["aa:bb:cc:dd:ee:ff"]; !ok {
		t.Fatal("fresh row should survive prune")
	}
	if _, ok := m["11:22:33:44:55:66"]; ok {
		t.Fatal("stale row should be pruned")
	}
}

func TestPruneOlderThan_NonPositiveWindowIsNoOp(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.Upsert("aa:bb:cc:dd:ee:ff", "keepme"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Backdate far into the past; a zero/negative window must NOT wipe it.
	if err := repo.db.Model(&HostName{}).Where("mac = ?", "aa:bb:cc:dd:ee:ff").
		Update("last_seen", time.Now().Add(-1000*time.Hour)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := repo.PruneOlderThan(0); err != nil {
		t.Fatalf("prune 0: %v", err)
	}
	if err := repo.PruneOlderThan(-time.Hour); err != nil {
		t.Fatalf("prune negative: %v", err)
	}

	m, err := repo.AllAsMap()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(m) != 1 {
		t.Fatalf("non-positive window must not prune; got %v", m)
	}
}
