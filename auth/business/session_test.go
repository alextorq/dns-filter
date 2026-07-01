package business

import (
	"errors"
	"testing"
	"time"

	authDb "github.com/alextorq/dns-filter/auth/db"
	"gorm.io/gorm"
)

type fakeRepo struct {
	usersByLogin map[string]*authDb.User
	usersByID    map[uint]*authDb.User
	sessions     map[string]*authDb.Session
	lookupErr    error
	createErr    error
	deleteErr    error
	deleted      []string
	createdUsers int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		usersByLogin: map[string]*authDb.User{},
		usersByID:    map[uint]*authDb.User{},
		sessions:     map[string]*authDb.Session{},
	}
}

func (r *fakeRepo) GetUserByLogin(login string) (*authDb.User, error) {
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	user, ok := r.usersByLogin[login]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return user, nil
}

func (r *fakeRepo) GetUserByID(id uint) (*authDb.User, error) {
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	user, ok := r.usersByID[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return user, nil
}

func (r *fakeRepo) CreateUser(login, passwordHash string) (*authDb.User, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.createdUsers++
	user := &authDb.User{ID: uint(r.createdUsers), Login: login, PasswordHash: passwordHash}
	r.usersByLogin[login] = user
	r.usersByID[user.ID] = user
	return user, nil
}

func (r *fakeRepo) CreateSession(session *authDb.Session) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.sessions[session.Token] = session
	return nil
}

func (r *fakeRepo) GetSessionByToken(token string) (*authDb.Session, error) {
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	session, ok := r.sessions[token]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return session, nil
}

func (r *fakeRepo) DeleteSession(token string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.sessions, token)
	r.deleted = append(r.deleted, token)
	return nil
}

func (r *fakeRepo) DeleteExpiredSessions(time.Time) error { return nil }

func (r *fakeRepo) addUser(t *testing.T, login, password string) *authDb.User {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &authDb.User{ID: uint(len(r.usersByID) + 1), Login: login, PasswordHash: hash}
	r.usersByLogin[login] = user
	r.usersByID[user.ID] = user
	return user
}

func TestAuthenticate_IssuesSession(t *testing.T) {
	repo := newFakeRepo()
	user := repo.addUser(t, "admin", "correct")
	module := NewModule(repo, "", "")

	gotUser, session, err := module.Authenticate("admin", "correct")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if gotUser.ID != user.ID || session.UserID != user.ID || len(session.Token) != 64 {
		t.Fatalf("unexpected auth result: user=%+v session=%+v", gotUser, session)
	}
	if _, ok := repo.sessions[session.Token]; !ok {
		t.Fatal("session was not persisted")
	}
	if _, ok := module.lookupCachedSession(session.Token); !ok {
		t.Fatal("session was not cached")
	}
}

func TestAuthenticate_RejectsInvalidCredentials(t *testing.T) {
	repo := newFakeRepo()
	repo.addUser(t, "admin", "correct")
	module := NewModule(repo, "", "")

	for _, tc := range []struct{ login, password string }{
		{login: "missing", password: "correct"},
		{login: "admin", password: "wrong"},
	} {
		if _, _, err := module.Authenticate(tc.login, tc.password); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Authenticate(%q): got %v, want ErrInvalidCredentials", tc.login, err)
		}
	}
	if len(repo.sessions) != 0 {
		t.Fatalf("invalid credentials created sessions: %d", len(repo.sessions))
	}
}

func TestRevokeSession_DBFailureKeepsCache(t *testing.T) {
	repo := newFakeRepo()
	repo.deleteErr = errors.New("db unavailable")
	module := NewModule(repo, "", "")
	module.cacheSession(&authDb.Session{Token: "tok", UserID: 42, ExpiresAt: time.Now().Add(time.Hour)})

	if err := module.RevokeSession("tok"); err == nil {
		t.Fatal("expected delete error")
	}
	if _, ok := module.lookupCachedSession("tok"); !ok {
		t.Fatal("cache was dropped despite DB failure")
	}
}

func TestResolveSession_ExpiredCacheKeepsEntryOnDeleteFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.deleteErr = errors.New("db unavailable")
	module := NewModule(repo, "", "")
	module.cacheSession(&authDb.Session{Token: "expired", UserID: 7, ExpiresAt: time.Now().Add(-time.Hour)})

	if _, _, err := module.ResolveSession("expired"); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("ResolveSession: got %v, want ErrSessionExpired", err)
	}
	if _, ok := module.lookupCachedSession("expired"); !ok {
		t.Fatal("cache was dropped despite DB failure")
	}
}

func TestRevokeSession_HappyPathDropsCache(t *testing.T) {
	repo := newFakeRepo()
	module := NewModule(repo, "", "")
	module.cacheSession(&authDb.Session{Token: "tok", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)})

	if err := module.RevokeSession("tok"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, ok := module.lookupCachedSession("tok"); ok {
		t.Fatal("cache should be empty after successful revoke")
	}
}

func TestBootstrapAdmin_CreatesHashedUserOnce(t *testing.T) {
	repo := newFakeRepo()
	module := NewModule(repo, "admin", "secret")

	if err := module.BootstrapAdmin(); err != nil {
		t.Fatalf("first BootstrapAdmin: %v", err)
	}
	if err := module.BootstrapAdmin(); err != nil {
		t.Fatalf("second BootstrapAdmin: %v", err)
	}
	if repo.createdUsers != 1 {
		t.Fatalf("created users = %d, want 1", repo.createdUsers)
	}
	if !CheckPassword(repo.usersByLogin["admin"].PasswordHash, "secret") {
		t.Fatal("bootstrap password was not hashed correctly")
	}
}
