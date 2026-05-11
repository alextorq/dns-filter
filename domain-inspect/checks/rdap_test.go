package checks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// withRDAPEndpoint swaps the package-level endpoint for the lifetime of a
// single test. Restores the original value via t.Cleanup so parallel tests
// (run via go test) cannot leak state.
func withRDAPEndpoint(t *testing.T, ts *httptest.Server) {
	t.Helper()
	prev := rdapEndpoint
	rdapEndpoint = ts.URL + "/"
	t.Cleanup(func() { rdapEndpoint = prev })
}

func rdapBody(registeredDaysAgo int) string {
	when := time.Now().AddDate(0, 0, -registeredDaysAgo).UTC().Format(time.RFC3339)
	return fmt.Sprintf(`{"ldhName":"x.example","status":["active"],"events":[{"eventAction":"registration","eventDate":%q}]}`, when)
}

func TestRDAPAge_YoungDomain_IsSuspicious(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rdap+json")
		fmt.Fprint(w, rdapBody(5))
	}))
	defer ts.Close()
	withRDAPEndpoint(t, ts)

	res := RDAPAge(context.Background(), "x.example")

	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("status: got %s, want OK", res.Status)
	}
	if res.Verdict != domain_inspect.VerdictSuspicious {
		t.Errorf("verdict: got %s, want suspicious (5 days old)", res.Verdict)
	}
}

func TestRDAPAge_OldDomain_IsClean(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, rdapBody(400))
	}))
	defer ts.Close()
	withRDAPEndpoint(t, ts)

	res := RDAPAge(context.Background(), "x.example")
	if res.Verdict != domain_inspect.VerdictClean {
		t.Errorf("verdict: got %s, want clean (400 days old)", res.Verdict)
	}
}

// 404 is the documented response for an unregistered TLD/domain in RDAP. We
// must distinguish it from a server error so the UI can show "not registered"
// instead of "check failed".
func TestRDAPAge_NotFound_IsNotRegistered(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	withRDAPEndpoint(t, ts)

	res := RDAPAge(context.Background(), "nope.example")
	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("status: got %s, want OK", res.Status)
	}
	if got, _ := res.Details["registered"].(bool); got {
		t.Error("expected registered=false on 404")
	}
}

func TestRDAPAge_5xx_IsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	withRDAPEndpoint(t, ts)

	res := RDAPAge(context.Background(), "x.example")
	if res.Status != domain_inspect.StatusError {
		t.Errorf("expected error status on 5xx, got %s", res.Status)
	}
}
