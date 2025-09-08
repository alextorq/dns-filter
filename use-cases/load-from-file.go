package use_cases

import (
	blacklists "dns-filter/black-lists"
	"dns-filter/db"
	"fmt"
)

func LoadFromFile() error {
	data, err := blacklists.LoadBlackListFromFile("./blocklist_hosts_no_ips.txt")
	if err != nil {
		return fmt.Errorf("error load black list from file: %w", err)
	}
	err = db.CreateRows(data)
	if err != nil {
		return fmt.Errorf("error create rows in db: %w", err)
	}
	return nil
}
