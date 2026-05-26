package db

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newTestRepo mirrors blocked-domain/db/repo_test.go: an in-memory SQLite pinned
// to a single connection (so :memory: per-connection state is shared) with the
// DomainTraffic schema migrated. The composite unique index that the additive
// upsert targets is created by AutoMigrate from the model's GORM tags.
func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlConn, err := conn.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(&DomainTraffic{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepo(conn)
}

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse day %q: %v", s, err)
	}
	return d
}

// countRows is a small helper for the assertions below.
func countRows(t *testing.T, r *Repo) int64 {
	t.Helper()
	var n int64
	r.db.Model(&DomainTraffic{}).Count(&n)
	return n
}

// fetch returns the single row matching the unique key, failing if absent.
func fetch(t *testing.T, r *Repo, kind, value, domain string, blocked bool, d time.Time) DomainTraffic {
	t.Helper()
	var row DomainTraffic
	err := r.db.Where(
		"client_kind = ? AND client_value = ? AND blocked = ? AND domain = ? AND day = ?",
		kind, value, blocked, domain, d,
	).First(&row).Error
	if err != nil {
		t.Fatalf("fetch (%s/%s/%s/%v/%v): %v", kind, value, domain, blocked, d, err)
	}
	return row
}

// ----- UpsertBatch: insert new rows -----

func TestRepo_UpsertBatch_InsertsNewRows(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	rows := []DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "192.168.1.10", Domain: "ads.example", Blocked: true, Day: d, Count: 1, LastSeen: now},
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "192.168.1.10", Domain: "good.example", Blocked: false, Day: d, Count: 5, LastSeen: now},
	}
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}
	if n := countRows(t, r); n != 2 {
		t.Fatalf("expected 2 rows, got %d", n)
	}
	got := fetch(t, r, "mac", "aa:bb", "good.example", false, d)
	if got.Count != 5 {
		t.Errorf("expected count=5, got %d", got.Count)
	}
}

// ----- UpsertBatch: conflict ADDS count and bumps last_seen/client_ip -----

func TestRepo_UpsertBatch_ConflictAddsCount(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	t0 := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

	first := []DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "192.168.1.10", Domain: "ads.example", Blocked: true, Day: d, Count: 1, LastSeen: t0},
	}
	if err := r.UpsertBatch(first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second := []DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "192.168.1.20", Domain: "ads.example", Blocked: true, Day: d, Count: 1, LastSeen: t1},
	}
	if err := r.UpsertBatch(second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if n := countRows(t, r); n != 1 {
		t.Fatalf("expected 1 row after conflict, got %d", n)
	}
	got := fetch(t, r, "mac", "aa:bb", "ads.example", true, d)
	if got.Count != 2 {
		t.Errorf("expected count=2 (1+1), got %d", got.Count)
	}
	if !got.LastSeen.Equal(t1) {
		t.Errorf("expected last_seen bumped to %v, got %v", t1, got.LastSeen)
	}
	if got.ClientIP != "192.168.1.20" {
		t.Errorf("expected client_ip updated to latest, got %q", got.ClientIP)
	}
}

// last_seen must be max(existing, incoming): an out-of-order (older) LastSeen
// must NOT roll the timestamp backwards, but count still accumulates.
func TestRepo_UpsertBatch_LastSeenIsMax(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	newer := time.Date(2026, 5, 25, 18, 0, 0, 0, time.UTC)
	older := time.Date(2026, 5, 25, 6, 0, 0, 0, time.UTC)

	if err := r.UpsertBatch([]DomainTraffic{
		{ClientKind: "ip", ClientValue: "10.0.0.5", ClientIP: "10.0.0.5", Domain: "x.example", Blocked: false, Day: d, Count: 3, LastSeen: newer},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// An older flush arrives late.
	if err := r.UpsertBatch([]DomainTraffic{
		{ClientKind: "ip", ClientValue: "10.0.0.5", ClientIP: "10.0.0.5", Domain: "x.example", Blocked: false, Day: d, Count: 2, LastSeen: older},
	}); err != nil {
		t.Fatalf("late upsert: %v", err)
	}
	got := fetch(t, r, "ip", "10.0.0.5", "x.example", false, d)
	if got.Count != 5 {
		t.Errorf("expected count=5, got %d", got.Count)
	}
	if !got.LastSeen.Equal(newer) {
		t.Errorf("expected last_seen to stay at %v (max), got %v", newer, got.LastSeen)
	}
}

// ----- distinct keys create separate rows -----

func TestRepo_UpsertBatch_DistinctKeysSeparateRows(t *testing.T) {
	r := newTestRepo(t)
	d1 := day(t, "2026-05-24")
	d2 := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)

	// Each row differs from the others by exactly one component of the unique key.
	base := DomainTraffic{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "d.example", Blocked: true, Day: d1, Count: 1, LastSeen: now}
	rows := []DomainTraffic{
		base,
		func() DomainTraffic { x := base; x.ClientKind = "ip"; return x }(),        // differ kind
		func() DomainTraffic { x := base; x.ClientValue = "cc:dd"; return x }(),    // differ value
		func() DomainTraffic { x := base; x.Blocked = false; return x }(),          // differ blocked
		func() DomainTraffic { x := base; x.Domain = "other.example"; return x }(), // differ domain
		func() DomainTraffic { x := base; x.Day = d2; return x }(),                 // differ day
	}
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}
	if n := countRows(t, r); n != 6 {
		t.Fatalf("expected 6 distinct rows, got %d", n)
	}
}

// rows with the same key but different ClientIP collapse to one row carrying the
// latest IP (the IP supplied by the row that wins last_seen / last write).
func TestRepo_UpsertBatch_SameKeyDifferentIPKeepsLatest(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	t0 := time.Date(2026, 5, 25, 8, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC)

	if err := r.UpsertBatch([]DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "192.168.1.10", Domain: "d.example", Blocked: false, Day: d, Count: 1, LastSeen: t0},
	}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := r.UpsertBatch([]DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "192.168.1.99", Domain: "d.example", Blocked: false, Day: d, Count: 1, LastSeen: t1},
	}); err != nil {
		t.Fatalf("second: %v", err)
	}
	if n := countRows(t, r); n != 1 {
		t.Fatalf("expected 1 row, got %d", n)
	}
	got := fetch(t, r, "mac", "aa:bb", "d.example", false, d)
	if got.ClientIP != "192.168.1.99" {
		t.Errorf("expected latest IP 192.168.1.99, got %q", got.ClientIP)
	}
}

// ----- batch larger than the SQLite param limit succeeds -----

func TestRepo_UpsertBatch_LargeBatchExceedsParamLimit(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	const n = 4096 // 8 cols × 4096 = 32768 > SQLite's 32766 limit, and > batchSize so it spans 2 batches
	rows := make([]DomainTraffic, n)
	for i := range rows {
		rows[i] = DomainTraffic{
			ClientKind:  "ip",
			ClientValue: "10.0.0.1",
			ClientIP:    "10.0.0.1",
			Domain:      "d" + itoa(i) + ".example",
			Blocked:     false,
			Day:         d,
			Count:       1,
			LastSeen:    now,
		}
	}
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("large UpsertBatch: %v", err)
	}
	if got := countRows(t, r); got != n {
		t.Errorf("expected %d rows, got %d", n, got)
	}
}

// ----- UpsertBatch empty input is a no-op -----

func TestRepo_UpsertBatch_EmptyIsNoOp(t *testing.T) {
	r := newTestRepo(t)
	if err := r.UpsertBatch(nil); err != nil {
		t.Fatalf("nil: %v", err)
	}
	if err := r.UpsertBatch([]DomainTraffic{}); err != nil {
		t.Fatalf("empty slice: %v", err)
	}
	if n := countRows(t, r); n != 0 {
		t.Errorf("expected 0 rows, got %d", n)
	}
}

// ----- DeleteOlderThan -----

func TestRepo_DeleteOlderThan_PrunesOldKeepsNew(t *testing.T) {
	r := newTestRepo(t)
	old := day(t, "2026-05-01")
	keep := day(t, "2026-05-20")
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)

	rows := []DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "old.example", Blocked: false, Day: old, Count: 1, LastSeen: now},
		{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "new.example", Blocked: false, Day: keep, Count: 1, LastSeen: now},
	}
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cutoff := day(t, "2026-05-10")
	if err := r.DeleteOlderThan(cutoff); err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if n := countRows(t, r); n != 1 {
		t.Fatalf("expected 1 row after prune, got %d", n)
	}
	// The kept row must be the newer one.
	fetch(t, r, "mac", "aa", "new.example", false, keep)
}

// boundary: a row whose Day equals the cutoff is NOT deleted (strict <).
func TestRepo_DeleteOlderThan_CutoffBoundaryIsKept(t *testing.T) {
	r := newTestRepo(t)
	cutoff := day(t, "2026-05-10")
	now := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	if err := r.UpsertBatch([]DomainTraffic{
		{ClientKind: "ip", ClientValue: "1.2.3.4", ClientIP: "1.2.3.4", Domain: "edge.example", Blocked: false, Day: cutoff, Count: 1, LastSeen: now},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.DeleteOlderThan(cutoff); err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if n := countRows(t, r); n != 1 {
		t.Errorf("row on the cutoff day must survive (strict <), got %d rows", n)
	}
}

func TestRepo_DeleteOlderThan_EmptyTableIsNoOp(t *testing.T) {
	r := newTestRepo(t)
	if err := r.DeleteOlderThan(day(t, "2026-05-10")); err != nil {
		t.Fatalf("DeleteOlderThan on empty table: %v", err)
	}
	if n := countRows(t, r); n != 0 {
		t.Errorf("expected 0 rows, got %d", n)
	}
}

// ----- GetAllowedDomains: DISTINCT allowed domains -----

// happy: returns the DISTINCT set of domains seen with blocked=false, deduped
// across days and devices.
func TestRepo_GetAllowedDomains_DistinctAcrossDaysAndDevices(t *testing.T) {
	r := newTestRepo(t)
	d1 := day(t, "2026-05-24")
	d2 := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)

	rows := []DomainTraffic{
		// same allowed domain across two devices and two days → still one entry
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "good.example", Blocked: false, Day: d1, Count: 1, LastSeen: now},
		{ClientKind: "mac", ClientValue: "cc:dd", ClientIP: "2.2.2.2", Domain: "good.example", Blocked: false, Day: d2, Count: 1, LastSeen: now},
		// a second distinct allowed domain
		{ClientKind: "ip", ClientValue: "10.0.0.5", ClientIP: "10.0.0.5", Domain: "other.example", Blocked: false, Day: d2, Count: 1, LastSeen: now},
	}
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := r.GetAllowedDomains()
	if err != nil {
		t.Fatalf("GetAllowedDomains: %v", err)
	}
	want := map[string]bool{"good.example": true, "other.example": true}
	if len(got) != len(want) {
		t.Fatalf("expected %d distinct domains, got %d (%v)", len(want), len(got), got)
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("unexpected domain %q in result", d)
		}
	}
}

// negative scope: a domain that was ONLY ever blocked must NOT appear in the
// allowed set. A domain seen both allowed and blocked appears once (allowed).
func TestRepo_GetAllowedDomains_ExcludesBlockedOnly(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)

	rows := []DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "ads.example", Blocked: true, Day: d, Count: 7, LastSeen: now},    // blocked only
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "mixed.example", Blocked: true, Day: d, Count: 1, LastSeen: now},  // mixed
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "mixed.example", Blocked: false, Day: d, Count: 3, LastSeen: now}, // mixed
	}
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := r.GetAllowedDomains()
	if err != nil {
		t.Fatalf("GetAllowedDomains: %v", err)
	}
	if len(got) != 1 || got[0] != "mixed.example" {
		t.Fatalf("expected only [mixed.example], got %v", got)
	}
}

// negative edge: empty table returns an empty (non-nil-panic) slice.
func TestRepo_GetAllowedDomains_EmptyTable(t *testing.T) {
	r := newTestRepo(t)
	got, err := r.GetAllowedDomains()
	if err != nil {
		t.Fatalf("GetAllowedDomains on empty table: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ----- IsAllowed: verdict-scoped membership -----

func TestRepo_IsAllowed(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	rows := []DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "allowed.example", Blocked: false, Day: d, Count: 1, LastSeen: now},
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "blockedonly.example", Blocked: true, Day: d, Count: 1, LastSeen: now},
	}
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// happy: present as allowed → true
	if ok, err := r.IsAllowed("allowed.example"); err != nil || !ok {
		t.Errorf("IsAllowed(allowed.example) = %v, %v; want true, nil", ok, err)
	}
	// negative scope: present only as blocked → not allowed
	if ok, err := r.IsAllowed("blockedonly.example"); err != nil || ok {
		t.Errorf("IsAllowed(blockedonly.example) = %v, %v; want false, nil", ok, err)
	}
	// negative: absent → false
	if ok, err := r.IsAllowed("never.example"); err != nil || ok {
		t.Errorf("IsAllowed(never.example) = %v, %v; want false, nil", ok, err)
	}
}

// ----- IsBlockedSeen: verdict-scoped membership over block events -----

func TestRepo_IsBlockedSeen(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	rows := []DomainTraffic{
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "blocked.example", Blocked: true, Day: d, Count: 1, LastSeen: now},
		{ClientKind: "mac", ClientValue: "aa:bb", ClientIP: "1.1.1.1", Domain: "allowedonly.example", Blocked: false, Day: d, Count: 1, LastSeen: now},
	}
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if ok, err := r.IsBlockedSeen("blocked.example"); err != nil || !ok {
		t.Errorf("IsBlockedSeen(blocked.example) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := r.IsBlockedSeen("allowedonly.example"); err != nil || ok {
		t.Errorf("IsBlockedSeen(allowedonly.example) = %v, %v; want false, nil", ok, err)
	}
	if ok, err := r.IsBlockedSeen("never.example"); err != nil || ok {
		t.Errorf("IsBlockedSeen(never.example) = %v, %v; want false, nil", ok, err)
	}
}

// itoa avoids pulling strconv into the hot test loop's import churn; keeps the
// large-batch test self-contained.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
