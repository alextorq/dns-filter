package business

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	authDb "github.com/alextorq/dns-filter/auth/db"
	"gorm.io/gorm"
)

const SessionTTL = 7 * 24 * time.Hour

var ErrSessionExpired = errors.New("session expired")

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func IssueSession(userID uint) (*authDb.Session, error) {
	token, err := generateToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s := &authDb.Session{
		Token:     token,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionTTL),
	}
	if err := authDb.CreateSession(s); err != nil {
		return nil, err
	}
	cacheSession(s)
	return s, nil
}

// ResolveSession returns the user behind a session token. The token → (userID,
// expiry) mapping is cached in an LRU so the DNS-style hot-path-off-the-DB
// convention is preserved; the user record itself is still loaded fresh.
func ResolveSession(token string) (*authDb.Session, *authDb.User, error) {
	if token == "" {
		return nil, nil, gorm.ErrRecordNotFound
	}

	if cached, ok := lookupCachedSession(token); ok {
		if time.Now().After(cached.ExpiresAt) {
			if err := authDb.DeleteSession(token); err == nil {
				dropCachedSession(token)
			}
			return nil, nil, ErrSessionExpired
		}
		user, err := authDb.GetUserByID(cached.UserID)
		if err != nil {
			return nil, nil, err
		}
		return &authDb.Session{
			Token:     token,
			UserID:    cached.UserID,
			ExpiresAt: cached.ExpiresAt,
		}, user, nil
	}

	s, err := authDb.GetSessionByToken(token)
	if err != nil {
		return nil, nil, err
	}
	if time.Now().After(s.ExpiresAt) {
		_ = authDb.DeleteSession(token)
		return nil, nil, ErrSessionExpired
	}
	cacheSession(s)
	user, err := authDb.GetUserByID(s.UserID)
	if err != nil {
		return nil, nil, err
	}
	return s, user, nil
}

func RevokeSession(token string) error {
	if err := authDb.DeleteSession(token); err != nil {
		return err
	}
	dropCachedSession(token)
	return nil
}
