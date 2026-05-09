package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

type Config struct {
	DoHUpstream     string
	DoHBootstrapIPs []string
	DbPath          string
	Enabled         bool

	LogLevel string

	MetricEnable bool
	MetricPort   string

	AdminLogin    string
	AdminPassword string
	CookieSecure  bool
	CookieSameSite string
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
			DoHUpstream:     getDoHUpstream(),
			DoHBootstrapIPs: getDoHBootstrapIPs(),
			DbPath:          getEnv("DNS_FILTER_DBPATH", "./filter.sqlite"),

			MetricPort:   getEnv("DNS_FILTER_METRIC_PORT", "2112"),
			MetricEnable: getEnv("DNS_FILTER_METRIC_ENABLE", "false") == "true",

			LogLevel: getEnv("DNS_FILTER_LOG_LEVEL", ""),
			Enabled:  true,

			AdminLogin:     os.Getenv("DNS_FILTER_ADMIN_LOGIN"),
			AdminPassword:  os.Getenv("DNS_FILTER_ADMIN_PASSWORD"),
			CookieSecure:   os.Getenv("DNS_FILTER_COOKIE_SECURE") == "true",
			CookieSameSite: getEnv("DNS_FILTER_COOKIE_SAMESITE", "Lax"),
		}
	})

	return instance
}
