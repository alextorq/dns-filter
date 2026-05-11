package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// rdapEndpoint is a var (not const) so tests can point it at httptest.Server.
var rdapEndpoint = "https://rdap.org/domain/"

type rdapEvent struct {
	EventAction string `json:"eventAction"`
	EventDate   string `json:"eventDate"`
}

type rdapResponse struct {
	LdhName string      `json:"ldhName"`
	Events  []rdapEvent `json:"events"`
	Status  []string    `json:"status"`
}

// RDAPAge looks up the domain in the public RDAP relay and reports the
// registration age. Domains younger than 30 days are flagged as suspicious:
// freshly registered names dominate phishing/malware traffic.
func RDAPAge(ctx context.Context, domain string) domain_inspect.CheckResult {
	u := rdapEndpoint + url.PathEscape(domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errorResult(err)
	}
	req.Header.Set("Accept", "application/rdap+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return contextErrorResult(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return domain_inspect.CheckResult{
			Status:  domain_inspect.StatusOK,
			Verdict: domain_inspect.VerdictUnknown,
			Details: map[string]any{"registered": false},
		}
	}
	if resp.StatusCode >= 400 {
		return domain_inspect.CheckResult{
			Status: domain_inspect.StatusError,
			Error:  fmt.Sprintf("rdap http %d", resp.StatusCode),
		}
	}

	var body rdapResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return errorResult(fmt.Errorf("decode rdap: %w", err))
	}

	details := map[string]any{
		"registered": true,
		"status":     body.Status,
	}

	var registered time.Time
	for _, e := range body.Events {
		if e.EventAction == "registration" {
			if t, err := time.Parse(time.RFC3339, e.EventDate); err == nil {
				registered = t
				details["registered_at"] = t.Format(time.RFC3339)
			}
			break
		}
	}

	verdict := domain_inspect.VerdictUnknown
	if !registered.IsZero() {
		ageDays := int(time.Since(registered).Hours() / 24)
		details["age_days"] = ageDays
		switch {
		case ageDays < 30:
			verdict = domain_inspect.VerdictSuspicious
		case ageDays > 365:
			verdict = domain_inspect.VerdictClean
		}
	}

	return domain_inspect.CheckResult{
		Status:  domain_inspect.StatusOK,
		Verdict: verdict,
		Details: details,
	}
}
