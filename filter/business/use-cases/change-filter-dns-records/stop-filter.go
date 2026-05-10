package change_filter_dns_records

import (
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/logger"
)

func ChangeFilterDnsRecords() bool {
	l := logger.GetLogger()
	conf := config.GetConfig()
	for {
		old := conf.Enabled.Load()
		if conf.Enabled.CompareAndSwap(old, !old) {
			// Any manual toggle invalidates an in-flight pause: otherwise a
			// switch off→on would show "Active" in the UI while the deadline
			// still suppresses blocking until it expires.
			conf.PausedUntilUnix.Store(0)
			l.Info("Change filter status to", !old)
			return !old
		}
	}
}
