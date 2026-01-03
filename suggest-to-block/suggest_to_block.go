package suggest_to_block

import (
	"time"

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

func StartCollectSuggest() {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	suggest_to_block_use_cases_collect.CollectSuggest(nil, nil)

	for range ticker.C {
		suggest_to_block_use_cases_collect.CollectSuggest(nil, nil)
	}
}
