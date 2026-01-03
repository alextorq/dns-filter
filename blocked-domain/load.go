package blocked_domain

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

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

func ParseGuardFile(r io.Reader) []string {
	scanner := bufio.NewScanner(r)
	var result []string

	for scanner.Scan() {
		line := scanner.Text()

		// Пропускаем комментарии и пустые строки
		if strings.HasPrefix(line, "!") || strings.TrimSpace(line) == "" {
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
	//https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt
}

func LoadStevenBlack(url string) []string {
	resp, err := http.Get(url)
	defer resp.Body.Close()

	if err != nil {
		fmt.Println(err)
	}

	items := ParseIpHostsLine(resp.Body)
	return items
}

var lists = []string{
	"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
}

func loadAll() []string {
	result := make([]string, 0)
	for _, url := range lists {
		partial := LoadStevenBlack(url)
		result = append(result, partial...)
	}
	return result
}

func LoadBlackListFromFile(path string) ([]string, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, 0644)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	data := ParseIpHostsLine(file)
	return data, err
}
