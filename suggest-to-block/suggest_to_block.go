package suggest_to_block

import (
	"time"

	allow_domain "github.com/alextorq/dns-filter/allow-domain"
	blocked_domain "github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/logger"
	suggest_to_block_use_cases_collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

func CreateSuggestBlockBatch(suggests []suggest_to_block_db.SuggestBlock) error {
	return suggest_to_block_db.CreateSuggestBlockBatch(suggests)
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
	suggests := make([]suggest_to_block_db.SuggestBlock, len(forBlock))

	for _, domain := range forBlock {
		suggests = append(suggests, suggest_to_block_db.SuggestBlock{
			Domain: domain.Domain,
			Reason: domain.Reason,
			Score:  domain.Score,
		})
	}

	err = CreateSuggestBlockBatch(suggests)
	if err != nil {
		return err
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
