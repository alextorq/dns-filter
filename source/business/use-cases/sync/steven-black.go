package sync

import (
	"bufio"
	"io"
	"net/http"
	"strings"
)

func LoadStevenBlack() ([]string, error) {
	resp, err := http.Get("https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	items := ParseIpHostsLine(resp.Body)
	return items, nil
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
