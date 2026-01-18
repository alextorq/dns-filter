package sync

import (
	blockDb "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/source/business/use-cases/sync/easy-list"
	"github.com/alextorq/dns-filter/source/db"
)

type DomainBySource struct {
	Source  db.BlockListSource
	Domains []string
}

func LoadAndParseActiveSources() []DomainBySource {
	l := logger.GetLogger()
	result := make([]DomainBySource, 0)

	items, err := db.GetAllActiveRecords()

	if err != nil {
		logger.GetLogger().Error(err)
		return result
	}

	for _, item := range items {
		switch item.Name {
		case db.SourceEasyList:
			partial := easy_list.LoadEasyList()
			l.Debug("Loaded EasyList domains:", len(partial))

			result = append(result, DomainBySource{
				Source:  db.SourceEasyList,
				Domains: partial,
			})
		case db.SourceStevenBlack:
			partial := LoadStevenBlack()
			result = append(result, DomainBySource{
				Source:  db.SourceStevenBlack,
				Domains: partial,
			})
			l.Debug("Loaded SourceStevenBlack domains:", len(partial))
		}
	}

	return result
}

func Sync() error {
	list := LoadAndParseActiveSources()
	for _, item := range list {
		err := blockDb.CreateDNSRecordsByDomains(item.Domains, item.Source.String())
		if err != nil {
			return err
		}
	}
	return nil
}
