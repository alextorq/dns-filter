package db

import (
	"log"

	"github.com/alextorq/dns-filter/config"
	"gorm.io/driver/sqlite" // Sqlite driver based on CGO
	"gorm.io/gorm"
)

var conf = config.GetConfig()

func GetDBConnectionString() string {
	return conf.DbPath
}

var db *gorm.DB = nil

func GetConnection() *gorm.DB {
	if db != nil {
		return db
	}
	connect, err := gorm.Open(sqlite.Open(GetDBConnectionString()), &gorm.Config{})
	// Открываем (или создаём, если нет) файл базы данных
	db = connect
	if err != nil {
		log.Fatal(err)
	}
	return db
}
