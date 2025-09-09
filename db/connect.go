package db

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/alextorq/dns-filter/config"
	_ "github.com/mattn/go-sqlite3" // драйвер SQLite для Go
)

var conf = config.GetConfig()

const tableName = "black_list"

func GetDBConnectionString() string {
	return conf.DbPath
}

var db *sql.DB = nil

func GetConnection() *sql.DB {
	if db != nil {
		return db
	}
	// Открываем (или создаём, если нет) файл базы данных
	connect, err := sql.Open("sqlite3", GetDBConnectionString())
	db = connect
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func init() {
	connect := GetConnection()
	// Создаём таблицу filters, если её нет
	sqlStmt := fmt.Sprintf(`
    CREATE TABLE IF NOT EXISTS %s (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        url varchar(255) UNIQUE,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        active boolean DEFAULT true
    );
`, tableName)
	_, err := connect.Exec(sqlStmt)
	if err != nil {
		log.Fatalf("%q: %s\n", err, sqlStmt)
	}
}

func CreateRows(urls []string) error {
	connect := GetConnection()
	// Готовим SQL-запрос
	stmt := fmt.Sprintf(`INSERT OR IGNORE INTO %s (url) VALUES (?)`, tableName)

	// Используем prepared statement
	prep, err := connect.Prepare(stmt)
	if err != nil {
		return err
	}
	defer prep.Close()

	// Вставляем все URL-ы
	for _, u := range urls {
		_, err := prep.Exec(u)
		if err != nil {
			return fmt.Errorf("failed to insert %s: %w", u, err)
		}
	}
	return nil
}

func GetAllRowsWhereActive() ([]string, error) {
	query := fmt.Sprintf(`SELECT url FROM %s WHERE active = true`, tableName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return urls, nil
}
