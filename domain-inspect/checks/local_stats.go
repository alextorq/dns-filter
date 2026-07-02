package checks

import (
	"context"

	blocked_db "github.com/alextorq/dns-filter/blocked-domain/db"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	"github.com/alextorq/dns-filter/utils"
)

type BlockLookup interface {
	LookupByDomain(domain string) (*blocked_db.BlockList, bool, error)
}

type AllowLookup interface {
	IsAllowed(domain string) (bool, error)
}

// NewLocalStats builds the local-knowledge check from explicit block-list and
// traffic readers. Both repositories are composed in main; the returned check
// has no package-global state and independent instances cannot affect each
// other.
func NewLocalStats(blocks BlockLookup, allowed AllowLookup) domain_inspect.CheckFunc {
	if blocks == nil {
		panic("domain-inspect/checks: block lookup is required")
	}
	if allowed == nil {
		panic("domain-inspect/checks: allow lookup is required")
	}

	return func(_ context.Context, domain string) domain_inspect.CheckResult {
		return localStats(blocks, allowed, domain)
	}
}

func localStats(blocks BlockLookup, allowed AllowLookup, domain string) domain_inspect.CheckResult {
	details := map[string]any{}
	canonical := utils.CanonicalDomain(domain)

	blocked, found, err := blocks.LookupByDomain(canonical)
	if err == nil && found && blocked != nil {
		details["in_block_list"] = true
		details["block_list_active"] = blocked.Active
		details["block_list_source"] = blocked.Source
	} else {
		details["in_block_list"] = false
	}

	// allow_list_active mirrors membership: a domain we have forwarded is, by
	// definition, currently allowed (the traffic counter has no "inactive"
	// state, and legacy allow events are always active).
	if inAllow, err := allowed.IsAllowed(canonical); err == nil && inAllow {
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
