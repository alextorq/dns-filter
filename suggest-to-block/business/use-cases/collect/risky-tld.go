package collect

import "strings"

// RiskyTLDs — TLD с подтверждённой высокой долей злоупотреблений
// (фишинг, malware, спам). Все ключи — lower-case, без точек и пробелов:
// именно в таком виде их ищет IsRiskyTLD.
var RiskyTLDs = map[string]struct{}{
	"tk":      {},
	"ml":      {},
	"cf":      {},
	"ga":      {},
	"gq":      {},
	"xyz":     {},
	"top":     {},
	"click":   {},
	"work":    {},
	"quest":   {},
	"surf":    {},
	"men":     {},
	"country": {},
	"stream":  {},
	"cyou":    {},
}

// IsRiskyTLD reports whether the domain ends in a TLD listed in RiskyTLDs.
// Сравнение case-insensitive; trailing dots (один или несколько) нормализуются.
// TLD извлекается как подстрока после последней точки — для целевых TLD
// (single-label gTLD) этого достаточно.
func IsRiskyTLD(domain string) bool {
	d := strings.TrimRight(strings.ToLower(domain), ".")
	if d == "" {
		return false
	}
	idx := strings.LastIndex(d, ".")
	if idx < 0 {
		return false
	}
	_, ok := RiskyTLDs[d[idx+1:]]
	return ok
}
