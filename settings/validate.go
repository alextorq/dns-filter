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
	for _, part := range strings.Split(raw, ",") {
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
// range [min, max]. Use it for bounded numeric settings (e.g. a retention window
// in days) where both a non-positive value and an absurdly large one are bugs.
func ValidateIntRange(min, max int) func(string) error {
	return func(raw string) error {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("not an integer: %w", err)
		}
		if n < min || n > max {
			return fmt.Errorf("must be between %d and %d, got %d", min, max, n)
		}
		return nil
	}
}

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
	for _, part := range strings.Split(raw, ",") {
		ip := strings.TrimSpace(part)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}
