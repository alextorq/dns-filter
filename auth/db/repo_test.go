package db

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Session{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return NewRepo(db)
}

func TestRepo_UserLifecycle(t *testing.T) {
	repo := newTestRepo(t)
	user, err := repo.CreateUser("admin", "hash-1")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	byLogin, err := repo.GetUserByLogin("admin")
	if err != nil || byLogin.ID != user.ID {
		t.Fatalf("GetUserByLogin: user=%+v err=%v", byLogin, err)
	}
	byID, err := repo.GetUserByID(user.ID)
	if err != nil || byID.Login != "admin" {
		t.Fatalf("GetUserByID: user=%+v err=%v", byID, err)
	}
	exists, err := repo.UserExists("admin")
	if err != nil || !exists {
		t.Fatalf("UserExists(admin): exists=%v err=%v", exists, err)
	}
	exists, err = repo.UserExists("missing")
	if err != nil || exists {
		t.Fatalf("UserExists(missing): exists=%v err=%v", exists, err)
	}
	if err := repo.UpdatePasswordHash(user.ID, "hash-2"); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}
	updated, err := repo.GetUserByID(user.ID)
	if err != nil || updated.PasswordHash != "hash-2" {
		t.Fatalf("updated user=%+v err=%v", updated, err)
	}
}

func TestRepo_SessionLifecycleAndExpiryCleanup(t *testing.T) {
	repo := newTestRepo(t)
	now := time.Now()
	for _, session := range []*Session{
		{Token: "active", UserID: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{Token: "boundary", UserID: 1, CreatedAt: now.Add(-time.Hour), ExpiresAt: now},
		{Token: "expired", UserID: 1, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)},
	} {
		if err := repo.CreateSession(session); err != nil {
			t.Fatalf("CreateSession(%s): %v", session.Token, err)
		}
	}

	got, err := repo.GetSessionByToken("active")
	if err != nil || got.UserID != 1 {
		t.Fatalf("GetSessionByToken: session=%+v err=%v", got, err)
	}
	if err := repo.DeleteExpiredSessions(now); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	if _, err := repo.GetSessionByToken("expired"); err == nil {
		t.Fatal("expired session still exists")
	}
	if _, err := repo.GetSessionByToken("boundary"); err == nil {
		t.Fatal("session at expiry boundary still exists")
	}
	if _, err := repo.GetSessionByToken("active"); err != nil {
		t.Fatalf("active session was deleted: %v", err)
	}
	if err := repo.DeleteSession("active"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := repo.GetSessionByToken("active"); err == nil {
		t.Fatal("deleted session still exists")
	}
}
