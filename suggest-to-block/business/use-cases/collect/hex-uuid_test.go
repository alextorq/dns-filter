package collect

import (
	"strings"
	"testing"
)

// ---- HasHexUUIDLabel: позитив + негатив + ловушки ----

func TestHasHexUUIDLabel(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		want   bool
	}{
		// Позитив: типичные хеш/UUID паттерны.
		{"32-char UUID without dashes", "f47ac10b58cc4372a5670e02b2c3d479.example.com", true},
		{"36-char UUID with dashes", "f47ac10b-58cc-4372-a567-0e02b2c3d479.example.com", true},
		{"36-char UUID with dashes in upper case", "F47AC10B-58CC-4372-A567-0E02B2C3D479.example.com", true},
		{"16-char half-SHA256 prefix", "e3b0c44298fc1c14.example.com", true},
		{"upper-case hex is normalised", "F47AC10B58CC4372A5670E02B2C3D479.example.com", true},
		{"hex label sits in second-level position", "front.f47ac10b58cc4372a5670e02b2c3d479.com", true},
		{"exact length boundary 16 with both classes", "abcdef0123456789.example.com", true},

		// Негатив: формально хекс-алфавит, но без обоих классов.
		{"all hex letters, no digit", "abcdefabcdefabcd.example.com", false},
		{"all digits, no hex letter", "1111111111111111.example.com", false},
		{"length one below threshold", "abc1234567abcde.example.com", false},

		// Негатив: чужие символы ломают признак.
		{"non-hex letter inside otherwise-hex label", "f47ac10bz58cc4372.example.com", false},
		{"punycode label is skipped", "xn--f47ac10b58cc4372.example.com", false},

		// Негатив: структурные крайние случаи.
		{"empty input", "", false},
		{"single label has no SLD to inspect", "f47ac10b58cc4372a5670e02b2c3d479", false},
		{"clean domain with no hex-looking labels", "example.com", false},
		{"trailing dot is normalised", "f47ac10b58cc4372a5670e02b2c3d479.example.com.", true},

		// Trap: 16-char лейбл из одних дефисов — ни буквы, ни цифры.
		{"all-dashes label of valid length is rejected", "----------------.example.com", false},

		// Trap: hex run в TLD-лейбле игнорируется.
		{"hex-shaped TLD-position is ignored", "example.f47ac10b58cc4372a5670e02b2c3d479", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasHexUUIDLabel(tc.domain); got != tc.want {
				t.Fatalf("HasHexUUIDLabel(%q) = %v, want %v", tc.domain, got, tc.want)
			}
		})
	}
}

// Property-style: при фиксированной форме лейбла "a" + "0" * (N-1)
// (одна hex-буква + длинная цепочка нулей) функция возвращает true тогда
// и только тогда, когда длина лейбла ≥ HexUUIDMinLength. Ловит off-by-one
// и перепутанные операторы.
func TestHasHexUUIDLabel_LengthBoundary(t *testing.T) {
	if HexUUIDMinLength < 1 {
		t.Fatalf("invariant: HexUUIDMinLength must be >= 1, got %d", HexUUIDMinLength)
	}
	for n := HexUUIDMinLength - 4; n <= HexUUIDMinLength+4; n++ {
		if n < 2 {
			continue
		}
		label := "a" + strings.Repeat("0", n-1)
		domain := label + ".example.com"
		want := n >= HexUUIDMinLength
		if got := HasHexUUIDLabel(domain); got != want {
			t.Errorf("length %d: HasHexUUIDLabel(%q) = %v, want %v",
				n, domain, got, want)
		}
	}
}

// Property-style: для известного валидного hex-лейбла каждое одиночное
// замещение символа на не-hex (`g`) должно сломать признак. Это охота
// на «забыли проверить i-ю позицию» / неполный обход в импле.
func TestHasHexUUIDLabel_AnyNonHexCharBreaksDetection(t *testing.T) {
	const base = "abcdef0123456789" // 16 chars, валидный по всем критериям
	if !HasHexUUIDLabel(base + ".example.com") {
		t.Fatalf("base fixture %q must be detected", base)
	}
	for i := 0; i < len(base); i++ {
		broken := []byte(base)
		broken[i] = 'g' // 'g' — первая буква вне hex-алфавита
		domain := string(broken) + ".example.com"
		if HasHexUUIDLabel(domain) {
			t.Errorf("substitution at index %d should break detection: %q",
				i, string(broken))
		}
	}
}

// Property-style: требование «есть и буква, и цифра» — критическое.
// Проверяем, что замена ВСЕХ цифр на hex-буквы (или наоборот) ломает
// признак, даже когда длина и алфавит в порядке.
func TestHasHexUUIDLabel_RequiresBothClasses(t *testing.T) {
	allLetters := strings.Repeat("abcdef", 3) // 18 chars, all hex letters
	allDigits := strings.Repeat("0123456789", 2)[:18]
	mixed := "abcdef0123456789ab" // 18 chars, both

	if HasHexUUIDLabel(allLetters + ".example.com") {
		t.Errorf("all-letters hex label must not be detected: %q", allLetters)
	}
	if HasHexUUIDLabel(allDigits + ".example.com") {
		t.Errorf("all-digits hex label must not be detected: %q", allDigits)
	}
	if !HasHexUUIDLabel(mixed + ".example.com") {
		t.Errorf("mixed hex label must be detected: %q", mixed)
	}
}

// ---- Интеграция с CollectSuggest ----

// Один сигнал hex-uuid (+10) не должен пересекать порог 30.
// Также проверяем, что у типичного UUID не срабатывает энтропия,
// потому что иначе тест поймал бы +30 из (entropy + hex-uuid).
func TestCollectSuggest_OnlyHexUUID_NotSuggested(t *testing.T) {
	const allowed = "f47ac10b58cc4372a5670e02b2c3d479.example.com"
	res := CollectSuggest(nil, []string{allowed})
	if len(res) != 0 {
		t.Fatalf("expected no suggestions for hex-uuid-only domain, got %+v", res)
	}
}

// Hex-UUID + subdomain заблокированного → 10+20=30 = порог → suggest.
func TestCollectSuggest_HexUUIDPlusSubdomain_Suggested(t *testing.T) {
	const allowed = "f47ac10b58cc4372a5670e02b2c3d479.example.com"
	blocked := []string{"example.com"}

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	want := ItemScoreHexUUID + ItemScoreSubdomainOfBlocked
	if res[0].Score != want {
		t.Fatalf("score = %d, want %d", res[0].Score, want)
	}
	if !strings.Contains(res[0].Reason, ReasonHexUUIDLabel) {
		t.Errorf("reason missing hex-uuid hint %q in %q",
			ReasonHexUUIDLabel, res[0].Reason)
	}
}

// Регрессия на «накопление сработало правильно» для hex-uuid ветки.
// Используем UUID-лейбл В ПАРЕ с явно высокоэнтропийным консонантным
// лейблом. UUID сам по себе энтропию не триггерит (entropy ≈ 3.80,
// consonant ratio ≈ 0.73 — оба ниже порогов в shannon.go); это косвенно
// подтверждается TestCollectSuggest_OnlyHexUUID_NotSuggested. Поэтому
// +20 ниже приходит от второго лейбла, а не от UUID.
// Итог: entropy(+20) + hex-uuid(+10) + subdomain(+20) = 50.
func TestCollectSuggest_HexUUIDAccumulatesWithEntropyAndSubdomain(t *testing.T) {
	const allowed = "f47ac10b58cc4372a5670e02b2c3d479.lzkdngfvtcwspbqxhrjm.com"
	blocked := []string{"lzkdngfvtcwspbqxhrjm.com"}

	res := CollectSuggest(blocked, []string{allowed})
	if len(res) != 1 {
		t.Fatalf("expected 1 suggestion, got %d (%+v)", len(res), res)
	}
	want := ItemScoreSuspiciousDomain +
		ItemScoreHexUUID +
		ItemScoreSubdomainOfBlocked
	if res[0].Score != want {
		t.Fatalf("score = %d, want %d (entropy+hex-uuid+subdomain accumulated)",
			res[0].Score, want)
	}
}
