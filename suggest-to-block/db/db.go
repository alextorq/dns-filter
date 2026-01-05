package db

import "github.com/alextorq/dns-filter/db"

type SuggestBlock struct {
	ID     uint   `gorm:"primarykey" json:"id"`
	Domain string `json:"domain" gorm:"uniqueIndex"`
	Score  int    `json:"score"`
	Reason string `json:"reasons"`
}

func CreateSuggestBlock(domain string, reason string) error {
	conn := db.GetConnection()
	suggest := SuggestBlock{
		Domain: domain,
		Score:  1,
		Reason: reason,
	}
	// Check for existing record to avoid duplicates
	var existingSuggest SuggestBlock

	if conn.Where("domain = ?", domain).Limit(1).Find(&existingSuggest).RowsAffected > 0 {
		// Запись существует, ничего не делаем
		return nil
	}

	return conn.Create(&suggest).Error
}

func DeleteSuggestBlock(domain string) error {
	conn := db.GetConnection()
	if err := conn.Where("domain = ?", domain).Delete(&SuggestBlock{}).Error; err != nil {
		return err
	}
	return nil
}
