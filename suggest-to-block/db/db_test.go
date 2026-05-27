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

// TestGetAllSuggestBlocks_FilterRelevanceOrder pins that a string search ranks
// by relevance (exact → subdomain → prefix → substring) ahead of the default
// score sort, and that without a filter the score sort is untouched.
func TestGetAllSuggestBlocks_FilterRelevanceOrder(t *testing.T) {
	newConn := func(t *testing.T) *gorm.DB {
		t.Helper()
		conn, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		if err := conn.AutoMigrate(&SuggestBlock{}, &SuggestBlockReason{}); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		return conn
	}

	// Все домены содержат "mail.ru". Score намеренно противоречит релевантности:
	// при сортировке только по score первой была бы webmail.ru.
	seed := []SuggestBlock{
		{Domain: "webmail.ru", Score: 100},          // подстрока → tier 3
		{Domain: "mail.ru.phishing.com", Score: 80}, // префикс   → tier 2
		{Domain: "ads.mail.ru", Score: 60},          // поддомен  → tier 1
		{Domain: "mail.ru", Score: 1},               // точное    → tier 0
	}

	t.Run("string search ranks by relevance over score", func(t *testing.T) {
		conn := newConn(t)
		if err := createSuggestBlockBatchOn(conn, seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
		res, err := getAllSuggestBlocksOn(conn, GetAllParams{Limit: 100, Filter: "mail.ru"})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := []string{"mail.ru", "ads.mail.ru", "mail.ru.phishing.com", "webmail.ru"}
		if len(res.List) != len(want) {
			t.Fatalf("got %d rows, want %d", len(res.List), len(want))
		}
		for i, w := range want {
			if res.List[i].Domain != w {
				got := make([]string, len(res.List))
				for j, rec := range res.List {
					got[j] = rec.Domain
				}
				t.Fatalf("order mismatch: got %v, want %v", got, want)
			}
		}
	})

	t.Run("no filter keeps the score sort", func(t *testing.T) {
		conn := newConn(t)
		if err := createSuggestBlockBatchOn(conn, seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
		res, err := getAllSuggestBlocksOn(conn, GetAllParams{Limit: 100})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := []string{"webmail.ru", "mail.ru.phishing.com", "ads.mail.ru", "mail.ru"}
		for i, w := range want {
			if res.List[i].Domain != w {
				t.Fatalf("score order broken at %d: got %q, want %q", i, res.List[i].Domain, w)
			}
		}
	})
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

// TestUpsertWithInspect_InsertsNewWithAllReasons: a domain not yet in the
// suggest list is created with its full reason set (lexical + inspect_*).
func TestUpsertWithInspect_InsertsNewWithAllReasons(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := conn.AutoMigrate(&SuggestBlock{}, &SuggestBlockReason{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	reasons := []SuggestBlockReason{
		{Code: "risky_tld"},
		{Code: "bad_keywords"},
		{Code: "inspect_vt_malicious", MatchValue: "malicious=5"},
	}
	if err := upsertWithInspectOn(conn, "evil.example.com", 15, reasons); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var got SuggestBlock
	if err := conn.Preload("Reasons").Where("domain = ?", "evil.example.com").First(&got).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Score != 15 {
		t.Errorf("score = %d, want 15 (lexical)", got.Score)
	}
	if !got.Active {
		t.Error("new suggest row must be active")
	}
	if len(got.Reasons) != 3 {
		t.Fatalf("expected 3 reasons on insert, got %d (%+v)", len(got.Reasons), got.Reasons)
	}
}

// TestUpsertWithInspect_RefreshesOnlyInspectReasons: on a re-run for an existing
// row, lexical reasons survive, stale inspect_* reasons are replaced, the score
// is refreshed, and no duplicates accumulate.
func TestUpsertWithInspect_RefreshesOnlyInspectReasons(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := conn.AutoMigrate(&SuggestBlock{}, &SuggestBlockReason{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// First pass: a young domain flagged by RDAP.
	first := []SuggestBlockReason{
		{Code: "risky_tld"},
		{Code: "inspect_rdap_young", MatchValue: "age_days=3"},
	}
	if err := upsertWithInspectOn(conn, "x.example.com", 12, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second pass (after TTL): now VirusTotal flags it malicious; RDAP signal gone.
	second := []SuggestBlockReason{
		{Code: "risky_tld"}, // lexical, already present — must not duplicate
		{Code: "inspect_vt_malicious", MatchValue: "malicious=7"},
	}
	if err := upsertWithInspectOn(conn, "x.example.com", 18, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var got SuggestBlock
	if err := conn.Preload("Reasons").Where("domain = ?", "x.example.com").First(&got).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Score != 18 {
		t.Errorf("score must refresh to 18, got %d", got.Score)
	}

	codes := map[string]int{}
	for _, r := range got.Reasons {
		codes[r.Code]++
	}
	if codes["risky_tld"] != 1 {
		t.Errorf("lexical risky_tld must survive exactly once, got %d", codes["risky_tld"])
	}
	if codes["inspect_rdap_young"] != 0 {
		t.Errorf("stale inspect_rdap_young must be removed, got %d", codes["inspect_rdap_young"])
	}
	if codes["inspect_vt_malicious"] != 1 {
		t.Errorf("new inspect_vt_malicious must be present once, got %d", codes["inspect_vt_malicious"])
	}
	if len(got.Reasons) != 2 {
		t.Errorf("expected exactly 2 reasons after refresh, got %d (%+v)", len(got.Reasons), got.Reasons)
	}

	// Only one suggest row — upsert, not duplicate insert.
	var count int64
	conn.Model(&SuggestBlock{}).Where("domain = ?", "x.example.com").Count(&count)
	if count != 1 {
		t.Errorf("expected single suggest row, got %d", count)
	}
}

// TestUpsertWithInspect_PreservesDeactivated: a row an operator turned off must
// stay off after a later worker pass.
func TestUpsertWithInspect_PreservesDeactivated(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := conn.AutoMigrate(&SuggestBlock{}, &SuggestBlockReason{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := conn.Create(&SuggestBlock{
		Domain: "off.example.com", Score: 12,
		Reasons: []SuggestBlockReason{{Code: "risky_tld"}},
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Force Active=false: GORM's default:true tag substitutes the default for a
	// zero-value bool on Create, so it must be set with an explicit Update.
	if err := conn.Model(&SuggestBlock{}).Where("domain = ?", "off.example.com").
		Update("active", false).Error; err != nil {
		t.Fatalf("deactivate seed: %v", err)
	}

	if err := upsertWithInspectOn(conn, "off.example.com", 18, []SuggestBlockReason{
		{Code: "risky_tld"},
		{Code: "inspect_vt_malicious", MatchValue: "malicious=4"},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var got SuggestBlock
	if err := conn.Where("domain = ?", "off.example.com").First(&got).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Active {
		t.Error("a deactivated suggest row must not be reactivated by the worker")
	}
}
