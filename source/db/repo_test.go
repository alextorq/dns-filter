package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) (*Repo, *gorm.DB) {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlConn, err := conn.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(&Source{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepo(conn), conn
}

// Seed must register every BlockListSource constant that the rest of the
// code can emit. Missing SourceAutoBlocked at one point caused auto-promoted
// domains to land in block_lists with source="AutoBlocked" while /api/sources
// did not expose that source — operators couldn't toggle / audit auto-block
// decisions from the UI.
func TestRepo_Seed_RegistersEveryKnownSource(t *testing.T) {
	repo, conn := newTestRepo(t)

	repo.Seed()

	expected := []BlockListSource{
		SourceStevenBlack,
		SourceEasyList,
		SourceRuAdList,
		SourceAdGuardRussian,
		SourceHaGeZiMulti,
		SourceUser,
		SourceSuggestedToBlock,
		SourceAutoBlocked,
	}

	for _, name := range expected {
		var got Source
		if err := conn.Where("name = ?", name).First(&got).Error; err != nil {
			t.Errorf("source %q not seeded: %v", name, err)
			continue
		}
		if !got.Active {
			t.Errorf("source %q must be Active=true by default, got false", name)
		}
	}

	var total int64
	conn.Model(&Source{}).Count(&total)
	if total != int64(len(expected)) {
		t.Errorf("expected exactly %d sources, got %d", len(expected), total)
	}
}

// Seed runs on every process start, so it must be idempotent: no duplicate
// rows, and — crucially — it must not resurrect a source that the operator
// has disabled. We rely on FirstOrCreate for that; this test pins the
// contract.
func TestRepo_Seed_IsIdempotentAndPreservesDisabledState(t *testing.T) {
	repo, conn := newTestRepo(t)

	repo.Seed()

	if err := conn.Model(&Source{}).
		Where("name = ?", SourceAutoBlocked).
		Update("active", false).Error; err != nil {
		t.Fatalf("disable AutoBlocked: %v", err)
	}

	repo.Seed()

	var total int64
	conn.Model(&Source{}).Count(&total)
	if total != 8 {
		t.Errorf("expected 8 sources after re-seed, got %d (duplicate insert?)", total)
	}

	var got Source
	if err := conn.Where("name = ?", SourceAutoBlocked).First(&got).Error; err != nil {
		t.Fatalf("load AutoBlocked: %v", err)
	}
	if got.Active {
		t.Errorf("re-seed must not flip a disabled source back to Active=true")
	}
}

// IsActive: missing row → false (fail-closed). Pinned by the same logic the
// suggest module relies on for the AutoBlocked kill-switch.
func TestRepo_IsActive(t *testing.T) {
	repo, conn := newTestRepo(t)

	t.Run("missing row reports inactive", func(t *testing.T) {
		got, err := repo.IsActive(SourceAutoBlocked)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got {
			t.Error("missing row must be inactive (fail-closed)")
		}
	})

	t.Run("seeded active row reports true", func(t *testing.T) {
		if err := conn.Create(&Source{Name: SourceUser, Active: true}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		got, err := repo.IsActive(SourceUser)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !got {
			t.Error("active source must report true")
		}
	})

	t.Run("disabled row reports false", func(t *testing.T) {
		if err := conn.Create(&Source{Name: SourceEasyList, Active: false}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		got, err := repo.IsActive(SourceEasyList)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got {
			t.Error("disabled source must report false")
		}
	})

	t.Run("DB error surfaces", func(t *testing.T) {
		repo, conn := newTestRepo(t)
		sqlConn, _ := conn.DB()
		_ = sqlConn.Close()
		if _, err := repo.IsActive(SourceUser); err == nil {
			t.Error("expected error from closed connection")
		}
	})
}

// GetAllActive must skip rows where Active=false. Pinned because the
// LoadAndParseActiveSources hot path uses this slice directly to decide
// what to download — leaking a disabled source would re-enable it.
func TestRepo_GetAllActive_SkipsDisabled(t *testing.T) {
	repo, conn := newTestRepo(t)
	if err := conn.Create(&Source{Name: SourceEasyList, Active: true}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	off := Source{Name: SourceUser}
	if err := conn.Create(&off).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := conn.Model(&off).Update("active", false).Error; err != nil {
		t.Fatalf("disable: %v", err)
	}

	rows, err := repo.GetAllActive()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != SourceEasyList {
		t.Errorf("expected only EasyList, got %+v", rows)
	}
}
