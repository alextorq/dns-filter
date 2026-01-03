package suggest_to_block

import (
	suggest_to_block_use_cases_collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

// Facade functions for collect use-case
func CollectSuggest(blockedDomains []string, allowedDomains []string) {
	suggest_to_block_use_cases_collect.CollectSuggest(blockedDomains, allowedDomains)
}

// Facade functions for database operations
func CreateSuggestBlock(domain string) (*suggest_to_block_db.SuggestBlock, error) {
	return suggest_to_block_db.CreateSuggestBlock(domain)
}

func DeleteSuggestBlock(domain string) error {
	return suggest_to_block_db.DeleteSuggestBlock(domain)
}
