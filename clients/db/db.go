package db

import (
	"time"

	database "github.com/alextorq/dns-filter/db"
	"gorm.io/gorm"
)

type ExcludeClient struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	UserId    string         `json:"user_id" gorm:"not null"`
	Active    bool           `json:"active"`
}

func GetAllClients() ([]ExcludeClient, error) {
	con := database.GetConnection()
	var clients []ExcludeClient
	err := con.Find(&clients).Error
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func GetAllActiveClients() ([]ExcludeClient, error) {
	con := database.GetConnection()
	var clients []ExcludeClient
	err := con.Where("active = ?", true).Find(&clients).Error
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func AddClient(userId string) error {
	con := database.GetConnection()
	client := ExcludeClient{
		UserId: userId,
		Active: true,
	}
	err := con.Create(&client).Error
	if err != nil {
		return err
	}
	return nil
}

func DeleteClient(id uint) error {
	con := database.GetConnection()
	err := con.Delete(&ExcludeClient{}, id).Error
	if err != nil {
		return err
	}
	return nil
}

func GetClientById(id uint) (*ExcludeClient, error) {
	con := database.GetConnection()
	var client ExcludeClient
	err := con.First(&client, id).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

func UpdateClientIsActive(id uint, isActive bool) error {
	con := database.GetConnection()
	err := con.Model(&ExcludeClient{}).Where("id = ?", id).Update("active", isActive).Error
	if err != nil {
		return err
	}
	return nil
}
