package seed

import (
	"testing"

	syncDb "github.com/alextorq/dns-filter/source/db"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := conn.AutoMigrate(&syncDb.Source{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

// SeedSyncs must register every BlockListSource constant that the rest of the
// code can emit. Missing SourceAutoBlocked here was the bug: auto-promoted
// domains landed in the blocked-domain table with source="AutoBlocked", but
// /api/sources never exposed that source, so operators couldn't toggle / audit
// auto-block decisions from the UI.
func TestSeedSyncs_RegistersAutoBlockedSource(t *testing.T) {
	conn := openTestDB(t)

	SeedSyncs(conn)

	expected := []syncDb.BlockListSource{
		syncDb.SourceStevenBlack,
		syncDb.SourceEasyList,
		syncDb.SourceRuAdList,
		syncDb.SourceAdGuardRussian,
		syncDb.SourceHaGeZiMulti,
		syncDb.SourceUser,
		syncDb.SourceSuggestedToBlock,
		syncDb.SourceAutoBlocked,
	}

	for _, name := range expected {
		var got syncDb.Source
		err := conn.Where("name = ?", name).First(&got).Error
		if err != nil {
			t.Errorf("source %q not seeded: %v", name, err)
			continue
		}
		if !got.Active {
			t.Errorf("source %q must be Active=true by default, got false", name)
		}
	}

	var total int64
	conn.Model(&syncDb.Source{}).Count(&total)
	if total != int64(len(expected)) {
		t.Errorf("expected exactly %d sources, got %d", len(expected), total)
	}
}

// SeedSyncs runs on every process start (main.go → source.Sync → SeedSyncs),
// so it must be idempotent: no duplicate rows, and — crucially — it must not
// resurrect a source that the operator has disabled. We rely on FirstOrCreate
// for that; this test pins the contract.
func TestSeedSyncs_IsIdempotentAndPreservesDisabledState(t *testing.T) {
	conn := openTestDB(t)

	SeedSyncs(conn)

	// Operator disables AutoBlocked from the UI.
	if err := conn.Model(&syncDb.Source{}).
		Where("name = ?", syncDb.SourceAutoBlocked).
		Update("active", false).Error; err != nil {
		t.Fatalf("disable AutoBlocked: %v", err)
	}

	// Process restarts → seed runs again.
	SeedSyncs(conn)

	var total int64
	conn.Model(&syncDb.Source{}).Count(&total)
	if total != 8 {
		t.Errorf("expected 8 sources after re-seed, got %d (duplicate insert?)", total)
	}

	var got syncDb.Source
	if err := conn.Where("name = ?", syncDb.SourceAutoBlocked).First(&got).Error; err != nil {
		t.Fatalf("load AutoBlocked: %v", err)
	}
	if got.Active {
		t.Errorf("re-seed must not flip a disabled source back to Active=true")
	}
}
