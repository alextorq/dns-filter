package db

import (
	"time"

	database "github.com/alextorq/dns-filter/db"
)

type Session struct {
	Token     string    `gorm:"primaryKey;type:varchar(64)" json:"-"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
}

func CreateSession(s *Session) error {
	return database.GetConnection().Create(s).Error
}

func GetSessionByToken(token string) (*Session, error) {
	conn := database.GetConnection()
	var s Session
	if err := conn.Where("token = ?", token).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func DeleteSession(token string) error {
	return database.GetConnection().Where("token = ?", token).Delete(&Session{}).Error
}

func DeleteExpiredSessions(now time.Time) error {
	return database.GetConnection().Where("expires_at < ?", now).Delete(&Session{}).Error
}
