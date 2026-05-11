package checks

import (
	"context"

	allow_db "github.com/alextorq/dns-filter/allow-domain/db"
	blocked_db "github.com/alextorq/dns-filter/blocked-domain/db"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	"github.com/alextorq/dns-filter/db"
)

// LocalStats reports what this server already knows about the domain: whether
// it is in the block/allow lists and how often it has been blocked. This is
// often the strongest signal — repeated queries from real clients say more
// than any external verdict.
func LocalStats(_ context.Context, domain string) domain_inspect.CheckResult {
	conn := db.GetConnection()

	details := map[string]any{}

	var blocked blocked_db.BlockList
	if err := conn.Where("url = ?", domain).First(&blocked).Error; err == nil {
		details["in_block_list"] = true
		details["block_list_active"] = blocked.Active
		details["block_list_source"] = blocked.Source

		var count int64
		conn.Model(&blocked_db.BlockDomainEvent{}).
			Where("domain_id = ?", blocked.ID).
			Count(&count)
		details["block_events_total"] = count
	} else {
		details["in_block_list"] = false
	}

	var allowed allow_db.AllowDomainEvent
	if err := conn.Where("domain = ?", domain).First(&allowed).Error; err == nil {
		details["in_allow_list"] = true
		details["allow_list_active"] = allowed.Active
	} else {
		details["in_allow_list"] = false
	}

	return domain_inspect.CheckResult{
		Status:  domain_inspect.StatusOK,
		Verdict: domain_inspect.VerdictUnknown,
		Details: details,
	}
}
