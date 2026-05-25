package checks

import (
	"context"

	allow_db "github.com/alextorq/dns-filter/allow-domain/db"
	blocked_db "github.com/alextorq/dns-filter/blocked-domain/db"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	"github.com/alextorq/dns-filter/db"
)

// allowLookup reports whether the server has ever forwarded this domain
// upstream (allow membership). It is an injectable package hook so the
// composition root can repoint the allow signal from the legacy
// allow_domain_events table to the unified domain_traffic counter without
// touching LocalStats' CheckFunc signature or the blocklist lookup. Default is
// the legacy reader; main.go swaps it via SetAllowLookup at startup. (Staged
// migration — see TRAFFIC_DASHBOARD_PLAN.md Step 3.)
var allowLookup = legacyAllowLookup

// SetAllowLookup repoints the allow-membership signal used by LocalStats. Call
// once at composition time, before the HTTP server starts serving.
func SetAllowLookup(fn func(domain string) (bool, error)) {
	allowLookup = fn
}

// legacyAllowLookup is the pre-migration behavior: a domain is "allowed" if it
// has an allow_domain_events row. Allow events are always written active, so
// membership and active are the same signal here.
func legacyAllowLookup(domain string) (bool, error) {
	var allowed allow_db.AllowDomainEvent
	err := db.GetConnection().Where("domain = ?", domain).First(&allowed).Error
	if err != nil {
		return false, err
	}
	return true, nil
}

// LocalStats reports what this server already knows about the domain: whether
// it is in the block/allow lists and how often it has been blocked. This is
// often the strongest signal — repeated queries from real clients say more
// than any external verdict.
//
// The blocklist lookup (in_block_list / block_list_active / block_list_source
// / block_events_total) reads the authoritative block_lists + block events and
// is unchanged. Only the allow-membership signal goes through the injectable
// allowLookup hook (traffic-backed in production, legacy events otherwise).
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

	// allow_list_active mirrors membership: a domain we have forwarded is, by
	// definition, currently allowed (the traffic counter has no "inactive"
	// state, and legacy allow events are always active).
	if inAllow, err := allowLookup(domain); err == nil && inAllow {
		details["in_allow_list"] = true
		details["allow_list_active"] = true
	} else {
		details["in_allow_list"] = false
	}

	return domain_inspect.CheckResult{
		Status:  domain_inspect.StatusOK,
		Verdict: domain_inspect.VerdictUnknown,
		Details: details,
	}
}
