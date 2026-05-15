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
	// SQLite :memory: is per-connection; pin to one so all queries share state.
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(&AllowDomainEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepo(conn)
}

// ----- GetAllActiveFilters -----

func TestRepo_GetAllActiveFilters_FiltersInactive(t *testing.T) {
	r := newTestRepo(t)
	if err := r.db.Create(&AllowDomainEvent{Domain: "active.example", Active: true}).Error; err != nil {
		t.Fatalf("seed active: %v", err)
	}
	if err := r.db.Create(&AllowDomainEvent{Domain: "inactive.example", Active: false}).Error; err != nil {
		t.Fatalf("seed inactive: %v", err)
	}

	domains, err := r.GetAllActiveFilters()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(domains) != 1 || domains[0] != "active.example" {
		t.Errorf("expected [active.example], got %v", domains)
	}
}

func TestRepo_GetAllActiveFilters_Empty(t *testing.T) {
	r := newTestRepo(t)
	domains, err := r.GetAllActiveFilters()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected empty, got %v", domains)
	}
}

// ----- CreateBatch -----

func TestRepo_CreateBatch(t *testing.T) {
	t.Run("inserts deduplicated batch", func(t *testing.T) {
		r := newTestRepo(t)
		err := r.CreateBatch([]string{"a.example", "b.example", "a.example"}) // dup
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		var n int64
		r.db.Model(&AllowDomainEvent{}).Count(&n)
		if n != 2 {
			t.Errorf("expected 2 rows after dedup, got %d", n)
		}
	})

	t.Run("empty input is no-op", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateBatch(nil); err != nil {
			t.Fatalf("err: %v", err)
		}
		var n int64
		r.db.Model(&AllowDomainEvent{}).Count(&n)
		if n != 0 {
			t.Errorf("expected 0 rows, got %d", n)
		}
	})

	t.Run("re-import is idempotent", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateBatch([]string{"x.example"}); err != nil {
			t.Fatalf("first: %v", err)
		}
		// Second import of the same domain must not error
		// (BatchUpsertOn / DoNothing on the unique index).
		if err := r.CreateBatch([]string{"x.example", "y.example"}); err != nil {
			t.Fatalf("second: %v", err)
		}
		var n int64
		r.db.Model(&AllowDomainEvent{}).Count(&n)
		if n != 2 {
			t.Errorf("expected 2 rows, got %d", n)
		}
	})

	t.Run("inserted rows are active", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateBatch([]string{"new.example"}); err != nil {
			t.Fatalf("err: %v", err)
		}
		got, err := r.GetAllActiveFilters()
		if err != nil {
			t.Fatalf("active: %v", err)
		}
		if len(got) != 1 || got[0] != "new.example" {
			t.Errorf("expected [new.example] active, got %v", got)
		}
	})
}

// ----- DeleteOlderThan -----

func TestRepo_DeleteOlderThan_DeletesOnlyOldRows(t *testing.T) {
	r := newTestRepo(t)
	if err := r.db.Create(&AllowDomainEvent{Domain: "fresh.example", Active: true}).Error; err != nil {
		t.Fatalf("seed fresh: %v", err)
	}
	old := AllowDomainEvent{Domain: "old.example", Active: true}
	if err := r.db.Create(&old).Error; err != nil {
		t.Fatalf("seed old: %v", err)
	}
	// Backdate the old row to a clearly-past timestamp.
	if err := r.db.Model(&AllowDomainEvent{}).
		Where("id = ?", old.ID).
		Update("created_at", time.Now().AddDate(0, 0, -30)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := r.DeleteOlderThan(2); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var domains []string
	if err := r.db.Model(&AllowDomainEvent{}).Pluck("domain", &domains).Error; err != nil {
		t.Fatalf("pluck: %v", err)
	}
	if len(domains) != 1 || domains[0] != "fresh.example" {
		t.Errorf("expected only fresh.example left, got %v", domains)
	}
}

func TestRepo_DeleteOlderThan_EmptyTableNoError(t *testing.T) {
	r := newTestRepo(t)
	if err := r.DeleteOlderThan(2); err != nil {
		t.Errorf("expected nil on empty table, got %v", err)
	}
}

func TestRepo_DeleteOlderThan_ClosedConnSurfacesError(t *testing.T) {
	r := newTestRepo(t)
	sqlConn, _ := r.db.DB()
	_ = sqlConn.Close()
	if err := r.DeleteOlderThan(2); err == nil {
		t.Error("expected error from closed connection, got nil")
	}
}
