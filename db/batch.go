package db

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultBatchSize is used by BatchInsertOn/BatchUpsertOn when batchSize <= 0.
const DefaultBatchSize = 1000

// BatchInsertOn inserts items on the given connection, one transaction per
// batch (see batchOn for why it is not one transaction over the whole set).
// Returns nil if items is empty. batchSize <= 0 falls back to DefaultBatchSize.
func BatchInsertOn[T any](conn *gorm.DB, items []T, batchSize int) error {
	return batchOn(conn, items, batchSize, nil)
}

// BatchUpsertOn inserts items on the given connection, ignoring rows that
// violate the unique constraint on conflictColumns. With no conflictColumns,
// any unique violation is silently skipped (SQLite "INSERT OR IGNORE"
// semantics). Each batch commits in its own transaction (see batchOn).
func BatchUpsertOn[T any](conn *gorm.DB, items []T, batchSize int, conflictColumns ...string) error {
	cols := make([]clause.Column, 0, len(conflictColumns))
	for _, c := range conflictColumns {
		cols = append(cols, clause.Column{Name: c})
	}
	return batchOn(conn, items, batchSize, &clause.OnConflict{Columns: cols, DoNothing: true})
}

// BatchUpsertWith inserts items on the given connection applying a
// caller-supplied ON CONFLICT clause, one transaction per batch (see batchOn).
// Unlike BatchUpsertOn (which only does INSERT-OR-IGNORE / DoNothing), this lets
// the caller pass an additive upsert — e.g. clause.OnConflict{Columns: ...,
// DoUpdates: clause.Assignments(...)} with gorm.Expr("count + excluded.count").
// Returns nil if items is empty. batchSize <= 0 falls back to DefaultBatchSize.
func BatchUpsertWith[T any](conn *gorm.DB, items []T, batchSize int, onConflict clause.OnConflict) error {
	return batchOn(conn, items, batchSize, &onConflict)
}

func batchOn[T any](conn *gorm.DB, items []T, batchSize int, onConflict *clause.OnConflict) error {
	if len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	// Commit each batch on its own rather than wrapping the whole set in one
	// transaction. A source list is tens of thousands of rows; a single
	// transaction over all of it holds SQLite's one write lock for the entire
	// insert (seconds on the prod hardware) — long enough that a concurrent
	// writer (the async block-event worker) waits out its busy_timeout and fails
	// with SQLITE_BUSY, dropping its batch. Per-batch commits release the lock
	// every few milliseconds so concurrent writes interleave. GORM still wraps
	// each Create in its own default transaction, so a batch stays atomic; the
	// trade-off is no all-or-nothing across the whole set, which is fine here —
	// every caller is idempotent (INSERT OR IGNORE upsert re-run each sync;
	// best-effort event telemetry).
	for start := 0; start < len(items); start += batchSize {
		end := min(start+batchSize, len(items))
		tx := conn
		if onConflict != nil {
			tx = tx.Clauses(*onConflict)
		}
		if err := tx.Create(items[start:end]).Error; err != nil {
			return err
		}
	}
	return nil
}
