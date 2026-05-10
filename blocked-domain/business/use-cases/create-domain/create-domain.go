package create_domain

import (
	"errors"
	"fmt"

	"github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
)

// ErrDomainAlreadyExists is returned when the domain is already present in the blocklist.
var ErrDomainAlreadyExists = errors.New("domain already exists")

type RequestBody struct {
	Domain string `json:"domain"`
	Source string `json:"source"`
}

func CreateDomain(domain RequestBody) error {
	l := logger.GetLogger()
	if domain.Domain == "" {
		return fmt.Errorf("domain is empty")
	}

	if !db.DomainNotExist(domain.Domain) {
		return fmt.Errorf("%w: %s", ErrDomainAlreadyExists, domain.Domain)
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
