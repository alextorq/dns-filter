package db

import (
	"testing"
	"time"
)

// happy: the adapter's GetAllActiveFilters returns exactly the traffic repo's
// allowed-domain pool (DISTINCT, blocked=false), satisfying suggest's AllowRepo.
func TestAllowFilterAdapter_GetAllActiveFilters(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	if err := r.UpsertBatch([]DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "good.example", Blocked: false, Day: d, Count: 1, LastSeen: now},
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "ads.example", Blocked: true, Day: d, Count: 1, LastSeen: now},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	adapter := NewAllowFilterAdapter(r)
	got, err := adapter.GetAllActiveFilters()
	if err != nil {
		t.Fatalf("GetAllActiveFilters: %v", err)
	}
	if len(got) != 1 || got[0] != "good.example" {
		t.Fatalf("adapter must return the allowed pool [good.example], got %v", got)
	}
}

// negative: a repo error propagates verbatim through the adapter (fail-closed
// — suggest's Collect returns the error rather than scoring a half-built pool).
func TestAllowFilterAdapter_PropagatesError(t *testing.T) {
	r := newTestRepo(t)
	if err := r.db.Migrator().DropTable(&DomainTraffic{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	adapter := NewAllowFilterAdapter(r)
	if _, err := adapter.GetAllActiveFilters(); err == nil {
		t.Fatal("expected error when domain_traffic is missing, got nil")
	}
}

// empty: the adapter returns an empty (non-nil-panic) slice, so Collect's
// CollectSuggest receives an empty candidate pool without crashing.
func TestAllowFilterAdapter_EmptyPool(t *testing.T) {
	r := newTestRepo(t)
	adapter := NewAllowFilterAdapter(r)
	got, err := adapter.GetAllActiveFilters()
	if err != nil {
		t.Fatalf("GetAllActiveFilters on empty table: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty pool, got %v", got)
	}
}
