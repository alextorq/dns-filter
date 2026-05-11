package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"

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

	VirusTotalKey string
	URLScanKey    string
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

			VirusTotalKey: os.Getenv("DNS_FILTER_VT_KEY"),
			URLScanKey:    os.Getenv("DNS_FILTER_URLSCAN_KEY"),
		}
		instance.Enabled.Store(true)
	})

	return instance
}
