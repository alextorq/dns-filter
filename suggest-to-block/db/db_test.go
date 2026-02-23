package db

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Helper function to test the logic without relying on the global DB connection
func createSuggestBlockBatchWithDB(conn *gorm.DB, suggests []SuggestBlock) error {
	// clause.OnConflict{DoNothing: true} говорит БД:
	// "Если запись с таким uniqueIndex уже есть, просто пропусти ее и не выдавай ошибку"
	return conn.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "domain"}},
		DoNothing: true,
	}).Create(&suggests).Error
}

func TestCreateSuggestBlockBatchLogic(t *testing.T) {
	// Use in-memory SQLite for testing
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// Migrate the schema
	err = db.AutoMigrate(&SuggestBlock{})
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// 1. Insert initial batch
	suggests := []SuggestBlock{
		{Domain: "example.com", Reason: "test", Score: 10},
		{Domain: "test.com", Reason: "test", Score: 20},
	}

	err = createSuggestBlockBatchWithDB(db, suggests)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	var count int64
	db.Model(&SuggestBlock{}).Count(&count)
	if count != 2 {
		t.Errorf("Expected 2 records, got %d", count)
	}

	// 2. Insert batch with duplicates
	suggests2 := []SuggestBlock{
		{Domain: "example.com", Reason: "duplicate", Score: 100},
		{Domain: "new.com", Reason: "new", Score: 30},
	}

	err = createSuggestBlockBatchWithDB(db, suggests2)
	if err != nil {
		t.Fatalf("Failed to create second batch: %v", err)
	}

	db.Model(&SuggestBlock{}).Count(&count)
	if count != 3 {
		t.Errorf("Expected 3 records, got %d", count)
	}

	var s SuggestBlock
	db.Where("domain = ?", "example.com").First(&s)
	if s.Score != 10 {
		t.Errorf("Expected score 10 (original), got %d", s.Score)
	}
}
