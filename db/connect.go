package db

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/alextorq/dns-filter/config"
	"github.com/glebarez/sqlite" // Pure-Go SQLite driver (modernc.org/sqlite)
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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
		// GORM по умолчанию логирует SQL целиком при превышении SlowThreshold
		// (200ms). Bulk-инсерты в source.Sync() — десятки тысяч строк за раз,
		// каждый запрос подходит под порог и пишет в stdout 100+ КБ VALUES.
		// Поднимаем порог до 5 сек (реальные тормоза всё ещё ловим) и просим
		// логгер использовать `?` вместо инлайн-значений — slow-warn остаётся
		// диагностически полезным, но не флудит.
		gormLog := gormlogger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			gormlogger.Config{
				SlowThreshold:             5 * time.Second,
				LogLevel:                  gormlogger.Warn,
				IgnoreRecordNotFoundError: true,
				ParameterizedQueries:      true,
				Colorful:                  true,
			},
		)
		db, err = gorm.Open(sqlite.Open(GetDBConnectionString()), &gorm.Config{
			Logger: gormLog,
		})
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
