// Package db is the storage layer for the reputation-enrichment worker
// (milestone 1). It owns two tables, both internal to the suggest-to-block
// pipeline and never exposed over the HTTP API:
//
//   - inspect_candidate — the work queue + result cache for the worker. Rows
//     are seeded by the lexical pass (Collect) for domains scoring 10..29 and
//     drained by the worker, which records the reputation verdict back onto the
//     same row so the next run skips it until the TTL expires.
//   - rdap_cache — a registrable-keyed (eTLD+1) cache so sibling FQDNs under one
//     domain do not each re-query RDAP. VirusTotal/Safe Browsing are keyed by
//     FQDN and cached on the candidate row instead; RDAP is the only check that
//     answers per registrable, hence the split.
//
// The cache lives on the suggest side, NOT inside domain-inspect, which is
// deliberately storage-less.
package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InspectCandidate is one domain awaiting (or holding the result of) a
// reputation inspection. Domain is the full FQDN as observed in traffic.
//
// Verdict is "" until the worker inspects it, then one of
// clean|suspicious|malicious|unknown. CheckedAt stamps the last inspection;
// together with the TTL it gates re-inspection. NextRetryAt/ErrorCount drive
// bounded backoff for transient failures (timeout, rate-limit, unknown).
type InspectCandidate struct {
	Domain       string     `gorm:"primaryKey"`
	LexicalScore int        `gorm:"index"`
	ReasonsJSON  string     // snapshot of the lexical reasons that flagged it
	Verdict      string     // ""|clean|suspicious|malicious|unknown
	CheckedAt    *time.Time `gorm:"index"`
	NextRetryAt  *time.Time
	ErrorCount   int
}

// RDAPCache stores the registration age per registrable domain (eTLD+1), keyed
// separately from the FQDN-keyed candidate rows. See package doc.
type RDAPCache struct {
	Registrable string `gorm:"primaryKey"`
	AgeDays     int
	// CheckedAt is a value, not a pointer: a row exists only because PutRDAP
	// wrote it with a real timestamp, so "checked" is never unknown here —
	// unlike InspectCandidate.CheckedAt, where NULL means "never inspected".
	CheckedAt time.Time
}

// upsertCandidateOn inserts a fresh candidate or, if Domain already exists,
// refreshes ONLY the lexical fields. It must never touch the inspection state
// (Verdict/CheckedAt/NextRetryAt/ErrorCount): a re-run of the lexical pass
// should not re-queue an already-inspected domain and burn the inspection
// budget on it again.
func upsertCandidateOn(conn *gorm.DB, domain string, lexicalScore int, reasonsJSON string) error {
	return conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain"}},
		DoUpdates: clause.AssignmentColumns([]string{"lexical_score", "reasons_json"}),
	}).Create(&InspectCandidate{
		Domain:       domain,
		LexicalScore: lexicalScore,
		ReasonsJSON:  reasonsJSON,
	}).Error
}

// pickForInspectionOn returns the highest-scoring eligible candidates, capped
// at budget. Eligible = never inspected OR inspected before now-ttl, AND not
// currently in retry-backoff (NextRetryAt in the future). Highest lexical score
// first so the most suspicious domains spend the scarce VirusTotal budget.
func pickForInspectionOn(conn *gorm.DB, now time.Time, ttl time.Duration, budget int) ([]InspectCandidate, error) {
	// Guard the budget: 0 means "no budget left this run" (return nothing), and
	// a negative value must never silently become an unbounded query — GORM
	// drops the LIMIT clause for Limit(<0), which would drain the whole queue
	// and blow the VirusTotal quota in a single tick.
	if budget <= 0 {
		return nil, nil
	}
	var out []InspectCandidate
	err := conn.
		Where("(checked_at IS NULL OR checked_at < ?)", now.Add(-ttl)).
		Where("(next_retry_at IS NULL OR next_retry_at <= ?)", now).
		Order("lexical_score DESC").
		Limit(budget).
		Find(&out).Error
	return out, err
}

// saveResultOn records a terminal verdict: stamps CheckedAt and clears any
// pending retry/backoff so the row leaves the eligible set until the TTL
// expires.
func saveResultOn(conn *gorm.DB, domain, verdict string, now time.Time) error {
	return conn.Model(&InspectCandidate{}).
		Where("domain = ?", domain).
		Updates(map[string]any{
			"verdict":       verdict,
			"checked_at":    now,
			"next_retry_at": nil,
			"error_count":   0,
		}).Error
}

// scheduleRetryOn records a transient failure: bumps the error counter and
// pushes the next attempt to nextRetryAt. The caller decides when to stop
// (MaxErrorCount) — this layer just persists the backoff.
func scheduleRetryOn(conn *gorm.DB, domain string, nextRetryAt time.Time) error {
	return conn.Model(&InspectCandidate{}).
		Where("domain = ?", domain).
		Updates(map[string]any{
			"error_count":   gorm.Expr("error_count + 1"),
			"next_retry_at": nextRetryAt,
		}).Error
}

// dropOn removes a candidate entirely. Used when reputation says "clean" for a
// low-lexical domain that never reached the suggest list — there is nothing to
// surface, so we forget it. Deleting a missing row is a no-op.
func dropOn(conn *gorm.DB, domain string) error {
	return conn.Where("domain = ?", domain).Delete(&InspectCandidate{}).Error
}

// deleteCandidatesOlderThanOn prunes candidates inspected before cutoff.
// Never-inspected rows (CheckedAt NULL) are pending work and are kept.
func deleteCandidatesOlderThanOn(conn *gorm.DB, cutoff time.Time) error {
	return conn.Where("checked_at IS NOT NULL AND checked_at < ?", cutoff).
		Delete(&InspectCandidate{}).Error
}

// deleteRDAPOlderThanOn prunes RDAP cache entries last refreshed before cutoff.
func deleteRDAPOlderThanOn(conn *gorm.DB, cutoff time.Time) error {
	return conn.Where("checked_at < ?", cutoff).Delete(&RDAPCache{}).Error
}

// getRDAPOn returns the cached registration age for a registrable domain if it
// exists and is still within ttl. A stale or absent entry misses (ok=false),
// forcing the adapter to re-query RDAP.
func getRDAPOn(conn *gorm.DB, registrable string, now time.Time, ttl time.Duration) (*RDAPCache, bool, error) {
	var c RDAPCache
	err := conn.First(&c, "registrable = ?", registrable).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if c.CheckedAt.Before(now.Add(-ttl)) {
		return nil, false, nil
	}
	return &c, true, nil
}

// putRDAPOn upserts the registration age for a registrable domain.
func putRDAPOn(conn *gorm.DB, registrable string, ageDays int, now time.Time) error {
	return conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "registrable"}},
		DoUpdates: clause.AssignmentColumns([]string{"age_days", "checked_at"}),
	}).Create(&RDAPCache{
		Registrable: registrable,
		AgeDays:     ageDays,
		CheckedAt:   now,
	}).Error
}
