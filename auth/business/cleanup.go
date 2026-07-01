package business

import (
	"time"

	"github.com/alextorq/dns-filter/periodic"
)

// ClearExpiredSessions runs a periodic sweep of expired sessions.
func (m *Module) ClearExpiredSessions() {
	periodic.Run("clear expired sessions", time.Hour, func() error {
		return m.repo.DeleteExpiredSessions(time.Now())
	})
}
