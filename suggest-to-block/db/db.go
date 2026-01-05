package db

import "github.com/alextorq/dns-filter/db"

type SuggestBlock struct {
	ID      uint     `gorm:"primarykey" json:"id"`
	Domain  string   `json:"domain" gorm:"uniqueIndex"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

func CreateSuggestBlock(domain string) (*SuggestBlock, error) {
	conn := db.GetConnection()
	suggest := SuggestBlock{
		Domain: domain,
		Score:  1,
	}
	// Check for existing record to avoid duplicates
	var existingSuggest SuggestBlock
	if err := conn.Where("domain = ?", domain).First(&existingSuggest).Error; err == nil {
		return &existingSuggest, nil
	}

	if err := conn.Create(&suggest).Error; err != nil {
		return nil, err
	}
	return &suggest, nil
}

func DeleteSuggestBlock(domain string) error {
	conn := db.GetConnection()
	if err := conn.Where("domain = ?", domain).Delete(&SuggestBlock{}).Error; err != nil {
		return err
	}
	return nil
}
