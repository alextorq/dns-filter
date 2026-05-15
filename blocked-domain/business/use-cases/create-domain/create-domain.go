package create_domain

import (
	"errors"
	"fmt"
)

// ErrDomainAlreadyExists is returned when the domain is already present in the blocklist.
var ErrDomainAlreadyExists = errors.New("domain already exists")

// ErrEmptyDomain is a sentinel for empty-string input — the HTTP layer maps
// it to 400 rather than 500.
var ErrEmptyDomain = errors.New("domain is empty")

type RequestBody struct {
	Domain string `json:"domain"`
	Source string `json:"source"`
}

// Repo is the output port: the narrow slice of blocklist storage this use-case
// needs. Implemented by *blocked-domain/db.Repo via structural typing.
type Repo interface {
	DomainNotExist(domain string) bool
	CreateDomain(domain, source string) error
}

type Logger interface {
	Info(args ...any)
	Error(err error)
}

type Deps struct {
	Repo Repo
	Log  Logger
}

// CreateDomain validates the request, refuses duplicates, and writes the new
// row through the repository port. The caller (HTTP handler, suggest worker)
// is responsible for refreshing the in-memory bloom filter afterwards.
func CreateDomain(d Deps, req RequestBody) error {
	if req.Domain == "" {
		return ErrEmptyDomain
	}

	if !d.Repo.DomainNotExist(req.Domain) {
		return fmt.Errorf("%w: %s", ErrDomainAlreadyExists, req.Domain)
	}

	if err := d.Repo.CreateDomain(req.Domain, req.Source); err != nil {
		wrap := fmt.Errorf("error create domain: %w", err)
		d.Log.Error(wrap)
		return wrap
	}

	d.Log.Info("Domain created:", req.Domain)
	return nil
}
