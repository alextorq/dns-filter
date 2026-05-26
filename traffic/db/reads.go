package db

import (
	"time"

	"gorm.io/gorm"
)

// DomainTotal is a (domain, summed-count) pair returned by the aggregation
// reads. Its JSON shape mirrors blocked-domain/db.DomainCount byte-for-byte so
// the repointed legacy block-stats endpoint stays wire-compatible with the
// existing frontend. It is named DomainTotal (not DomainCount) so the two
// package `db` types do not collide in the generated OpenAPI schema, which
// would otherwise force swag to fully-qualify blocked-domain's DomainCount.
type DomainTotal struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

// DeviceSummary is one row of the per-device dashboard rollup: a single device
// (identified by ClientKind/ClientValue), its allowed/blocked query totals, the
// most recent IP it was seen using, and the last time it queried anything.
type DeviceSummary struct {
	ClientKind   string    `json:"client_kind"`
	ClientValue  string    `json:"client_value"`
	CurrentIP    string    `json:"current_ip"`
	AllowedCount int64     `json:"allowed_count"`
	BlockedCount int64     `json:"blocked_count"`
	LastSeen     time.Time `json:"last_seen"`
}

// DeviceDomainsParams selects the per-device domain breakdown. Kind+Value
// identify the device (the composite key); Blocked optionally scopes the
// verdict; From/To optionally bound Day (inclusive); Limit/Offset paginate the
// count-desc ordering. Limit 0 yields an empty page (GORM emits LIMIT 0); the
// web layer always passes a positive limit, so callers must do the same.
type DeviceDomainsParams struct {
	Kind    string
	Value   string
	Blocked *bool
	From    *time.Time
	To      *time.Time
	Limit   int
	Offset  int
}

// DomainsResult is a page of per-device domains plus the total number of
// distinct domains matching the filter (before limit/offset) for pagination.
type DomainsResult struct {
	Total int64         `json:"total"`
	List  []DomainTotal `json:"list"`
}

// CountByDomain returns SUM(count) per domain scoped to the given verdict,
// across all devices and days. It replaces blocked-domain's GetEventsByDomain
// for the legacy block-stats endpoint (called with blocked=true). Uses the
// (blocked, day) index for the verdict scan. Empty table → empty (non-nil)
// slice.
func (r *Repo) CountByDomain(blocked bool) ([]DomainTotal, error) {
	results := []DomainTotal{}
	err := r.db.Model(&DomainTraffic{}).
		Select("domain, SUM(count) as count").
		Where("blocked = ?", blocked).
		Group("domain").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// TotalCount returns the grand SUM(count) over the verdict scope. It replaces
// blocked-domain's GetEventsAmount (called with blocked=true). COALESCE keeps an
// empty table at 0 rather than a NULL scan target.
func (r *Repo) TotalCount(blocked bool) (int64, error) {
	var total int64
	err := r.db.Model(&DomainTraffic{}).
		Select("COALESCE(SUM(count), 0)").
		Where("blocked = ?", blocked).
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return total, nil
}

// applyDayRange adds inclusive Day >= from / Day <= to predicates when set.
func applyDayRange(q *gorm.DB, from, to *time.Time) *gorm.DB {
	if from != nil {
		q = q.Where("day >= ?", *from)
	}
	if to != nil {
		q = q.Where("day <= ?", *to)
	}
	return q
}

// DeviceSummary returns one row per (client_kind, client_value) device with its
// allowed/blocked totals, the IP of its most-recent row (by last_seen), and the
// device's max last_seen. An optional [from, to] Day range scopes the rollup.
//
// CurrentIP is computed via a correlated subselect picking the client_ip of the
// row with the greatest last_seen for that device (ties broken by id) — this is
// the "latest IP" the UI shows to tell two same-vendor devices apart. The outer
// aggregation uses the (client_kind, client_value, day) index. Empty result →
// empty (non-nil) slice.
func (r *Repo) DeviceSummary(from, to *time.Time) ([]DeviceSummary, error) {
	results := []DeviceSummary{}
	// Resolve the actual (pluralized) table name from the model so the
	// correlated subselect below references the real table rather than a
	// hardcoded name that would drift if the model's TableName ever changed.
	tbl := r.db.NamingStrategy.TableName("DomainTraffic")
	// current_ip and last_seen both come from the device's most-recent row via a
	// correlated subselect rather than MAX() aggregates: the modernc/glebarez
	// SQLite driver stores time as TEXT and hands MAX(last_seen) back as a bare
	// string that database/sql cannot Scan into time.Time, whereas a plain column
	// select preserves the field's declared type. The latest row (greatest
	// last_seen, id tie-break) is exactly the device's current IP and its last
	// activity, so one subselect serves both.
	q := r.db.Model(&DomainTraffic{}).
		Select(
			"client_kind, client_value, " +
				"SUM(CASE WHEN blocked = 0 THEN count ELSE 0 END) as allowed_count, " +
				"SUM(CASE WHEN blocked = 1 THEN count ELSE 0 END) as blocked_count, " +
				"(SELECT t2.last_seen FROM " + tbl + " t2 " +
				"  WHERE t2.client_kind = " + tbl + ".client_kind " +
				"    AND t2.client_value = " + tbl + ".client_value " +
				"  ORDER BY t2.last_seen DESC, t2.id DESC LIMIT 1) as last_seen, " +
				"(SELECT t3.client_ip FROM " + tbl + " t3 " +
				"  WHERE t3.client_kind = " + tbl + ".client_kind " +
				"    AND t3.client_value = " + tbl + ".client_value " +
				"  ORDER BY t3.last_seen DESC, t3.id DESC LIMIT 1) as current_ip")
	q = applyDayRange(q, from, to)
	err := q.Group("client_kind, client_value").
		Order("blocked_count + allowed_count DESC").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// DomainsForDevice returns the domains a single device queried, grouped by
// domain with summed counts, ordered by count desc. Verdict, Day range and
// pagination are optional. Total is the number of distinct domains matching the
// filter (before limit/offset) so the UI can render pagination. An unknown
// device yields Total 0 and an empty list.
func (r *Repo) DomainsForDevice(p DeviceDomainsParams) (DomainsResult, error) {
	out := DomainsResult{List: []DomainTotal{}}

	base := r.db.Model(&DomainTraffic{}).
		Where("client_kind = ? AND client_value = ?", p.Kind, p.Value)
	if p.Blocked != nil {
		base = base.Where("blocked = ?", *p.Blocked)
	}
	base = applyDayRange(base, p.From, p.To)

	// Total = number of distinct domains for this device under the filter.
	var distinct []string
	if err := base.Session(&gorm.Session{}).
		Distinct().
		Pluck("domain", &distinct).Error; err != nil {
		return out, err
	}
	out.Total = int64(len(distinct))

	err := base.Session(&gorm.Session{}).
		Select("domain, SUM(count) as count").
		Group("domain").
		Order("count DESC, domain ASC").
		Limit(p.Limit).
		Offset(p.Offset).
		Scan(&out.List).Error
	if err != nil {
		return out, err
	}
	return out, nil
}

// TopDomains returns the highest-traffic domains across all devices, optionally
// scoped to a verdict, ordered by summed count desc, capped at limit. A nil
// blocked counts both verdicts. Empty table → empty (non-nil) slice.
func (r *Repo) TopDomains(blocked *bool, limit int) ([]DomainTotal, error) {
	results := []DomainTotal{}
	q := r.db.Model(&DomainTraffic{}).
		Select("domain, SUM(count) as count")
	if blocked != nil {
		q = q.Where("blocked = ?", *blocked)
	}
	err := q.Group("domain").
		Order("count DESC, domain ASC").
		Limit(limit).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}
