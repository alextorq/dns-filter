package allow_domain

import (
	allow_domain_use_cases "github.com/alextorq/dns-filter/allow-domain/business/use-cases"
	allow_domain_use_cases_clear_events "github.com/alextorq/dns-filter/allow-domain/business/use-cases/clear-events"
	allow_domain_db "github.com/alextorq/dns-filter/allow-domain/db"
)

func ClearOldEvent() {
	allow_domain_use_cases_clear_events.ClearEvent()
}

func GetAllActiveFilters() ([]string, error) {
	return allow_domain_db.GetAllActiveFilters()
}

func CreateAllowDomainEventStore(capacity int) *allow_domain_use_cases.AllowDomainEventStore {
	return allow_domain_use_cases.CreateAllowDomainEventStore(capacity)
}
