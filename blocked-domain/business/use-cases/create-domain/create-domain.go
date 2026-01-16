package create_domain

import (
	"fmt"

	"github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
)

type RequestBody struct {
	Domain string             `json:"domain"`
	Source db.BlockListSource `json:"source"`
}

func CreateDomain(domain RequestBody) error {
	l := logger.GetLogger()
	if domain.Domain == "" {
		return fmt.Errorf("domain is empty")
	}

	if !db.DomainNotExist(domain.Domain) {
		wrap := fmt.Errorf("domain %s already exists", domain.Domain)
		return wrap
	}

	err := db.CreateDomain(domain.Domain, domain.Source)
	if err != nil {
		wrap := fmt.Errorf("error create domain: %w", err)
		l.Error(wrap)
		return wrap
	}

	l.Info("Domain created:", domain.Domain)

	return nil
}
