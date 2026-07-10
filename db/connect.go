package db

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite" // Pure-Go SQLite driver (modernc.org/sqlite)
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	// busyTimeoutMs pins how long a writer blocked by SQLite's single-writer lock
	// waits before failing with SQLITE_BUSY. The glebarez driver already defaults
	// this to 5000 on every connection (glebarez/go-sqlite sqlite.go), so we set
	// it explicitly only to make the value visible here and survive a driver
	// default change. It is NOT what fixed the dropped event batches seen in
	// prod: there the bulk source-sync transaction held the lock LONGER than the
	// timeout — see the per-batch commits in db/batch.go. Matches the GORM
	// slow-query threshold set in Open.
	busyTimeoutMs = 5000
	// sqliteCacheSizeKiB is the page-cache size; a negative value means KiB, so
	// -64000 is a 64 MiB cache (vs the 2 MiB SQLite default).
	sqliteCacheSizeKiB = -64000
	// maxOpenConns bounds the database/sql pool. The previous default (0 =
	// unbounded) let the pool open arbitrary connections, which for SQLite only
	// amplifies write contention. A small bound keeps read concurrency (WAL)
	// while writes serialise behind busy_timeout, and — together with DSN-level
	// PRAGMAs (see buildDSN) — guarantees every connection is tuned identically.
	maxOpenConns = 4
)

// OpenDeps is the complete construction contract for an instrumented SQLite
// connection. DBName must be unique within Metrics' registry so every pool
// keeps its own go_sql_* metric series.
type OpenDeps struct {
	Path    string
	Log     ErrorLogger
	Metrics *Metrics
	DBName  string
}

// buildDSN appends the per-connection PRAGMAs to the SQLite path as DSN query
// parameters. The modernc driver (via glebarez) runs every `_pragma=` on each
// new connection, so the tuning applies to the WHOLE pool — unlike a one-off
// `db.Exec("PRAGMA ...")`, which only configures whichever single pooled
// connection happened to serve it. In prod the pool opened 3 connections, so
// synchronous/cache_size stuck to just one and the other two silently ran with
// synchronous=FULL + a 2 MiB cache. journal_mode=WAL is persisted in the file
// header (global regardless), but it is harmless to repeat here.
func buildDSN(path string) string {
	pragmas := []string{
		fmt.Sprintf("busy_timeout(%d)", busyTimeoutMs),
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"temp_store(MEMORY)",
		fmt.Sprintf("cache_size(%d)", sqliteCacheSizeKiB),
	}
	parts := make([]string, len(pragmas))
	for i, p := range pragmas {
		parts[i] = "_pragma=" + p
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + strings.Join(parts, "&")
}

// openConnection opens the GORM DB at path with the DSN PRAGMAs and pool bounds
// applied. Split out of Open so tests can exercise the real connection
// config against a temp file without the package-level singleton.
func openConnection(path string, gormLog gormlogger.Interface) (*gorm.DB, error) {
	gdb, err := gorm.Open(sqlite.Open(buildDSN(path)), &gorm.Config{Logger: gormLog})
	if err != nil {
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	return gdb, nil
}

// Open creates a configured SQLite connection. The caller owns the resulting
// *gorm.DB and decides how to surface a startup failure; this package never
// terminates the process itself.
func Open(deps OpenDeps) (*gorm.DB, error) {
	if deps.Log == nil {
		return nil, errors.New("db open: error logger is required")
	}
	if deps.Metrics == nil {
		return nil, errors.New("db open: metrics are required")
	}
	if strings.TrimSpace(deps.DBName) == "" {
		return nil, errors.New("db open: metrics DB name is required")
	}

	conn, err := openConnection(deps.Path, newGormLogger())
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// Query callback registration remains best-effort, but a pool collector name
	// collision would silently leave this connection unobserved. Reject it at
	// construction time rather than returning a pool with stale metrics.
	if err := instrumentConnection(conn, deps.Log, deps.Metrics, deps.DBName); err != nil {
		if sqlDB, dbErr := conn.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, fmt.Errorf("instrument sqlite database: %w", err)
	}
	return conn, nil
}

func newGormLogger() gormlogger.Interface {
	// GORM по умолчанию логирует SQL целиком при превышении SlowThreshold
	// (200ms). Bulk-инсерты в source.Sync() — десятки тысяч строк за раз,
	// каждый запрос подходит под порог и пишет в stdout 100+ КБ VALUES.
	// Поднимаем порог до 5 сек (реальные тормоза всё ещё ловим) и просим
	// логгер использовать `?` вместо инлайн-значений — slow-warn остаётся
	// диагностически полезным, но не флудит.
	return gormlogger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             5 * time.Second,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  true,
		},
	)
}
