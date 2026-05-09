package db

import (
	"errors"
	"time"

	database "github.com/alextorq/dns-filter/db"
	"gorm.io/gorm"
)

type User struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Login        string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"login"`
	PasswordHash string         `gorm:"type:varchar(255);not null" json:"-"`
}

func GetUserByLogin(login string) (*User, error) {
	conn := database.GetConnection()
	var u User
	err := conn.Where("login = ?", login).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func GetUserByID(id uint) (*User, error) {
	conn := database.GetConnection()
	var u User
	err := conn.First(&u, id).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func CreateUser(login, passwordHash string) (*User, error) {
	conn := database.GetConnection()
	u := &User{Login: login, PasswordHash: passwordHash}
	if err := conn.Create(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

func UpdatePasswordHash(id uint, hash string) error {
	conn := database.GetConnection()
	return conn.Model(&User{}).Where("id = ?", id).Update("password_hash", hash).Error
}

func UserExists(login string) (bool, error) {
	_, err := GetUserByLogin(login)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}
