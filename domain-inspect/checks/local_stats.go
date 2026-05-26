package checks

import (
	"context"

	blocked_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/db"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// allowLookup reports whether the server has ever forwarded this domain
// upstream (allow membership). It is an injectable package hook so the
// composition root supplies the real reader (traffic-backed IsAllowed) without
// touching LocalStats' CheckFunc signature or the blocklist lookup. The default
// is a harmless no-op: the legacy allow_domain_events table was removed in the
// traffic-dashboard migration, so any consumer that cares about allow
// membership MUST inject a lookup via SetAllowLookup at startup (production
// wires trafficRepo.IsAllowed). Tests set their own.
var allowLookup = noopAllowLookup

// SetAllowLookup repoints the allow-membership signal used by LocalStats. Call
// once at composition time, before the HTTP server starts serving.
func SetAllowLookup(fn func(domain string) (bool, error)) {
	allowLookup = fn
}

// noopAllowLookup is the safe default before SetAllowLookup is called: it
// reports "not allowed" without touching the DB, so LocalStats never panics on
// the removed allow_domain_events table when no real lookup was injected.
func noopAllowLookup(string) (bool, error) {
	return false, nil
}

// LocalStats reports what this server already knows about the domain: whether
// it is in the block/allow lists and how often it has been blocked. This is
// often the strongest signal — repeated queries from real clients say more
// than any external verdict.
//
// The blocklist lookup (in_block_list / block_list_active / block_list_source)
// reads the authoritative block_lists table and is unchanged. The legacy
// per-domain block-event count was dropped together with block_domain_events in
// the traffic-dashboard migration. Only the allow-membership signal goes through
// the injectable allowLookup hook (traffic-backed in production).
func LocalStats(_ context.Context, domain string) domain_inspect.CheckResult {
	conn := db.GetConnection()

	details := map[string]any{}

	var blocked blocked_db.BlockList
	if err := conn.Where("url = ?", domain).First(&blocked).Error; err == nil {
		details["in_block_list"] = true
		details["block_list_active"] = blocked.Active
		details["block_list_source"] = blocked.Source
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
