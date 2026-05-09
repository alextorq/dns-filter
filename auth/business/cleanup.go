package business

import (
	"fmt"
	"time"

	authDb "github.com/alextorq/dns-filter/auth/db"
	"github.com/alextorq/dns-filter/logger"
)

// ClearExpiredSessions runs a periodic sweep of expired sessions.
// Mirrors the pattern used by blocked_domain.ClearOldEvent.
func ClearExpiredSessions() {
	l := logger.GetLogger()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	if err := authDb.DeleteExpiredSessions(time.Now()); err != nil {
		l.Error(fmt.Errorf("clear expired sessions: %w", err))
	}

	for range ticker.C {
		if err := authDb.DeleteExpiredSessions(time.Now()); err != nil {
			l.Error(fmt.Errorf("clear expired sessions: %w", err))
		}
	}
}
