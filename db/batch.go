package db

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultBatchSize is used by BatchInsert/BatchUpsert when batchSize <= 0.
const DefaultBatchSize = 1000

// BatchInsert inserts items in batches inside a single transaction.
// Returns nil if items is empty. batchSize <= 0 falls back to DefaultBatchSize.
func BatchInsert[T any](items []T, batchSize int) error {
	return batchOn(GetConnection(), items, batchSize, nil)
}

// BatchUpsert inserts items in batches, ignoring rows that violate the unique
// constraint on conflictColumns. With no conflictColumns, any unique violation
// is silently skipped (SQLite "INSERT OR IGNORE" semantics).
// Wraps all batches in a single transaction so SQLite issues one fsync at commit
// instead of one per batch.
func BatchUpsert[T any](items []T, batchSize int, conflictColumns ...string) error {
	cols := make([]clause.Column, 0, len(conflictColumns))
	for _, c := range conflictColumns {
		cols = append(cols, clause.Column{Name: c})
	}
	return batchOn(GetConnection(), items, batchSize, &clause.OnConflict{Columns: cols, DoNothing: true})
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
