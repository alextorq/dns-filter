package use_cases

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/filter"
)

func Sync() error {
	list := blacklists.LoadAll()
	err := blacklists.CreateFilter(list)

	filter.UpdateFilter(list)
	return err
}
