package collect

import (
	"strings"
	"testing"
)

// ---- CheckItIsSubDomain ----

func TestCheckItIsSubDomain(t *testing.T) {
	cases := []struct {
		name   string
		parent string
		child  string
		want   bool
	}{
		{"identical domains count as match", "google.com", "google.com", true},
		{"direct subdomain", "google.com", "ads.google.com", true},
		{"deep subdomain", "google.com", "x.y.google.com", true},
		{"child shorter than parent", "google.com", "com", false},
		{"suffix without dot boundary is not a subdomain", "google.com", "agoogle.com", false},
		{"unrelated tail", "google.com", "yandex.ru", false},
		{"reversed (parent is the deeper one)", "ads.google.com", "google.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckItIsSubDomain(tc.parent, tc.child); got != tc.want {
				t.Fatalf("CheckItIsSubDomain(%q,%q)=%v, want %v",
					tc.parent, tc.child, got, tc.want)
			}
		})
	}
}

// ---- CheckForBadKeywords ----

func TestCheckForBadKeywords(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		want   bool
	}{
		{"exact ad token as label", "ad.example.com", true},
		{"ads as middle hyphenated token", "my-ads-server.com", true},
		{"upload contains ad as substring but is not tokenised", "upload.com", false},
		{"adsystem matches as a whole token", "adsystem.example.com", true},
		{"tracker label", "tracker.example.com", true},
		{"clean domain", "example.com", false},
		{"upper-cased label still matches", "AD.example.com", true},
		{"pixel label", "pixel.beacon.io", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckForBadKeywords(tc.domain); got != tc.want {
				t.Fatalf("CheckForBadKeywords(%q)=%v, want %v",
					tc.domain, got, tc.want)
			}
		})
	}
}

// ---- IsDomainSuspicious (Shannon entropy + consonant ratio) ----

func TestIsDomainSuspicious(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		want   bool
	}{
		{"popular english domain", "google.com", false},
		{"popular domain with subdomain", "mail.google.com", false},
		{"facebook label is well-formed", "facebook.com", false},
		{"trailing dot is normalised", "google.com.", false},
		{"single label is treated as normal", "localhost", false},
		{"short label is skipped from analysis", "abc.com", false},
		{"random-looking hash label triggers detector", "x8z7c4kqjfpw9.example.com", true},
		{"long all-consonants label triggers detector", "lzkdngfvtcwspbqxhrjm.example.com", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDomainSuspicious(tc.domain); got != tc.want {
				t.Fatalf("IsDomainSuspicious(%q)=%v, want %v",
					tc.domain, got, tc.want)
			}
		})
	}
}

// ---- DamerauLevenshtein / Similarity ----

func TestDamerauLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"ab", "ba", 1},     // single transposition is one operation, not two
		{"abcd", "acbd", 1}, // adjacent transposition
	}
	for _, tc := range cases {
		if got := DamerauLevenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("DamerauLevenshtein(%q,%q)=%d, want %d",
				tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	if got := Similarity("", ""); got != 100.0 {
		t.Errorf(`Similarity("","")=%v, want 100.0`, got)
	}
	if got := Similarity("foo", "foo"); got != 100.0 {
		t.Errorf("Similarity identical strings = %v, want 100.0", got)
	}
	// distance 1 over max-len 6 → (1 - 1/6) * 100 = 83.33
	if got := Similarity("cdn123", "cdn124"); got <= 80.0 || got >= 90.0 {
		t.Errorf("Similarity(cdn123, cdn124)=%v, want in (80, 90)", got)
	}
	// distance 1 over max-len 4 → 75
	if got := Similarity("cdn1", "cdn2"); got >= 80.0 {
		t.Errorf("Similarity(cdn1, cdn2)=%v, want < 80", got)
	}
}

// ---- CheckIfBlockSameDomainLevelAndHaveSameBlockedDomain ----

func TestCheckIfBlockSameDomainLevel(t *testing.T) {
	cases := []struct {
		name    string
		blocked string
		allowed string
		want    bool
	}{
		{
			name:    "fewer than 4 parts is rejected",
			blocked: "ads.example.com",
			allowed: "ad.example.com",
			want:    false,
		},
		{
			name:    "different number of parts",
			blocked: "a.b.c.example.com",
			allowed: "b.c.example.com",
			want:    false,
		},
		{
			name:    "different tails do not match",
			blocked: "cdn1.ads.evil.com",
			allowed: "cdn1.ads.good.com",
			want:    false,
		},
		{
			name:    "first label too far apart",
			blocked: "cdn1.ads.example.com",
			allowed: "totallydifferent.ads.example.com",
			want:    false,
		},
		{
			name:    "first labels similar enough (one char diff in long label)",
			blocked: "cdn-master.ads.example.com",
			allowed: "cdn-mister.ads.example.com",
			want:    true,
		},
		{
			name:    "short labels with one char diff fall under 80% similarity",
			blocked: "cdn1.ads.example.com",
			allowed: "cdn2.ads.example.com",
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckIfBlockSameDomainLevelAndHaveSameBlockedDomain(tc.blocked, tc.allowed)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- CollectSuggest (orchestrator) ----

func TestCollectSuggest_EmptyInput(t *testing.T) {
	if got := CollectSuggest(nil, nil); len(got) != 0 {
		t.Fatalf("expected empty result, got %+v", got)
	}
	if got := CollectSuggest([]string{"example.com"}, nil); len(got) != 0 {
		t.Fatalf("expected empty result with no allowed domains, got %+v", got)
	}
}

// Plain domains with no suspicious signals must not be suggested.
func TestCollectSuggest_NoSignals_NotSuggested(t *testing.T) {
	res := CollectSuggest([]string{"blocked.com"}, []string{"plain.example"})
	if len(res) != 0 {
		t.Fatalf("expected no suggestions, got %+v", res)
	}
}

// Anything below ThresholdToSuggestBlocking must be filtered out. The largest
// single-signal score is 20 (entropy or subdomain), so a domain with only one
// signal cannot reach the threshold.
func TestCollectSuggest_BelowThreshold_NotSuggested(t *testing.T) {
	cases := []struct {
		name    string
		blocked []string
		allowed string
	}{
		{
			name:    "only bad keyword (+5)",
			blocked: nil,
			allowed: "tracker.legit-site.com",
		},
		{
			name:    "only suspicious entropy (+20)",
			blocked: nil,
			allowed: "lzkdngfvtcwspbqxhrjm.example.com",
		},
		{
			name:    "only subdomain of blocked (+20)",
			blocked: []string{"example.com"},
			allowed: "child.example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := CollectSuggest(tc.blocked, []string{tc.allowed})
			if len(res) != 0 {
				t.Fatalf("expected no suggestions (score < %d), got %+v",
					ThresholdToSuggestBlocking, res)
			}
		})
	}
}

// Regression test for the `Score = +ItemScoreSubdomainOfBlocked` bug
// (was overwriting accumulated score instead of adding to it). With the bug,
// the entropy bonus would be erased once the subdomain match fires and the
// final score would stay at 20, below the threshold of 30 — so the domain
// would never be suggested. With the fix, score = 20 + 20 = 40 and the
// domain is suggested.
func TestCollectSuggest_AccumulatesEntropyAndSubdomain(t *testing.T) {
	const allowed = "x8z7c4kqjfpw9.example.com"
	res := CollectSuggest([]string{"example.com"}, []string{allowed})

	if len(res) != 1 {
		t.Fatalf("expected exactly 1 suggestion, got %d (%+v)", len(res), res)
	}
	got := res[0]
	if got.Domain != allowed {
		t.Fatalf("unexpected domain %q", got.Domain)
	}

	wantScore := ItemScoreSuspiciousDomain + ItemScoreSubdomainOfBlocked
	if got.Score != wantScore {
		t.Fatalf("score=%d, want %d (entropy+subdomain accumulated)",
			got.Score, wantScore)
	}
	if got.Score < ThresholdToSuggestBlocking {
		t.Fatalf("score %d should clear threshold %d",
			got.Score, ThresholdToSuggestBlocking)
	}
	if !strings.Contains(got.Reason, "suspicious") {
		t.Errorf("reason missing entropy hint: %q", got.Reason)
	}
	if !strings.Contains(got.Reason, "subdomain of blocked") {
		t.Errorf("reason missing subdomain hint: %q", got.Reason)
	}
}

// Two weaker signals (subdomain + similar) should also clear the threshold.
func TestCollectSuggest_SubdomainPlusSimilar(t *testing.T) {
	blocked := []string{
		"cdn-master.ads.example.com", // for similarity match
		"example.com",                // for subdomain match
	}
	allowed := "cdn-mister.ads.example.com"

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	if res[0].Score < ThresholdToSuggestBlocking {
		t.Fatalf("score=%d below threshold %d", res[0].Score, ThresholdToSuggestBlocking)
	}
}
