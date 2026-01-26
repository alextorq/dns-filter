package suggest_to_block

import (
	"time"

	allow_domain "github.com/alextorq/dns-filter/allow-domain"
	blocked_domain "github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/logger"
	suggest_to_block_use_cases_collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

func CreateSuggestBlock(domain string, reason string, score int) error {
	return suggest_to_block_db.CreateSuggestBlock(domain, reason, score)
}

func GetRecordsByFilter(params suggest_to_block_db.GetAllParams) (*suggest_to_block_db.GetAllResult, error) {
	return suggest_to_block_db.GetAllSuggestBlocks(params)
}

func ChangeActiveStatus(id uint, active bool) error {
	return suggest_to_block_db.UpdateActiveStatus(id, active)
}

func Collect() error {
	l := logger.GetLogger()
	l.Info("Start collecting suggestions to block domains")
	blocked, err := blocked_domain.GetAllActiveFilters()
	if err != nil {
		return err
	}

	allowed, err := allow_domain.GetAllActiveFilters()
	if err != nil {
		return err
	}

	forBlock := suggest_to_block_use_cases_collect.CollectSuggest(blocked, allowed)

	for _, domain := range forBlock {
		err := CreateSuggestBlock(domain.Domain, domain.Reason, domain.Score)
		if err != nil {
			l := logger.GetLogger()
			l.Error(err)
		}
	}
	l.Info("Finished collecting suggestions to block domains")
	return nil
}

func StartCollectSuggest() {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	var tryCollect = func() {
		l := logger.GetLogger()
		err := Collect()
		if err != nil {
			l.Error(err)
		}
	}

	tryCollect()

	for range ticker.C {
		tryCollect()
	}
}
