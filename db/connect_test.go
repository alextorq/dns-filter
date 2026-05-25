package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func silentGorm() gormlogger.Interface {
	return gormlogger.Default.LogMode(gormlogger.Silent)
}

// ----- buildDSN -----

func TestBuildDSN_ContainsTuningPragmas(t *testing.T) {
	dsn := buildDSN("./filter.sqlite")
	for _, want := range []string{
		"./filter.sqlite?",
		"_pragma=busy_timeout(5000)",
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=temp_store(MEMORY)",
		"_pragma=cache_size(-64000)",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("buildDSN = %q, missing %q", dsn, want)
		}
	}
}

func TestBuildDSN_PreservesExistingQuery(t *testing.T) {
	// Edge case: a path already carrying a query string must keep it and gain
	// the pragmas via & rather than emitting a second ?.
	dsn := buildDSN("file:test.db?cache=shared")
	if !strings.Contains(dsn, "cache=shared") {
		t.Errorf("dropped existing query: %q", dsn)
	}
	if got := strings.Count(dsn, "?"); got != 1 {
		t.Errorf("expected exactly one '?', got %d in %q", got, dsn)
	}
	if !strings.Contains(dsn, "&_pragma=busy_timeout(5000)") {
		t.Errorf("pragmas not appended with &: %q", dsn)
	}
}

// ----- per-connection pragma application -----

// TestOpenConnection_PragmasApplyToEveryPooledConnection is the regression for
// tuning that used to stick to a single pooled connection: it pins every
// connection in the pool at once and asserts each one carries the PRAGMAs.
func TestOpenConnection_PragmasApplyToEveryPooledConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pragmas.sqlite")
	gdb, err := openConnection(path, silentGorm())
	if err != nil {
		t.Fatalf("openConnection: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("DB(): %v", err)
	}
	defer sqlDB.Close()

	ctx := context.Background()
	conns := make([]*sql.Conn, 0, maxOpenConns)
	for i := range maxOpenConns {
		c, err := sqlDB.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()

	for i, c := range conns {
		var busy, syncMode int
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatalf("conn %d busy_timeout: %v", i, err)
		}
		if busy != busyTimeoutMs {
			t.Errorf("conn %d busy_timeout = %d, want %d", i, busy, busyTimeoutMs)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&syncMode); err != nil {
			t.Fatalf("conn %d synchronous: %v", i, err)
		}
		if syncMode != 1 { // 1 = NORMAL, 2 = FULL
			t.Errorf("conn %d synchronous = %d, want 1 (NORMAL)", i, syncMode)
		}
	}
}

// TestPlainConnection_DefaultsAreUntuned documents the bug from the other side:
// a connection opened WITHOUT the DSN pragmas keeps SQLite's defaults —
// synchronous=FULL (fsync on every commit) and a 2 MiB page cache. That is what
// the un-tuned pooled connections ran with in prod, where the tuning had been
// applied via a one-off Exec to only a single connection. (busy_timeout is NOT
// part of this: the glebarez driver already defaults it to 5000 on every
// connection — see busyTimeoutMs.) This proves the DSN is the lever for the
// per-connection tuning that matters.
func TestPlainConnection_DefaultsAreUntuned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.sqlite")
	gdb, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: silentGorm()})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()

	var syncMode, cache int
	if err := sqlDB.QueryRow("PRAGMA synchronous").Scan(&syncMode); err != nil {
		t.Fatalf("synchronous: %v", err)
	}
	if syncMode != 2 { // 2 = FULL (the untuned default), 1 = NORMAL (what the DSN sets)
		t.Errorf("plain synchronous = %d, want 2 (FULL); the DSN lowers this to NORMAL", syncMode)
	}
	if err := sqlDB.QueryRow("PRAGMA cache_size").Scan(&cache); err != nil {
		t.Fatalf("cache_size: %v", err)
	}
	if cache != -2000 { // -2000 = 2 MiB default; the DSN raises it to -64000 (64 MiB)
		t.Errorf("plain cache_size = %d, want -2000 (the untuned default)", cache)
	}
}

// ----- write contention / busy_timeout behaviour -----

// TestOpenConnection_ConcurrentWritesDoNotReturnBusy reproduces the prod
// scenario (several writers contending) and asserts busy_timeout prevents the
// SQLITE_BUSY that was dropping event batches.
func TestOpenConnection_ConcurrentWritesDoNotReturnBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.sqlite")
	gdb, err := openConnection(path, silentGorm())
	if err != nil {
		t.Fatalf("openConnection: %v", err)
	}
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	if err := gdb.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, v INTEGER)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	const writers, perWriter = 8, 30
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for range writers {
		wg.Go(func() {
			for i := range perWriter {
				if err := gdb.Exec("INSERT INTO t (v) VALUES (?)", i).Error; err != nil {
					errCh <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent write returned an error (busy_timeout should prevent SQLITE_BUSY): %v", err)
	}

	var count int64
	if err := gdb.Raw("SELECT count(*) FROM t").Scan(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if want := int64(writers * perWriter); count != want {
		t.Errorf("rows = %d, want %d", count, want)
	}
}

// TestBusyTimeout_HeldLockBeyondTimeoutFails reproduces the exact prod failure
// mode: a writer that holds SQLite's write lock for LONGER than busy_timeout
// makes a concurrent writer fail with SQLITE_BUSY. In prod the bulk source-sync
// transaction outlived the 5s busy_timeout and the async event worker dropped
// its batch (the bug the per-batch commits in batch.go fix). Here a short
// busy_timeout keeps the test fast and deterministic.
func TestBusyTimeout_HeldLockBeyondTimeoutFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held.sqlite")
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(100)"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: silentGorm()})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, _ := gdb.DB()
	sqlDB.SetMaxOpenConns(2)
	defer sqlDB.Close()
	if err := gdb.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, v INTEGER)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx := context.Background()
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Acquire the WAL write lock and never release it for the duration of the
	// competing write — i.e. the lock is held far beyond the 100ms busy_timeout.
	if _, err := tx.ExecContext(ctx, "INSERT INTO t (v) VALUES (1)"); err != nil {
		t.Fatalf("holder insert: %v", err)
	}

	// The competing writer waits out busy_timeout (~100ms), then fails because
	// the lock is still held.
	_, err = sqlDB.ExecContext(ctx, "INSERT INTO t (v) VALUES (2)")
	if err == nil {
		t.Fatal("expected SQLITE_BUSY when the write lock is held beyond busy_timeout, got nil")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "lock") && !strings.Contains(msg, "busy") {
		t.Fatalf("expected a busy/locked error, got: %v", err)
	}
}

// TestBusyTimeout_WaitsForWriteLockRelease is the positive counterpart: with
// the production busy_timeout, a blocked writer waits for the lock to free and
// then succeeds, instead of erroring out.
func TestBusyTimeout_WaitsForWriteLockRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busywait.sqlite")
	gdb, err := openConnection(path, silentGorm()) // busy_timeout(5000)
	if err != nil {
		t.Fatalf("openConnection: %v", err)
	}
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	if err := gdb.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, v INTEGER)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx := context.Background()
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO t (v) VALUES (1)"); err != nil {
		t.Fatalf("holder insert: %v", err)
	}

	const holdFor = 300 * time.Millisecond
	go func() {
		time.Sleep(holdFor)
		_ = tx.Commit()
	}()

	start := time.Now()
	if _, err := sqlDB.ExecContext(ctx, "INSERT INTO t (v) VALUES (2)"); err != nil {
		t.Fatalf("writer should have waited out the lock and succeeded, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed < holdFor-50*time.Millisecond {
		t.Fatalf("writer did not wait for the lock (%s); busy_timeout not effective", elapsed)
	}
}
