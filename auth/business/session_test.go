package business

import (
	"context"
	"encoding/hex"
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
	expiredAt    []time.Time
	onExpire     func()
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

func (r *fakeRepo) DeleteExpiredSessions(now time.Time) error {
	r.expiredAt = append(r.expiredAt, now)
	if r.onExpire != nil {
		r.onExpire()
	}
	return nil
}

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

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

type fakeTokenGenerator struct {
	token string
	err   error
	calls int
}

func (g *fakeTokenGenerator) Generate() (string, error) {
	g.calls++
	return g.token, g.err
}

func newTestModule(repo Repo, now time.Time) *Module {
	return NewModule(Deps{
		Repo:           repo,
		Clock:          &fakeClock{now: now},
		TokenGenerator: &fakeTokenGenerator{token: "generated-token"},
	})
}

func TestAuthenticate_IssuesSession(t *testing.T) {
	repo := newFakeRepo()
	user := repo.addUser(t, "admin", "correct")
	now := time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)
	module := newTestModule(repo, now)

	gotUser, session, err := module.Authenticate("admin", "correct")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if gotUser.ID != user.ID || session.UserID != user.ID || session.Token != "generated-token" {
		t.Fatalf("unexpected auth result: user=%+v session=%+v", gotUser, session)
	}
	if !session.CreatedAt.Equal(now) || !session.ExpiresAt.Equal(now.Add(SessionTTL)) {
		t.Fatalf("unexpected session times: created=%s expires=%s", session.CreatedAt, session.ExpiresAt)
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
	module := newTestModule(repo, time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC))

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

func TestAuthenticate_PropagatesTokenGeneratorFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.addUser(t, "admin", "correct")
	wantErr := errors.New("entropy unavailable")
	generator := &fakeTokenGenerator{err: wantErr}
	module := NewModule(Deps{
		Repo:           repo,
		Clock:          &fakeClock{now: time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)},
		TokenGenerator: generator,
	})

	if _, _, err := module.Authenticate("admin", "correct"); !errors.Is(err, wantErr) {
		t.Fatalf("Authenticate: got %v, want %v", err, wantErr)
	}
	if generator.calls != 1 {
		t.Fatalf("Generate calls = %d, want 1", generator.calls)
	}
	if len(repo.sessions) != 0 {
		t.Fatalf("generator failure created sessions: %d", len(repo.sessions))
	}
}

func TestResolveSession_ExpiryBoundary(t *testing.T) {
	expiresAt := time.Date(2026, time.July, 22, 12, 30, 0, 0, time.UTC)
	boundaries := []struct {
		name        string
		now         time.Time
		wantExpired bool
	}{
		{name: "just before expiry", now: expiresAt.Add(-time.Nanosecond)},
		{name: "at expiry", now: expiresAt, wantExpired: true},
	}

	for _, cached := range []bool{false, true} {
		for _, boundary := range boundaries {
			name := "persisted/" + boundary.name
			if cached {
				name = "cached/" + boundary.name
			}
			t.Run(name, func(t *testing.T) {
				repo := newFakeRepo()
				user := repo.addUser(t, "admin", "correct")
				session := &authDb.Session{Token: "tok", UserID: user.ID, ExpiresAt: expiresAt}
				module := newTestModule(repo, boundary.now)
				if cached {
					module.cacheSession(session)
				} else {
					repo.sessions[session.Token] = session
				}

				gotSession, gotUser, err := module.ResolveSession(session.Token)
				if boundary.wantExpired {
					if !errors.Is(err, ErrSessionExpired) {
						t.Fatalf("ResolveSession: got %v, want ErrSessionExpired", err)
					}
					if len(repo.deleted) != 1 || repo.deleted[0] != session.Token {
						t.Fatalf("deleted tokens = %v, want [%s]", repo.deleted, session.Token)
					}
					return
				}
				if err != nil {
					t.Fatalf("ResolveSession: %v", err)
				}
				if gotSession.Token != session.Token || gotUser.ID != user.ID {
					t.Fatalf("unexpected result: session=%+v user=%+v", gotSession, gotUser)
				}
			})
		}
	}
}

func TestRevokeSession_DBFailureKeepsCache(t *testing.T) {
	repo := newFakeRepo()
	repo.deleteErr = errors.New("db unavailable")
	now := time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)
	module := newTestModule(repo, now)
	module.cacheSession(&authDb.Session{Token: "tok", UserID: 42, ExpiresAt: now.Add(time.Hour)})

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
	now := time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)
	module := newTestModule(repo, now)
	module.cacheSession(&authDb.Session{Token: "expired", UserID: 7, ExpiresAt: now.Add(-time.Hour)})

	if _, _, err := module.ResolveSession("expired"); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("ResolveSession: got %v, want ErrSessionExpired", err)
	}
	if _, ok := module.lookupCachedSession("expired"); !ok {
		t.Fatal("cache was dropped despite DB failure")
	}
}

func TestRevokeSession_HappyPathDropsCache(t *testing.T) {
	repo := newFakeRepo()
	now := time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)
	module := newTestModule(repo, now)
	module.cacheSession(&authDb.Session{Token: "tok", UserID: 1, ExpiresAt: now.Add(time.Hour)})

	if err := module.RevokeSession("tok"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, ok := module.lookupCachedSession("tok"); ok {
		t.Fatal("cache should be empty after successful revoke")
	}
}

func TestBootstrapAdmin_CreatesHashedUserOnce(t *testing.T) {
	repo := newFakeRepo()
	module := NewModule(Deps{
		Repo:           repo,
		Clock:          &fakeClock{now: time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)},
		TokenGenerator: &fakeTokenGenerator{token: "generated-token"},
		AdminLogin:     "admin",
		AdminPassword:  "secret",
	})

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

func TestClearExpiredSessions_PreCanceledContextSkipsRepo(t *testing.T) {
	repo := newFakeRepo()
	module := newTestModule(repo, time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	module.ClearExpiredSessions(ctx, cleanupTestLogger{})

	if len(repo.expiredAt) != 0 {
		t.Fatalf("DeleteExpiredSessions calls = %d, want 0", len(repo.expiredAt))
	}
}

func TestClearExpiredSessions_UsesInjectedClock(t *testing.T) {
	repo := newFakeRepo()
	now := time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)
	module := newTestModule(repo, now)
	ctx, cancel := context.WithCancel(context.Background())
	repo.onExpire = cancel

	module.ClearExpiredSessions(ctx, cleanupTestLogger{})

	if len(repo.expiredAt) != 1 || !repo.expiredAt[0].Equal(now) {
		t.Fatalf("DeleteExpiredSessions times = %v, want [%s]", repo.expiredAt, now)
	}
}

func TestNewModule_RejectsMissingDependencies(t *testing.T) {
	repo := newFakeRepo()
	clock := &fakeClock{now: time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)}
	generator := &fakeTokenGenerator{token: "generated-token"}
	var typedNilRepo *fakeRepo
	var typedNilClock *fakeClock
	var typedNilGenerator *fakeTokenGenerator

	for _, tc := range []struct {
		name string
		deps Deps
	}{
		{name: "repo", deps: Deps{Clock: clock, TokenGenerator: generator}},
		{name: "typed nil repo", deps: Deps{Repo: typedNilRepo, Clock: clock, TokenGenerator: generator}},
		{name: "clock", deps: Deps{Repo: repo, TokenGenerator: generator}},
		{name: "typed nil clock", deps: Deps{Repo: repo, Clock: typedNilClock, TokenGenerator: generator}},
		{name: "token generator", deps: Deps{Repo: repo, Clock: clock}},
		{name: "typed nil token generator", deps: Deps{Repo: repo, Clock: clock, TokenGenerator: typedNilGenerator}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("NewModule accepted missing %s", tc.name)
				}
			}()
			NewModule(tc.deps)
		})
	}
}

func TestCryptoTokenGenerator_GeneratesHexToken(t *testing.T) {
	token, err := NewCryptoTokenGenerator().Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	decoded, err := hex.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not hexadecimal: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded token length = %d, want 32", len(decoded))
	}
}

type cleanupTestLogger struct{}

func (cleanupTestLogger) Error(error) {}
