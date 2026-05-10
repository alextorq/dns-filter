package collect

import (
	"strings"
	"unicode"

	"golang.org/x/net/idna"
)

// HasHomographLabel reports whether any non-TLD label of domain decodes
// (after Punycode) to a string that contains characters from more than
// one of the recognised alphabetic scripts: Latin, Cyrillic, Greek.
//
// Классический фишинг-кейс — `xn--ggle-55da.com`, который декодируется в
// `gооgle.com`: Latin g/g/l/e смешан с Cyrillic о/о, визуально неотличим
// от `google.com`.
//
// Trailing dots нормализуются, регистр приводится к lower. Single-label
// вход возвращает false (ожидаем FQDN). Чисто-ASCII лейблы (без `xn--`
// префикса) не могут быть mixed-script и пропускаются. Малформенный ACE
// → false (трактуем как «не triggered»).
func HasHomographLabel(domain string) bool {
	d := strings.TrimRight(strings.ToLower(domain), ".")
	if d == "" {
		return false
	}
	parts := strings.Split(d, ".")
	if len(parts) < 2 {
		return false
	}
	for _, label := range parts[:len(parts)-1] {
		if looksLikeHomograph(label) {
			return true
		}
	}
	return false
}

// looksLikeHomograph анализирует одну метку (lower-case) и возвращает
// true только если это ACE-кодированный лейбл, декодированная Unicode-форма
// которого содержит руны из более чем одного из recognised-скриптов.
//
// Non-ACE лейблы возвращают false: только Punycode-форма может нести
// non-Latin руны в нашем wire-формате. Малформенный ACE → false.
func looksLikeHomograph(label string) bool {
	if !strings.HasPrefix(label, "xn--") {
		return false
	}
	decoded, err := idna.Punycode.ToUnicode(label)
	if err != nil {
		return false
	}
	return hasMixedScripts(decoded)
}

// hasMixedScripts reports whether s contains characters from more than
// one of recognised alphabetic scripts (Latin, Cyrillic, Greek).
//
// `unicode.Common` (digits, hyphen, ZWJ, etc.) — нейтральны. Прочие
// скрипты (Han, Arabic, Hebrew, ...) тоже нейтральны в v1: они out of
// scope, их присутствие ни добавляет к detection, ни блокирует
// обнаружение Latin/Cyrillic/Greek-пары в той же строке.
func hasMixedScripts(s string) bool {
	var hasLatin, hasCyrillic, hasGreek bool
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Latin, r):
			hasLatin = true
		case unicode.Is(unicode.Cyrillic, r):
			hasCyrillic = true
		case unicode.Is(unicode.Greek, r):
			hasGreek = true
		}
	}
	count := 0
	if hasLatin {
		count++
	}
	if hasCyrillic {
		count++
	}
	if hasGreek {
		count++
	}
	return count > 1
}
