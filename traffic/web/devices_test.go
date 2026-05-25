package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	traffic_db "github.com/alextorq/dns-filter/traffic/db"
	"github.com/gin-gonic/gin"
)

type fakeLog struct{}

func (fakeLog) Info(args ...any) {}
func (fakeLog) Error(err error)  {}

// fakeRepo records the params it was called with and returns canned results /
// errors, so handler tests assert wire shape and param plumbing without a DB.
type fakeRepo struct {
	summaries   []traffic_db.DeviceSummary
	summaryErr  error
	gotFrom     *time.Time
	gotTo       *time.Time
	domains     traffic_db.DomainsResult
	domainsErr  error
	gotParams   traffic_db.DeviceDomainsParams
	top         []traffic_db.DomainCount
	topErr      error
	gotBlocked  *bool
	gotTopLimit int
}

func (f *fakeRepo) DeviceSummary(from, to *time.Time) ([]traffic_db.DeviceSummary, error) {
	f.gotFrom, f.gotTo = from, to
	return f.summaries, f.summaryErr
}

func (f *fakeRepo) DomainsForDevice(p traffic_db.DeviceDomainsParams) (traffic_db.DomainsResult, error) {
	f.gotParams = p
	return f.domains, f.domainsErr
}

func (f *fakeRepo) TopDomains(blocked *bool, limit int) ([]traffic_db.DomainCount, error) {
	f.gotBlocked, f.gotTopLimit = blocked, limit
	return f.top, f.topErr
}

func newHandlers(repo TrafficRepo) *Handlers {
	gin.SetMode(gin.TestMode)
	// vendor stub: a fixed prefix → name, everything else "".
	vendor := func(mac string) string {
		if len(mac) >= 8 && mac[:8] == "aa:bb:cc" {
			return "AcmeCorp"
		}
		return ""
	}
	return NewHandlers(repo, vendor, fakeLog{})
}

func doGET(h gin.HandlerFunc, path string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/x", h)
	req := httptest.NewRequest(http.MethodGet, "/x"+path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ----- GetDevices -----

func TestGetDevices_HappyEnrichesVendorAndIP(t *testing.T) {
	repo := &fakeRepo{summaries: []traffic_db.DeviceSummary{
		{ClientKind: "mac", ClientValue: "aa:bb:cc:11:22:33", CurrentIP: "192.168.1.20", AllowedCount: 10, BlockedCount: 4, LastSeen: time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)},
		{ClientKind: "ip", ClientValue: "10.0.0.5", CurrentIP: "10.0.0.5", AllowedCount: 1, BlockedCount: 0, LastSeen: time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)},
	}}
	h := newHandlers(repo)

	w := doGET(h.GetDevices, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp DevicesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(resp.Devices))
	}
	mac := resp.Devices[0]
	if mac.Vendor != "AcmeCorp" {
		t.Errorf("mac device must be vendor-enriched, got %q", mac.Vendor)
	}
	if mac.CurrentIP != "192.168.1.20" {
		t.Errorf("expected current_ip passthrough, got %q", mac.CurrentIP)
	}
	if mac.BlockedCount != 4 || mac.AllowedCount != 10 {
		t.Errorf("totals not plumbed: %+v", mac)
	}
	// ip-kind device gets no vendor lookup.
	if resp.Devices[1].Vendor != "" {
		t.Errorf("ip-kind device must have empty vendor, got %q", resp.Devices[1].Vendor)
	}
}

func TestGetDevices_PassesDateRange(t *testing.T) {
	repo := &fakeRepo{}
	h := newHandlers(repo)
	w := doGET(h.GetDevices, "?from=2026-05-01&to=2026-05-31")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if repo.gotFrom == nil || repo.gotTo == nil {
		t.Fatalf("expected from/to plumbed, got from=%v to=%v", repo.gotFrom, repo.gotTo)
	}
	if repo.gotFrom.Format(dateLayout) != "2026-05-01" || repo.gotTo.Format(dateLayout) != "2026-05-31" {
		t.Errorf("date range mis-parsed: from=%v to=%v", repo.gotFrom, repo.gotTo)
	}
}

func TestGetDevices_EmptyReturnsEmptyArray(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	w := doGET(h.GetDevices, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// must be [] not null so the frontend can render an empty state.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["devices"]) != "[]" {
		t.Errorf("expected devices:[], got %s", raw["devices"])
	}
}

func TestGetDevices_BadDate_Returns400(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	w := doGET(h.GetDevices, "?from=not-a-date")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestGetDevices_RepoError_Returns500(t *testing.T) {
	h := newHandlers(&fakeRepo{summaryErr: errors.New("db down")})
	w := doGET(h.GetDevices, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// ----- GetDeviceDomains -----

func TestGetDeviceDomains_HappyPlumbsParams(t *testing.T) {
	repo := &fakeRepo{domains: traffic_db.DomainsResult{
		Total: 2,
		List:  []traffic_db.DomainCount{{Domain: "ads.example", Count: 9}, {Domain: "track.example", Count: 3}},
	}}
	h := newHandlers(repo)

	w := doGET(h.GetDeviceDomains, "?kind=mac&value=aa:bb:cc:11:22:33&blocked=true&limit=10&offset=5&from=2026-05-01&to=2026-05-31")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp DeviceDomainsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.List) != 2 {
		t.Fatalf("unexpected body: %+v", resp)
	}
	p := repo.gotParams
	if p.Kind != "mac" || p.Value != "aa:bb:cc:11:22:33" {
		t.Errorf("device id not plumbed: %+v", p)
	}
	if p.Blocked == nil || !*p.Blocked {
		t.Errorf("blocked not plumbed: %v", p.Blocked)
	}
	if p.Limit != 10 || p.Offset != 5 {
		t.Errorf("paging not plumbed: limit=%d offset=%d", p.Limit, p.Offset)
	}
	if p.From == nil || p.To == nil {
		t.Errorf("date range not plumbed: from=%v to=%v", p.From, p.To)
	}
}

func TestGetDeviceDomains_MissingKindOrValue_Returns400(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	for _, q := range []string{"", "?kind=mac", "?value=aa:bb", "?kind=&value=aa:bb"} {
		w := doGET(h.GetDeviceDomains, q)
		if w.Code != http.StatusBadRequest {
			t.Errorf("query %q: expected 400, got %d (body=%s)", q, w.Code, w.Body.String())
		}
	}
}

func TestGetDeviceDomains_BadKind_Returns400(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	w := doGET(h.GetDeviceDomains, "?kind=wifi&value=aa:bb")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad kind, got %d", w.Code)
	}
}

func TestGetDeviceDomains_BadBlocked_Returns400(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	w := doGET(h.GetDeviceDomains, "?kind=mac&value=aa:bb&blocked=maybe")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad blocked, got %d", w.Code)
	}
}

func TestGetDeviceDomains_BadLimit_Returns400(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	for _, q := range []string{"?kind=mac&value=aa:bb&limit=0", "?kind=mac&value=aa:bb&limit=-3", "?kind=mac&value=aa:bb&limit=abc", "?kind=mac&value=aa:bb&limit=99999"} {
		w := doGET(h.GetDeviceDomains, q)
		if w.Code != http.StatusBadRequest {
			t.Errorf("query %q: expected 400, got %d", q, w.Code)
		}
	}
}

func TestGetDeviceDomains_BadOffset_Returns400(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	w := doGET(h.GetDeviceDomains, "?kind=mac&value=aa:bb&offset=-1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative offset, got %d", w.Code)
	}
}

func TestGetDeviceDomains_RepoError_Returns500(t *testing.T) {
	h := newHandlers(&fakeRepo{domainsErr: errors.New("scan failed")})
	w := doGET(h.GetDeviceDomains, "?kind=mac&value=aa:bb")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestGetDeviceDomains_DefaultLimitWhenAbsent(t *testing.T) {
	repo := &fakeRepo{}
	h := newHandlers(repo)
	w := doGET(h.GetDeviceDomains, "?kind=ip&value=10.0.0.5")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if repo.gotParams.Limit != defaultTopLimit {
		t.Errorf("expected default limit %d, got %d", defaultTopLimit, repo.gotParams.Limit)
	}
	if repo.gotParams.Blocked != nil {
		t.Errorf("expected nil blocked when absent, got %v", repo.gotParams.Blocked)
	}
}

// ----- GetTopDomains -----

func TestGetTopDomains_HappyPlumbsParams(t *testing.T) {
	repo := &fakeRepo{top: []traffic_db.DomainCount{{Domain: "x.example", Count: 99}}}
	h := newHandlers(repo)
	w := doGET(h.GetTopDomains, "?blocked=false&limit=5")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp TopDomainsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.List) != 1 || resp.List[0].Domain != "x.example" {
		t.Fatalf("unexpected list: %+v", resp.List)
	}
	if repo.gotBlocked == nil || *repo.gotBlocked != false {
		t.Errorf("blocked=false not plumbed: %v", repo.gotBlocked)
	}
	if repo.gotTopLimit != 5 {
		t.Errorf("limit not plumbed: got %d", repo.gotTopLimit)
	}
}

func TestGetTopDomains_DefaultLimit(t *testing.T) {
	repo := &fakeRepo{}
	h := newHandlers(repo)
	w := doGET(h.GetTopDomains, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if repo.gotTopLimit != defaultTopLimit {
		t.Errorf("expected default limit %d, got %d", defaultTopLimit, repo.gotTopLimit)
	}
}

func TestGetTopDomains_EmptyReturnsEmptyArray(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	w := doGET(h.GetTopDomains, "")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["list"]) != "[]" {
		t.Errorf("expected list:[], got %s", raw["list"])
	}
}

func TestGetTopDomains_BadLimit_Returns400(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	w := doGET(h.GetTopDomains, "?limit=0")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetTopDomains_BadBlocked_Returns400(t *testing.T) {
	h := newHandlers(&fakeRepo{})
	w := doGET(h.GetTopDomains, "?blocked=nope")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetTopDomains_RepoError_Returns500(t *testing.T) {
	h := newHandlers(&fakeRepo{topErr: errors.New("boom")})
	w := doGET(h.GetTopDomains, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
