package db

import (
	"os"
	"testing"

	app_db "github.com/alextorq/dns-filter/db"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "blocked-domain-db-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	conn := app_db.GetConnection()
	if err := conn.AutoMigrate(&BlockList{}, &BlockDomainEvent{}); err != nil {
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
		conn.Unscoped().Where("url = ?", u).Delete(&BlockList{})
	}
}

// Locks in #25: a record with active=false must not be reported as blocked,
// otherwise a bloom-filter hit on a stale URL keeps blocking the domain.
func TestIsDomainActivelyBlocked(t *testing.T) {
	const (
		activeDomain   = "active.example"
		inactiveDomain = "inactive.example"
		missingDomain  = "missing.example"
	)
	t.Cleanup(func() { cleanup(t, activeDomain, inactiveDomain) })

	conn := app_db.GetConnection()
	if err := conn.Create(&BlockList{Url: activeDomain, Active: true, Source: "test"}).Error; err != nil {
		t.Fatalf("seed active: %v", err)
	}
	// Create as active first, then deactivate. Direct Active:false on Create
	// gets silently overridden by the GORM default on the Active column.
	if err := conn.Create(&BlockList{Url: inactiveDomain, Active: true, Source: "test"}).Error; err != nil {
		t.Fatalf("seed inactive: %v", err)
	}
	if err := conn.Model(&BlockList{}).Where("url = ?", inactiveDomain).Update("active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	mustCheck := func(domain string) bool {
		t.Helper()
		got, err := IsDomainActivelyBlocked(domain)
		if err != nil {
			t.Fatalf("IsDomainActivelyBlocked(%s): unexpected error %v", domain, err)
		}
		return got
	}

	if !mustCheck(activeDomain) {
		t.Errorf("active domain must be reported as blocked")
	}
	if mustCheck(inactiveDomain) {
		t.Errorf("inactive domain must NOT be reported as blocked (issue #25)")
	}
	if mustCheck(missingDomain) {
		t.Errorf("missing domain must NOT be reported as blocked")
	}

	// Flip active flag and re-check — agrees with what GetAllActiveFilters returns.
	if err := conn.Model(&BlockList{}).Where("url = ?", activeDomain).Update("active", false).Error; err != nil {
		t.Fatalf("flip active: %v", err)
	}
	if mustCheck(activeDomain) {
		t.Errorf("after deactivation domain must NOT be reported as blocked")
	}
}
