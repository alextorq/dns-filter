package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/gin-gonic/gin"
)

// fakeBlockStats is a stub BlockStatsRepo: handler tests assert the wire shape
// without standing up a real traffic table, and exercise the error path.
type fakeBlockStats struct {
	total      int64
	totalErr   error
	groups     []blocked_domain_db.DomainCount
	groupsErr  error
	totalCalls int
	groupCalls int
}

func (f *fakeBlockStats) BlockedTotalCount() (int64, error) {
	f.totalCalls++
	return f.total, f.totalErr
}

func (f *fakeBlockStats) BlockedCountByDomain() ([]blocked_domain_db.DomainCount, error) {
	f.groupCalls++
	return f.groups, f.groupsErr
}

func eventsHandler(stats BlockStatsRepo) *Handlers {
	gin.SetMode(gin.TestMode)
	return &Handlers{Log: fakeLog{}, BlockStats: stats}
}

func callGET(t *testing.T, h gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.POST("/x", h)
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// GetAmount must read the blocked grand total from the traffic-backed port and
// return it under the existing {amount} shape.
func TestGetAmount_ReadsFromBlockStats(t *testing.T) {
	stats := &fakeBlockStats{total: 42}
	h := eventsHandler(stats)

	w := callGET(t, h.GetAmount)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if stats.totalCalls != 1 {
		t.Errorf("expected BlockedTotalCount called once, got %d", stats.totalCalls)
	}
	var resp GetAmountResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Amount != 42 {
		t.Errorf("expected amount 42, got %d", resp.Amount)
	}
}

// Negative: an error from the port surfaces as 500.
func TestGetAmount_PortError_Returns500(t *testing.T) {
	h := eventsHandler(&fakeBlockStats{totalErr: errors.New("db down")})
	w := callGET(t, h.GetAmount)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// GetAmountByDomain must read grouped blocked counts from the traffic-backed
// port and keep the {groups:[{domain,count}]} shape byte-for-byte.
func TestGetAmountByDomain_ReadsFromBlockStats(t *testing.T) {
	stats := &fakeBlockStats{groups: []blocked_domain_db.DomainCount{
		{Domain: "ads.example", Count: 7},
		{Domain: "track.example", Count: 3},
	}}
	h := eventsHandler(stats)

	w := callGET(t, h.GetAmountByDomain)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if stats.groupCalls != 1 {
		t.Errorf("expected BlockedCountByDomain called once, got %d", stats.groupCalls)
	}
	var resp GetAmountByDomainResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp.Groups))
	}
	if resp.Groups[0].Domain != "ads.example" || resp.Groups[0].Count != 7 {
		t.Errorf("unexpected first group: %+v", resp.Groups[0])
	}
}

// The wire JSON must be exactly {"groups":[{"domain":...,"count":...}]} — the
// existing frontend reads these keys. Assert raw keys, not just the decoded DTO.
func TestGetAmountByDomain_WireShapeUnchanged(t *testing.T) {
	stats := &fakeBlockStats{groups: []blocked_domain_db.DomainCount{{Domain: "a.example", Count: 1}}}
	h := eventsHandler(stats)

	w := callGET(t, h.GetAmountByDomain)
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	groups, ok := raw["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("expected top-level groups array of len 1, got %v", raw)
	}
	first, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatalf("group not an object: %v", groups[0])
	}
	if _, ok := first["domain"]; !ok {
		t.Error("group missing 'domain' key")
	}
	if _, ok := first["count"]; !ok {
		t.Error("group missing 'count' key")
	}
}

// Negative: an error from the grouped query surfaces as 500.
func TestGetAmountByDomain_PortError_Returns500(t *testing.T) {
	h := eventsHandler(&fakeBlockStats{groupsErr: errors.New("scan failed")})
	w := callGET(t, h.GetAmountByDomain)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d (body=%s)", w.Code, w.Body.String())
	}
}
