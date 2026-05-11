package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/alextorq/dns-filter/config"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// vtEndpoint is a var (not const) so tests can point it at httptest.Server.
var vtEndpoint = "https://www.virustotal.com/api/v3/domains/"

type vtResponse struct {
	Data struct {
		Attributes struct {
			LastAnalysisStats struct {
				Harmless   int `json:"harmless"`
				Malicious  int `json:"malicious"`
				Suspicious int `json:"suspicious"`
				Undetected int `json:"undetected"`
				Timeout    int `json:"timeout"`
			} `json:"last_analysis_stats"`
			Reputation int      `json:"reputation"`
			Categories any      `json:"categories"`
			Tags       []string `json:"tags"`
		} `json:"attributes"`
	} `json:"data"`
}

// VirusTotal asks VT v3 for the aggregated verdict of ~90 antivirus engines.
// Skipped silently when no API key is configured — the endpoint should still
// run for environments that simply chose not to enable VT.
func VirusTotal(ctx context.Context, domain string) domain_inspect.CheckResult {
	key := config.GetConfig().VirusTotalKey
	if key == "" {
		return skipped("DNS_FILTER_VT_KEY not set")
	}

	u := vtEndpoint + url.PathEscape(domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errorResult(err)
	}
	req.Header.Set("x-apikey", key)

	resp, err := httpClient.Do(req)
	if err != nil {
		return contextErrorResult(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return domain_inspect.CheckResult{
			Status:  domain_inspect.StatusOK,
			Verdict: domain_inspect.VerdictUnknown,
			Details: map[string]any{"known_to_vt": false},
		}
	}
	if resp.StatusCode >= 400 {
		return domain_inspect.CheckResult{
			Status: domain_inspect.StatusError,
			Error:  fmt.Sprintf("virustotal http %d", resp.StatusCode),
		}
	}

	var body vtResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return errorResult(fmt.Errorf("decode virustotal: %w", err))
	}

	stats := body.Data.Attributes.LastAnalysisStats
	verdict := domain_inspect.VerdictUnknown
	switch {
	case stats.Malicious >= 3:
		verdict = domain_inspect.VerdictMalicious
	case stats.Malicious >= 1 || stats.Suspicious >= 2:
		verdict = domain_inspect.VerdictSuspicious
	case stats.Harmless+stats.Undetected > 0:
		verdict = domain_inspect.VerdictClean
	}

	return domain_inspect.CheckResult{
		Status:  domain_inspect.StatusOK,
		Verdict: verdict,
		Details: map[string]any{
			"known_to_vt": true,
			"malicious":   stats.Malicious,
			"suspicious":  stats.Suspicious,
			"harmless":    stats.Harmless,
			"undetected":  stats.Undetected,
			"reputation":  body.Data.Attributes.Reputation,
			"categories":  body.Data.Attributes.Categories,
			"tags":        body.Data.Attributes.Tags,
		},
	}
}
