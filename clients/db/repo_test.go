package db

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) *Repo {
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
	if err := conn.AutoMigrate(&Client{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepo(conn)
}

func TestRepo_ClientLifecycle(t *testing.T) {
	r := newTestRepo(t)
	c := &Client{IP: "10.0.0.2", MAC: "aa:bb:cc:dd:ee:ff", Name: "phone"}
	if err := r.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByID(c.ID)
	if err != nil || got.Name != "phone" {
		t.Fatalf("GetByID: client=%+v err=%v", got, err)
	}
	if err := r.UpdateFields(c.ID, map[string]any{"name": "tablet", "filtered": false}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}
	byIP, err := r.GetByIP(c.IP)
	if err != nil || byIP.Name != "tablet" || byIP.Filtered {
		t.Fatalf("GetByIP: client=%+v err=%v", byIP, err)
	}
	byMAC, err := r.GetByMAC(c.MAC)
	if err != nil || byMAC.ID != c.ID {
		t.Fatalf("GetByMAC: client=%+v err=%v", byMAC, err)
	}
	excluded, err := r.GetExcluded()
	if err != nil || len(excluded) != 1 || excluded[0].ID != c.ID {
		t.Fatalf("GetExcluded: clients=%+v err=%v", excluded, err)
	}
	if err := r.Delete(c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetByID(c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRepo_GetAllOrdersByID(t *testing.T) {
	r := newTestRepo(t)
	for _, ip := range []string{"10.0.0.3", "10.0.0.1", "10.0.0.2"} {
		if err := r.Create(&Client{IP: ip}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rows, err := r.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].ID >= rows[i].ID {
			t.Fatalf("rows not ordered by id: %+v", rows)
		}
	}
}

func TestRepo_CreatePreservesExplicitExclusion(t *testing.T) {
	r := newTestRepo(t)
	c := &Client{IP: "10.0.0.9", Filtered: false}
	if err := r.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByID(c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Filtered || c.Filtered {
		t.Fatalf("explicit filtered=false was lost: persisted=%v input=%v", got.Filtered, c.Filtered)
	}
}

func TestRepo_EmptyLookupReturnsErrNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetByIP(""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByIP empty: %v", err)
	}
	if _, err := r.GetByMAC(""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByMAC empty: %v", err)
	}
}
