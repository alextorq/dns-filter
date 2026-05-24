package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) (*Repo, *gorm.DB) {
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
	if err := conn.AutoMigrate(&Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepo(conn), conn
}

func TestRepo_SetThenGet(t *testing.T) {
	repo, _ := newTestRepo(t)

	if err := repo.Set("log_level", "DEBUG"); err != nil {
		t.Fatalf("set: %v", err)
	}

	val, found, err := repo.Get("log_level")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true after Set")
	}
	if val != "DEBUG" {
		t.Errorf("expected DEBUG, got %q", val)
	}
}

// Get of a key that was never stored must report found=false with no error so
// callers can cleanly fall back to the env/compiled default. An empty-string
// override and an absent row must be distinguishable.
func TestRepo_Get_MissingKey(t *testing.T) {
	repo, _ := newTestRepo(t)

	val, found, err := repo.Get("does_not_exist")
	if err != nil {
		t.Fatalf("expected nil error for missing key, got %v", err)
	}
	if found {
		t.Error("expected found=false for missing key")
	}
	if val != "" {
		t.Errorf("expected empty value for missing key, got %q", val)
	}
}

// Set must upsert: a second write to the same key updates the value in place
// rather than inserting a duplicate row (Key is the primary key).
func TestRepo_Set_UpsertsInPlace(t *testing.T) {
	repo, conn := newTestRepo(t)

	if err := repo.Set("doh_upstream", "https://one.example/dns-query"); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if err := repo.Set("doh_upstream", "https://two.example/dns-query"); err != nil {
		t.Fatalf("second set: %v", err)
	}

	var count int64
	conn.Model(&Setting{}).Where("key = ?", "doh_upstream").Count(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 row after upsert, got %d", count)
	}

	val, _, err := repo.Get("doh_upstream")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if val != "https://two.example/dns-query" {
		t.Errorf("expected the second value to win, got %q", val)
	}
}

func TestRepo_GetAll_SortedByKey(t *testing.T) {
	repo, _ := newTestRepo(t)

	_ = repo.Set("log_level", "INFO")
	_ = repo.Set("doh_upstream", "https://x.example/dns-query")

	rows, err := repo.GetAll()
	if err != nil {
		t.Fatalf("getall: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Key != "doh_upstream" || rows[1].Key != "log_level" {
		t.Errorf("expected key-sorted order, got %q then %q", rows[0].Key, rows[1].Key)
	}
}

func TestRepo_Delete(t *testing.T) {
	repo, _ := newTestRepo(t)

	t.Run("removes an existing override", func(t *testing.T) {
		if err := repo.Set("log_level", "WARN"); err != nil {
			t.Fatalf("set: %v", err)
		}
		if err := repo.Delete("log_level"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		_, found, err := repo.Get("log_level")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if found {
			t.Error("expected the row to be gone after Delete")
		}
	})

	t.Run("deleting a missing key is not an error", func(t *testing.T) {
		if err := repo.Delete("never_existed"); err != nil {
			t.Errorf("expected nil error deleting missing key, got %v", err)
		}
	})
}

// A broken connection must surface as an error rather than be swallowed —
// settings.Set persists before applying, so a silent failure here would apply
// an unpersisted change and lose it on the next restart.
func TestRepo_DBErrorSurfaces(t *testing.T) {
	repo, conn := newTestRepo(t)
	sqlConn, _ := conn.DB()
	_ = sqlConn.Close()

	if _, _, err := repo.Get("log_level"); err == nil {
		t.Error("expected error from closed connection on Get")
	}
	if err := repo.Set("log_level", "INFO"); err == nil {
		t.Error("expected error from closed connection on Set")
	}
}
