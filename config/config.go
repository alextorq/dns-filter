package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
)

// Mode selects which deployment profile the server runs as.
//
// "lan" is the historical mode: bind UDP+TCP on :53, identify clients by their
// remote address (IP or, after PR3, MAC via an ARP cache), and accept that the
// service is only reachable on the local broadcast domain.
//
// "public" is reserved for a future DoH-over-HTTPS frontend that identifies
// clients by an opaque token in the request URL. The Mode flag exists today so
// the wiring in main.go can branch on it without further refactoring later.
type Mode string

const (
	ModeLAN    Mode = "lan"
	ModePublic Mode = "public"
)

type Config struct {
	Mode            Mode
	DoHUpstream     string
	DoHBootstrapIPs []string
	DbPath          string
	Enabled         atomic.Bool
	// PausedUntilUnix holds the unix-second deadline of a temporary pause.
	// 0 means no pause; any value <= time.Now().Unix() is treated as expired.
	PausedUntilUnix atomic.Int64

	LogLevel string

	MetricEnable bool
	MetricPort   string

	AdminLogin     string
	AdminPassword  string
	CookieSecure   bool
	CookieSameSite string

	VirusTotalKey   string
	URLScanKey      string
	SafeBrowsingKey string

	// CacheSWR toggles proactive stale-while-revalidate: on a stale-window hit
	// (TTL expired but still inside CacheStaleGrace) the cached response is
	// returned immediately and a background refresh is fired. When false the
	// resolver behaves exactly like before SWR — stale hits fall through to a
	// synchronous upstream lookup. Serve-stale-on-error is independent of this
	// flag and is always on as long as CacheStaleGrace > 0.
	CacheSWR bool
	// CacheStaleGrace is how long past expiresAt a positive cache entry is
	// still allowed to be served (as stale). Negative responses (NXDOMAIN,
	// NODATA) never go into stale-window — staleUntil is forced to expiresAt
	// for them so a misbehaving zone cannot pin "does not exist" past TTL.
	CacheStaleGrace time.Duration
	// CacheStaleTTL is the TTL written to RRs in a stale response handed back
	// to the client. RFC 8767 §6 recommends a small value (≤ 30s) so the
	// client comes back quickly enough for our async refresh to have landed.
	CacheStaleTTL time.Duration
	// CacheRefreshConcurrency caps how many background refresh goroutines may
	// run at once. When the semaphore is full additional refresh attempts are
	// dropped (counted in metrics) and stale is still served — the next stale
	// hit will try again.
	CacheRefreshConcurrency int

	// TrafficRetentionDays is how many days of per-device traffic counters
	// (domain_traffic) are kept. It is the env/compiled default for the
	// traffic_retention_days dynamic setting; a DB override (set from the UI)
	// takes precedence at runtime. The daily prune deletes day-buckets older
	// than this window.
	TrafficRetentionDays int
}

func (c *Config) UpdateLogLevel(l string) {
	c.LogLevel = l
}

var (
	instance *Config
	once     sync.Once
)

// getEnv возвращает значение переменной или дефолт
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	fmt.Println("Используется значение по умолчанию для", key, ":", fallback)
	return fallback
}

func getDoHUpstream() string {
	if value := os.Getenv("DNS_FILTER_DOH_UPSTREAM"); value != "" {
		return value
	}

	if legacy := os.Getenv("DNS_FILTER_UPSTREAM"); strings.HasPrefix(legacy, "https://") || strings.HasPrefix(legacy, "http://") {
		return legacy
	}

	return getEnv("DNS_FILTER_DOH_UPSTREAM", "https://cloudflare-dns.com/dns-query")
}

// getMode returns the deployment mode parsed from DNS_FILTER_MODE. Unknown
// values fall back to ModeLAN with a log message — the wrong default here is
// loud (queries would refuse to resolve) so a typo should not silently switch
// the server into public mode.
func getMode() Mode {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("DNS_FILTER_MODE")))
	switch Mode(raw) {
	case ModeLAN, ModePublic:
		return Mode(raw)
	case "":
		return ModeLAN
	default:
		log.Printf("DNS_FILTER_MODE=%q is not recognized, falling back to %q", raw, ModeLAN)
		return ModeLAN
	}
}

// getBool parses a bool env var with a default. Accepts "true"/"false"
// case-insensitively; anything else logs and falls back.
func getBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "":
		fmt.Println("Используется значение по умолчанию для", key, ":", fallback)
		return fallback
	case "true":
		return true
	case "false":
		return false
	default:
		log.Printf("%s=%q is not a bool, falling back to %v", key, raw, fallback)
		return fallback
	}
}

// getDuration parses a Go duration (e.g. "24h", "30s") with a default.
func getDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		fmt.Println("Используется значение по умолчанию для", key, ":", fallback)
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("%s=%q is not a duration, falling back to %s", key, raw, fallback)
		return fallback
	}
	return d
}

// getInt parses a positive int env var with a default. Non-positive or
// non-numeric values fall back so a typo cannot e.g. disable the refresh pool.
func getInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		fmt.Println("Используется значение по умолчанию для", key, ":", fallback)
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		log.Printf("%s=%q is not a positive int, falling back to %d", key, raw, fallback)
		return fallback
	}
	return n
}

func getDoHBootstrapIPs() []string {
	value := os.Getenv("DNS_FILTER_DOH_BOOTSTRAP_IPS")
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	ips := make([]string, 0, len(parts))
	for _, part := range parts {
		ip := strings.TrimSpace(part)
		if ip != "" {
			ips = append(ips, ip)
		}
	}

	return ips
}

func GetConfig() *Config {
	once.Do(func() {
		if err := godotenv.Load(); err != nil {
			log.Println(err)
			log.Println("Нет .env файла, читаем только из окружения")
		}

		instance = &Config{
			Mode:            getMode(),
			DoHUpstream:     getDoHUpstream(),
			DoHBootstrapIPs: getDoHBootstrapIPs(),
			DbPath:          getEnv("DNS_FILTER_DBPATH", "./filter.sqlite"),

			MetricPort:   getEnv("DNS_FILTER_METRIC_PORT", "2112"),
			MetricEnable: getEnv("DNS_FILTER_METRIC_ENABLE", "false") == "true",

			LogLevel: getEnv("DNS_FILTER_LOG_LEVEL", ""),

			AdminLogin:     os.Getenv("DNS_FILTER_ADMIN_LOGIN"),
			AdminPassword:  os.Getenv("DNS_FILTER_ADMIN_PASSWORD"),
			CookieSecure:   os.Getenv("DNS_FILTER_COOKIE_SECURE") == "true",
			CookieSameSite: getEnv("DNS_FILTER_COOKIE_SAMESITE", "Lax"),

			VirusTotalKey:   os.Getenv("DNS_FILTER_VT_KEY"),
			URLScanKey:      os.Getenv("DNS_FILTER_URLSCAN_KEY"),
			SafeBrowsingKey: os.Getenv("DNS_FILTER_SAFE_BROWSING_KEY"),

			CacheSWR:                getBool("DNS_FILTER_CACHE_SWR", true),
			CacheStaleGrace:         getDuration("DNS_FILTER_CACHE_STALE_GRACE", 24*time.Hour),
			CacheStaleTTL:           getDuration("DNS_FILTER_CACHE_STALE_TTL", 30*time.Second),
			CacheRefreshConcurrency: getInt("DNS_FILTER_CACHE_REFRESH_CONCURRENCY", 32),
			TrafficRetentionDays:    getInt("DNS_FILTER_TRAFFIC_RETENTION_DAYS", 30),
		}
		instance.Enabled.Store(true)
	})

	return instance
}
