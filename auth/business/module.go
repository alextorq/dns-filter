package business

import (
	"errors"
	"reflect"
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

// Clock supplies wall time for behavior that depends on the session lifetime.
type Clock interface {
	Now() time.Time
}

// TokenGenerator creates opaque session credentials.
type TokenGenerator interface {
	Generate() (string, error)
}

// Deps is the complete construction contract for the auth module. The session
// cache remains instance-owned; only external sources of persistence, time and
// randomness cross the module boundary.
type Deps struct {
	Repo           Repo
	Clock          Clock
	TokenGenerator TokenGenerator
	AdminLogin     string
	AdminPassword  string
}

type Module struct {
	repo          Repo
	clock         Clock
	tokens        TokenGenerator
	cache         *lru.LRUCache[cachedSession]
	adminLogin    string
	adminPassword string
}

func NewModule(deps Deps) *Module {
	requireDependency("repo", deps.Repo)
	requireDependency("clock", deps.Clock)
	requireDependency("token generator", deps.TokenGenerator)
	return &Module{
		repo:          deps.Repo,
		clock:         deps.Clock,
		tokens:        deps.TokenGenerator,
		cache:         newSessionCache(),
		adminLogin:    deps.AdminLogin,
		adminPassword: deps.AdminPassword,
	}
}

func requireDependency(name string, dependency any) {
	if dependency == nil {
		panic("auth/business: " + name + " is required")
	}
	v := reflect.ValueOf(dependency)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			panic("auth/business: " + name + " is required")
		}
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
