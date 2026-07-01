package business

import (
	"time"

	authDb "github.com/alextorq/dns-filter/auth/db"
	lru "github.com/alextorq/dns-filter/lru-cache"
)

const sessionCacheCapacity = 512

type cachedSession struct {
	UserID    uint
	ExpiresAt time.Time
}

func newSessionCache() *lru.LRUCache[cachedSession] {
	return lru.CreateCache[cachedSession](sessionCacheCapacity)
}

func (m *Module) cacheSession(s *authDb.Session) {
	m.cache.Add(s.Token, cachedSession{
		UserID:    s.UserID,
		ExpiresAt: s.ExpiresAt,
	})
}

func (m *Module) dropCachedSession(token string) {
	m.cache.Delete(token)
}

func (m *Module) lookupCachedSession(token string) (cachedSession, bool) {
	return m.cache.Get(token)
}
