package use_cases

import (
	blacklists "dns-filter/black-lists"
	"dns-filter/db"
	"dns-filter/filter"
)

func Sync() error {
	list := blacklists.LoadAll()
	err := db.CreateRows(getKeys(list))

	filter.UpdateFilter(getKeys(list))
	return err
}

func getKeys(list map[string]bool) []string {
	result := make([]string, 0, len(list))
	for k := range list {
		result = append(result, k)
	}
	return result
}
