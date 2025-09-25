package config

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/joho/godotenv"
)

type Config struct {
	Upstream string
	DbPath   string
	Enabled  bool

	LogLevel string

	MetricEnable bool
	MetricPort   string
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

func GetConfig() *Config {
	once.Do(func() {
		if err := godotenv.Load(); err != nil {
			log.Println(err)
			log.Println("Нет .env файла, читаем только из окружения")
		}

		instance = &Config{
			Upstream: getEnv("DNS_FILTER_UPSTREAM", "8.8.8.8:53"),
			DbPath:   getEnv("DNS_FILTER_DBPATH", "./filter.sqlite"),

			MetricPort:   getEnv("DNS_FILTER_METRIC_PORT", "2112"),
			MetricEnable: getEnv("DNS_FILTER_METRIC_ENABLE", "false") == "true",

			LogLevel: getEnv("DNS_FILTER_LOG_LEVEL", ""),
			Enabled:  true,
		}
	})

	return instance
}
