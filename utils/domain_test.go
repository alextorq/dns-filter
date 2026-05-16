package utils

import "testing"

func TestCanonicalDomain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Позитивные случаи.
		{"already canonical", "example.com.", "example.com."},
		{"no trailing dot", "example.com", "example.com."},
		{"mixed case", "Example.COM", "example.com."},
		{"surrounding spaces", "  example.com  ", "example.com."},
		{"leading dot", ".example.com", "example.com."},
		{"subdomain", "Ads.Tracker.Example.COM", "ads.tracker.example.com."},

		// Негативные / краевые случаи.
		{"empty", "", ""},
		{"spaces only", "   ", ""},
		{"dots only", "...", ""},
		{"single dot", ".", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalDomain(tt.input); got != tt.expected {
				t.Errorf("CanonicalDomain(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestCanonicalDomainIdempotent — повторный вызов на уже нормализованном
// значении не должен ничего менять: функцию применяют и на записи, и на чтении.
func TestCanonicalDomainIdempotent(t *testing.T) {
	for _, in := range []string{"example.com", "Example.COM ", ".sub.example.com"} {
		once := CanonicalDomain(in)
		twice := CanonicalDomain(once)
		if once != twice {
			t.Errorf("CanonicalDomain not idempotent for %q: %q vs %q", in, once, twice)
		}
	}
}
