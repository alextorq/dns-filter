package collect

import "strings"

// MinNumericRunLength — минимальная длина последовательности подряд идущих
// ASCII-цифр в одном лейбле, при которой домен считается подозрительным.
// Год (4) и обычные version-маркеры (≤6) не должны срабатывать; типичные
// tracking-ID и timestamp-ы (7+) — должны.
const MinNumericRunLength = 7

// HasNumericRun reports whether any non-TLD label of the domain contains
// at least MinNumericRunLength consecutive ASCII digit characters.
//
// Trailing dots (одна или несколько) нормализуются. TLD (последний лейбл)
// пропускается — как и в IsRiskyTLD / IsDomainSuspicious. Для входов из
// одного лейбла (без точки) функция возвращает false: ожидается FQDN.
//
// Punycode (ACE) лейблы с префиксом "xn--" игнорируются: цифры там —
// артефакт кодирования и не имеют семантического значения.
func HasNumericRun(domain string) bool {
	d := strings.TrimRight(strings.ToLower(domain), ".")
	if d == "" {
		return false
	}
	parts := strings.Split(d, ".")
	if len(parts) < 2 {
		return false
	}
	for _, label := range parts[:len(parts)-1] {
		if strings.HasPrefix(label, "xn--") {
			continue
		}
		if longestDigitRun(label) >= MinNumericRunLength {
			return true
		}
	}
	return false
}

// longestDigitRun возвращает длину наибольшей подстроки из подряд идущих
// ASCII-цифр в s. Любой не-цифровой символ (включая дефис) обрывает run.
func longestDigitRun(s string) int {
	longest, cur := 0, 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			cur++
			if cur > longest {
				longest = cur
			}
		} else {
			cur = 0
		}
	}
	return longest
}
