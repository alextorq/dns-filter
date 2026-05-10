package collect

import (
	"strings"
	"testing"
)

// ---- HasNumericRun: позитив + негатив + охота на типовые баги ----

func TestHasNumericRun(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		want   bool
	}{
		// Позитив: запуски ≥ MinNumericRunLength.
		{"exact threshold run", "id1234567.example.com", true},
		{"long pure-numeric label", "1234567890.example.com", true},
		{"run in the middle of a label", "idxx1234567backup.example.com", true},
		{"run sits inside hyphenated label", "track-1234567.example.com", true},
		{"run lives in second-level subdomain, not first", "front.0987654321.example.com", true},

		// Негатив: ниже порога или прерывается.
		{"one digit short of threshold", "id123456.example.com", false},
		{"year-style 4-digit run", "s3-bucket-2023.example.com", false},
		{"version+year combo with separators", "route53-2024-zone.example.com", false},
		{"phone-formatted with hyphen separators", "123-456-7890.example.com", false},
		{"hyphen interrupts the run", "id12345-6789.example.com", false},
		{"alphanumeric mix without long run", "id-1a2b3c4d.example.com", false},

		// Структурные крайние случаи.
		{"no digits anywhere", "example.com", false},
		{"empty input", "", false},
		{"single label has no SLD to inspect", "localhost", false},
		{"single label of pure digits", "1234567", false},
		{"trailing dot is normalised", "track-1234567.example.com.", true},
		{"multiple trailing dots are normalised", "track-1234567.example.com..", true},

		// Trap: запуск находится в TLD-лейбле — мы его игнорируем.
		// Реальных gTLD из 7+ цифр не существует, но проверяем явно.
		{"digit run in TLD position is ignored", "example.1234567", false},

		// Trap: punycode (ACE) лейбл — цифры в "xn--..." это артефакт
		// кодирования IDN, а не семантический ID. Пропускаем такой лейбл,
		// чтобы не получать FP от обычных интернационализованных доменов.
		{"punycode label is skipped", "xn--1234567abc.example.com", false},
		{"punycode in upper case is also skipped", "XN--1234567abc.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasNumericRun(tc.domain); got != tc.want {
				t.Fatalf("HasNumericRun(%q) = %v, want %v", tc.domain, got, tc.want)
			}
		})
	}
}

// Property-style: для строки из ровно N цифр подряд (плюс безопасный
// non-digit "x." впереди и ".com" в конце) функция возвращает true тогда
// и только тогда, когда N >= MinNumericRunLength. Ловит off-by-one и
// спутанные сравнения (>, >=, <, <=).
func TestHasNumericRun_RunLengthBoundary(t *testing.T) {
	if MinNumericRunLength < 1 {
		t.Fatalf("invariant: MinNumericRunLength must be >= 1, got %d", MinNumericRunLength)
	}
	for n := 0; n <= MinNumericRunLength+3; n++ {
		domain := "x" + strings.Repeat("1", n) + ".com"
		want := n >= MinNumericRunLength
		if got := HasNumericRun(domain); got != want {
			t.Errorf("run length %d: HasNumericRun(%q) = %v, want %v",
				n, domain, got, want)
		}
	}
}

// Property-style: цифры могут встречаться в нескольких отдельных
// сегментах, но если ни один сегмент не дотягивает до порога — false.
// Это охота на потенциальный баг «считаем все цифры, не различая
// разрывы» (т.е. ratio вместо run).
func TestHasNumericRun_FragmentedDigitsDoNotAccumulate(t *testing.T) {
	// 20 цифр всего, разбитых на 4 сегмента по 5 — самый длинный
	// сегмент короче порога, домен не должен флагаться.
	domain := "12345-12345-12345-67890.example.com"
	if HasNumericRun(domain) {
		t.Fatalf("HasNumericRun(%q) = true, but no segment reaches %d",
			domain, MinNumericRunLength)
	}
}

// ---- Интеграция с CollectSuggest ----

// Один сигнал numeric-run (+5) не должен пересекать порог 30.
func TestCollectSuggest_OnlyNumericRun_NotSuggested(t *testing.T) {
	res := CollectSuggest(nil, []string{"id1234567.example.com"})
	if len(res) != 0 {
		t.Fatalf("expected no suggestions for numeric-run-only domain, got %+v", res)
	}
}

// Чистый домен на легитимном TLD с year-маркером (4-run) не должен
// провоцировать numeric-run сигнал. Граничный кейс из реальных AWS-стилей.
func TestCollectSuggest_BucketStyleNameWithYear_NotSuggested(t *testing.T) {
	res := CollectSuggest(nil, []string{"s3-bucket-2023.example.com"})
	if len(res) != 0 {
		t.Fatalf("expected no suggestions for bucket-style name, got %+v", res)
	}
}

// Numeric-run должен суммироваться с другими сигналами и преодолевать порог.
// Цель: entropy(+20) + bad-keyword(+5) + numeric-run(+5) = 30 = порог.
func TestCollectSuggest_NumericRunPushesOverThreshold(t *testing.T) {
	// tracker-1234567 → numeric run (+5) и bad keyword "tracker" (+5)
	// lzkdngfvtcwspbqxhrjm → suspicious entropy (+20)
	allowed := "tracker-1234567.lzkdngfvtcwspbqxhrjm.com"
	res := CollectSuggest(nil, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	got := res[0]
	wantScore := ItemScoreSuspiciousDomain + ItemScoreContainsBadKeywords + ItemScoreNumericRun
	if got.Score != wantScore {
		t.Fatalf("score = %d, want %d", got.Score, wantScore)
	}
	if !hasCode(got.Reasons, CodeNumericRun) {
		t.Errorf("reasons missing numeric-run code %q in %+v",
			CodeNumericRun, got.Reasons)
	}
}

// Регрессия на «накопление сработало правильно». Если в новой ветке
// numeric-run кто-то по ошибке использует `=` вместо `+=`, эта сборка
// уйдёт в обмеление и тест упадёт.
func TestCollectSuggest_NumericRunAccumulatesWithEntropyAndSubdomain(t *testing.T) {
	const allowed = "x8z7c4kqjfpw9-1234567.example.com"
	blocked := []string{"example.com"}

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	want := ItemScoreSuspiciousDomain +
		ItemScoreNumericRun +
		ItemScoreSubdomainOfBlocked
	if res[0].Score != want {
		t.Fatalf("score = %d, want %d (entropy+numeric-run+subdomain accumulated)",
			res[0].Score, want)
	}
}
