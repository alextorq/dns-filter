package black_lists

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
)

func LoadStevenBlack(url string) map[string]bool {
	result := make(map[string]bool)

	resp, err := http.Get(url)
	defer resp.Body.Close()

	if err != nil {
		fmt.Println(err)
	}

	scanner := bufio.NewScanner(resp.Body)
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
			result[domain+"."] = true
		}
	}

	return result
}

var lists = []string{
	"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
}

func LoadAll() map[string]bool {
	result := make(map[string]bool)
	for _, url := range lists {
		partial := LoadStevenBlack(url)
		for k, v := range partial {
			result[k] = v
		}
	}
	return result
}
