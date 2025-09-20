package change_filter_dns_records

import (
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/logger"
)

func ChangeFilterDnsRecords() bool {
	l := logger.GetLogger()
	conf := config.GetConfig()
	conf.Enabled = !conf.Enabled
	l.Info("Change filter status to", conf.Enabled)
	return conf.Enabled
}
