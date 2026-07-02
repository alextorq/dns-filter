package business

import (
	"context"
	"time"

	"github.com/alextorq/dns-filter/periodic"
)

// ClearExpiredSessions runs a periodic sweep of expired sessions.
func (m *Module) ClearExpiredSessions(ctx context.Context, log periodic.Logger) {
	periodic.Run(ctx, "clear expired sessions", time.Hour, log, func() error {
		return m.repo.DeleteExpiredSessions(time.Now())
	})
}
