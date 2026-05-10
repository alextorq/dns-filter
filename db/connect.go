package db

import (
	"log"
	"sync"

	"github.com/alextorq/dns-filter/config"
	"github.com/glebarez/sqlite" // Pure-Go SQLite driver (modernc.org/sqlite)
	"gorm.io/gorm"
)

var conf = config.GetConfig()

func GetDBConnectionString() string {
	return conf.DbPath
}

var (
	db   *gorm.DB
	once sync.Once
)

func GetConnection() *gorm.DB {
	once.Do(func() {
		var err error
		db, err = gorm.Open(sqlite.Open(GetDBConnectionString()), &gorm.Config{})
		if err != nil {
			log.Fatal(err)
		}

		// PRAGMA для bulk-операций: WAL даёт конкурентное чтение во время записи,
		// synchronous=NORMAL убирает fsync на каждом коммите (durable до OS-краша),
		// cache_size=-64000 = 64 МБ страничного кэша.
		pragmas := []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA synchronous=NORMAL",
			"PRAGMA temp_store=MEMORY",
			"PRAGMA cache_size=-64000",
		}
		for _, p := range pragmas {
			if err := db.Exec(p).Error; err != nil {
				log.Fatal(err)
			}
		}
	})
	return db
}
