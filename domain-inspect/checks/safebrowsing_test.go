package checks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alextorq/dns-filter/config"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

func withSafeBrowsingEndpointAndKey(t *testing.T, ts *httptest.Server, key string) {
	t.Helper()
	prev := sbEndpoint
	sbEndpoint = ts.URL + "/"

	cfg := config.GetConfig()
	prevKey := cfg.SafeBrowsingKey
	cfg.SafeBrowsingKey = key

	t.Cleanup(func() {
		sbEndpoint = prev
		cfg.SafeBrowsingKey = prevKey
	})
}

// Without a key the check must be skipped (not errored). Operators who don't
// have a Google Cloud project should still see a sensible aggregated result.
func TestSafeBrowsing_NoKey_Skipped(t *testing.T) {
	cfg := config.GetConfig()
	prev := cfg.SafeBrowsingKey
	cfg.SafeBrowsingKey = ""
	t.Cleanup(func() { cfg.SafeBrowsingKey = prev })

	res := SafeBrowsing(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusSkipped {
		t.Errorf("expected skipped, got %s", res.Status)
	}
}

// Positive: any non-empty matches[] from Google means the domain is on a
// blocklist Google trusts. We escalate straight to malicious — this is one of
// the highest-signal checks in the suite, so a single hit is enough.
func TestSafeBrowsing_MaliciousMatch(t *testing.T) {
	const apiKey = "test-key-sb"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API key travels in the query string, not a header.
		if got := r.URL.Query().Get("key"); got != apiKey {
			t.Errorf("missing/wrong api key in query: %q", got)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Body should mention the domain at least once — covers both http://
		// and https:// entries we submit.
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		if got, _ := parsed["client"].(map[string]any); got == nil {
			t.Error("missing client identification in request")
		}

		_, _ = w.Write([]byte(`{"matches":[
			{"threatType":"MALWARE","threat":{"url":"http://x.example/"},"platformType":"ANY_PLATFORM"},
			{"threatType":"SOCIAL_ENGINEERING","threat":{"url":"https://x.example/"},"platformType":"ANY_PLATFORM"}
		]}`))
	}))
	defer ts.Close()
	withSafeBrowsingEndpointAndKey(t, ts, apiKey)

	res := SafeBrowsing(context.Background(), "x.example")

	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("status: got %s, want OK", res.Status)
	}
	if res.Verdict != domain_inspect.VerdictMalicious {
		t.Errorf("verdict: got %s, want malicious", res.Verdict)
	}
	threats, _ := res.Details["threat_types"].([]string)
	if len(threats) != 2 {
		t.Errorf("threat_types length: got %d, want 2", len(threats))
	}
}

// Negative (in the verdict sense): an empty matches[] from a 200 OK is the
// documented "we have nothing on this" response. That's a clean signal — we
// must NOT report this as unknown, because Safe Browsing speaking up at all
// is a meaningful endorsement.
func TestSafeBrowsing_NoMatch_IsClean(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	withSafeBrowsingEndpointAndKey(t, ts, "k")

	res := SafeBrowsing(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("status: got %s, want OK", res.Status)
	}
	if res.Verdict != domain_inspect.VerdictClean {
		t.Errorf("verdict: got %s, want clean", res.Verdict)
	}
	threats, _ := res.Details["threat_types"].([]string)
	if len(threats) != 0 {
		t.Errorf("threat_types length: got %d, want 0", len(threats))
	}
}

// Negative: a 403 from Google usually means a bad/expired/over-quota key.
// We surface that as a normal error result so the operator notices something
// is misconfigured — silently degrading to unknown would hide the problem.
func TestSafeBrowsing_Forbidden_IsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"PERMISSION_DENIED"}}`))
	}))
	defer ts.Close()
	withSafeBrowsingEndpointAndKey(t, ts, "k")

	res := SafeBrowsing(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusError {
		t.Errorf("expected error status on 403, got %s", res.Status)
	}
}

// 429 surfaces as its own rate_limited status (distinct from a generic error)
// so a batch caller can pause and back off instead of failing the domain.
func TestSafeBrowsing_RateLimited_IsRateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()
	withSafeBrowsingEndpointAndKey(t, ts, "k")

	res := SafeBrowsing(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusRateLimited {
		t.Errorf("429 must surface as rate_limited, got status=%s", res.Status)
	}
}

// Negative: malformed upstream JSON should not panic — must return error.
func TestSafeBrowsing_GarbageJSON_IsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer ts.Close()
	withSafeBrowsingEndpointAndKey(t, ts, "k")

	res := SafeBrowsing(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusError {
		t.Errorf("expected error on garbage json, got %s", res.Status)
	}
}
