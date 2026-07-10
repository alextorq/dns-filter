package config

import (
	"testing"
	"time"
)

// TestGetDurationPositive guards the fix for the NewTicker panic: a zero or
// negative duration from the environment must fall back to the default rather
// than reach time.NewTicker (which panics on a non-positive interval).
func TestGetDurationPositive(t *testing.T) {
	const key = "DNS_FILTER_TEST_DURATION_POS"
	fallback := time.Hour

	cases := []struct {
		name string
		env  string
		set  bool
		want time.Duration
	}{
		{"unset falls back", "", false, fallback},
		{"empty falls back", "", true, fallback},
		{"zero falls back", "0s", true, fallback},
		{"negative falls back", "-5m", true, fallback},
		{"garbage falls back", "notaduration", true, fallback},
		{"valid positive is used", "2h", true, 2 * time.Hour},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				t.Setenv(key, c.env)
			}
			if got := getDurationPositive(key, fallback); got != c.want {
				t.Errorf("getDurationPositive(%q)=%s, want %s", c.env, got, c.want)
			}
		})
	}
}

func TestLoad_ReturnsIndependentInstancesAndReadsCurrentEnvironment(t *testing.T) {
	t.Setenv("DNS_FILTER_DBPATH", "first.sqlite")
	first := Load()

	t.Setenv("DNS_FILTER_DBPATH", "second.sqlite")
	second := Load()

	if first == second {
		t.Fatal("Load must return a fresh config instance")
	}
	if first.DbPath != "first.sqlite" {
		t.Fatalf("first DbPath = %q, want first.sqlite", first.DbPath)
	}
	if second.DbPath != "second.sqlite" {
		t.Fatalf("second DbPath = %q, want second.sqlite", second.DbPath)
	}
}
