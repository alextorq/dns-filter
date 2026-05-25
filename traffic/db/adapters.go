package db

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
