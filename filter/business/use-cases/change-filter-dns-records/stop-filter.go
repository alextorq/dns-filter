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
			l.Info("Change filter status to", !old)
			return !old
		}
	}
}
