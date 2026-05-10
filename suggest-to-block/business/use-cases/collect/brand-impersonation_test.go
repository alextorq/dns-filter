package collect

import (
	"sort"
	"strings"
	"testing"
)

// ---- IsBrandImpersonation: positive + negative + edge + traps ----

func TestIsBrandImpersonation(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		want   bool
	}{
		// --- Positive: typical typosquat substitutions on apex ---
		{"google with digit one for l", "goog1e.com", true},
		{"paypal with digit one for l", "paypa1.com", true},
		{"microsoft with digit one for i", "m1crosoft.com", true},
		{"amazon with rn replacing m", "arnazon.com", true},
		{"sberbank with c for a", "sberbcnk.ru", true},
		{"tinkoff missing letter", "tinkof.ru", true},
		{"gosuslugi missing letter", "gosuslgi.ru", true},
		{"wildberries missing letter", "wildberris.ru", true},

		// --- Negative: not similar enough to any brand ---
		{"unrelated startup", "cool-app.com", false},
		{"random io domain", "randomstartup.io", false},

		// --- Negative: equality with a legit brand ---
		// Trap: similarity = 100% with brand X, но identity не typosquat.
		{"google equality", "google.com", false},
		{"sberbank equality", "sberbank.ru", false},
		{"yandex equality", "yandex.ru", false},

		// --- Negative: subdomain of legit brand → apex equals brand ---
		// Trap: «mail.google.com содержит google.com», на naive substring-check
		// мог бы сработать. extractApex даёт apex google.com → equality → false.
		{"subdomain of google", "mail.google.com", false},
		{"subdomain of sberbank", "online.sberbank.ru", false},

		// --- Negative: single-label / empty / TLD-only ---
		{"single label has no apex", "localhost", false},
		{"empty input", "", false},
		{"only a dot", ".", false},

		// --- Negative: short-apex FP guard (MinBrandImpersonationLength) ---
		// Trap: vc.com (6 рун) vs vk.com (5 рун) — distance 1, similarity ≈83%
		// без gate. На таких длинах процентное сходство теряет различимость и
		// любой случайный 5-6-рунный домен начинает совпадать с коротким
		// брендом. Min-length cuts отсекает класс целиком.
		{"short apex vc.com near vk.com is filtered out", "vc.com", false},
		{"short apex s8.ru near s7.ru is filtered out", "s8.ru", false},
		{"short apex pk.com near vk.com is filtered out", "pk.com", false},

		// --- Edge: non-ASCII skip (homograph → Task 4) ---
		// Trap: на rune-уровне distance(g_оо_gle, google)=2, similarity=80%
		// на 10 рунах — мог бы сработать как brand-impersonation. Non-ASCII
		// skip явно отсекает Cyrillic/Greek/Han homograph: их обрабатывает
		// отдельный сигнал (Task 4), здесь не дублируем.
		{"cyrillic homograph not handled here", "gооgle.com", false},

		// --- Edge: punycode skip (homograph → Task 4) ---
		// Trap: ACE-encoded форма (xn--) внешне состоит из ASCII и могла бы
		// случайно совпасть с брендом по символам после декодирования.
		{"punycode label is skipped", "xn--ggle-jum.com", false},

		// --- Edge: trailing dots normalised (один и несколько) ---
		// Trap: TrimSuffix(d, ".") убрал бы только одну точку — `goog1e.com..`
		// прошёл бы дальше с лишним пустым лейблом и сломал extractApex.
		// Используется TrimRight, поэтому оба варианта корректны.
		{"single trailing dot", "goog1e.com.", true},
		{"multiple trailing dots", "goog1e.com..", true},

		// --- Edge: case-insensitive ---
		{"upper-case typosquat", "GooG1e.COM", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBrandImpersonation(tc.domain); got != tc.want {
				t.Fatalf("IsBrandImpersonation(%q) = %v, want %v",
					tc.domain, got, tc.want)
			}
		})
	}
}

// Property: ни один зарегистрированный бренд не должен флагаться как
// impersonation сам себя. Ловит дрейф между списком и функцией —
// например, опечатка в ключе KnownBrands, из-за которой equality-check
// промахнётся и легитимный бренд внезапно начнёт совпадать «по similarity»
// с другим брендом из набора.
func TestIsBrandImpersonation_AllBrandsAreLegit(t *testing.T) {
	if len(KnownBrands) == 0 {
		t.Fatal("KnownBrands must be non-empty")
	}
	for brand := range KnownBrands {
		if IsBrandImpersonation(brand) {
			t.Errorf("known brand %q should not be flagged as impersonation", brand)
		}
	}
}

// knownBrandCollisions — пары брендов в KnownBrands, у которых попарная
// Similarity ≥ BrandSimilarityThreshold. Сегодня коллизия не вызывает FP:
// если оба бренда присутствуют в KnownBrands, equality short-circuit
// в IsBrandImpersonation срабатывает раньше, чем similarity-цикл. Но
// опечатка в одном ключе (`githhub.com` вместо `github.com`) ломает эту
// защиту — equality промахнётся, и legit `github.com` будет помечен как
// impersonation своей же опечатки. Допустимые коллизии перечислены
// явно ниже; добавление новой требует ревью списка брендов.
var knownBrandCollisions = map[[2]string]struct{}{
	// shopify.com vs spotify.com — distance 2 (h↔p, p↔t), similarity 81.82%.
	{"shopify.com", "spotify.com"}: {},
	// github.com vs gitlab.com — distance 2 (h↔l, u↔a + transposition), 80.00%.
	{"github.com", "gitlab.com"}: {},
}

// Brand-vs-brand коллизии не должны возникать незаметно. Тест проходит
// все пары KnownBrands, отсекает короткие (gate из IsBrandImpersonation
// их runtime'ом тоже не сравнивает) и фейлит на любой паре ≥ threshold,
// которой нет в knownBrandCollisions. См. комментарий выше — runtime
// safe только пока ключи корректны.
func TestKnownBrands_NoUndocumentedCollisions(t *testing.T) {
	brands := make([]string, 0, len(KnownBrands))
	for b := range KnownBrands {
		brands = append(brands, b)
	}
	sort.Strings(brands)

	for i := 0; i < len(brands); i++ {
		if len(brands[i]) < MinBrandImpersonationLength {
			continue
		}
		for j := i + 1; j < len(brands); j++ {
			if len(brands[j]) < MinBrandImpersonationLength {
				continue
			}
			sim := Similarity(brands[i], brands[j])
			if sim < BrandSimilarityThreshold {
				continue
			}
			pair := [2]string{brands[i], brands[j]}
			if _, ok := knownBrandCollisions[pair]; ok {
				continue
			}
			t.Errorf(
				"brands %q and %q are %.2f%% similar; runtime FP guard relies on equality short-circuit and breaks if a key is mistyped. Pick distinct names or add to knownBrandCollisions with rationale.",
				brands[i], brands[j], sim,
			)
		}
	}
}

// Hygiene: ключи KnownBrands должны быть нормализованы под формат, который
// возвращает extractApex (lower-case, без trailing dots, ровно 2 лейбла,
// без whitespace). Опечатка в данных не покажется на runtime — equality
// просто промахнётся, и легитимный бренд начнёт триггерить impersonation.
// Поэтому проверяем форму ключей здесь.
func TestKnownBrands_KeysAreNormalised(t *testing.T) {
	for brand := range KnownBrands {
		if brand == "" {
			t.Errorf("empty brand key in KnownBrands")
			continue
		}
		if brand != strings.ToLower(brand) {
			t.Errorf("brand %q must be lower-case", brand)
		}
		if strings.HasPrefix(brand, ".") || strings.HasSuffix(brand, ".") {
			t.Errorf("brand %q must not start or end with a dot", brand)
		}
		if strings.ContainsAny(brand, " \t") {
			t.Errorf("brand %q must not contain whitespace", brand)
		}
		if parts := strings.Split(brand, "."); len(parts) != 2 {
			t.Errorf("brand %q must have exactly 2 labels (eTLD+1), got %d",
				brand, len(parts))
		}
	}
}

// ---- extractApex ----

func TestExtractApex(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain apex", "google.com", "google.com"},
		{"two-label subdomain", "mail.google.com", "google.com"},
		{"deep subdomain", "a.b.c.example.org", "example.org"},
		{"upper-cased and trailing dot", "GOOGLE.COM.", "google.com"},
		{"single label has no apex", "localhost", ""},
		{"empty input", "", ""},
		{"only a dot", ".", ""},
		{"multiple trailing dots normalised", "google.com..", "google.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractApex(tc.in); got != tc.want {
				t.Fatalf("extractApex(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ---- Интеграция с CollectSuggest ----

// +25 в одиночку не пересекает порог 30. Это и есть «typosquat требует
// подтверждения вторым слабым сигналом» — сознательная страховка от FP.
// Чистый typosquat без других признаков suggest'ом не становится.
func TestCollectSuggest_OnlyBrandImpersonation_NotSuggested(t *testing.T) {
	res := CollectSuggest(nil, []string{"paypa1.com"})
	if len(res) != 0 {
		t.Fatalf("expected no suggestions for typosquat-only domain, got %+v", res)
	}
}

// Brand-impersonation добивает домен до порога вместе с другими сигналами.
// Здесь: subdomain-of-blocked (+20) + brand-impersonation (+25) = 45.
// Сравниваем с точной суммой через константы — формулировки и веса можно
// будет менять, тест не сломается. Reason должен явно упоминать brand-сигнал.
func TestCollectSuggest_BrandImpersonationPlusSubdomain(t *testing.T) {
	const allowed = "sub.paypa1.com"
	blocked := []string{"paypa1.com"}

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	got := res[0]
	if got.Domain != allowed {
		t.Fatalf("unexpected domain %q", got.Domain)
	}
	wantScore := ItemScoreSubdomainOfBlocked + ItemScoreBrandImpersonation
	if got.Score != wantScore {
		t.Fatalf("score = %d, want %d (subdomain+brand accumulated)",
			got.Score, wantScore)
	}
	if got.Score < ThresholdToSuggestBlocking {
		t.Fatalf("score %d should clear threshold %d",
			got.Score, ThresholdToSuggestBlocking)
	}
	if !strings.Contains(got.Reason, ReasonBrandImpersonation) {
		t.Errorf("reason missing brand hint %q in %q",
			ReasonBrandImpersonation, got.Reason)
	}
	if !strings.Contains(got.Reason, ReasonSubdomainOfBlocked) {
		t.Errorf("reason missing subdomain hint %q in %q",
			ReasonSubdomainOfBlocked, got.Reason)
	}
}

// Регрессия `=` vs `+=` на трёх сигналах. Точная сумма ловит классический
// баг (`Score = ItemScoreX` вместо `Score += ItemScoreX`), при котором
// последняя сработавшая ветка перезаписывала бы итоговый score.
// Сигналы: subdomain (+20) + brand-impersonation (+25) + numeric-run (+5) = 50.
func TestCollectSuggest_BrandImpersonationAccumulatesWithSubdomainAndNumericRun(t *testing.T) {
	const allowed = "sub1234567.goog1e.com"
	blocked := []string{"goog1e.com"}

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	want := ItemScoreSubdomainOfBlocked +
		ItemScoreBrandImpersonation +
		ItemScoreNumericRun
	if res[0].Score != want {
		t.Fatalf("score = %d, want %d (subdomain+brand+numeric accumulated)",
			res[0].Score, want)
	}
	if !strings.Contains(res[0].Reason, ReasonBrandImpersonation) {
		t.Errorf("reason missing brand hint %q in %q",
			ReasonBrandImpersonation, res[0].Reason)
	}
	if !strings.Contains(res[0].Reason, ReasonNumericRun) {
		t.Errorf("reason missing numeric-run hint %q in %q",
			ReasonNumericRun, res[0].Reason)
	}
}
