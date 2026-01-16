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

func LoadStevenBlack() []string {
	resp, err := http.Get("https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts")
	if err == nil {
		defer resp.Body.Close()
	}

	if err != nil {
		fmt.Println(err)
	}

	items := ParseIpHostsLine(resp.Body)
	return items
}

func ParseEasyList(r io.Reader) []string {
	scanner := bufio.NewScanner(r)
	var result []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Правила блокировки в EasyList обычно начинаются с ||
		// Мы игнорируем правила исключений (начинаются с @@)
		if strings.HasPrefix(line, "||") && !strings.HasPrefix(line, "@@") {
			// Убираем префикс '||'
			domain := line[2:]

			// Находим конец домена. Он может заканчиваться на ^, $, или /
			endPos := strings.IndexAny(domain, "^$/")
			if endPos != -1 {
				domain = domain[:endPos]
			}

			// Пропускаем записи, содержащие '*', так как это wildcard-правила,
			// которые сложно обрабатывать в простом DNS-фильтре.
			// Также проверяем, что это похоже на домен.
			if domain != "" && !strings.Contains(domain, "*") && strings.Contains(domain, ".") {
				// Добавляем точку в конце для соответствия формату DNS
				result = append(result, domain+".")
			}
		}
	}
	return result
}

func LoadEasyList() []string {
	resp, err := http.Get("https://easylist.to/easylist/easylist.txt")
	if err == nil {
		defer resp.Body.Close()
	}

	if err != nil {
		fmt.Println(err)
	}

	items := ParseEasyList(resp.Body)
	return items
}

type DomainBySource struct {
	Source  db.BlockListSource
	Domains []string
}

func LoadAllDomains() []DomainBySource {
	result := make([]DomainBySource, 0)
	partial := LoadStevenBlack()
	result = append(result, DomainBySource{
		Source:  db.SourceStevenBlack,
		Domains: partial,
	})

	partial = LoadEasyList()
	fmt.Println("Loaded EasyList domains:", len(partial))

	result = append(result, DomainBySource{
		Source:  db.SourceEasyList,
		Domains: partial,
	})

	return result
}

func Sync() error {
	l := logger.GetLogger()
	amount := db.GetAmountRecords()
	if amount == 0 {
		l.Info("No records in the database. Start loading from blocked-domain.")
		list := LoadAllDomains()
		for _, item := range list {
			err := db.CreateDNSRecordsByDomains(item.Domains, item.Source)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		l.Info(fmt.Sprintf("There are %d records in the database. Skip loading from blocked-domain.", amount))
	}
	return nil
}
