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

// urlscanEndpoint is a var (not const) so tests can point it at httptest.Server.
var urlscanEndpoint = "https://urlscan.io/api/v1/search/"

type urlscanSearchResponse struct {
	Total   int `json:"total"`
	Results []struct {
		Verdicts struct {
			Overall struct {
				Score      int      `json:"score"`
				Malicious  bool     `json:"malicious"`
				Categories []string `json:"categories"`
			} `json:"overall"`
		} `json:"verdicts"`
		Page struct {
			URL    string `json:"url"`
			Domain string `json:"domain"`
		} `json:"page"`
		Task struct {
			Time string `json:"time"`
		} `json:"task"`
	} `json:"results"`
}

// URLScan looks up recent scans for the domain via the urlscan.io search API.
// We do not submit new scans here — that costs an API quota per call and the
// result is asynchronous. The search endpoint returns whatever was scanned by
// the community already, which is typically enough for popular domains.
func URLScan(ctx context.Context, domain string) domain_inspect.CheckResult {
	key := config.GetConfig().URLScanKey
	if key == "" {
		return skipped("DNS_FILTER_URLSCAN_KEY not set")
	}

	q := url.QueryEscape("domain:" + domain)
	u := urlscanEndpoint + "?q=" + q + "&size=10"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errorResult(err)
	}
	req.Header.Set("API-Key", key)

	resp, err := httpClient.Do(req)
	if err != nil {
		return contextErrorResult(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return domain_inspect.CheckResult{
			Status: domain_inspect.StatusError,
			Error:  fmt.Sprintf("urlscan http %d", resp.StatusCode),
		}
	}

	var body urlscanSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return errorResult(fmt.Errorf("decode urlscan: %w", err))
	}

	maliciousHits := 0
	maxScore := 0
	categories := map[string]struct{}{}
	for _, r := range body.Results {
		if r.Verdicts.Overall.Malicious {
			maliciousHits++
		}
		if r.Verdicts.Overall.Score > maxScore {
			maxScore = r.Verdicts.Overall.Score
		}
		for _, c := range r.Verdicts.Overall.Categories {
			categories[c] = struct{}{}
		}
	}

	verdict := domain_inspect.VerdictUnknown
	switch {
	case maliciousHits > 0:
		verdict = domain_inspect.VerdictMalicious
	case maxScore >= 50:
		verdict = domain_inspect.VerdictSuspicious
	case body.Total > 0:
		verdict = domain_inspect.VerdictClean
	}

	cats := make([]string, 0, len(categories))
	for c := range categories {
		cats = append(cats, c)
	}

	return domain_inspect.CheckResult{
		Status:  domain_inspect.StatusOK,
		Verdict: verdict,
		Details: map[string]any{
			"scans_found":    body.Total,
			"malicious_hits": maliciousHits,
			"max_score":      maxScore,
			"categories":     cats,
		},
	}
}
