package collect

import (
	"strings"
	"testing"
)

// hasCode reports whether reasons contains a Reason with the given code.
// Tests assert by code instead of substring search since reasons are now
// structured records, not concatenated strings.
func hasCode(reasons []Reason, code string) bool {
	for _, r := range reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

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
	if !hasCode(got.Reasons, CodeSuspiciousEntropy) {
		t.Errorf("reasons missing entropy code %q in %+v", CodeSuspiciousEntropy, got.Reasons)
	}
	if !hasCode(got.Reasons, CodeSubdomainOfBlocked) {
		t.Errorf("reasons missing subdomain code %q in %+v", CodeSubdomainOfBlocked, got.Reasons)
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

// Regression guard for the index refactor: when multiple blocked entries are
// ancestors of the same allowed domain, each match must contribute its own
// score and Reason — same as the old O(A×B) loop. Prior to the index this
// emerged for free; with the index we must walk all suffixes, not stop at
// the first hit.
func TestCollectSuggest_MultipleSubdomainAncestors_AccumulateScore(t *testing.T) {
	// Используем нейтральные labels без bad-keywords / risky-TLD / numeric-run
	// и т.п., чтобы scoring был полностью обусловлен subdomain-сигналом.
	blocked := []string{"site.org", "shop.site.org"}
	allowed := "store.shop.site.org"

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	wantScore := 2 * ItemScoreSubdomainOfBlocked
	if res[0].Score != wantScore {
		t.Fatalf("score=%d, want %d (two subdomain ancestors must accumulate)",
			res[0].Score, wantScore)
	}
	subdomainReasons := 0
	for _, r := range res[0].Reasons {
		if r.Code == CodeSubdomainOfBlocked {
			subdomainReasons++
		}
	}
	if subdomainReasons != 2 {
		t.Fatalf("expected 2 subdomain reasons, got %d (%+v)",
			subdomainReasons, res[0].Reasons)
	}
}

// Симметрично MultipleSubdomainAncestors: similar-ветка тоже должна
// эмитить score+Reason на КАЖДОЕ совпадение в бакете, а не «first match
// wins». Без этого теста перевод similar на «short-circuit» прошёл бы
// мимо CI.
func TestCollectSuggest_MultipleSimilarMatches_AccumulateScore(t *testing.T) {
	blocked := []string{
		"cdn-master.shop.example.com",
		"cdn-mester.shop.example.com",
	}
	allowed := "cdn-mister.shop.example.com"

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	similarReasons := 0
	for _, r := range res[0].Reasons {
		if r.Code == CodeSimilarToBlocked {
			similarReasons++
		}
	}
	if similarReasons != 2 {
		t.Fatalf("expected 2 similar reasons, got %d (%+v)",
			similarReasons, res[0].Reasons)
	}
}

// Inputs из miekg/dns несут trailing dot (q.Name). До нормализации:
// (а) self-match по subdomain работал, но Match-поле приходило с точкой,
// (б) similar depth-гейт ≥4 ложно триггерил на «логически-3-label»
// доменах из-за фантомного пустого segment'а. Нормализуем на входе и
// фиксируем ожидание тестом.
func TestCollectSuggest_TrailingDotNormalization(t *testing.T) {
	blocked := []string{"example.com."}
	allowed := "x8z7c4kqjfpw9.example.com."

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	if res[0].Domain != "x8z7c4kqjfpw9.example.com" {
		t.Errorf("Domain not normalized: got %q", res[0].Domain)
	}
	if !hasCode(res[0].Reasons, CodeSubdomainOfBlocked) {
		t.Errorf("expected subdomain reason, got %+v", res[0].Reasons)
	}
	for _, r := range res[0].Reasons {
		if r.Code == CodeSubdomainOfBlocked && r.Match != "example.com" {
			t.Errorf("Match not normalized: got %q", r.Match)
		}
	}
}

// Regression for the 2026-05-14 mass auto-block: a poisoned source rule
// (RuAdList "||ru^$third-party") landed bare "ru." in block_lists, and the
// next Collect() then walked subdomainAncestors("ir.ozone.ru") all the way
// up to "ru", which matched and (via ShouldAutoBlock's CodeSubdomainOfBlocked
// gate) auto-blocked 25 popular *.ru domains in one shot. PSL ancestors must
// not contribute a subdomain-of-blocked signal.
func TestCollectSuggest_PSLAncestorIsIgnored(t *testing.T) {
	cases := []struct {
		name    string
		blocked []string
		allowed string
	}{
		{"bare ICANN TLD as blocked", []string{"ru"}, "ir.ozone.ru"},
		{"bare eTLD as blocked", []string{"co.uk"}, "x.example.co.uk"},
		{"trailing-dot TLD normalised then ignored", []string{"ru."}, "ir.ozone.ru."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := CollectSuggest(tc.blocked, []string{tc.allowed})
			for _, s := range res {
				for _, r := range s.Reasons {
					if r.Code == CodeSubdomainOfBlocked {
						t.Fatalf("PSL ancestor leaked into reasons: %+v", r)
					}
				}
			}
		})
	}
}

// Counterpart to PSLAncestorIsIgnored: a real registrable parent (eTLD+1 or
// deeper) must still match. Without this we couldn't tell whether the PSL
// guard is doing its job or just disabling subdomain detection wholesale.
func TestCollectSuggest_RealParentStillMatches(t *testing.T) {
	res := CollectSuggest([]string{"ozone.ru"}, []string{"ir.ozone.ru"})
	if len(res) != 0 {
		// Score = 20 (one ancestor) is below ThresholdToSuggestBlocking=30,
		// so it shouldn't surface as a suggestion — but ShouldAutoBlock would
		// still trip on the reason. Verify via the index directly.
	}
	idx := buildBlockedIndex([]string{"ozone.ru"})
	matches := idx.subdomainAncestors("ir.ozone.ru")
	if len(matches) != 1 || matches[0] != "ozone.ru" {
		t.Fatalf("expected [ozone.ru], got %v", matches)
	}
}

// ---- ShouldAutoBlock ----

func TestShouldAutoBlock(t *testing.T) {
	cases := []struct {
		name string
		s    Suggestion
		want bool
	}{
		{
			name: "score at threshold triggers auto-block",
			s:    Suggestion{Domain: "a.example", Score: ThresholdToAutoBlock},
			want: true,
		},
		{
			name: "score above threshold triggers auto-block",
			s:    Suggestion{Domain: "a.example", Score: ThresholdToAutoBlock + 5},
			want: true,
		},
		{
			name: "subdomain-of-blocked reason triggers regardless of score",
			s: Suggestion{
				Domain:  "child.example.com",
				Score:   ThresholdToSuggestBlocking,
				Reasons: []Reason{{Code: CodeSubdomainOfBlocked, Match: "example.com"}},
			},
			want: true,
		},
		{
			name: "subdomain-of-blocked reason triggers even when below suggest threshold",
			s: Suggestion{
				Domain:  "child.example.com",
				Score:   ItemScoreSubdomainOfBlocked,
				Reasons: []Reason{{Code: CodeSubdomainOfBlocked, Match: "example.com"}},
			},
			want: true,
		},
		{
			name: "score below threshold without trigger reason stays in suggest",
			s: Suggestion{
				Domain:  "a.example",
				Score:   ThresholdToSuggestBlocking,
				Reasons: []Reason{{Code: CodeRiskyTLD}, {Code: CodeBadKeywords}},
			},
			want: false,
		},
		{
			name: "empty suggestion is not auto-blocked",
			s:    Suggestion{},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldAutoBlock(tc.s); got != tc.want {
				t.Fatalf("ShouldAutoBlock(%+v)=%v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// SimilarityAtLeast must short-circuit on length mismatch (no DL call), and
// agree with Similarity on the boundary cases. The pre-check is sound only
// if it never produces a false negative — locking that down with a test.
func TestSimilarityAtLeast(t *testing.T) {
	cases := []struct {
		name      string
		a, b      string
		threshold float64
		want      bool
	}{
		{"identical strings always pass any threshold ≤ 100", "abcd", "abcd", 100.0, true},
		{"length differs > 20% of max → can't reach 80%", "abc", "abcdefgh", 80.0, false},
		{"length differs == 20% of max → upper bound = 80, allowed", "abcd", "abcde", 80.0, true},
		{"close strings clear 80% threshold", "cdn-master", "cdn-mister", 80.0, true},
		{"clearly different strings fail 80%", "totallydifferent", "cdn1", 80.0, false},
		{"empty vs empty at threshold 100", "", "", 100.0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SimilarityAtLeast(tc.a, tc.b, tc.threshold); got != tc.want {
				t.Fatalf("SimilarityAtLeast(%q,%q,%v)=%v, want %v",
					tc.a, tc.b, tc.threshold, got, tc.want)
			}
		})
	}
}

// TestScoreCandidates_ReturnsWeakBandThatCollectSuggestDrops pins the split
// introduced for the inspect queue: ScoreCandidates returns weak candidates
// (score in [MinInspectCandidateScore, ThresholdToSuggestBlocking)) that
// CollectSuggest deliberately filters out, while the heavy scoring runs once.
func TestScoreCandidates_ReturnsWeakBandThatCollectSuggestDrops(t *testing.T) {
	// "ads" exact bad-keyword token (+5) + ".xyz" risky TLD (+5) = 10: a weak
	// candidate, below the suggest threshold but inside the inspect band.
	const weak = "ads.shop.xyz"

	scored := ScoreCandidates(nil, []string{weak})
	if len(scored) != 1 {
		t.Fatalf("expected weak domain to be scored, got %+v", scored)
	}
	if s := scored[0].Score; s < MinInspectCandidateScore || s >= ThresholdToSuggestBlocking {
		t.Fatalf("weak score %d not in inspect band [%d,%d)", s, MinInspectCandidateScore, ThresholdToSuggestBlocking)
	}

	// The same input through CollectSuggest must drop it — the UI sees only the
	// strong band.
	if got := CollectSuggest(nil, []string{weak}); len(got) != 0 {
		t.Errorf("CollectSuggest must drop the weak band, got %+v", got)
	}
}

// TestScoreCandidates_DropsZeroScore confirms domains that trigger no signal are
// absent entirely (not returned with score 0), so the inspect queue is not
// flooded with clean traffic.
func TestScoreCandidates_DropsZeroScore(t *testing.T) {
	scored := ScoreCandidates(nil, []string{"plain.example", "ads.shop.xyz"})
	for _, s := range scored {
		if s.Score == 0 {
			t.Errorf("zero-score domain leaked into ScoreCandidates: %+v", s)
		}
		if s.Domain == "plain.example" {
			t.Errorf("signalless domain plain.example must not be returned: %+v", s)
		}
	}
}

// TestLexicalCodes_NoInspectPrefix pins the load-bearing invariant that the
// reputation worker's upsert relies on: no lexical signal code may start with
// "inspect_", because UpsertWithInspect refreshes worker reasons by matching
// that prefix. A lexical code carrying the prefix would be silently wiped on
// every worker pass.
//
// We enumerate from the catalog (the canonical source of every code the system
// emits) rather than a hardcoded list — adding a new lexical code requires
// extending the catalog for its UI label anyway, so this test will see it
// automatically. The known inspect_* codes are listed below; anything else with
// the prefix is a regression.
func TestLexicalCodes_NoInspectPrefix(t *testing.T) {
	knownInspect := map[string]struct{}{
		CodeInspectVTMalicious:   {},
		CodeInspectSafeBrowsing:  {},
		CodeInspectRDAPYoung:     {},
		CodeInspectCleanEndorsed: {},
	}
	for _, d := range Catalog() {
		if _, isInspect := knownInspect[d.Code]; isInspect {
			continue
		}
		if strings.HasPrefix(d.Code, "inspect_") {
			t.Errorf("non-inspect catalog code %q must not use the reserved inspect_ prefix", d.Code)
		}
	}
}

// TestSignalCatalog_InspectCodes pins catalog completeness for the worker codes:
// all four inspect_* codes are present (so the UI has a label for every reason
// the worker can emit) and no stray inspect_*-prefixed code leaks in.
func TestSignalCatalog_InspectCodes(t *testing.T) {
	want := map[string]bool{
		CodeInspectVTMalicious:   false,
		CodeInspectSafeBrowsing:  false,
		CodeInspectRDAPYoung:     false,
		CodeInspectCleanEndorsed: false,
	}
	for _, d := range Catalog() {
		if strings.HasPrefix(d.Code, "inspect_") {
			seen, known := want[d.Code]
			if !known {
				t.Errorf("unexpected inspect_ code in catalog: %q", d.Code)
				continue
			}
			if seen {
				t.Errorf("duplicate catalog entry for %q", d.Code)
			}
			want[d.Code] = true
		}
	}
	for code, seen := range want {
		if !seen {
			t.Errorf("inspect code %q missing from catalog (UI would show no label)", code)
		}
	}
}
