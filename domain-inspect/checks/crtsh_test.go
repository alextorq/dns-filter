package checks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

func withCrtshEndpoint(t *testing.T, ts *httptest.Server) {
	t.Helper()
	prev := crtshEndpoint
	crtshEndpoint = ts.URL + "/"
	t.Cleanup(func() { crtshEndpoint = prev })
}

func TestCrtSh_CountsCertsAndUniqueNames(t *testing.T) {
	// Two entries, one with a multi-line name_value (CT logs return CN + SAN
	// joined with newlines). Unique subdomain count must dedupe across that.
	body := `[
		{"name_value":"example.com\nwww.example.com","issuer_name":"Let's Encrypt","not_before":"2023-01-15"},
		{"name_value":"api.example.com","issuer_name":"Let's Encrypt","not_before":"2022-06-01"}
	]`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	withCrtshEndpoint(t, ts)

	res := CrtSh(context.Background(), "example.com")

	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("status: got %s, want OK", res.Status)
	}
	if got, _ := res.Details["certificates"].(int); got != 2 {
		t.Errorf("certificates: got %d, want 2", got)
	}
	if got, _ := res.Details["unique_names"].(int); got != 3 {
		t.Errorf("unique_names: got %d, want 3", got)
	}
	if got, _ := res.Details["earliest_issued"].(string); got != "2022-06-01" {
		t.Errorf("earliest_issued: got %q, want 2022-06-01", got)
	}
}

func TestCrtSh_EmptyResultIsNotAnError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer ts.Close()
	withCrtshEndpoint(t, ts)

	res := CrtSh(context.Background(), "nope.example")
	if res.Status != domain_inspect.StatusOK {
		t.Errorf("empty list should be OK, got %s", res.Status)
	}
	if got, _ := res.Details["certificates"].(int); got != 0 {
		t.Errorf("certificates: got %d, want 0", got)
	}
}
