package suggest_to_block

import (
	"time"

	allow_domain "github.com/alextorq/dns-filter/allow-domain"
	blocked_domain "github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/logger"
	suggest_to_block_use_cases_collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

func CollectSuggest(blockedDomains []string, allowedDomains []string) {
	suggest_to_block_use_cases_collect.CollectSuggest(blockedDomains, allowedDomains)
}

func CreateSuggestBlock(domain string) (*suggest_to_block_db.SuggestBlock, error) {
	return suggest_to_block_db.CreateSuggestBlock(domain)
}

func DeleteSuggestBlock(domain string) error {
	return suggest_to_block_db.DeleteSuggestBlock(domain)
}

func Collect() error {
	blocked, err := blocked_domain.GetAllActiveFilters()
	if err != nil {
		return err
	}

	allowed, err := allow_domain.GetAllActiveFilters()
	if err != nil {
		return err
	}
	suggest_to_block_use_cases_collect.CollectSuggest(blocked, allowed)
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
