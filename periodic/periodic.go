package periodic

import (
	"context"
	"fmt"
	"time"
)

// Logger is the narrow logging port used by periodic jobs.
type Logger interface {
	Error(error)
}

// Run invokes cleanup once immediately, then on every tick of interval.
// Errors are logged with name as the prefix and never stop the loop.
// Cancellation stops scheduling new cleanups; an already-running cleanup is
// allowed to finish before Run returns. Call it from a goroutine when the
// caller must continue doing other work.
func Run(ctx context.Context, name string, interval time.Duration, log Logger, cleanup func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	tick := func() {
		if err := cleanup(); err != nil {
			log.Error(fmt.Errorf("%s: %w", name, err))
		}
	}

	select {
	case <-ctx.Done():
		return
	default:
	}
	tick()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Re-check cancellation when it raced with a tick already waiting in
			// the ticker channel, reducing the chance of starting extra work.
			select {
			case <-ctx.Done():
				return
			default:
				tick()
			}
		}
	}
}
