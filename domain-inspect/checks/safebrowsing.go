package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// sbEndpoint is a var (not const) so tests can point it at httptest.Server.
var sbEndpoint = "https://safebrowsing.googleapis.com/v4/threatMatches:find"

// Safe Browsing v4 operates on URLs, not bare domains. We submit both http://
// and https:// forms with a trailing slash — Google's list cares about scheme
// + host, and a malicious entry indexed under one scheme will not match the
// other. Two entries per call is well within the 500-URL request limit.
var sbThreatTypes = []string{
	"MALWARE",
	"SOCIAL_ENGINEERING",
	"UNWANTED_SOFTWARE",
	"POTENTIALLY_HARMFUL_APPLICATION",
}

type sbRequest struct {
	Client     sbClient     `json:"client"`
	ThreatInfo sbThreatInfo `json:"threatInfo"`
}

type sbClient struct {
	ClientID      string `json:"clientId"`
	ClientVersion string `json:"clientVersion"`
}

type sbThreatInfo struct {
	ThreatTypes      []string             `json:"threatTypes"`
	PlatformTypes    []string             `json:"platformTypes"`
	ThreatEntryTypes []string             `json:"threatEntryTypes"`
	ThreatEntries    []sbThreatEntryInput `json:"threatEntries"`
}

type sbThreatEntryInput struct {
	URL string `json:"url"`
}

type sbResponse struct {
	Matches []sbMatch `json:"matches"`
}

type sbMatch struct {
	ThreatType   string             `json:"threatType"`
	PlatformType string             `json:"platformType"`
	Threat       sbThreatEntryInput `json:"threat"`
}

// SafeBrowsing checks the domain against Google's Safe Browsing v4 list.
// A non-empty matches[] is treated as a strong "malicious" signal because
// the list is conservative: Google only adds confirmed malware, phishing,
// or unwanted-software endpoints. An empty matches[] from a 200 means
// "Google has nothing on this", which we surface as `clean` — that's a
// real endorsement, not "unknown".
func SafeBrowsing(ctx context.Context, domain string) domain_inspect.CheckResult {
	// Ключ держится в атомике (см. keys.go): дескриптор настройки
	// safebrowsing_key обновляет его без рестарта.
	key := GetSBKey()
	if key == "" {
		return skipped("safebrowsing_key not set")
	}

	body, err := json.Marshal(sbRequest{
		Client: sbClient{ClientID: "dns-filter", ClientVersion: "1.0"},
		ThreatInfo: sbThreatInfo{
			ThreatTypes:      sbThreatTypes,
			PlatformTypes:    []string{"ANY_PLATFORM"},
			ThreatEntryTypes: []string{"URL"},
			ThreatEntries: []sbThreatEntryInput{
				{URL: "http://" + domain + "/"},
				{URL: "https://" + domain + "/"},
			},
		},
	})
	if err != nil {
		return errorResult(fmt.Errorf("marshal safe browsing request: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sbEndpoint+"?key="+key, bytes.NewReader(body))
	if err != nil {
		return errorResult(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return contextErrorResult(ctx, err)
	}
	defer resp.Body.Close()

	// 429 is reported as its own status so a batch caller can pause + back off
	// rather than burn the rest of its run getting the same answer.
	if resp.StatusCode == http.StatusTooManyRequests {
		return domain_inspect.CheckResult{
			Status: domain_inspect.StatusRateLimited,
			Error:  "safe browsing http 429",
		}
	}
	if resp.StatusCode >= 400 {
		return domain_inspect.CheckResult{
			Status: domain_inspect.StatusError,
			Error:  fmt.Sprintf("safe browsing http %d", resp.StatusCode),
		}
	}

	var parsed sbResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return errorResult(fmt.Errorf("decode safe browsing: %w", err))
	}

	threatTypes := make([]string, 0, len(parsed.Matches))
	for _, m := range parsed.Matches {
		threatTypes = append(threatTypes, m.ThreatType)
	}

	verdict := domain_inspect.VerdictClean
	if len(parsed.Matches) > 0 {
		verdict = domain_inspect.VerdictMalicious
	}

	return domain_inspect.CheckResult{
		Status:  domain_inspect.StatusOK,
		Verdict: verdict,
		Details: map[string]any{
			"matches":      len(parsed.Matches),
			"threat_types": threatTypes,
		},
	}
}
