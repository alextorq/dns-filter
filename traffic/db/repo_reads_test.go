package db

import (
	"testing"
	"time"
)

// seed is a small helper to upsert a batch and fail the test on error.
func seed(t *testing.T, r *Repo, rows ...DomainTraffic) {
	t.Helper()
	if err := r.UpsertBatch(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func tp(v time.Time) *time.Time { return &v }
func bp(v bool) *bool           { return &v }

// ----- TotalCount -----

// happy: TotalCount sums Count over the verdict scope.
func TestRepo_TotalCount_VerdictScoped(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "a.example", Blocked: true, Day: d, Count: 3, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "b.example", Blocked: true, Day: d, Count: 4, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "c.example", Blocked: false, Day: d, Count: 99, LastSeen: now},
	)
	blocked, err := r.TotalCount(true)
	if err != nil {
		t.Fatalf("TotalCount(true): %v", err)
	}
	if blocked != 7 {
		t.Errorf("expected blocked total 7, got %d", blocked)
	}
	allowed, err := r.TotalCount(false)
	if err != nil {
		t.Fatalf("TotalCount(false): %v", err)
	}
	if allowed != 99 {
		t.Errorf("expected allowed total 99, got %d", allowed)
	}
}

// negative: empty table → 0, no error.
func TestRepo_TotalCount_EmptyTable(t *testing.T) {
	r := newTestRepo(t)
	n, err := r.TotalCount(true)
	if err != nil {
		t.Fatalf("TotalCount on empty table: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

// ----- DeviceSummary -----

// happy: per-device allowed/blocked totals; current IP = latest by last_seen;
// last_seen = max across the device's rows.
func TestRepo_DeviceSummary_TotalsAndLatestIP(t *testing.T) {
	r := newTestRepo(t)
	d1 := day(t, "2026-05-24")
	d2 := day(t, "2026-05-25")
	early := time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC)
	late := time.Date(2026, 5, 25, 20, 0, 0, 0, time.UTC)

	seed(t, r,
		// device aa: blocked 3 (early, ip .10) + blocked 2 (late, ip .20) + allowed 5
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "192.168.1.10", Domain: "ads.example", Blocked: true, Day: d1, Count: 3, LastSeen: early},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "192.168.1.20", Domain: "track.example", Blocked: true, Day: d2, Count: 2, LastSeen: late},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "192.168.1.20", Domain: "good.example", Blocked: false, Day: d2, Count: 5, LastSeen: late},
		// device bb: allowed only
		DomainTraffic{ClientKind: "ip", ClientValue: "10.0.0.5", ClientIP: "10.0.0.5", Domain: "good.example", Blocked: false, Day: d2, Count: 1, LastSeen: late},
	)

	got, err := r.DeviceSummary(nil, nil)
	if err != nil {
		t.Fatalf("DeviceSummary: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 devices, got %d (%v)", len(got), got)
	}

	byKey := map[string]DeviceSummary{}
	for _, s := range got {
		byKey[s.ClientKind+"/"+s.ClientValue] = s
	}
	aa := byKey["mac/aa"]
	if aa.AllowedCount != 5 {
		t.Errorf("aa allowed: expected 5, got %d", aa.AllowedCount)
	}
	if aa.BlockedCount != 5 {
		t.Errorf("aa blocked: expected 5 (3+2), got %d", aa.BlockedCount)
	}
	if aa.CurrentIP != "192.168.1.20" {
		t.Errorf("aa current IP: expected latest .20, got %q", aa.CurrentIP)
	}
	if !aa.LastSeen.Equal(late) {
		t.Errorf("aa last_seen: expected %v, got %v", late, aa.LastSeen)
	}
	bb := byKey["ip/10.0.0.5"]
	if bb.AllowedCount != 1 || bb.BlockedCount != 0 {
		t.Errorf("bb totals: expected allowed=1 blocked=0, got allowed=%d blocked=%d", bb.AllowedCount, bb.BlockedCount)
	}
}

// device summary supports an optional date range filtering by Day.
func TestRepo_DeviceSummary_DateRange(t *testing.T) {
	r := newTestRepo(t)
	old := day(t, "2026-05-01")
	mid := day(t, "2026-05-15")
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "old.example", Blocked: true, Day: old, Count: 50, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "mid.example", Blocked: true, Day: mid, Count: 2, LastSeen: now},
	)
	from := day(t, "2026-05-10")
	got, err := r.DeviceSummary(tp(from), nil)
	if err != nil {
		t.Fatalf("DeviceSummary: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 device in range, got %d", len(got))
	}
	if got[0].BlockedCount != 2 {
		t.Errorf("expected only the in-range blocked count 2, got %d", got[0].BlockedCount)
	}
}

// negative: a date range that excludes everything → no devices.
func TestRepo_DeviceSummary_OutOfRangeEmpty(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-01")
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "x.example", Blocked: true, Day: d, Count: 1, LastSeen: now},
	)
	from := day(t, "2026-06-01")
	got, err := r.DeviceSummary(tp(from), nil)
	if err != nil {
		t.Fatalf("DeviceSummary: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no devices out of range, got %v", got)
	}
}

// negative: empty table → empty (non-nil) slice.
func TestRepo_DeviceSummary_EmptyTable(t *testing.T) {
	r := newTestRepo(t)
	got, err := r.DeviceSummary(nil, nil)
	if err != nil {
		t.Fatalf("DeviceSummary on empty table: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ----- DomainsForDevice -----

// happy: domains for a device with counts; verdict filter + pagination + total.
func TestRepo_DomainsForDevice_VerdictFilterAndPaging(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "ads1.example", Blocked: true, Day: d, Count: 10, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "ads2.example", Blocked: true, Day: d, Count: 5, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "good.example", Blocked: false, Day: d, Count: 99, LastSeen: now},
		// a different device's rows must not leak in
		DomainTraffic{ClientKind: "mac", ClientValue: "bb", ClientIP: "2.2.2.2", Domain: "ads1.example", Blocked: true, Day: d, Count: 1000, LastSeen: now},
	)

	// blocked-only, ordered by count desc, limit 1 → top blocked domain + total 2
	res, err := r.DomainsForDevice(DeviceDomainsParams{
		Kind: "mac", Value: "aa", Blocked: bp(true), Limit: 1, Offset: 0,
	})
	if err != nil {
		t.Fatalf("DomainsForDevice: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("expected total 2 blocked domains, got %d", res.Total)
	}
	if len(res.List) != 1 {
		t.Fatalf("expected 1 row (limit), got %d (%v)", len(res.List), res.List)
	}
	if res.List[0].Domain != "ads1.example" || res.List[0].Count != 10 {
		t.Errorf("expected top blocked {ads1.example 10}, got %v", res.List[0])
	}

	// page 2 (offset 1) → second blocked domain
	res2, err := r.DomainsForDevice(DeviceDomainsParams{
		Kind: "mac", Value: "aa", Blocked: bp(true), Limit: 1, Offset: 1,
	})
	if err != nil {
		t.Fatalf("DomainsForDevice page2: %v", err)
	}
	if len(res2.List) != 1 || res2.List[0].Domain != "ads2.example" {
		t.Fatalf("expected second page {ads2.example}, got %v", res2.List)
	}
}

// no verdict filter → both blocked and allowed domains for the device.
func TestRepo_DomainsForDevice_NoVerdictFilter(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "ads.example", Blocked: true, Day: d, Count: 3, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "good.example", Blocked: false, Day: d, Count: 7, LastSeen: now},
	)
	res, err := r.DomainsForDevice(DeviceDomainsParams{Kind: "mac", Value: "aa", Limit: 10})
	if err != nil {
		t.Fatalf("DomainsForDevice: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("expected total 2, got %d", res.Total)
	}
	if len(res.List) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.List))
	}
}

// a domain queried both blocked and allowed by the same device, with no verdict
// filter, collapses into one row whose count sums both verdicts (GROUP BY domain).
func TestRepo_DomainsForDevice_MixedVerdictSums(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "mixed.example", Blocked: true, Day: d, Count: 3, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "mixed.example", Blocked: false, Day: d, Count: 4, LastSeen: now},
	)
	res, err := r.DomainsForDevice(DeviceDomainsParams{Kind: "mac", Value: "aa", Limit: 10})
	if err != nil {
		t.Fatalf("DomainsForDevice: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("expected 1 grouped domain, got total %d (%v)", res.Total, res.List)
	}
	if res.List[0].Count != 7 {
		t.Errorf("expected mixed.example count 7 (3+4), got %d", res.List[0].Count)
	}
}

// date range filters the per-device domains.
func TestRepo_DomainsForDevice_DateRange(t *testing.T) {
	r := newTestRepo(t)
	old := day(t, "2026-05-01")
	mid := day(t, "2026-05-15")
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "old.example", Blocked: true, Day: old, Count: 1, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "mid.example", Blocked: true, Day: mid, Count: 1, LastSeen: now},
	)
	from := day(t, "2026-05-10")
	to := day(t, "2026-05-20")
	res, err := r.DomainsForDevice(DeviceDomainsParams{Kind: "mac", Value: "aa", From: tp(from), To: tp(to), Limit: 10})
	if err != nil {
		t.Fatalf("DomainsForDevice: %v", err)
	}
	if res.Total != 1 || res.List[0].Domain != "mid.example" {
		t.Fatalf("expected only mid.example in range, got %v", res.List)
	}
}

// negative: a device with no rows → empty list, total 0, no error.
func TestRepo_DomainsForDevice_UnknownDevice(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "x.example", Blocked: true, Day: d, Count: 1, LastSeen: now},
	)
	res, err := r.DomainsForDevice(DeviceDomainsParams{Kind: "mac", Value: "zz", Limit: 10})
	if err != nil {
		t.Fatalf("DomainsForDevice unknown: %v", err)
	}
	if res.Total != 0 || len(res.List) != 0 {
		t.Errorf("expected empty result for unknown device, got total=%d list=%v", res.Total, res.List)
	}
}

// ----- TopDomains -----

// happy: top domains across all devices, verdict-filterable, ordered desc, limited.
func TestRepo_TopDomains_OrderingAndLimit(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "high.example", Blocked: true, Day: d, Count: 100, LastSeen: now},
		DomainTraffic{ClientKind: "ip", ClientValue: "10.0.0.5", ClientIP: "10.0.0.5", Domain: "high.example", Blocked: true, Day: d, Count: 50, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "mid.example", Blocked: true, Day: d, Count: 30, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "low.example", Blocked: true, Day: d, Count: 5, LastSeen: now},
	)
	got, err := r.TopDomains(bp(true), 2)
	if err != nil {
		t.Fatalf("TopDomains: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 (limit), got %d (%v)", len(got), got)
	}
	if got[0].Domain != "high.example" || got[0].Count != 150 {
		t.Errorf("expected #1 {high.example 150}, got %v", got[0])
	}
	if got[1].Domain != "mid.example" || got[1].Count != 30 {
		t.Errorf("expected #2 {mid.example 30}, got %v", got[1])
	}
}

// no verdict filter → both verdicts summed per domain.
func TestRepo_TopDomains_NoVerdictFilter(t *testing.T) {
	r := newTestRepo(t)
	d := day(t, "2026-05-25")
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	seed(t, r,
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "x.example", Blocked: true, Day: d, Count: 2, LastSeen: now},
		DomainTraffic{ClientKind: "mac", ClientValue: "aa", ClientIP: "1.1.1.1", Domain: "x.example", Blocked: false, Day: d, Count: 3, LastSeen: now},
	)
	got, err := r.TopDomains(nil, 10)
	if err != nil {
		t.Fatalf("TopDomains: %v", err)
	}
	if len(got) != 1 || got[0].Count != 5 {
		t.Fatalf("expected [{x.example 5}], got %v", got)
	}
}

// negative: empty table → empty (non-nil) slice.
func TestRepo_TopDomains_EmptyTable(t *testing.T) {
	r := newTestRepo(t)
	got, err := r.TopDomains(bp(true), 10)
	if err != nil {
		t.Fatalf("TopDomains on empty table: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}
