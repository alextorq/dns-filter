package use_cases

import (
	"fmt"

	blacklists "github.com/alextorq/dns-filter/black-lists"
)

func LoadFromFile() error {
	data, err := blacklists.LoadBlackListFromFile("./blocklist_hosts_no_ips.txt")
	if err != nil {
		return fmt.Errorf("error load black list from file: %w", err)
	}
	err = blacklists.CreateDNSRecordsByDomains(data)
	if err != nil {
		return fmt.Errorf("error create rows in db: %w", err)
	}
	return nil
}
