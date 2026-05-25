package db

import blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"

// AllowFilterAdapter adapts the traffic Repo to suggest-to-block's AllowRepo
// port (GetAllActiveFilters). The suggest module's candidate pool used to be
// "active allowed domains" from allow_domain_events; under the staged
// migration it is now "domains ever forwarded upstream" from domain_traffic.
// The port is unchanged — only the source of the list moves — so the consumer
// compiles without edits and only main.go wiring swaps the implementation.
type AllowFilterAdapter struct {
	repo *Repo
}

// NewAllowFilterAdapter wraps a traffic Repo so it satisfies AllowRepo.
func NewAllowFilterAdapter(repo *Repo) *AllowFilterAdapter {
	return &AllowFilterAdapter{repo: repo}
}

// GetAllActiveFilters delegates to the traffic repo's allowed-domain pool,
// propagating its error verbatim (fail-closed at the caller).
func (a *AllowFilterAdapter) GetAllActiveFilters() ([]string, error) {
	return a.repo.GetAllowedDomains()
}

// BlockStatsAdapter adapts the traffic Repo to blocked-domain/web's BlockStatsRepo
// port. Step 4 of the staged migration repoints the legacy block-stats endpoints
// (/api/events/block/*) off block_domain_events onto domain_traffic's blocked
// scope. The port returns blocked_domain_db.DomainCount so the HTTP response
// stays byte-compatible with the existing frontend; this adapter projects the
// traffic repo's DomainCount onto that type. Only main.go wiring changes — the
// blocked-domain/web handlers depend on the unchanged port.
type BlockStatsAdapter struct {
	repo *Repo
}

// NewBlockStatsAdapter wraps a traffic Repo so it satisfies BlockStatsRepo.
func NewBlockStatsAdapter(repo *Repo) *BlockStatsAdapter {
	return &BlockStatsAdapter{repo: repo}
}

// BlockedTotalCount is the grand total of blocked queries (SUM(count) WHERE
// blocked), replacing block-domain's GetEventsAmount.
func (a *BlockStatsAdapter) BlockedTotalCount() (int64, error) {
	return a.repo.TotalCount(true)
}

// BlockedCountByDomain is the per-domain blocked counts, projected onto
// blocked_domain_db.DomainCount to preserve the legacy endpoint's wire shape.
func (a *BlockStatsAdapter) BlockedCountByDomain() ([]blocked_domain_db.DomainCount, error) {
	rows, err := a.repo.CountByDomain(true)
	if err != nil {
		return nil, err
	}
	out := make([]blocked_domain_db.DomainCount, len(rows))
	for i, r := range rows {
		out[i] = blocked_domain_db.DomainCount{Domain: r.Domain, Count: r.Count}
	}
	return out, nil
}
