package checks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alextorq/dns-filter/config"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

func withVTEndpointAndKey(t *testing.T, ts *httptest.Server, key string) {
	t.Helper()
	prev := vtEndpoint
	vtEndpoint = ts.URL + "/"

	cfg := config.GetConfig()
	prevKey := cfg.VirusTotalKey
	cfg.VirusTotalKey = key

	t.Cleanup(func() {
		vtEndpoint = prev
		cfg.VirusTotalKey = prevKey
	})
}

// Without a key the check must be skipped, never errored — operators who
// don't have a VT API key should still get a sensible aggregated result.
func TestVirusTotal_NoKey_Skipped(t *testing.T) {
	cfg := config.GetConfig()
	prev := cfg.VirusTotalKey
	cfg.VirusTotalKey = ""
	t.Cleanup(func() { cfg.VirusTotalKey = prev })

	res := VirusTotal(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusSkipped {
		t.Errorf("expected skipped, got %s", res.Status)
	}
}

func TestVirusTotal_Malicious(t *testing.T) {
	const apiKey = "test-key-vt"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-apikey"); got != apiKey {
			t.Errorf("missing/wrong api key header: %q", got)
		}
		_, _ = w.Write([]byte(`{"data":{"attributes":{
			"last_analysis_stats":{"harmless":40,"malicious":7,"suspicious":1,"undetected":10,"timeout":0},
			"reputation":-50,"tags":["malware"]
		}}}`))
	}))
	defer ts.Close()
	withVTEndpointAndKey(t, ts, apiKey)

	res := VirusTotal(context.Background(), "x.example")

	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("status: got %s, want OK", res.Status)
	}
	if res.Verdict != domain_inspect.VerdictMalicious {
		t.Errorf("verdict: got %s, want malicious", res.Verdict)
	}
	if got, _ := res.Details["malicious"].(int); got != 7 {
		t.Errorf("malicious count: got %d, want 7", got)
	}
}

func TestVirusTotal_OneMalicious_IsSuspicious(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"attributes":{
			"last_analysis_stats":{"harmless":50,"malicious":1,"suspicious":0,"undetected":10,"timeout":0}
		}}}`))
	}))
	defer ts.Close()
	withVTEndpointAndKey(t, ts, "k")

	res := VirusTotal(context.Background(), "x.example")
	if res.Verdict != domain_inspect.VerdictSuspicious {
		t.Errorf("verdict: got %s, want suspicious", res.Verdict)
	}
}

func TestVirusTotal_Clean(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"attributes":{
			"last_analysis_stats":{"harmless":80,"malicious":0,"suspicious":0,"undetected":10,"timeout":0}
		}}}`))
	}))
	defer ts.Close()
	withVTEndpointAndKey(t, ts, "k")

	res := VirusTotal(context.Background(), "x.example")
	if res.Verdict != domain_inspect.VerdictClean {
		t.Errorf("verdict: got %s, want clean", res.Verdict)
	}
}

// VT returns 404 for domains it has never observed. That's an "unknown" verdict
// with status OK — definitely not an error.
func TestVirusTotal_NotFound_IsUnknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	withVTEndpointAndKey(t, ts, "k")

	res := VirusTotal(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("status: got %s, want OK", res.Status)
	}
	if res.Verdict != domain_inspect.VerdictUnknown {
		t.Errorf("verdict: got %s, want unknown", res.Verdict)
	}
}

func TestVirusTotal_RateLimited_IsRateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()
	withVTEndpointAndKey(t, ts, "k")

	res := VirusTotal(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusRateLimited {
		t.Errorf("429 must surface as rate_limited (distinct from error), got status=%s", res.Status)
	}
	if !strings.Contains(res.Error, "429") {
		t.Errorf("error should mention http code, got %q", res.Error)
	}
}
