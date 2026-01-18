package seed

import (
	syncDb "github.com/alextorq/dns-filter/source/db"
	"gorm.io/gorm"
)

func SeedSyncs(db *gorm.DB) {
	// Список того, что хотим добавить
	defaults := []syncDb.Source{
		{Name: syncDb.SourceStevenBlack, Active: true},
		{Name: syncDb.SourceEasyList, Active: true},
		{Name: syncDb.SourceUser, Active: true},
		{Name: syncDb.SourceSuggestedToBlock, Active: true},
	}

	for _, item := range defaults {
		// FirstOrCreate делает следующее:
		// 1. Ищет запись по условию во втором аргументе (syncDb.Source{Name: item.Name})
		// 2. Если нашел -> ничего не делает (записывает найденное в &item)
		// 3. Если не нашел -> создает запись с данными из item
		err := db.FirstOrCreate(&item, syncDb.Source{Name: item.Name}).Error

		if err != nil {
			// Можно просто залогировать ошибку, паниковать необязательно
			println("Ошибка при создании дефолтной записи:", item.Name, err.Error())
		}
	}
}
