package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCreateSuggestBlockBatchLogic(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	if err := conn.AutoMigrate(&SuggestBlock{}, &SuggestBlockReason{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// 1. Insert initial batch — both rows should land with their reasons.
	first := []SuggestBlock{
		{Domain: "example.com", Score: 10, Reasons: []SuggestBlockReason{{Code: "risky_tld"}}},
		{Domain: "test.com", Score: 20, Reasons: []SuggestBlockReason{{Code: "bad_keywords"}}},
	}
	if err := createSuggestBlockBatchOn(conn, first); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	var count int64
	conn.Model(&SuggestBlock{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 suggest rows, got %d", count)
	}

	var reasonCount int64
	conn.Model(&SuggestBlockReason{}).Count(&reasonCount)
	if reasonCount != 2 {
		t.Errorf("expected 2 reason rows, got %d", reasonCount)
	}

	// 2. Second batch contains a duplicate domain ("example.com") and a
	// fresh one ("new.com"). The duplicate must be skipped completely —
	// no new SuggestBlock and no extra reasons attached to the existing one.
	second := []SuggestBlock{
		{Domain: "example.com", Score: 100, Reasons: []SuggestBlockReason{{Code: "homograph"}}},
		{Domain: "new.com", Score: 30, Reasons: []SuggestBlockReason{{Code: "numeric_run"}}},
	}
	if err := createSuggestBlockBatchOn(conn, second); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	conn.Model(&SuggestBlock{}).Count(&count)
	if count != 3 {
		t.Errorf("expected 3 suggest rows after dedup, got %d", count)
	}

	conn.Model(&SuggestBlockReason{}).Count(&reasonCount)
	if reasonCount != 3 {
		t.Errorf("expected 3 reason rows after dedup (no extra for duplicate), got %d", reasonCount)
	}

	var s SuggestBlock
	if err := conn.Preload("Reasons").Where("domain = ?", "example.com").First(&s).Error; err != nil {
		t.Fatalf("load example.com: %v", err)
	}
	if s.Score != 10 {
		t.Errorf("expected original score 10, got %d", s.Score)
	}
	if len(s.Reasons) != 1 || s.Reasons[0].Code != "risky_tld" {
		t.Errorf("expected reasons preserved as [risky_tld], got %+v", s.Reasons)
	}
}

// TestGetAllSuggestBlocks_FilterByCodes pins the OR-semantic of the Codes
// filter and the invariant that Preload("Reasons") returns ALL reasons of a
// matched suggest, not only the ones that satisfied the filter.
func TestGetAllSuggestBlocks_FilterByCodes(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := conn.AutoMigrate(&SuggestBlock{}, &SuggestBlockReason{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	seed := []SuggestBlock{
		{
			Domain: "alpha.com", Score: 30,
			Reasons: []SuggestBlockReason{
				{Code: "risky_tld"},
				{Code: "homograph"},
			},
		},
		{
			Domain: "beta.com", Score: 20,
			Reasons: []SuggestBlockReason{
				{Code: "bad_keywords"},
			},
		},
		{
			Domain: "gamma.com", Score: 10,
			Reasons: []SuggestBlockReason{
				{Code: "numeric_run"},
			},
		},
	}
	if err := createSuggestBlockBatchOn(conn, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Single code → keep only suggests that have that code among their reasons.
	res, err := getAllSuggestBlocksOn(conn, GetAllParams{Limit: 100, Codes: []string{"risky_tld"}})
	if err != nil {
		t.Fatalf("filter by single code: %v", err)
	}
	if res.Total != 1 || len(res.List) != 1 || res.List[0].Domain != "alpha.com" {
		t.Fatalf("expected only alpha.com, got total=%d list=%+v", res.Total, res.List)
	}
	// Critical: filter must not strip the OTHER reasons of the matched suggest.
	if len(res.List[0].Reasons) != 2 {
		t.Errorf("expected both reasons of alpha.com loaded, got %+v", res.List[0].Reasons)
	}

	// Multiple codes → OR-semantic, any match wins.
	res, err = getAllSuggestBlocksOn(conn, GetAllParams{Limit: 100, Codes: []string{"bad_keywords", "numeric_run"}})
	if err != nil {
		t.Fatalf("filter by multiple codes: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("expected 2 matches (beta+gamma), got total=%d list=%+v", res.Total, res.List)
	}

	// Empty filter → no narrowing, all rows visible.
	res, err = getAllSuggestBlocksOn(conn, GetAllParams{Limit: 100})
	if err != nil {
		t.Fatalf("no filter: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("expected all 3 rows, got %d", res.Total)
	}

	// Code that nobody has → empty result.
	res, err = getAllSuggestBlocksOn(conn, GetAllParams{Limit: 100, Codes: []string{"nonexistent"}})
	if err != nil {
		t.Fatalf("filter by missing code: %v", err)
	}
	if res.Total != 0 || len(res.List) != 0 {
		t.Errorf("expected empty result for unknown code, got total=%d list=%+v", res.Total, res.List)
	}
}
