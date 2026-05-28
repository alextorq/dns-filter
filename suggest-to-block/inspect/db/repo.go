package db

import (
	"time"

	"gorm.io/gorm"
)

// Repo is the DI adapter over the inspect tables. Construct one at the
// composition root with NewRepo(conn) and pass it to the worker / prune loop.
// It supplies the wall clock to the package-level helpers, which take an
// explicit `now` so they stay deterministically testable.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo { return &Repo{db: conn} }

// UpsertCandidate seeds or refreshes a lexical candidate without disturbing its
// inspection state. Called from Collect for domains scoring 10..29.
func (r *Repo) UpsertCandidate(domain string, lexicalScore int, reasonsJSON string) error {
	return upsertCandidateOn(r.db, domain, lexicalScore, reasonsJSON)
}

// PickForInspection returns up to budget eligible candidates, freshest-TTL and
// retry-backoff rows excluded, highest lexical score first.
func (r *Repo) PickForInspection(ttl time.Duration, budget int) ([]InspectCandidate, error) {
	return pickForInspectionOn(r.db, time.Now(), ttl, budget)
}

// SaveResult records a terminal verdict and clears any backoff.
func (r *Repo) SaveResult(domain, verdict string) error {
	return saveResultOn(r.db, domain, verdict, time.Now())
}

// ScheduleRetry records a transient failure and pushes the next attempt out by
// backoff from now.
func (r *Repo) ScheduleRetry(domain string, backoff time.Duration) error {
	return scheduleRetryOn(r.db, domain, time.Now().Add(backoff))
}

// Drop forgets a candidate (clean verdict on a low-lexical domain).
func (r *Repo) Drop(domain string) error {
	return dropOn(r.db, domain)
}

// DeleteOlderThan prunes inspected candidates and RDAP cache entries last
// touched before cutoff. Never-inspected candidates are kept (pending work).
func (r *Repo) DeleteOlderThan(cutoff time.Time) error {
	if err := deleteCandidatesOlderThanOn(r.db, cutoff); err != nil {
		return err
	}
	return deleteRDAPOlderThanOn(r.db, cutoff)
}

// QueueDepth reports how many candidates currently exist — backs the
// suggest_inspect_queue_depth gauge.
func (r *Repo) QueueDepth() (int64, error) {
	var n int64
	err := r.db.Model(&InspectCandidate{}).Count(&n).Error
	return n, err
}

// GetRDAP returns the cached registration age for a registrable domain if fresh.
func (r *Repo) GetRDAP(registrable string, ttl time.Duration) (*RDAPCache, bool, error) {
	return getRDAPOn(r.db, registrable, time.Now(), ttl)
}

// PutRDAP upserts the registration age for a registrable domain.
func (r *Repo) PutRDAP(registrable string, ageDays int) error {
	return putRDAPOn(r.db, registrable, ageDays, time.Now())
}
