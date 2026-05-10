package business

import (
	"time"

	authDb "github.com/alextorq/dns-filter/auth/db"
	"github.com/alextorq/dns-filter/periodic"
)

// ClearExpiredSessions runs a periodic sweep of expired sessions.
func ClearExpiredSessions() {
	periodic.Run("clear expired sessions", time.Hour, func() error {
		return authDb.DeleteExpiredSessions(time.Now())
	})
}
