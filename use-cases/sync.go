package use_cases

import (
	blacklists "dns-filter/black-lists"
	"dns-filter/db"
	"dns-filter/filter"
)

func Sync() error {
	list := blacklists.LoadAll()
	err := db.CreateRows(list)

	filter.UpdateFilter(list)
	return err
}
