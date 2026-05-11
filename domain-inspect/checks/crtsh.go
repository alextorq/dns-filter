package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// crtshEndpoint is a var (not const) so tests can point it at httptest.Server.
var crtshEndpoint = "https://crt.sh/"

type crtshEntry struct {
	NameValue string `json:"name_value"`
	Issuer    string `json:"issuer_name"`
	NotBefore string `json:"not_before"`
}

// CrtSh queries certificate transparency logs through crt.sh. A long-lived
// domain with diverse subdomains and certificates is a clean signal; a brand
// new domain with no certs is mildly suspicious. We do not call this
// authoritative — just useful context.
func CrtSh(ctx context.Context, domain string) domain_inspect.CheckResult {
	u := crtshEndpoint + "?q=" + url.QueryEscape(domain) + "&output=json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errorResult(err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return contextErrorResult(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return domain_inspect.CheckResult{
			Status: domain_inspect.StatusError,
			Error:  fmt.Sprintf("crt.sh http %d", resp.StatusCode),
		}
	}

	var entries []crtshEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return errorResult(fmt.Errorf("decode crt.sh: %w", err))
	}

	subdomains := make(map[string]struct{})
	for _, e := range entries {
		for line := range strings.SplitSeq(e.NameValue, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				subdomains[strings.ToLower(line)] = struct{}{}
			}
		}
	}

	earliest := ""
	for _, e := range entries {
		if earliest == "" || e.NotBefore < earliest {
			earliest = e.NotBefore
		}
	}

	return domain_inspect.CheckResult{
		Status:  domain_inspect.StatusOK,
		Verdict: domain_inspect.VerdictUnknown,
		Details: map[string]any{
			"certificates":    len(entries),
			"unique_names":    len(subdomains),
			"earliest_issued": earliest,
		},
	}
}
