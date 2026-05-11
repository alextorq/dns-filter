package checks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// RDAP only knows about registerable domains (eTLD+1). The check must reduce
// any sub-FQDN to its registerable parent before querying — otherwise
// `report.appmetrica.yandex.net` 404s with "registered=false", which is a
// false signal for a deeply legitimate subdomain.
func TestRDAPAge_Subdomain_QueriesRegistrableParent(t *testing.T) {
	var lastPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		fmt.Fprint(w, rdapBody(1000))
	}))
	defer ts.Close()
	withRDAPEndpoint(t, ts)

	res := RDAPAge(context.Background(), "report.appmetrica.yandex.net")

	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("status: got %s, want OK", res.Status)
	}
	// The path must end with the registrable parent, not the full FQDN.
	if !strings.HasSuffix(lastPath, "/yandex.net") {
		t.Errorf("RDAP must query eTLD+1, but path was %q", lastPath)
	}
	// Sanity: the upstream returned a 1000-day-old registration, so the
	// verdict has to be clean — proves the parent's age was actually used.
	if res.Verdict != domain_inspect.VerdictClean {
		t.Errorf("verdict: got %s, want clean (parent is 1000 days old)", res.Verdict)
	}
	if got, _ := res.Details["queried_domain"].(string); got != "yandex.net" {
		t.Errorf("queried_domain in details: got %q, want yandex.net", got)
	}
}

// An apex domain (eTLD+1 already) must not be mangled — covers the regression
// where a naive .Split(".")[1:] would strip the SLD itself.
func TestRDAPAge_ApexDomain_QueriesSelf(t *testing.T) {
	var lastPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		fmt.Fprint(w, rdapBody(1000))
	}))
	defer ts.Close()
	withRDAPEndpoint(t, ts)

	RDAPAge(context.Background(), "example.com")

	if !strings.HasSuffix(lastPath, "/example.com") {
		t.Errorf("apex domain must be queried as-is, but path was %q", lastPath)
	}
}

// A bare TLD has no registerable parent — we should not panic and not invent
// a fake "registered=true" by querying something nonsensical. Returning
// "unknown" with status OK is the correct degrade.
func TestRDAPAge_BareTLD_IsUnknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be called for a bare TLD")
	}))
	defer ts.Close()
	withRDAPEndpoint(t, ts)

	res := RDAPAge(context.Background(), "com")
	if res.Status != domain_inspect.StatusOK {
		t.Errorf("status: got %s, want OK", res.Status)
	}
	if res.Verdict != domain_inspect.VerdictUnknown {
		t.Errorf("verdict: got %s, want unknown", res.Verdict)
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
