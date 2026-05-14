package db

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlConn, err := conn.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	// SQLite :memory: is per-connection; pin to one so all queries share state.
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(&BlockList{}, &BlockDomainEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepo(conn)
}

// ----- GetByID -----

func TestRepo_GetByID_Found(t *testing.T) {
	r := newTestRepo(t)
	if err := r.CreateDomain("a.example", "test"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var seeded BlockList
	if err := r.db.Where("url = ?", "a.example").First(&seeded).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	got, err := r.GetByID(seeded.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Url != "a.example" {
		t.Errorf("expected url=a.example, got %q", got.Url)
	}
}

func TestRepo_GetByID_NotFound(t *testing.T) {
	r := newTestRepo(t)
	_, err := r.GetByID(9999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("expected ErrRecordNotFound, got %v", err)
	}
}

// ----- GetRecordsByFilter -----

func TestRepo_GetRecordsByFilter_PaginationAndFilters(t *testing.T) {
	r := newTestRepo(t)
	seed := []struct{ url, src string }{
		{"ads.example.com", "easy-list"},
		{"tracker.example.com", "easy-list"},
		{"malware.example.com", "steven-black"},
		{"other.example.com", "steven-black"},
	}
	for _, s := range seed {
		if err := r.CreateDomain(s.url, s.src); err != nil {
			t.Fatalf("seed %s: %v", s.url, err)
		}
	}

	t.Run("filter by source", func(t *testing.T) {
		res, err := r.GetRecordsByFilter(GetAllParams{Source: "easy-list", Limit: 10})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.Total != 2 || len(res.List) != 2 {
			t.Errorf("expected 2 rows, got total=%d len=%d", res.Total, len(res.List))
		}
	})

	t.Run("filter by url substring", func(t *testing.T) {
		res, err := r.GetRecordsByFilter(GetAllParams{Filter: "tracker", Limit: 10})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.Total != 1 {
			t.Errorf("expected 1 row, got %d", res.Total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		res, err := r.GetRecordsByFilter(GetAllParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.Total != 4 || len(res.List) != 2 {
			t.Errorf("expected total=4 page=2, got total=%d page=%d", res.Total, len(res.List))
		}
	})

	t.Run("no matches", func(t *testing.T) {
		res, err := r.GetRecordsByFilter(GetAllParams{Filter: "zzz", Limit: 10})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.Total != 0 || len(res.List) != 0 {
			t.Errorf("expected 0 rows, got total=%d len=%d", res.Total, len(res.List))
		}
	})
}

// ----- GetAllActiveURLs -----

func TestRepo_GetAllActiveURLs(t *testing.T) {
	r := newTestRepo(t)
	if err := r.CreateDomain("active.example", "src"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// GORM tag default:true on Active means a plain Create with Active=false is
	// silently flipped to true (zero-value→default). Seed in two steps.
	inactive := BlockList{Url: "inactive.example", Source: "src"}
	if err := r.db.Create(&inactive).Error; err != nil {
		t.Fatalf("seed inactive: %v", err)
	}
	if err := r.db.Model(&inactive).Update("active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	urls, err := r.GetAllActiveURLs()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(urls) != 1 || urls[0] != "active.example" {
		t.Errorf("expected [active.example], got %v", urls)
	}
}

func TestRepo_GetAllActiveURLs_Empty(t *testing.T) {
	r := newTestRepo(t)
	urls, err := r.GetAllActiveURLs()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(urls) != 0 {
		t.Errorf("expected empty, got %v", urls)
	}
}

// ----- DomainNotExist -----

func TestRepo_DomainNotExist(t *testing.T) {
	r := newTestRepo(t)
	if !r.DomainNotExist("missing.example") {
		t.Error("expected true for missing domain")
	}
	if err := r.CreateDomain("present.example", "src"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if r.DomainNotExist("present.example") {
		t.Error("expected false for present domain")
	}
}

// ----- IsActivelyBlocked -----

func TestRepo_IsActivelyBlocked(t *testing.T) {
	r := newTestRepo(t)

	t.Run("active record returns true", func(t *testing.T) {
		if err := r.CreateDomain("on.example", "src"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		got, err := r.IsActivelyBlocked("on.example")
		if err != nil || !got {
			t.Errorf("expected (true, nil), got (%v, %v)", got, err)
		}
	})

	t.Run("inactive record returns false", func(t *testing.T) {
		off := BlockList{Url: "off.example", Source: "src"}
		if err := r.db.Create(&off).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := r.db.Model(&off).Update("active", false).Error; err != nil {
			t.Fatalf("deactivate: %v", err)
		}
		got, err := r.IsActivelyBlocked("off.example")
		if err != nil || got {
			t.Errorf("expected (false, nil), got (%v, %v)", got, err)
		}
	})

	t.Run("missing record returns false", func(t *testing.T) {
		got, err := r.IsActivelyBlocked("never-seen.example")
		if err != nil || got {
			t.Errorf("expected (false, nil), got (%v, %v)", got, err)
		}
	})

	t.Run("closed connection surfaces error", func(t *testing.T) {
		r := newTestRepo(t)
		sqlConn, _ := r.db.DB()
		_ = sqlConn.Close()
		_, err := r.IsActivelyBlocked("anything.example")
		if err == nil {
			t.Error("expected error from closed connection, got nil")
		}
	})
}

// ----- CreateDomain -----

func TestRepo_CreateDomain(t *testing.T) {
	r := newTestRepo(t)
	if err := r.CreateDomain("new.example", "user"); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Duplicate must fail — unique index on url.
	err := r.CreateDomain("new.example", "user")
	if err == nil {
		t.Error("expected unique-violation error on duplicate, got nil")
	}
}

// ----- UpdateBlockList -----

func TestRepo_UpdateBlockList(t *testing.T) {
	r := newTestRepo(t)
	if err := r.CreateDomain("toggle.example", "src"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var rec BlockList
	if err := r.db.Where("url = ?", "toggle.example").First(&rec).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	rec.Active = false
	if err := r.UpdateBlockList(&rec); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := r.GetByID(rec.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Active {
		t.Error("expected Active=false after update")
	}
}

// ----- CreateDNSRecordsByDomains -----

func TestRepo_CreateDNSRecordsByDomains(t *testing.T) {
	r := newTestRepo(t)

	t.Run("inserts deduplicated batch", func(t *testing.T) {
		err := r.CreateDNSRecordsByDomains(
			[]string{"a.example", "b.example", "a.example"}, // dup
			"steven-black",
		)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		var n int64
		r.db.Model(&BlockList{}).Count(&n)
		if n != 2 {
			t.Errorf("expected 2 rows after dedup, got %d", n)
		}
	})

	t.Run("empty input is no-op", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateDNSRecordsByDomains(nil, "x"); err != nil {
			t.Fatalf("err: %v", err)
		}
		var n int64
		r.db.Model(&BlockList{}).Count(&n)
		if n != 0 {
			t.Errorf("expected 0 rows, got %d", n)
		}
	})

	t.Run("re-import is idempotent", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateDNSRecordsByDomains([]string{"x.example"}, "src"); err != nil {
			t.Fatalf("first: %v", err)
		}
		// Second import of the same URL must not error (BatchUpsert/DoNothing).
		if err := r.CreateDNSRecordsByDomains([]string{"x.example", "y.example"}, "src"); err != nil {
			t.Fatalf("second: %v", err)
		}
		var n int64
		r.db.Model(&BlockList{}).Count(&n)
		if n != 2 {
			t.Errorf("expected 2 rows, got %d", n)
		}
	})
}

// ----- ChangeRecordStatusBySource -----

func TestRepo_ChangeRecordStatusBySource(t *testing.T) {
	r := newTestRepo(t)
	if err := r.CreateDomain("a.example", "easy-list"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.CreateDomain("b.example", "easy-list"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.CreateDomain("c.example", "steven-black"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := r.ChangeRecordStatusBySource("easy-list", false); err != nil {
		t.Fatalf("err: %v", err)
	}
	urls, err := r.GetAllActiveURLs()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if len(urls) != 1 || urls[0] != "c.example" {
		t.Errorf("expected only c.example active, got %v", urls)
	}
}

// ----- BatchCreateBlockDomainEvents -----

func TestRepo_BatchCreateBlockDomainEvents(t *testing.T) {
	r := newTestRepo(t)
	if err := r.CreateDomain("known.example", "src"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("links known domains, ignores unknown", func(t *testing.T) {
		err := r.BatchCreateBlockDomainEvents([]string{
			"known.example", "known.example", "unknown.example",
		})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got := r.GetEventsAmount(); got != 2 {
			// Two events for "known.example", "unknown.example" skipped silently.
			t.Errorf("expected 2 events, got %d", got)
		}
	})

	t.Run("empty input is no-op", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.BatchCreateBlockDomainEvents(nil); err != nil {
			t.Fatalf("err: %v", err)
		}
		if got := r.GetEventsAmount(); got != 0 {
			t.Errorf("expected 0 events, got %d", got)
		}
	})
}

// ----- DeleteEventsOlderThan -----

func TestRepo_DeleteEventsOlderThan(t *testing.T) {
	r := newTestRepo(t)
	if err := r.CreateDomain("d.example", "src"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Insert two events: one recent, one ancient.
	if err := r.BatchCreateBlockDomainEvents([]string{"d.example"}); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	old := BlockDomainEvent{DomainId: 1}
	if err := r.db.Create(&old).Error; err != nil {
		t.Fatalf("seed old: %v", err)
	}
	// Backdate the second event to 30 days ago.
	if err := r.db.Model(&BlockDomainEvent{}).
		Where("id = ?", old.ID).
		Update("created_at", "1999-01-01").Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := r.DeleteEventsOlderThan(2); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := r.GetEventsAmount(); got != 1 {
		t.Errorf("expected 1 event left, got %d", got)
	}
}

// ----- GetEventsByDomain -----

func TestRepo_GetEventsByDomain(t *testing.T) {
	r := newTestRepo(t)
	if err := r.CreateDomain("a.example", "src"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.CreateDomain("b.example", "src"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for range 3 {
		if err := r.BatchCreateBlockDomainEvents([]string{"a.example"}); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	if err := r.BatchCreateBlockDomainEvents([]string{"b.example"}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	rows, err := r.GetEventsByDomain()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := map[string]int64{}
	for _, row := range rows {
		got[row.Domain] = row.Count
	}
	if got["a.example"] != 3 || got["b.example"] != 1 {
		t.Errorf("expected a=3,b=1, got %v", got)
	}
}
