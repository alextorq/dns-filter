package create_domain

import (
	"fmt"

	black_lists "github.com/alextorq/dns-filter/dns-records"
	"github.com/alextorq/dns-filter/logger"
	use_cases "github.com/alextorq/dns-filter/use-cases"
)

type RequestBody struct {
	Domain string `json:"domain"`
}

func CreateDomain(domain RequestBody) error {
	l := logger.GetLogger()
	if domain.Domain == "" {
		return fmt.Errorf("domain is empty")
	}

	if !black_lists.DomainNotExist(domain.Domain) {
		wrap := fmt.Errorf("domain %s already exists", domain.Domain)
		return wrap
	}

	err := black_lists.CreateDomain(domain.Domain)
	if err != nil {
		wrap := fmt.Errorf("error create domain: %w", err)
		l.Error(wrap)
		return wrap
	} else {
		l.Info("Domain created:", domain.Domain)
	}

	err = use_cases.UpdateFilterFromDb()
	if err != nil {
		wrap := fmt.Errorf("error update filter from db when change record: %w", err)
		l.Error(wrap)
		return wrap
	}
	return err
}
