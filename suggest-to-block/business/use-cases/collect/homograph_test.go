package collect

import (
	"strings"
	"testing"

	"golang.org/x/net/idna"
)

// mustToASCII encodes a Unicode label to its ACE (xn--) form via the raw
// Punycode profile, bypassing IDNA validation. Lookup/Display profiles
// reject the very inputs we want to flag (mixed-script). Only call on
// bare labels — joining is done by hand.
func mustToASCII(t *testing.T, label string) string {
	t.Helper()
	ace, err := idna.Punycode.ToASCII(label)
	if err != nil {
		t.Fatalf("encode label %q to ACE: %v", label, err)
	}
	return ace
}

// ---- HasHomographLabel: positive + negative + traps ----

func TestHasHomographLabel(t *testing.T) {
	homographSLD := mustToASCII(t, "gооgle") // Latin g/g/l/e + Cyrillic о/о
	cyrillicSLD := mustToASCII(t, "яндекс")  // Cyrillic only
	cyrillicTLD := mustToASCII(t, "рф")      // Cyrillic only
	diacriticSLD := mustToASCII(t, "bücher") // Latin only (diacritic = Latin)
	greekSLD := mustToASCII(t, "αβγδε")      // Greek only

	cases := []struct {
		name   string
		domain string
		want   bool
	}{
		// Positive: ACE label decodes to mixed Latin+Cyrillic.
		{"latin/cyrillic homograph SLD under com", homographSLD + ".com", true},
		{"upper-case ACE is normalised", strings.ToUpper(homographSLD) + ".COM", true},
		{"single trailing dot is normalised", homographSLD + ".com.", true},
		// Trap: TrimSuffix(d, ".") убрал бы только одну точку — несколько
		// точек в конце реального DNS-входа должны нормализоваться через
		// TrimRight, как в numeric-run / hex-uuid.
		{"multiple trailing dots are normalised", homographSLD + ".com..", true},
		{"homograph SLD under cyrillic TLD", homographSLD + "." + cyrillicTLD, true},

		// Negative: single-script labels.
		{"cyrillic-only SLD under cyrillic TLD", cyrillicSLD + "." + cyrillicTLD, false},
		{"cyrillic SLD under com (single-script, not homograph)", cyrillicSLD + ".com", false},
		{"latin with diacritic still single-script", diacriticSLD + ".com", false},
		{"greek-only SLD under com", greekSLD + ".com", false},

		// Negative: nothing to decode.
		{"plain ASCII domain", "example.com", false},
		{"empty input", "", false},
		{"single label has no SLD to inspect", "localhost", false},

		// Trap: signal skips TLD label так же, как numeric-run / hex-uuid.
		// Гомограф в TLD-позиции не должен срабатывать — иначе любой IDN
		// TLD без mismatch с SLD начнёт ловиться.
		{"homograph in TLD position is ignored", "example." + homographSLD, false},
		{"only TLD without SLD", cyrillicTLD, false},

		// Trap: malformed ACE — punycode error → not a homograph.
		// Нельзя ни паниковать, ни возвращать true «на всякий случай».
		{"malformed ACE label is ignored", "xn--.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasHomographLabel(tc.domain); got != tc.want {
				t.Fatalf("HasHomographLabel(%q) = %v, want %v",
					tc.domain, got, tc.want)
			}
		})
	}
}

// ---- hasMixedScripts: pure rune classification (без punycode) ----

// Property-style таблица для внутреннего хелпера. Ловит:
//   - Common-руны (digits, hyphen) НЕ должны считаться скриптом
//   - Han / Arabic / Hebrew в v1 нейтральны (out of scope)
//   - Любая комбинация двух из {Latin, Cyrillic, Greek} → true
func TestHasMixedScripts(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{"latin only", "google", false},
		{"latin with diacritic", "bücher", false},
		{"cyrillic only", "яндекс", false},
		{"greek only", "αβγδε", false},
		{"latin + cyrillic", "gооgle", true},
		{"latin + greek", "αbc", true},
		// Greek α + Cyrillic вгд — оба recognised, count=2 → true.
		{"cyrillic + greek", "αвгд", true},
		// Тройной mix: Latin + Cyrillic + Greek в одной строке. Логика
		// count > 1 покрывает и count == 3, явная фикстура фиксирует это.
		{"latin + cyrillic + greek triple", "αbcабв", true},
		{"common (digits/hyphen) is neutral", "123-456", false},
		{"latin + common is single-script", "abc-123", false},
		{"cyrillic + common is single-script", "абв-123", false},
		// Han / Arabic — currently neutral (out of scope for v1).
		{"latin + han is neutral (han ignored in v1)", "abc漢字", false},
		{"cyrillic + han is neutral (han ignored in v1)", "абв漢字", false},
		{"han only is single-script (not flagged)", "漢字测试", false},
		// Mixing two recognised scripts wins даже с han noise.
		{"latin + cyrillic with han noise still triggers", "abcабв漢字", true},
		{"empty string", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasMixedScripts(tc.s); got != tc.want {
				t.Fatalf("hasMixedScripts(%q) = %v, want %v",
					tc.s, got, tc.want)
			}
		})
	}
}

// ---- looksLikeHomograph: single-label gate ----

// Чисто-ASCII лейбл без xn-- префикса — не candidate. Это инвариант:
// non-ACE никогда не может быть mixed-script под нашим определением.
func TestLooksLikeHomograph_NonAceReturnsFalse(t *testing.T) {
	for _, label := range []string{"google", "example", "abc-def", ""} {
		if looksLikeHomograph(label) {
			t.Errorf("non-ACE label %q must not be flagged", label)
		}
	}
}

// Punycode error на «битом» ACE — это «не homograph», не «assume worst».
// И не должно паниковать. Покрытие пути «декодер вернул error».
func TestLooksLikeHomograph_PunycodeErrorReturnsFalse(t *testing.T) {
	// "xn--$$$" — Punycode профиль возвращает error «invalid label».
	if looksLikeHomograph("xn--$$$") {
		t.Errorf("decode-error ACE %q must not be flagged", "xn--$$$")
	}
}

// Покрытие отдельного пути: ACE декодируется успешно, но результат не
// содержит ни одного из recognised-скриптов. Эти кейсы не «битые», но
// точно не homograph; разделено с PunycodeErrorReturnsFalse, чтобы было
// видно — оба пути (error и success-no-mix) явно проверены.
func TestLooksLikeHomograph_DecodesToNoScriptReturnsFalse(t *testing.T) {
	// "xn--" → пустая строка (success, без рун скриптов).
	// "xn-----" → "--" (success, только Common-руны).
	for _, label := range []string{"xn--", "xn-----"} {
		if looksLikeHomograph(label) {
			t.Errorf("ACE %q decodes to no-script content and must not be flagged", label)
		}
	}
}

// Property: для набора Cyrillic-конфузаблов каждый, добавленный в начало
// Latin "google", после ACE-кодирования даёт срабатывание.
// Ловит: (1) забыли подключить unicode.Cyrillic в подсчёт, (2) счётчик
// не инкрементируется на одном новом руне, (3) Punycode payload не
// декодируется. Также косвенно проверяет round-trip Unicode↔ACE.
func TestHasHomographLabel_AnyCyrillicConfusableTriggers(t *testing.T) {
	confusables := []rune{
		'о', // U+043E, конфузабл с Latin 'o'
		'а', // U+0430, конфузабл с Latin 'a'
		'е', // U+0435, конфузабл с Latin 'e'
		'р', // U+0440, конфузабл с Latin 'p'
		'с', // U+0441, конфузабл с Latin 'c'
		'у', // U+0443, конфузабл с Latin 'y'
		'х', // U+0445, конфузабл с Latin 'x'
	}
	for _, r := range confusables {
		unicode := string(r) + "google" // 1 Cyrillic + 6 Latin
		ace := mustToASCII(t, unicode)
		domain := ace + ".com"
		if !HasHomographLabel(domain) {
			t.Errorf(
				"confusable %q (U+%04X) prepended to 'google' must trigger; ACE=%q",
				string(r), r, domain,
			)
		}
	}
}

// Property: чисто-Latin метка после форсированного ACE-кодирования (если
// Punycode-профиль вообще такое произведёт) не должна триггерить — это
// страховка от того, что мы не путаем «есть xn-- префикс» с «есть mixed».
func TestHasHomographLabel_LatinOnlyNeverTriggers(t *testing.T) {
	for _, s := range []string{"hello", "example", "longwordnodiacritics"} {
		ace, err := idna.Punycode.ToASCII(s)
		if err != nil {
			t.Fatalf("encode %q: %v", s, err)
		}
		domain := ace + ".com"
		if HasHomographLabel(domain) {
			t.Errorf("latin-only label %q (ACE %q) must not trigger", s, ace)
		}
	}
}

// ---- Интеграция с CollectSuggest ----

// Один сигнал homograph (+10) сильно ниже порога 30.
func TestCollectSuggest_OnlyHomograph_NotSuggested(t *testing.T) {
	domain := mustToASCII(t, "gооgle") + ".com"
	res := CollectSuggest(nil, []string{domain})
	if len(res) != 0 {
		t.Fatalf("expected no suggestions for homograph-only domain %q, got %+v",
			domain, res)
	}
}

// Homograph + risky-TLD = 10+5 = 15 — всё ещё под порогом. Тест явно
// фиксирует, что одного homograph + одного weak-сигнала мало для suggest.
func TestCollectSuggest_HomographPlusRiskyTLD_NotSuggested(t *testing.T) {
	domain := mustToASCII(t, "gооgle") + ".tk"
	res := CollectSuggest(nil, []string{domain})
	if len(res) != 0 {
		t.Fatalf("homograph + risky-TLD should remain under threshold, got %+v", res)
	}
}

// Регрессия `=` vs `+=` на трёх сигналах: точная сумма ловит баг,
// при котором последняя сработавшая ветка перезаписывала бы общий score.
//
// Сигналы (см. collect.go):
//   - homograph(+10) на ACE-лейбле SLD
//   - numeric-run(+5) на label с 7+ цифрами
//   - subdomain-of-blocked(+20) — apex example.com явно в blocked
//
// Сумма 35 ≥ 30 → suggest. Reason должен содержать константу homograph.
func TestCollectSuggest_HomographAccumulatesWithNumericRunAndSubdomain(t *testing.T) {
	homographLabel := mustToASCII(t, "gооgle")
	domain := homographLabel + ".id1234567.example.com"
	blocked := []string{"example.com"}

	res := CollectSuggest(blocked, []string{domain})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	want := ItemScoreHomograph +
		ItemScoreNumericRun +
		ItemScoreSubdomainOfBlocked
	if res[0].Score != want {
		t.Fatalf(
			"score = %d, want %d (homograph+numeric-run+subdomain accumulated)",
			res[0].Score, want,
		)
	}
	if !hasCode(res[0].Reasons, CodeHomograph) {
		t.Errorf("reasons missing homograph code %q in %+v",
			CodeHomograph, res[0].Reasons)
	}
	if !hasCode(res[0].Reasons, CodeNumericRun) {
		t.Errorf("reasons missing numeric-run code %q in %+v",
			CodeNumericRun, res[0].Reasons)
	}
}
