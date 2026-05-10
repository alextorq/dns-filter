package check_exist_domain

import (
	"os"
	"testing"

	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	app_db "github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/filter/cache"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "check-block-test-*")
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

// Locks in #25: CheckCacheOrDb must not block a deactivated domain even when
// bloom yields a hit. The previous implementation went through DomainNotExist
// which did not consider the Active column, so any bloom hit (true positive
// from a stale build, or 0.1% false positive) would resurrect the block.
func TestCheckCacheOrDb_DeactivatedDomainNotBlocked(t *testing.T) {
	const domain = "deactivated.example"

	conn := app_db.GetConnection()
	t.Cleanup(func() {
		conn.Unscoped().Where("url = ?", domain).Delete(&blocked_domain_db.BlockList{})
	})

	if err := conn.Create(&blocked_domain_db.BlockList{Url: domain, Active: true, Source: "test"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := conn.Model(&blocked_domain_db.BlockList{}).Where("url = ?", domain).Update("active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	// Cache singleton may have a stale verdict from another test — clear it so
	// CheckCacheOrDb actually consults the DB on this call.
	cache.GetCache().Clear()

	if got := CheckCacheOrDb(domain); got {
		t.Fatal("deactivated domain must not be reported as blocked")
	}
}

func TestCheckCacheOrDb_ActiveDomainBlocked(t *testing.T) {
	const domain = "active-block.example"

	conn := app_db.GetConnection()
	t.Cleanup(func() {
		conn.Unscoped().Where("url = ?", domain).Delete(&blocked_domain_db.BlockList{})
	})

	if err := conn.Create(&blocked_domain_db.BlockList{Url: domain, Active: true, Source: "test"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	cache.GetCache().Clear()

	if got := CheckCacheOrDb(domain); !got {
		t.Fatal("active domain must be reported as blocked")
	}
}
