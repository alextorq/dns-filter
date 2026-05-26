package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// fakeBlockStats is a stub BlockStatsRepo: handler tests assert the wire shape
// without standing up a real traffic table, and exercise the error path.
type fakeBlockStats struct {
	total      int64
	totalErr   error
	totalCalls int
}

func (f *fakeBlockStats) BlockedTotalCount() (int64, error) {
	f.totalCalls++
	return f.total, f.totalErr
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
