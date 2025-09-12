package use_cases

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/filter"
)

func Sync() error {
	list := blacklists.LoadAll()
	err := blacklists.CreateFilter(list)

	f := filter.GetFilter()
	f.UpdateFilter(list)
	return err
}
