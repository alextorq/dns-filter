package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repo is the explicit persistence adapter for users and sessions.
type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) GetUserByLogin(login string) (*User, error) {
	var user User
	if err := r.db.Where("login = ?", login).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repo) GetUserByID(id uint) (*User, error) {
	var user User
	if err := r.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repo) CreateUser(login, passwordHash string) (*User, error) {
	user := &User{Login: login, PasswordHash: passwordHash}
	if err := r.db.Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func (r *Repo) UpdatePasswordHash(id uint, hash string) error {
	return r.db.Model(&User{}).Where("id = ?", id).Update("password_hash", hash).Error
}

func (r *Repo) UserExists(login string) (bool, error) {
	_, err := r.GetUserByLogin(login)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func (r *Repo) CreateSession(session *Session) error {
	return r.db.Create(session).Error
}

func (r *Repo) GetSessionByToken(token string) (*Session, error) {
	var session Session
	if err := r.db.Where("token = ?", token).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *Repo) DeleteSession(token string) error {
	return r.db.Where("token = ?", token).Delete(&Session{}).Error
}

func (r *Repo) DeleteExpiredSessions(now time.Time) error {
	return r.db.Where("expires_at <= ?", now).Delete(&Session{}).Error
}
