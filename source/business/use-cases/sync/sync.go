package sync

import (
	"fmt"

	"github.com/alextorq/dns-filter/source/business/use-cases/sync/easy-list"
	"github.com/alextorq/dns-filter/source/db"
)

type Logger interface {
	Debug(args ...any)
	Error(err error)
}

// SourceLister is the narrow read port over the sources table.
type SourceLister interface {
	GetAllActive() ([]db.Source, error)
}

// BlockWriter is the narrow write port over the blocklist.
type BlockWriter interface {
	CreateDNSRecordsByDomains(urls []string, source string) error
}

type DomainBySource struct {
	Source  db.BlockListSource
	Domains []string
}

// LoadAndParseActiveSources downloads + parses every enabled source. Network /
// parser errors are logged and skipped so a single bad source does not abort
// the whole batch.
func LoadAndParseActiveSources(repo SourceLister, log Logger) []DomainBySource {
	result := make([]DomainBySource, 0)

	items, err := repo.GetAllActive()
	if err != nil {
		log.Error(err)
		return result
	}

	loadAdBlock := func(source db.BlockListSource, url string) {
		partial, err := easy_list.LoadFromURL(url)
		if err != nil {
			log.Error(fmt.Errorf("failed to load %s: %w", source, err))
			return
		}
		log.Debug(fmt.Sprintf("Loaded %s domains: %d", source, len(partial)))
		result = append(result, DomainBySource{Source: source, Domains: partial})
	}

	loadHosts := func(source db.BlockListSource, url string) {
		partial, err := LoadHostsFromURL(url)
		if err != nil {
			log.Error(fmt.Errorf("failed to load %s: %w", source, err))
			return
		}
		log.Debug(fmt.Sprintf("Loaded %s domains: %d", source, len(partial)))
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

// Sync downloads every active source and upserts the parsed domains into the
// blocklist via blockRepo. Errors from a single upsert abort the whole batch
// (a half-imported source would leave the bloom out-of-sync with the DB).
func Sync(repo SourceLister, blockRepo BlockWriter, log Logger) error {
	list := LoadAndParseActiveSources(repo, log)
	for _, item := range list {
		if err := blockRepo.CreateDNSRecordsByDomains(item.Domains, item.Source.String()); err != nil {
			return err
		}
	}
	return nil
}
