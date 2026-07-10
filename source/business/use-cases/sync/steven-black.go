package sync

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	easy_list "github.com/alextorq/dns-filter/source/business/use-cases/sync/easy-list"
	"github.com/alextorq/dns-filter/utils"
)

const (
	StevenBlackURL = "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
	HaGeZiMultiURL = "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/hosts/multi.txt"
)

// HostsLoader downloads and parses one configured hosts-format source.
type HostsLoader struct {
	client HTTPDoer
	url    string
}

func NewHostsLoader(client HTTPDoer, url string) *HostsLoader {
	return &HostsLoader{client: client, url: url}
}

func (l *HostsLoader) Load(ctx context.Context) ([]string, error) {
	if isNilHTTPDoer(l.client) {
		return nil, fmt.Errorf("hosts loader: HTTP client is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	domains, err := parseIPHostsLine(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return domains, nil
}

func ParseIpHostsLine(r io.Reader) []string {
	domains, _ := parseIPHostsLine(r)
	return domains
}

// parseIPHostsLine is the loader-facing parser. It reports scanner failures so
// a canceled/truncated response is incomplete rather than a successful partial
// list that could trigger destructive pruning.
func parseIPHostsLine(r io.Reader) ([]string, error) {
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
			// Та же PSL-защита, что и в EasyList-парсере: запись вроде
			// "0.0.0.0 ru" положила бы голый TLD в блок-лист и спровоцировала
			// массовый auto-block через subdomainAncestors.
			if !easy_list.IsSafeDNSDomain(domain) {
				continue
			}
			// Каноническая FQDN-форма — единая для всего блок-листа (#30).
			result = append(result, utils.CanonicalDomain(domain))
		}
	}
	return result, scanner.Err()
}
