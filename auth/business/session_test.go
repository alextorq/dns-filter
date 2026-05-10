package business

import (
	"os"
	"testing"
	"time"

	authDb "github.com/alextorq/dns-filter/auth/db"
	app_db "github.com/alextorq/dns-filter/db"
)

func TestMain(m *testing.M) {
	// Same trick as create-domain tests: chdir into a temp dir so the default
	// ./filter.sqlite path resolves locally.
	tmp, err := os.MkdirTemp("", "auth-session-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// Locks in #36: if DeleteSession fails, the in-memory cache must NOT be
// dropped — otherwise logout looks succeeded but the session still
// resolves on next request.
func TestRevokeSession_DBFailureKeepsCache(t *testing.T) {
	conn := app_db.GetConnection()

	// Ensure the sessions table is gone so DeleteSession returns an error.
	if err := conn.Migrator().DropTable(&authDb.Session{}); err != nil {
		t.Fatalf("drop sessions table: %v", err)
	}

	const token = "tok-db-fail"
	cacheSession(&authDb.Session{
		Token:     token,
		UserID:    42,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	if err := RevokeSession(token); err == nil {
		t.Fatal("expected error from RevokeSession when DB delete fails, got nil")
	}

	if _, ok := lookupCachedSession(token); !ok {
		t.Fatal("cache was dropped despite DB delete failure — logout would be falsely effective")
	}

	// Restore for any subsequent tests.
	if err := conn.AutoMigrate(&authDb.Session{}); err != nil {
		t.Fatalf("restore sessions table: %v", err)
	}
}

func TestRevokeSession_HappyPathDropsCache(t *testing.T) {
	conn := app_db.GetConnection()
	if err := conn.AutoMigrate(&authDb.Session{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	t.Cleanup(func() {
		conn.Where("token = ?", "tok-ok").Delete(&authDb.Session{})
	})

	s := &authDb.Session{
		Token:     "tok-ok",
		UserID:    1,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := authDb.CreateSession(s); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	cacheSession(s)

	if err := RevokeSession(s.Token); err != nil {
		t.Fatalf("RevokeSession returned %v", err)
	}
	if _, ok := lookupCachedSession(s.Token); ok {
		t.Fatal("cache should be empty after successful revoke")
	}
}
