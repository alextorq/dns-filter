package use_cases

import (
	"dns-filter/db"
	"dns-filter/filter"
)

func GetFromDb() error {
	list, err := db.GetAllRowsWhereActive()

	filter.UpdateFilter(list)
	return err
}
