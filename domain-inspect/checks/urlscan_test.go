package checks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alextorq/dns-filter/config"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

func withURLScanEndpointAndKey(t *testing.T, ts *httptest.Server, key string) {
	t.Helper()
	prev := urlscanEndpoint
	urlscanEndpoint = ts.URL + "/"

	cfg := config.GetConfig()
	prevKey := cfg.URLScanKey
	cfg.URLScanKey = key

	t.Cleanup(func() {
		urlscanEndpoint = prev
		cfg.URLScanKey = prevKey
	})
}

func TestURLScan_NoKey_Skipped(t *testing.T) {
	cfg := config.GetConfig()
	prev := cfg.URLScanKey
	cfg.URLScanKey = ""
	t.Cleanup(func() { cfg.URLScanKey = prev })

	res := URLScan(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusSkipped {
		t.Errorf("expected skipped, got %s", res.Status)
	}
}

func TestURLScan_MaliciousHit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("API-Key"); got != "k" {
			t.Errorf("missing API-Key header: %q", got)
		}
		_, _ = w.Write([]byte(`{"total":2,"results":[
			{"verdicts":{"overall":{"score":80,"malicious":true,"categories":["phishing"]}},"page":{"url":"https://x.example/","domain":"x.example"},"task":{"time":"2024-01-01"}},
			{"verdicts":{"overall":{"score":40,"malicious":false}},"page":{"url":"https://x.example/2","domain":"x.example"},"task":{"time":"2024-02-01"}}
		]}`))
	}))
	defer ts.Close()
	withURLScanEndpointAndKey(t, ts, "k")

	res := URLScan(context.Background(), "x.example")
	if res.Verdict != domain_inspect.VerdictMalicious {
		t.Errorf("verdict: got %s, want malicious", res.Verdict)
	}
	if got, _ := res.Details["malicious_hits"].(int); got != 1 {
		t.Errorf("malicious_hits: got %d, want 1", got)
	}
	if got, _ := res.Details["max_score"].(int); got != 80 {
		t.Errorf("max_score: got %d, want 80", got)
	}
}

// A high score without an explicit malicious flag should be flagged as
// suspicious but not malicious. Locks in the "≥50 but not flagged" rule.
func TestURLScan_HighScoreNotMalicious_IsSuspicious(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"total":1,"results":[
			{"verdicts":{"overall":{"score":60,"malicious":false}},"page":{"url":"https://x.example/","domain":"x.example"}}
		]}`))
	}))
	defer ts.Close()
	withURLScanEndpointAndKey(t, ts, "k")

	res := URLScan(context.Background(), "x.example")
	if res.Verdict != domain_inspect.VerdictSuspicious {
		t.Errorf("verdict: got %s, want suspicious", res.Verdict)
	}
}

func TestURLScan_NoScans_IsUnknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"total":0,"results":[]}`))
	}))
	defer ts.Close()
	withURLScanEndpointAndKey(t, ts, "k")

	res := URLScan(context.Background(), "x.example")
	if res.Verdict != domain_inspect.VerdictUnknown {
		t.Errorf("verdict: got %s, want unknown", res.Verdict)
	}
}
