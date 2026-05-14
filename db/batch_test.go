package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type testItem struct {
	ID   uint   `gorm:"primarykey"`
	Name string `gorm:"uniqueIndex"`
	Age  int
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlConn, err := conn.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	// SQLite ":memory:" is per-connection, so pin to one connection so all
	// queries see the same in-memory database.
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(&testItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

func countItems(t *testing.T, conn *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := conn.Model(&testItem{}).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestBatchOn_EmptySliceIsNoOp(t *testing.T) {
	conn := newTestDB(t)
	if err := batchOn(conn, []testItem{}, 100, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countItems(t, conn); got != 0 {
		t.Errorf("expected 0 rows, got %d", got)
	}
}

func TestBatchOn_NilSliceIsNoOp(t *testing.T) {
	conn := newTestDB(t)
	var nothing []testItem
	if err := batchOn(conn, nothing, 100, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchOn_InsertsAllItems(t *testing.T) {
	conn := newTestDB(t)
	items := []testItem{
		{Name: "a", Age: 1},
		{Name: "b", Age: 2},
		{Name: "c", Age: 3},
	}
	if err := batchOn(conn, items, 100, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := countItems(t, conn); got != 3 {
		t.Errorf("expected 3 rows, got %d", got)
	}
}

func TestBatchOn_RespectsSmallBatchSize(t *testing.T) {
	conn := newTestDB(t)
	items := make([]testItem, 0, 10)
	for i := range 10 {
		items = append(items, testItem{Name: string(rune('a' + i)), Age: i})
	}
	// batchSize=3 means 4 batches (3+3+3+1) — all rows must still land.
	if err := batchOn(conn, items, 3, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := countItems(t, conn); got != 10 {
		t.Errorf("expected 10 rows, got %d", got)
	}
}

func TestBatchOn_DefaultBatchSizeWhenZero(t *testing.T) {
	conn := newTestDB(t)
	items := []testItem{{Name: "a"}, {Name: "b"}}
	if err := batchOn(conn, items, 0, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := countItems(t, conn); got != 2 {
		t.Errorf("expected 2 rows, got %d", got)
	}
}

func TestBatchOn_NoOnConflict_DuplicateUniqueErrors(t *testing.T) {
	conn := newTestDB(t)
	if err := batchOn(conn, []testItem{{Name: "a"}}, 100, nil); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := batchOn(conn, []testItem{{Name: "a"}}, 100, nil)
	if err == nil {
		t.Fatal("expected error on duplicate unique key, got nil")
	}
}

func TestBatchOn_OnConflictIgnoresDuplicates(t *testing.T) {
	conn := newTestDB(t)
	first := []testItem{
		{Name: "a", Age: 1},
		{Name: "b", Age: 2},
	}
	if err := batchOn(conn, first, 100, &clause.OnConflict{DoNothing: true}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := []testItem{
		{Name: "a", Age: 99}, // would violate unique on Name
		{Name: "c", Age: 3},
	}
	if err := batchOn(conn, second, 100, &clause.OnConflict{DoNothing: true}); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	if got := countItems(t, conn); got != 3 {
		t.Errorf("expected 3 rows, got %d", got)
	}

	// Original "a" row must be untouched.
	var existing testItem
	if err := conn.Where("name = ?", "a").First(&existing).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if existing.Age != 1 {
		t.Errorf("expected original Age=1 to be preserved, got %d", existing.Age)
	}
}

func TestBatchOn_OnConflictWithExplicitColumn(t *testing.T) {
	conn := newTestDB(t)
	if err := batchOn(conn, []testItem{{Name: "a", Age: 1}}, 100, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	onConflict := &clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true,
	}
	err := batchOn(conn, []testItem{{Name: "a", Age: 99}, {Name: "b", Age: 2}}, 100, onConflict)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := countItems(t, conn); got != 2 {
		t.Errorf("expected 2 rows, got %d", got)
	}
}

// Public BatchInsert / BatchUpsert are thin wrappers around batchOn(GetConnection(), ...);
// the singleton makes them awkward to test here. Their integration is exercised by
// the existing tests in blocked-domain/db, allow-domain/db, and suggest-to-block/db.

// The *On variants accept the connection explicitly so they can be unit-tested
// directly without touching the global singleton.

func TestBatchInsertOn_InsertsAll(t *testing.T) {
	conn := newTestDB(t)
	items := []testItem{{Name: "a", Age: 1}, {Name: "b", Age: 2}}
	if err := BatchInsertOn(conn, items, 100); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := countItems(t, conn); got != 2 {
		t.Errorf("expected 2 rows, got %d", got)
	}
}

func TestBatchInsertOn_DuplicateUniqueErrors(t *testing.T) {
	conn := newTestDB(t)
	if err := BatchInsertOn(conn, []testItem{{Name: "a"}}, 100); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := BatchInsertOn(conn, []testItem{{Name: "a"}}, 100); err == nil {
		t.Fatal("expected error on duplicate unique key, got nil")
	}
}

func TestBatchUpsertOn_IgnoresDuplicates(t *testing.T) {
	conn := newTestDB(t)
	if err := BatchUpsertOn(conn, []testItem{{Name: "a", Age: 1}}, 100, "name"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Re-insert with the same Name must be silently ignored, not error.
	if err := BatchUpsertOn(conn, []testItem{{Name: "a", Age: 99}, {Name: "b", Age: 2}}, 100, "name"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := countItems(t, conn); got != 2 {
		t.Errorf("expected 2 rows, got %d", got)
	}
	// And the original row's Age stays untouched (DoNothing, not DoUpdate).
	var existing testItem
	if err := conn.Where("name = ?", "a").First(&existing).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if existing.Age != 1 {
		t.Errorf("expected Age=1 preserved, got %d", existing.Age)
	}
}

func TestBatchUpsertOn_EmptyIsNoOp(t *testing.T) {
	conn := newTestDB(t)
	if err := BatchUpsertOn(conn, []testItem{}, 100, "name"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := countItems(t, conn); got != 0 {
		t.Errorf("expected 0 rows, got %d", got)
	}
}
