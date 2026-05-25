package traffic_use_cases_prune

import (
	"errors"
	"testing"
	"time"

	traffic_db "github.com/alextorq/dns-filter/traffic/db"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// fakeRepo records the cutoff it was asked to prune with.
type fakeRepo struct {
	calls  int
	cutoff time.Time
	err    error
}

func (f *fakeRepo) DeleteOlderThan(cutoff time.Time) error {
	f.calls++
	f.cutoff = cutoff
	return f.err
}

// cutoffFor must return local midnight today minus N days, so that a row whose
// Day equals the cutoff (i.e. exactly N days ago) is KEPT (DeleteOlderThan is a
// strict <), and the day N+1 days ago is pruned.
func TestCutoffFor_LocalMidnightMinusDays(t *testing.T) {
	loc := time.FixedZone("UTC+5", 5*3600)
	// A time late in the local day to prove we truncate to local midnight, not
	// to the UTC day.
	now := time.Date(2026, 5, 26, 23, 30, 0, 0, loc)

	got := cutoffForIn(now, 30, loc)
	want := time.Date(2026, 4, 26, 0, 0, 0, 0, loc) // 30 days before local midnight 2026-05-26

	if !got.Equal(want) {
		t.Fatalf("cutoff = %s, want %s", got, want)
	}
	if got.Hour() != 0 || got.Minute() != 0 || got.Second() != 0 || got.Nanosecond() != 0 {
		t.Errorf("cutoff must be at local midnight, got %s", got)
	}
}

// Happy path: pruneTask reads the current retention atomic and calls the repo
// with the matching cutoff.
func TestPruneTask_UsesCurrentRetention(t *testing.T) {
	SetRetentionDays(30)
	repo := &fakeRepo{}
	loc := time.Local
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, loc)

	if err := pruneTaskAt(repo, now); err != nil {
		t.Fatalf("pruneTask: %v", err)
	}
	if repo.calls != 1 {
		t.Fatalf("expected 1 DeleteOlderThan call, got %d", repo.calls)
	}
	want := cutoffForIn(now, 30, loc)
	if !repo.cutoff.Equal(want) {
		t.Errorf("cutoff = %s, want %s (30-day retention)", repo.cutoff, want)
	}
}

// The loop must read the CURRENT atomic value on each tick: a retention change
// takes effect on the next prune without a restart. We simulate two ticks with
// different retentions and assert the cutoff moves accordingly.
func TestPruneTask_RetentionChangeTakesEffectNextTick(t *testing.T) {
	repo := &fakeRepo{}
	loc := time.Local
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, loc)

	SetRetentionDays(30)
	if err := pruneTaskAt(repo, now); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	first := repo.cutoff

	SetRetentionDays(7) // operator shortens retention via the UI between ticks
	if err := pruneTaskAt(repo, now); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	second := repo.cutoff

	if !first.Equal(cutoffForIn(now, 30, loc)) {
		t.Errorf("first cutoff should reflect 30 days, got %s", first)
	}
	if !second.Equal(cutoffForIn(now, 7, loc)) {
		t.Errorf("second cutoff should reflect the changed 7-day retention, got %s", second)
	}
	if !second.After(first) {
		t.Error("shortening retention from 30 to 7 days must move the cutoff forward")
	}
}

// Negative: a repo error propagates so the periodic loop can log it.
func TestPruneTask_PropagatesRepoError(t *testing.T) {
	SetRetentionDays(30)
	boom := errors.New("db down")
	repo := &fakeRepo{err: boom}
	if err := pruneTaskAt(repo, time.Now()); !errors.Is(err, boom) {
		t.Errorf("expected %v, got %v", boom, err)
	}
}

// End-to-end against the real repo: with a 30-day retention, a prune run at a
// fixed `now` deletes day-buckets older than the cutoff and keeps newer ones.
// Day buckets are local-midnight, matching how the write path stores them.
func TestPrune_EndToEnd_DeletesOldKeepsNew(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlConn, _ := conn.DB()
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(&traffic_db.DomainTraffic{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := traffic_db.NewRepo(conn)

	loc := time.Local
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, loc)
	midnightToday := time.Date(2026, 5, 26, 0, 0, 0, 0, loc)

	old := midnightToday.AddDate(0, 0, -40)  // 40 days ago — pruned (retention 30)
	edge := midnightToday.AddDate(0, 0, -30) // exactly 30 days ago — kept (strict <)
	fresh := midnightToday.AddDate(0, 0, -1) // yesterday — kept

	seed := []traffic_db.DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "old.example", Blocked: false, Day: old, Count: 1, LastSeen: now},
		{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "edge.example", Blocked: false, Day: edge, Count: 1, LastSeen: now},
		{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "fresh.example", Blocked: false, Day: fresh, Count: 1, LastSeen: now},
	}
	if err := repo.UpsertBatch(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	SetRetentionDays(30)
	if err := pruneTaskAt(repo, now); err != nil {
		t.Fatalf("prune: %v", err)
	}

	var remaining []string
	if err := conn.Model(&traffic_db.DomainTraffic{}).Order("domain").Pluck("domain", &remaining).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	// old.example pruned; edge.example (cutoff boundary) and fresh.example kept.
	want := []string{"edge.example", "fresh.example"}
	if len(remaining) != len(want) {
		t.Fatalf("after prune got %v, want %v", remaining, want)
	}
	for i := range want {
		if remaining[i] != want[i] {
			t.Fatalf("after prune got %v, want %v", remaining, want)
		}
	}
}

// The atomic setter/getter round-trip is what the settings Apply hook writes to.
func TestSetGetRetentionDays_RoundTrip(t *testing.T) {
	SetRetentionDays(45)
	if got := GetRetentionDays(); got != 45 {
		t.Errorf("GetRetentionDays = %d, want 45", got)
	}
	SetRetentionDays(1)
	if got := GetRetentionDays(); got != 1 {
		t.Errorf("GetRetentionDays = %d, want 1", got)
	}
}
