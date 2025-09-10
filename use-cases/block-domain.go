package use_cases

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/logger"
)

func BlockDomain(domain string) error {
	l := logger.GetLogger()
	l.Warn("Заблокирован:", domain)
	domainE, err := blacklists.GetBlockListByDomain(domain)
	if err != nil {
		return err
	}
	err = blacklists.CreateBlockDomain(domainE.ID)

	return err
}
