package db

import (
	"github.com/alextorq/dns-filter/db"
)

type SuggestBlock struct {
	ID     uint   `gorm:"primarykey" json:"id"`
	Domain string `json:"domain" gorm:"uniqueIndex"`
	Score  int    `json:"score"`
	Reason string `json:"reasons"`
	Active bool   `gorm:"default:true" json:"active"`
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

func UpdateActiveStatus(id uint, active bool) error {
	conn := db.GetConnection()
	if err := conn.Model(&SuggestBlock{}).Where("id = ?", id).Update("active", active).Error; err != nil {
		return err
	}
	return nil
}

type GetAllParams struct {
	Limit  int
	Offset int
	Filter string
	Active *bool
}

type GetAllResult struct {
	List  []SuggestBlock `json:"list"`
	Total int64          `json:"total"`
}

func GetAllSuggestBlocks(params GetAllParams) (*GetAllResult, error) {
	conn := db.GetConnection()

	var suggests []SuggestBlock
	query := conn.Model(&SuggestBlock{})
	var total int64

	// если нужно фильтровать по строке
	if params.Filter != "" {
		query = query.Where("domain LIKE ?", "%"+params.Filter+"%")
	}

	// если передан параметр Active, фильтруем по нему
	if params.Active != nil {
		query = query.Where("active = ?", *params.Active)
	}

	// сначала считаем количество
	query.Count(&total)

	err := query.
		Order("score DESC, id DESC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&suggests).
		Error

	return &GetAllResult{
		List:  suggests,
		Total: total,
	}, err
}
