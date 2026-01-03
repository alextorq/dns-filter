package seed

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
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

func LoadStevenBlack(url string) []string {
	resp, err := http.Get(url)
	defer resp.Body.Close()

	if err != nil {
		fmt.Println(err)
	}

	items := ParseIpHostsLine(resp.Body)
	return items
}

func LoadAllDomains() []string {
	result := make([]string, 0)
	var lists = []string{
		"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
	}
	for _, url := range lists {
		partial := LoadStevenBlack(url)
		result = append(result, partial...)
	}
	return result
}

func Sync() error {
	l := logger.GetLogger()
	amount := db.GetAmountRecords()
	if amount == 0 {
		list := LoadAllDomains()
		err := db.CreateDNSRecordsByDomains(list)
		return err
	} else {
		l.Info(fmt.Sprintf("There are %d records in the database. Skip loading from blocked-domain.", amount))
	}
	return nil
}
