package collect

import "strings"

// HexUUIDMinLength — минимальная длина лейбла, при которой он может быть
// признан хеш/UUID-подобным. 16 покрывает half-SHA256 / truncated-MD5
// и оставляет «обычные» 4-15-символьные лейблы вне зоны срабатывания.
const HexUUIDMinLength = 16

// HasHexUUIDLabel reports whether any non-TLD label of the domain looks
// like a hex hash or UUID — то есть состоит только из символов
// [a-f0-9-], имеет длину ≥ HexUUIDMinLength и содержит и букву (a-f),
// и цифру (0-9). Дефис разрешён внутри лейбла (для UUID-with-dashes),
// но не зачитывается ни в букву, ни в цифру.
//
// Для входов из одного лейбла (без точки) функция возвращает false:
// ожидается FQDN. Trailing dots нормализуются. Punycode (xn--) лейблы
// игнорируются: hex-вид там — артефакт кодирования.
func HasHexUUIDLabel(domain string) bool {
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
		if looksLikeHexUUID(label) {
			return true
		}
	}
	return false
}

// looksLikeHexUUID анализирует одну метку, уже приведённую к lower-case
// и не являющуюся ACE-префиксированной. Возвращает true, если метка
// проходит остальные критерии hex-uuid из godoc выше.
func looksLikeHexUUID(label string) bool {
	if len(label) < HexUUIDMinLength {
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range label {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'a' && r <= 'f':
			hasLetter = true
		case r == '-':
			// разрешён, но не учитывается в required-classes
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}
