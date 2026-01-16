package db

import (
	"log"
	"sync"

	"github.com/alextorq/dns-filter/config"
	"gorm.io/driver/sqlite" // Sqlite driver based on CGO
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
	})
	return db
}
