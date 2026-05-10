package collect

import (
	"strings"
	"testing"
)

// ---- IsRiskyTLD: позитив + негатив + охота на типовые баги ----

func TestIsRiskyTLD(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		want   bool
	}{
		{"direct match on .tk", "example.tk", true},
		{"legit .com is not risky", "example.com", false},
		{"deep subdomain under risky TLD", "a.b.c.example.xyz", true},
		{"upper-case TLD", "EXAMPLE.TK", true},
		{"mixed-case across labels", "ExAmPlE.Cf", true},
		{"trailing dot is normalised", "example.tk.", true},
		{"multiple trailing dots are normalised", "example.tk..", true},
		{"risky TLD label in the middle is ignored", "tk.example.com", false},
		// Trap: naive HasSuffix(d, "tk") would falsely match here.
		{"label ending in risky TLD without dot boundary", "attack.com", false},
		// Trap: partial-prefix lookup would falsely match here.
		{"partial suffix is not a real TLD", "example.tkk", false},
		{"single label has no TLD", "localhost", false},
		{"empty input", "", false},
		{"only the TLD without any label", "tk", false},
		{"another risky TLD broadens coverage", "evil.click", true},
		{"hyphen in label does not interfere", "free-vpn-now.work", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRiskyTLD(tc.domain); got != tc.want {
				t.Fatalf("IsRiskyTLD(%q) = %v, want %v", tc.domain, got, tc.want)
			}
		})
	}
}

// Property-style: every TLD declared in RiskyTLDs must be picked up by
// IsRiskyTLD. Catches drift between the declared list and the function
// (typos, accidental upper-case keys, stray spaces).
func TestIsRiskyTLD_AllListedTLDsAreDetected(t *testing.T) {
	if len(RiskyTLDs) == 0 {
		t.Fatal("RiskyTLDs must be non-empty")
	}
	for tld := range RiskyTLDs {
		domain := "example." + tld
		if !IsRiskyTLD(domain) {
			t.Errorf("declared risky TLD %q not detected via %q", tld, domain)
		}
	}
}

// Hygiene: keys in RiskyTLDs must already be lower-case and contain no dots
// or whitespace — otherwise the lookup in IsRiskyTLD will silently miss them.
func TestRiskyTLDs_KeysAreNormalised(t *testing.T) {
	for tld := range RiskyTLDs {
		if tld == "" {
			t.Errorf("empty TLD key in RiskyTLDs")
		}
		if tld != strings.ToLower(tld) {
			t.Errorf("TLD %q must be lower-case", tld)
		}
		if strings.ContainsAny(tld, ". \t") {
			t.Errorf("TLD %q must not contain dots or whitespace", tld)
		}
	}
}

// ---- Интеграция с CollectSuggest ----

// +5 в одиночку никогда не пересекает порог 30, так что чистый домен
// на рисковом TLD не должен попадать в suggest.
func TestCollectSuggest_OnlyRiskyTLD_NotSuggested(t *testing.T) {
	res := CollectSuggest(nil, []string{"legit.tk"})
	if len(res) != 0 {
		t.Fatalf("expected no suggestions for risky-TLD-only domain, got %+v", res)
	}
}

// Контроль того, что мы не сделали из слабого сигнала авто-блок:
// пользовательский стартап на .xyz без других признаков — пропуск.
func TestCollectSuggest_CleanDomainOnRiskyTLD_NotSuggested(t *testing.T) {
	res := CollectSuggest(nil, []string{"my-startup.xyz"})
	if len(res) != 0 {
		t.Fatalf("expected no suggestions, got %+v", res)
	}
}

// Risky-TLD должен **именно вместе** с другими сигналами добивать
// домен до порога. Здесь: entropy(+20) + bad-keyword(+5) + risky-TLD(+5) = 30,
// что ровно равно ThresholdToSuggestBlocking и должно пройти.
func TestCollectSuggest_RiskyTLDPushesOverThreshold(t *testing.T) {
	// x8z7c4kqjfpw9 → подозрительная энтропия (все согласные + цифры)
	// tracker       → bad keyword
	// .tk           → risky TLD
	allowed := "x8z7c4kqjfpw9.tracker.tk"
	res := CollectSuggest(nil, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	got := res[0]
	if got.Domain != allowed {
		t.Fatalf("unexpected domain %q", got.Domain)
	}
	wantScore := ItemScoreSuspiciousDomain + ItemScoreContainsBadKeywords + ItemScoreRiskyTLD
	if got.Score != wantScore {
		t.Fatalf("score = %d, want %d", got.Score, wantScore)
	}
	if got.Score < ThresholdToSuggestBlocking {
		t.Fatalf("score %d should clear threshold %d",
			got.Score, ThresholdToSuggestBlocking)
	}
	// Reasons должны явно содержать risky-TLD код, иначе модератор не
	// поймёт, почему домен попал в список.
	if !hasCode(got.Reasons, CodeRiskyTLD) {
		t.Errorf("reasons missing TLD code %q in %+v", CodeRiskyTLD, got.Reasons)
	}
}

// Регрессия на «накопление сработало правильно» — ловит случай, когда
// risky-TLD ветка по ошибке использует `=` вместо `+=` (как было
// исправлено в subdomain-ветке). Тут собираем 4 положительных сигнала
// и убеждаемся, что итог — точная сумма всех четырёх.
func TestCollectSuggest_RiskyTLDAccumulatesWithEntropyAndSubdomain(t *testing.T) {
	const allowed = "x8z7c4kqjfpw9.example.tk"
	blocked := []string{"example.tk"}

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	want := ItemScoreSuspiciousDomain +
		ItemScoreSubdomainOfBlocked +
		ItemScoreRiskyTLD
	if res[0].Score != want {
		t.Fatalf("score = %d, want %d (entropy+subdomain+TLD accumulated)",
			res[0].Score, want)
	}
}
