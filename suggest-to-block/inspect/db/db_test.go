package db

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newConn(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := conn.AutoMigrate(&InspectCandidate{}, &RDAPCache{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

// TestUpsertCandidate_InsertThenPreserve pins the central invariant: a repeated
// upsert from the lexical pass refreshes the score/reasons of an existing row
// but MUST NOT reset the inspection state (Verdict/CheckedAt/NextRetryAt/
// ErrorCount) — otherwise a domain already inspected this TTL window would be
// re-queued forever and we'd burn the VirusTotal budget on it.
func TestUpsertCandidate_InsertThenPreserve(t *testing.T) {
	conn := newConn(t)

	// 1. First upsert inserts a fresh candidate with empty inspection state.
	if err := upsertCandidateOn(conn, "evil.example.com", 12, `[{"code":"risky_tld"}]`); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	var got InspectCandidate
	if err := conn.First(&got, "domain = ?", "evil.example.com").Error; err != nil {
		t.Fatalf("load after insert: %v", err)
	}
	if got.LexicalScore != 12 || got.Verdict != "" || got.CheckedAt != nil {
		t.Fatalf("unexpected fresh row: %+v", got)
	}

	// 2. Simulate the worker having inspected it: mark a verdict.
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	if err := saveResultOn(conn, "evil.example.com", "suspicious", now); err != nil {
		t.Fatalf("save result: %v", err)
	}

	// 3. Next lexical pass upserts again with a different score/reasons.
	if err := upsertCandidateOn(conn, "evil.example.com", 18, `[{"code":"risky_tld"},{"code":"bad_keywords"}]`); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if err := conn.First(&got, "domain = ?", "evil.example.com").Error; err != nil {
		t.Fatalf("load after re-upsert: %v", err)
	}
	if got.LexicalScore != 18 {
		t.Errorf("expected score refreshed to 18, got %d", got.LexicalScore)
	}
	if got.ReasonsJSON != `[{"code":"risky_tld"},{"code":"bad_keywords"}]` {
		t.Errorf("expected reasons refreshed, got %q", got.ReasonsJSON)
	}
	// The crucial part: inspection state survives the re-upsert.
	if got.Verdict != "suspicious" {
		t.Errorf("verdict must survive re-upsert, got %q", got.Verdict)
	}
	if got.CheckedAt == nil {
		t.Error("CheckedAt must survive re-upsert, got nil")
	}

	// Exactly one row — upsert, not a second insert.
	var count int64
	conn.Model(&InspectCandidate{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 row after re-upsert, got %d", count)
	}
}

// TestPickForInspection covers the selection contract: never-inspected and
// stale rows are eligible, fresh rows and rows in retry-backoff are not, the
// order is by lexical score desc, and the budget caps the result.
func TestPickForInspection(t *testing.T) {
	conn := newConn(t)
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	ttl := 7 * 24 * time.Hour

	seed := func(domain string, score int, mutate func(*InspectCandidate)) {
		c := InspectCandidate{Domain: domain, LexicalScore: score}
		if mutate != nil {
			mutate(&c)
		}
		if err := conn.Create(&c).Error; err != nil {
			t.Fatalf("seed %s: %v", domain, err)
		}
	}

	fresh := now.Add(-24 * time.Hour)      // inspected 1 day ago → within TTL
	stale := now.Add(-10 * 24 * time.Hour) // inspected 10 days ago → past TTL
	future := now.Add(time.Hour)           // retry not due yet

	seed("never.com", 25, nil)                                                   // eligible (never inspected)
	seed("stale.com", 30, func(c *InspectCandidate) { c.CheckedAt = &stale })    // eligible (TTL expired)
	seed("fresh.com", 99, func(c *InspectCandidate) { c.CheckedAt = &fresh })    // NOT eligible (fresh)
	seed("retry.com", 50, func(c *InspectCandidate) { c.NextRetryAt = &future }) // NOT eligible (backoff)

	got, err := pickForInspectionOn(conn, now, ttl, 10)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}

	gotDomains := make([]string, len(got))
	for i, c := range got {
		gotDomains[i] = c.Domain
	}
	// stale (30) before never (25); fresh and retry excluded.
	want := []string{"stale.com", "never.com"}
	if len(gotDomains) != len(want) {
		t.Fatalf("eligible set wrong: got %v, want %v", gotDomains, want)
	}
	for i, w := range want {
		if gotDomains[i] != w {
			t.Fatalf("order/selection mismatch: got %v, want %v", gotDomains, want)
		}
	}

	// Budget caps the result size; highest score wins the single slot.
	capped, err := pickForInspectionOn(conn, now, ttl, 1)
	if err != nil {
		t.Fatalf("pick budget=1: %v", err)
	}
	if len(capped) != 1 || capped[0].Domain != "stale.com" {
		t.Fatalf("budget=1 should yield only stale.com, got %+v", capped)
	}
}

// TestPickForInspection_Eligibility pins the boundary cases that the AND of
// the two predicates must satisfy, so a future refactor (e.g. AND→OR) is
// caught: empty table, exhausted budget, the exact TTL boundary, and the
// fresh-but-retry-overdue row (must stay excluded because freshness wins).
func TestPickForInspection_Eligibility(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	ttl := 7 * 24 * time.Hour

	t.Run("empty table returns nothing", func(t *testing.T) {
		conn := newConn(t)
		got, err := pickForInspectionOn(conn, now, ttl, 10)
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("empty table should yield 0, got %d", len(got))
		}
	})

	t.Run("budget zero yields nothing", func(t *testing.T) {
		conn := newConn(t)
		if err := conn.Create(&InspectCandidate{Domain: "a.com", LexicalScore: 25}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		got, err := pickForInspectionOn(conn, now, ttl, 0)
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("budget=0 must yield nothing (not unbounded), got %d", len(got))
		}
	})

	t.Run("row exactly at TTL boundary is still fresh", func(t *testing.T) {
		conn := newConn(t)
		boundary := now.Add(-ttl) // checked_at == now-ttl → strict < excludes it
		if err := conn.Create(&InspectCandidate{Domain: "b.com", LexicalScore: 25, CheckedAt: &boundary}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		got, err := pickForInspectionOn(conn, now, ttl, 10)
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("row at exact TTL boundary must count as fresh, got %d", len(got))
		}
	})

	t.Run("fresh row with overdue retry stays excluded", func(t *testing.T) {
		conn := newConn(t)
		fresh := now.Add(-24 * time.Hour) // within TTL
		past := now.Add(-time.Hour)       // retry already due
		if err := conn.Create(&InspectCandidate{
			Domain: "c.com", LexicalScore: 25, CheckedAt: &fresh, NextRetryAt: &past,
		}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		got, err := pickForInspectionOn(conn, now, ttl, 10)
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		// freshness (AND) must keep it out even though the retry is overdue.
		if len(got) != 0 {
			t.Errorf("fresh row must stay excluded regardless of overdue retry, got %d", len(got))
		}
	})
}

// TestUpdateMissingDomain_NoOp pins that mutating a non-existent candidate is a
// silent no-op (no error, no phantom row) — the worker may race a prune.
func TestUpdateMissingDomain_NoOp(t *testing.T) {
	conn := newConn(t)
	now := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)

	if err := saveResultOn(conn, "ghost.com", "clean", now); err != nil {
		t.Errorf("saveResult on missing row should be no-op, got %v", err)
	}
	if err := scheduleRetryOn(conn, "ghost.com", now.Add(time.Hour)); err != nil {
		t.Errorf("scheduleRetry on missing row should be no-op, got %v", err)
	}

	var count int64
	conn.Model(&InspectCandidate{}).Count(&count)
	if count != 0 {
		t.Errorf("updates on missing rows must not create phantom rows, got %d", count)
	}
}

// TestRepoDeleteOlderThan_PrunesBothTables verifies the Repo-level glue that is
// the ONLY place candidate + RDAP pruning are joined: one DeleteOlderThan call
// must prune stale rows from BOTH tables while keeping recent ones.
func TestRepoDeleteOlderThan_PrunesBothTables(t *testing.T) {
	conn := newConn(t)
	r := NewRepo(conn)

	now := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour)
	recent := now.Add(-1 * 24 * time.Hour)
	cutoff := now.Add(-14 * 24 * time.Hour)

	if err := conn.Create(&InspectCandidate{Domain: "old.com", CheckedAt: &old}).Error; err != nil {
		t.Fatalf("seed candidate old: %v", err)
	}
	if err := conn.Create(&InspectCandidate{Domain: "recent.com", CheckedAt: &recent}).Error; err != nil {
		t.Fatalf("seed candidate recent: %v", err)
	}
	if err := conn.Create(&RDAPCache{Registrable: "old.net", AgeDays: 10, CheckedAt: old}).Error; err != nil {
		t.Fatalf("seed rdap old: %v", err)
	}
	if err := conn.Create(&RDAPCache{Registrable: "recent.net", AgeDays: 10, CheckedAt: recent}).Error; err != nil {
		t.Fatalf("seed rdap recent: %v", err)
	}

	if err := r.DeleteOlderThan(cutoff); err != nil {
		t.Fatalf("prune: %v", err)
	}

	var candidates, rdaps int64
	conn.Model(&InspectCandidate{}).Count(&candidates)
	conn.Model(&RDAPCache{}).Count(&rdaps)
	if candidates != 1 {
		t.Errorf("expected 1 candidate left (recent), got %d", candidates)
	}
	if rdaps != 1 {
		t.Errorf("expected 1 rdap row left (recent) — both tables must prune, got %d", rdaps)
	}

	var rc RDAPCache
	if err := conn.First(&rc, "registrable = ?", "recent.net").Error; err != nil {
		t.Errorf("recent rdap row must survive, got %v", err)
	}
}

// TestSaveResult confirms a verdict write stamps CheckedAt and clears any
// pending retry/backoff so the row drops out of the eligible set.
func TestSaveResult(t *testing.T) {
	conn := newConn(t)
	now := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)

	// Row with an in-flight retry that we now resolve.
	if err := conn.Create(&InspectCandidate{
		Domain: "x.com", LexicalScore: 15, NextRetryAt: &past, ErrorCount: 2,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := saveResultOn(conn, "x.com", "malicious", now); err != nil {
		t.Fatalf("save: %v", err)
	}

	var got InspectCandidate
	if err := conn.First(&got, "domain = ?", "x.com").Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Verdict != "malicious" {
		t.Errorf("verdict = %q, want malicious", got.Verdict)
	}
	if got.CheckedAt == nil || !got.CheckedAt.Equal(now) {
		t.Errorf("CheckedAt = %v, want %v", got.CheckedAt, now)
	}
	if got.NextRetryAt != nil {
		t.Errorf("NextRetryAt must be cleared, got %v", got.NextRetryAt)
	}
	if got.ErrorCount != 0 {
		t.Errorf("ErrorCount must reset to 0, got %d", got.ErrorCount)
	}
}

// TestScheduleRetry confirms a transient failure increments the error counter
// and pushes the next attempt into the future (so it is skipped until then).
func TestScheduleRetry(t *testing.T) {
	conn := newConn(t)
	now := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	next := now.Add(30 * time.Minute)

	if err := conn.Create(&InspectCandidate{Domain: "y.com", LexicalScore: 15}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := scheduleRetryOn(conn, "y.com", next); err != nil {
		t.Fatalf("retry 1: %v", err)
	}
	if err := scheduleRetryOn(conn, "y.com", next); err != nil {
		t.Fatalf("retry 2: %v", err)
	}

	var got InspectCandidate
	if err := conn.First(&got, "domain = ?", "y.com").Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ErrorCount != 2 {
		t.Errorf("ErrorCount = %d, want 2", got.ErrorCount)
	}
	if got.NextRetryAt == nil || !got.NextRetryAt.Equal(next) {
		t.Errorf("NextRetryAt = %v, want %v", got.NextRetryAt, next)
	}
}

// TestDrop removes a candidate entirely (used when reputation says "clean" for
// a low-lexical domain that never reached the suggest list).
func TestDrop(t *testing.T) {
	conn := newConn(t)
	if err := conn.Create(&InspectCandidate{Domain: "clean.com", LexicalScore: 12}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := dropOn(conn, "clean.com"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	var count int64
	conn.Model(&InspectCandidate{}).Where("domain = ?", "clean.com").Count(&count)
	if count != 0 {
		t.Errorf("expected row deleted, still %d", count)
	}
	// Dropping a missing row is a no-op, not an error (idempotent cleanup).
	if err := dropOn(conn, "clean.com"); err != nil {
		t.Errorf("drop of missing row should be no-op, got %v", err)
	}
}

// TestDeleteOlderThan prunes candidates inspected before the cutoff while
// keeping recent and never-inspected rows. Never-inspected rows (CheckedAt
// nil) are kept — they are still pending work, not stale results.
func TestDeleteOlderThan(t *testing.T) {
	conn := newConn(t)
	now := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour)
	recent := now.Add(-1 * 24 * time.Hour)
	cutoff := now.Add(-14 * 24 * time.Hour)

	mustCreate := func(c InspectCandidate) {
		if err := conn.Create(&c).Error; err != nil {
			t.Fatalf("seed %s: %v", c.Domain, err)
		}
	}
	mustCreate(InspectCandidate{Domain: "old.com", CheckedAt: &old})
	mustCreate(InspectCandidate{Domain: "recent.com", CheckedAt: &recent})
	mustCreate(InspectCandidate{Domain: "pending.com"}) // CheckedAt nil

	if err := deleteCandidatesOlderThanOn(conn, cutoff); err != nil {
		t.Fatalf("prune: %v", err)
	}

	var domains []string
	conn.Model(&InspectCandidate{}).Order("domain").Pluck("domain", &domains)
	want := []string{"pending.com", "recent.com"}
	if len(domains) != len(want) {
		t.Fatalf("after prune got %v, want %v", domains, want)
	}
	for i, w := range want {
		if domains[i] != w {
			t.Fatalf("after prune got %v, want %v", domains, want)
		}
	}
}

// TestRDAPCache covers the separate registrable-keyed cache used so sibling
// FQDNs under one eTLD+1 do not re-query RDAP: fresh hit returns the value,
// stale and absent both miss.
func TestRDAPCache(t *testing.T) {
	conn := newConn(t)
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	ttl := 7 * 24 * time.Hour

	// Absent → miss.
	if _, ok, err := getRDAPOn(conn, "example.net", now, ttl); err != nil || ok {
		t.Fatalf("absent should miss: ok=%v err=%v", ok, err)
	}

	if err := putRDAPOn(conn, "example.net", 4, now); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Fresh → hit with the stored age.
	got, ok, err := getRDAPOn(conn, "example.net", now.Add(24*time.Hour), ttl)
	if err != nil || !ok {
		t.Fatalf("fresh should hit: ok=%v err=%v", ok, err)
	}
	if got.AgeDays != 4 {
		t.Errorf("AgeDays = %d, want 4", got.AgeDays)
	}

	// Past TTL → miss (forces a re-query).
	if _, ok, err := getRDAPOn(conn, "example.net", now.Add(8*24*time.Hour), ttl); err != nil || ok {
		t.Fatalf("stale should miss: ok=%v err=%v", ok, err)
	}

	// Re-put updates age + timestamp (upsert, not duplicate).
	if err := putRDAPOn(conn, "example.net", 400, now.Add(8*24*time.Hour)); err != nil {
		t.Fatalf("re-put: %v", err)
	}
	var count int64
	conn.Model(&RDAPCache{}).Where("registrable = ?", "example.net").Count(&count)
	if count != 1 {
		t.Errorf("expected single rdap row after re-put, got %d", count)
	}
}
