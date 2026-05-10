package sync

import (
	"fmt"

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

	loadAdBlock := func(source db.BlockListSource, url string) {
		partial, err := easy_list.LoadFromURL(url)
		if err != nil {
			l.Error(fmt.Errorf("failed to load %s: %w", source, err))
			return
		}
		l.Debug(fmt.Sprintf("Loaded %s domains: %d", source, len(partial)))
		result = append(result, DomainBySource{Source: source, Domains: partial})
	}

	loadHosts := func(source db.BlockListSource, url string) {
		partial, err := LoadHostsFromURL(url)
		if err != nil {
			l.Error(fmt.Errorf("failed to load %s: %w", source, err))
			return
		}
		l.Debug(fmt.Sprintf("Loaded %s domains: %d", source, len(partial)))
		result = append(result, DomainBySource{Source: source, Domains: partial})
	}

	for _, item := range items {
		switch item.Name {
		case db.SourceEasyList:
			loadAdBlock(db.SourceEasyList, easy_list.EasyListURL)
		case db.SourceRuAdList:
			loadAdBlock(db.SourceRuAdList, easy_list.RuAdListURL)
		case db.SourceAdGuardRussian:
			loadAdBlock(db.SourceAdGuardRussian, easy_list.AdGuardRussianURL)
		case db.SourceStevenBlack:
			loadHosts(db.SourceStevenBlack, StevenBlackURL)
		case db.SourceHaGeZiMulti:
			loadHosts(db.SourceHaGeZiMulti, HaGeZiMultiURL)
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
