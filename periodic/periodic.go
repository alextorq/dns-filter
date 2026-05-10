package periodic

import (
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/logger"
)

// Run invokes cleanup once immediately, then on every tick of interval.
// Errors are logged with name as the prefix and never stop the loop.
// Blocks forever — call from a goroutine.
func Run(name string, interval time.Duration, cleanup func() error) {
	l := logger.GetLogger()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	tick := func() {
		if err := cleanup(); err != nil {
			l.Error(fmt.Errorf("%s: %w", name, err))
		}
	}

	tick()
	for range ticker.C {
		tick()
	}
}
