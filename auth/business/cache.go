package business

import (
	"sync"
	"time"

	authDb "github.com/alextorq/dns-filter/auth/db"
	lru "github.com/alextorq/dns-filter/lru-cache"
)

const sessionCacheCapacity = 512

type cachedSession struct {
	UserID    uint
	ExpiresAt time.Time
}

var (
	sessionCache     *lru.LRUCache[cachedSession]
	sessionCacheOnce sync.Once
)

func getSessionCache() *lru.LRUCache[cachedSession] {
	sessionCacheOnce.Do(func() {
		sessionCache = lru.CreateCache[cachedSession](sessionCacheCapacity)
	})
	return sessionCache
}

func cacheSession(s *authDb.Session) {
	getSessionCache().Add(s.Token, cachedSession{
		UserID:    s.UserID,
		ExpiresAt: s.ExpiresAt,
	})
}

func dropCachedSession(token string) {
	getSessionCache().Delete(token)
}

func lookupCachedSession(token string) (cachedSession, bool) {
	return getSessionCache().Get(token)
}
