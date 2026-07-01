package business

import (
	"errors"
	"time"

	authDb "github.com/alextorq/dns-filter/auth/db"
	lru "github.com/alextorq/dns-filter/lru-cache"
	"gorm.io/gorm"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

// Repo is the persistence contract consumed by authentication business logic.
type Repo interface {
	GetUserByLogin(login string) (*authDb.User, error)
	GetUserByID(id uint) (*authDb.User, error)
	CreateUser(login, passwordHash string) (*authDb.User, error)
	CreateSession(session *authDb.Session) error
	GetSessionByToken(token string) (*authDb.Session, error)
	DeleteSession(token string) error
	DeleteExpiredSessions(now time.Time) error
}

type Module struct {
	repo          Repo
	cache         *lru.LRUCache[cachedSession]
	adminLogin    string
	adminPassword string
}

func NewModule(repo Repo, adminLogin, adminPassword string) *Module {
	return &Module{
		repo:          repo,
		cache:         newSessionCache(),
		adminLogin:    adminLogin,
		adminPassword: adminPassword,
	}
}

// Authenticate verifies credentials and issues a persisted session.
func (m *Module) Authenticate(login, password string) (*authDb.User, *authDb.Session, error) {
	user, err := m.repo.GetUserByLogin(login)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}
	if !CheckPassword(user.PasswordHash, password) {
		return nil, nil, ErrInvalidCredentials
	}
	session, err := m.IssueSession(user.ID)
	if err != nil {
		return nil, nil, err
	}
	return user, session, nil
}
