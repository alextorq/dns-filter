package change_filter_dns_records

import (
	"github.com/alextorq/dns-filter/config"
)

type Logger interface {
	Info(args ...any)
}

// ChangeFilterDnsRecords atomically toggles the global filter flag and clears
// any in-flight pause (otherwise a switch off→on would show "Active" in the
// UI while the deadline still suppresses blocking until it expires). Returns
// the new state.
func ChangeFilterDnsRecords(conf *config.Config, log Logger) bool {
	for {
		old := conf.Enabled.Load()
		if conf.Enabled.CompareAndSwap(old, !old) {
			conf.PausedUntilUnix.Store(0)
			log.Info("Change filter status to", !old)
			return !old
		}
	}
}
