package settings

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The validators below are the generic, dependency-free building blocks the
// composition root reuses when declaring descriptors. Domain-specific checks
// that need another package (e.g. log-level parsing) are wired in directly at
// the composition root instead.

// ValidateHTTPURL accepts an absolute http(s) URL with a host. It rejects
// empty strings, non-http schemes and host-less URLs so a typo cannot point
// the DoH resolver at something unreachable.
func ValidateHTTPURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("not a URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL must include a host")
	}
	return nil
}

// ValidateIPList accepts a comma-separated list of IP addresses. An empty
// string is allowed (means "no bootstrap IPs"); any non-empty element must
// parse as an IP.
func ValidateIPList(raw string) error {
	for part := range strings.SplitSeq(raw, ",") {
		ip := strings.TrimSpace(part)
		if ip == "" {
			continue
		}
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("%q is not a valid IP", ip)
		}
	}
	return nil
}

// ValidateBool accepts "true"/"false" (case-insensitive), matching the
// parser used by Apply.
func ValidateBool(raw string) error {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "false":
		return nil
	default:
		return fmt.Errorf("must be true or false, got %q", raw)
	}
}

// ValidateDuration accepts a non-negative Go duration (e.g. "24h", "30s").
func ValidateDuration(raw string) error {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("not a duration: %w", err)
	}
	if d < 0 {
		return fmt.Errorf("duration must be non-negative, got %s", d)
	}
	return nil
}

// ValidatePositiveInt accepts a strictly positive integer.
func ValidatePositiveInt(raw string) error {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("not an integer: %w", err)
	}
	if n <= 0 {
		return fmt.Errorf("must be a positive integer, got %d", n)
	}
	return nil
}

// ValidateIntRange returns a validator that accepts an integer in the inclusive
// range [lo, hi]. Use it for bounded numeric settings (e.g. a retention window
// in days) where both a non-positive value and an absurdly large one are bugs.
// The bounds are named lo/hi rather than min/max so they don't shadow the
// builtin min/max functions.
func ValidateIntRange(lo, hi int) func(string) error {
	return func(raw string) error {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("not an integer: %w", err)
		}
		if n < lo || n > hi {
			return fmt.Errorf("must be between %d and %d, got %d", lo, hi, n)
		}
		return nil
	}
}

// ValidateSecret accepts only a non-empty, trimmed value of at least 8
// characters — короткие "ключи" почти всегда опечатка/обрезанный буфер обмена,
// а пустое значение через Set путает API/UI (нет способа отличить «явная
// пустота» от «сброс к env-default»). Очистка делается через DELETE
// /api/settings/:key, не через Set("").
//
// Здесь мы не пытаемся валидировать формат провайдера (VT, SB и т.п.):
// ассортимент возможных схем неустойчив; единственно полезные signal'ы — длина
// и непустота — мы и проверяем.
func ValidateSecret(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		return fmt.Errorf("значение не может быть пустым; используйте сброс, чтобы очистить")
	}
	if len(s) < 8 {
		return fmt.Errorf("значение слишком короткое (%d), ожидается ≥8 символов", len(s))
	}
	return nil
}

// ParseSecret возвращает обрезанное значение секрета. Скопированные из
// браузера/терминала ключи часто содержат хвостовые пробелы/перевод строки;
// Apply должен класть в атомик нормализованную строку.
func ParseSecret(raw string) string { return strings.TrimSpace(raw) }

// ValidateEnum returns a validator that accepts only the given values
// (case-insensitive).
func ValidateEnum(allowed ...string) func(string) error {
	return func(raw string) error {
		got := strings.ToUpper(strings.TrimSpace(raw))
		for _, a := range allowed {
			if strings.ToUpper(a) == got {
				return nil
			}
		}
		return fmt.Errorf("must be one of %s, got %q", strings.Join(allowed, ", "), raw)
	}
}

// ParseBool / ParseDuration / ParseInt mirror the validators so Apply hooks
// can convert a validated raw value without re-handling errors.
func ParseBool(raw string) bool {
	return strings.ToLower(strings.TrimSpace(raw)) == "true"
}

func ParseDuration(raw string) time.Duration {
	d, _ := time.ParseDuration(strings.TrimSpace(raw))
	return d
}

func ParseInt(raw string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	return n
}

// ParseIPList splits a comma-separated IP list into trimmed, non-empty
// elements (nil when the input is empty).
func ParseIPList(raw string) []string {
	var ips []string
	for part := range strings.SplitSeq(raw, ",") {
		ip := strings.TrimSpace(part)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}
