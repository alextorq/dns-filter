package sync

import (
	"bufio"
	"io"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

const (
	StevenBlackURL  = "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
	HaGeZiMultiURL  = "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/hosts/multi.txt"
)

func LoadStevenBlack() ([]string, error) {
	return LoadHostsFromURL(StevenBlackURL)
}

func LoadHostsFromURL(url string) ([]string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ParseIpHostsLine(resp.Body), nil
}

func ParseIpHostsLine(r io.Reader) []string {
	scanner := bufio.NewScanner(r)
	var result []string
	for scanner.Scan() {
		line := scanner.Text()

		// Пропускаем комментарии и пустые строки
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Формат строки: "0.0.0.0 domain.com"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			domain := strings.ToLower(parts[1])
			// В DNS запросах домены обычно с точкой в конце: "domain.com."
			result = append(result, domain+".")
		}
	}
	return result
}
