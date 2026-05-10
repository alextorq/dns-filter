package create_domain

import (
	"errors"
	"os"
	"testing"

	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	app_db "github.com/alextorq/dns-filter/db"
)

func TestMain(m *testing.M) {
	// config singleton is already initialized via app_db's package-level var,
	// so changing DNS_FILTER_DBPATH here has no effect. Instead chdir to a temp
	// directory so the default relative ./filter.sqlite path resolves inside it.
	tmp, err := os.MkdirTemp("", "create-domain-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	conn := app_db.GetConnection()
	if err := conn.AutoMigrate(&blocked_domain_db.BlockList{}, &blocked_domain_db.BlockDomainEvent{}); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func cleanup(t *testing.T, urls ...string) {
	t.Helper()
	conn := app_db.GetConnection()
	for _, u := range urls {
		conn.Unscoped().Where("url = ?", u).Delete(&blocked_domain_db.BlockList{})
	}
}

func TestCreateDomain_NewDomain(t *testing.T) {
	const domain = "fresh.example"
	t.Cleanup(func() { cleanup(t, domain) })

	if err := CreateDomain(RequestBody{Domain: domain, Source: "test"}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if blocked_domain_db.DomainNotExist(domain) {
		t.Fatal("domain should exist in DB after CreateDomain")
	}
}

func TestCreateDomain_DuplicateReturnsSentinel(t *testing.T) {
	const domain = "dup.example"
	t.Cleanup(func() { cleanup(t, domain) })

	if err := CreateDomain(RequestBody{Domain: domain, Source: "test"}); err != nil {
		t.Fatalf("seed: expected nil, got %v", err)
	}

	err := CreateDomain(RequestBody{Domain: domain, Source: "test"})
	if err == nil {
		t.Fatal("expected error on duplicate, got nil")
	}
	if !errors.Is(err, ErrDomainAlreadyExists) {
		t.Fatalf("expected errors.Is(err, ErrDomainAlreadyExists), got %v", err)
	}
}

func TestCreateDomain_EmptyDomain(t *testing.T) {
	err := CreateDomain(RequestBody{Domain: "", Source: "test"})
	if err == nil {
		t.Fatal("expected error for empty domain, got nil")
	}
	if errors.Is(err, ErrDomainAlreadyExists) {
		t.Fatal("empty domain must not be reported as already-exists")
	}
}
