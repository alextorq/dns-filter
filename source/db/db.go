package db

import (
	"time"

	dbCon "github.com/alextorq/dns-filter/db"
	"gorm.io/gorm"
)

type Source struct {
	ID        uint            `gorm:"primarykey" json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt gorm.DeletedAt  `gorm:"index" json:"deletedAt"`
	Name      BlockListSource `gorm:"type:varchar(255);not null;uniqueIndex:idx_sync_name" json:"name"`
	Active    bool            `json:"active"`
}

type GetAllParams struct {
	Limit  int
	Offset int
	Filter string
}

func GetAllRecords(filter GetAllParams) ([]Source, error) {
	conn := dbCon.GetConnection()
	var records []Source
	query := conn.Model(&Source{})

	// если нужно фильтровать по строке
	if filter.Filter != "" {
		query = query.Where("name LIKE ?", "%"+filter.Filter+"%")
	}

	return records, query.Find(&records).Error
}

func GetAllActiveRecords() ([]Source, error) {
	conn := dbCon.GetConnection()
	var records []Source
	query := conn.Model(&Source{})
	query = query.Where("active = true")

	return records, query.Find(&records).Error
}

func GetAmountRecords() int64 {
	conn := dbCon.GetConnection()
	var count int64
	conn.Model(&Source{}).Count(&count)
	return count
}

func GetRecordByID(id uint) (*Source, error) {
	conn := dbCon.GetConnection()
	var source Source
	err := conn.Where("id = ?", id).First(&source).Error
	if err != nil {
		return nil, err
	}
	return &source, nil
}

func UpdateRecord(s *Source) {
	conn := dbCon.GetConnection()
	conn.Save(s)
}
