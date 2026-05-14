package db

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultBatchSize is used by BatchInsert/BatchUpsert when batchSize <= 0.
const DefaultBatchSize = 1000

// BatchInsertOn inserts items in batches inside a single transaction on the
// given connection. Returns nil if items is empty. batchSize <= 0 falls back
// to DefaultBatchSize.
func BatchInsertOn[T any](conn *gorm.DB, items []T, batchSize int) error {
	return batchOn(conn, items, batchSize, nil)
}

// BatchUpsertOn inserts items in batches on the given connection, ignoring
// rows that violate the unique constraint on conflictColumns. With no
// conflictColumns, any unique violation is silently skipped (SQLite
// "INSERT OR IGNORE" semantics). All batches run inside a single transaction.
func BatchUpsertOn[T any](conn *gorm.DB, items []T, batchSize int, conflictColumns ...string) error {
	cols := make([]clause.Column, 0, len(conflictColumns))
	for _, c := range conflictColumns {
		cols = append(cols, clause.Column{Name: c})
	}
	return batchOn(conn, items, batchSize, &clause.OnConflict{Columns: cols, DoNothing: true})
}

// Deprecated: use BatchInsertOn(conn, items, batchSize) so the connection is
// passed in explicitly. This wrapper exists only until allow-domain / auth /
// suggest-to-block migrate off the singleton.
func BatchInsert[T any](items []T, batchSize int) error {
	return BatchInsertOn(GetConnection(), items, batchSize)
}

// Deprecated: use BatchUpsertOn(conn, items, batchSize, conflictColumns...).
// Same rationale as BatchInsert above.
func BatchUpsert[T any](items []T, batchSize int, conflictColumns ...string) error {
	return BatchUpsertOn(GetConnection(), items, batchSize, conflictColumns...)
}

func batchOn[T any](conn *gorm.DB, items []T, batchSize int, onConflict *clause.OnConflict) error {
	if len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	return conn.Transaction(func(tx *gorm.DB) error {
		if onConflict != nil {
			tx = tx.Clauses(*onConflict)
		}
		return tx.CreateInBatches(items, batchSize).Error
	})
}
