package db

import (
	"errors"
	"fmt"
	"testing"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
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
	if err := conn.AutoMigrate(&BlockList{}, &BlockListReason{}, &BlockDomainEvent{}); err != nil {
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

func TestRepo_GetRecordsByFilter_RelevanceOrder(t *testing.T) {
	// Все четыре домена содержат подстроку "mail.ru", но релевантность разная:
	// точное совпадение → поддомен → префикс → произвольная подстрока.
	seed := []string{
		"webmail.ru",           // подстрока  → tier 3
		"mail.ru.phishing.com", // префикс    → tier 2
		"ads.mail.ru",          // поддомен   → tier 1
		"mail.ru",              // точное     → tier 0
	}

	t.Run("exact and subdomain rank above substring", func(t *testing.T) {
		r := newTestRepo(t)
		for _, u := range seed {
			if err := r.CreateDomain(u, "easy-list"); err != nil {
				t.Fatalf("seed %s: %v", u, err)
			}
		}
		res, err := r.GetRecordsByFilter(GetAllParams{Filter: "mail.ru", Limit: 10})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := []string{"mail.ru", "ads.mail.ru", "mail.ru.phishing.com", "webmail.ru"}
		if len(res.List) != len(want) {
			t.Fatalf("got %d rows, want %d", len(res.List), len(want))
		}
		for i, w := range want {
			if res.List[i].Url != w {
				got := make([]string, len(res.List))
				for j, rec := range res.List {
					got[j] = rec.Url
				}
				t.Fatalf("order mismatch: got %v, want %v", got, want)
			}
		}
	})

	t.Run("limit keeps the most relevant match", func(t *testing.T) {
		r := newTestRepo(t)
		for _, u := range seed {
			if err := r.CreateDomain(u, "easy-list"); err != nil {
				t.Fatalf("seed %s: %v", u, err)
			}
		}
		// При Limit=1 должно вернуться точное совпадение, а не случайная подстрока.
		res, err := r.GetRecordsByFilter(GetAllParams{Filter: "mail.ru", Limit: 1})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.Total != 4 {
			t.Errorf("expected total=4, got %d", res.Total)
		}
		if len(res.List) != 1 || res.List[0].Url != "mail.ru" {
			t.Errorf("expected only exact match on page 1, got %v", res.List)
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
		// Second import of the same URL must not error (BatchUpsertOn/DoNothing).
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

// ----- DeleteDNSRecordsBySourceNotIn -----

func countBlockList(t *testing.T, r *Repo) int64 {
	t.Helper()
	var n int64
	r.db.Model(&BlockList{}).Count(&n)
	return n
}

func urlsOfSource(t *testing.T, r *Repo, source string) map[string]struct{} {
	t.Helper()
	var urls []string
	if err := r.db.Model(&BlockList{}).Where("source = ?", source).Pluck("url", &urls).Error; err != nil {
		t.Fatalf("pluck %s: %v", source, err)
	}
	set := make(map[string]struct{}, len(urls))
	for _, u := range urls {
		set[u] = struct{}{}
	}
	return set
}

func TestRepo_DeleteDNSRecordsBySourceNotIn(t *testing.T) {
	t.Run("drops domains absent from keep, leaves the rest", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateDNSRecordsByDomains([]string{"a.example", "b.example", "c.example"}, "src"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// b исчезла из всех источников — keep её не содержит.
		if err := r.DeleteDNSRecordsBySourceNotIn("src", []string{"a.example", "c.example"}); err != nil {
			t.Fatalf("prune: %v", err)
		}
		got := urlsOfSource(t, r, "src")
		if len(got) != 2 {
			t.Fatalf("got %v, want 2 domains", got)
		}
		for _, u := range []string{"a.example", "c.example"} {
			if _, ok := got[u]; !ok {
				t.Errorf("missing %q", u)
			}
		}
		if _, ok := got["b.example"]; ok {
			t.Error("b.example должна быть удалена как отсутствующая в keep")
		}
	})

	t.Run("empty keep is a no-op and never wipes the source", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateDNSRecordsByDomains([]string{"a.example", "b.example"}, "src"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// Сбойный синк → пустой keep. Источник стирать нельзя.
		if err := r.DeleteDNSRecordsBySourceNotIn("src", nil); err != nil {
			t.Fatalf("prune nil: %v", err)
		}
		if err := r.DeleteDNSRecordsBySourceNotIn("src", []string{}); err != nil {
			t.Fatalf("prune empty: %v", err)
		}
		if n := countBlockList(t, r); n != 2 {
			t.Errorf("источник вычистился на пустом keep: got %d rows, want 2", n)
		}
	})

	t.Run("touches only the named source", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateDNSRecordsByDomains([]string{"a.example", "b.example"}, "easylist"); err != nil {
			t.Fatalf("seed easylist: %v", err)
		}
		if err := r.CreateDNSRecordsByDomains([]string{"c.example", "d.example"}, "ruadlist"); err != nil {
			t.Fatalf("seed ruadlist: %v", err)
		}
		// Ручная запись — её синк не трогает, даже когда её url нет в keep.
		if err := r.CreateDomain("manual.example", "User"); err != nil {
			t.Fatalf("seed user: %v", err)
		}

		// Прун easylist с keep, где нет ни b.example, ни ruadlist-, ни User-доменов.
		if err := r.DeleteDNSRecordsBySourceNotIn("easylist", []string{"a.example"}); err != nil {
			t.Fatalf("prune easylist: %v", err)
		}

		if easy := urlsOfSource(t, r, "easylist"); len(easy) != 1 {
			t.Errorf("easylist=%v, want only a.example", easy)
		}
		if ru := urlsOfSource(t, r, "ruadlist"); len(ru) != 2 {
			t.Errorf("ruadlist затронут: %v", ru)
		}
		if user := urlsOfSource(t, r, "User"); len(user) != 1 {
			t.Errorf("ручная запись затронута: %v", user)
		}
	})

	t.Run("keeps id and created_at of surviving rows", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateDNSRecordsByDomains([]string{"keep.example", "gone.example"}, "src"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		var before BlockList
		if err := r.db.Where("url = ?", "keep.example").First(&before).Error; err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if err := r.DeleteDNSRecordsBySourceNotIn("src", []string{"keep.example"}); err != nil {
			t.Fatalf("prune: %v", err)
		}
		var after BlockList
		if err := r.db.Where("url = ?", "keep.example").First(&after).Error; err != nil {
			t.Fatalf("lookup after: %v", err)
		}
		if after.ID != before.ID {
			t.Errorf("id уцелевшей строки сменился: %d → %d (история block_domain_events осиротела бы)", before.ID, after.ID)
		}
		if !after.CreatedAt.Equal(before.CreatedAt) {
			t.Errorf("created_at уцелевшей строки сброшен: %v → %v", before.CreatedAt, after.CreatedAt)
		}
	})

	t.Run("idempotent on repeated prune", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateDNSRecordsByDomains([]string{"a.example", "b.example"}, "src"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		keep := []string{"a.example"}
		if err := r.DeleteDNSRecordsBySourceNotIn("src", keep); err != nil {
			t.Fatalf("first: %v", err)
		}
		if err := r.DeleteDNSRecordsBySourceNotIn("src", keep); err != nil {
			t.Fatalf("second: %v", err)
		}
		if n := countBlockList(t, r); n != 1 {
			t.Errorf("повторный прун изменил число строк: got %d, want 1", n)
		}
	})

	t.Run("removes block_domain_events of pruned domains, keeps the rest", func(t *testing.T) {
		r := newTestRepo(t)
		if err := r.CreateDNSRecordsByDomains([]string{"stays.example", "vanishes.example"}, "src"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// По событию блокировки на каждый домен.
		if err := r.BatchCreateBlockDomainEvents([]string{"stays.example", "vanishes.example"}); err != nil {
			t.Fatalf("seed events: %v", err)
		}
		if err := r.DeleteDNSRecordsBySourceNotIn("src", []string{"stays.example"}); err != nil {
			t.Fatalf("prune: %v", err)
		}

		var total int64
		r.db.Model(&BlockDomainEvent{}).Count(&total)
		if total != 1 {
			t.Errorf("осталось %d событий, want 1 — события удалённого домена не вычищены", total)
		}
		var stays BlockList
		if err := r.db.Where("url = ?", "stays.example").First(&stays).Error; err != nil {
			t.Fatalf("lookup stays: %v", err)
		}
		var orphans int64
		r.db.Model(&BlockDomainEvent{}).Where("domain_id <> ?", stays.ID).Count(&orphans)
		if orphans != 0 {
			t.Errorf("остались осиротевшие события: %d", orphans)
		}
	})

	t.Run("prunes a large vanished set across delete batches", func(t *testing.T) {
		r := newTestRepo(t)
		const n = 5000 // > staleDeleteBatch, заставляет удаление идти пакетами
		seed := make([]string, n)
		for i := range seed {
			seed[i] = fmt.Sprintf("d%d.example", i)
		}
		if err := r.CreateDNSRecordsByDomains(seed, "src"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := r.BatchCreateBlockDomainEvents(seed); err != nil {
			t.Fatalf("seed events: %v", err)
		}
		if err := r.DeleteDNSRecordsBySourceNotIn("src", []string{"d0.example"}); err != nil {
			t.Fatalf("prune: %v", err)
		}
		if got := countBlockList(t, r); got != 1 {
			t.Errorf("got %d rows after large prune, want 1", got)
		}
		var events int64
		r.db.Model(&BlockDomainEvent{}).Count(&events)
		if events != 1 {
			t.Errorf("got %d events after large prune, want 1 (события удалённых не вычищены)", events)
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

// ----- CreateDomainWithReasons -----

// TestRepo_CreateDomainWithReasons_PersistsReasons — happy-path: домен и его
// reason-коды ложатся в block_lists / block_list_reasons, связаны FK, и
// читаются обратно через Preload (#95).
func TestRepo_CreateDomainWithReasons_PersistsReasons(t *testing.T) {
	r := newTestRepo(t)
	reasons := []create_domain.Reason{
		{Code: "subdomain_of_blocked", Match: "example.com"},
		{Code: "suspicious_entropy"},
	}
	if err := r.CreateDomainWithReasons("ads.example.com", "AutoBlocked", reasons); err != nil {
		t.Fatalf("CreateDomainWithReasons: %v", err)
	}

	var got BlockList
	if err := r.db.Preload("Reasons").Where("url = ?", "ads.example.com").First(&got).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Source != "AutoBlocked" || !got.Active {
		t.Errorf("row stored wrong: source=%q active=%v", got.Source, got.Active)
	}
	if len(got.Reasons) != 2 {
		t.Fatalf("expected 2 reasons, got %d (%+v)", len(got.Reasons), got.Reasons)
	}
	byCode := map[string]BlockListReason{}
	for _, rs := range got.Reasons {
		if rs.BlockListID != got.ID {
			t.Errorf("reason %q FK=%d, want block_list id %d", rs.Code, rs.BlockListID, got.ID)
		}
		byCode[rs.Code] = rs
	}
	if byCode["subdomain_of_blocked"].MatchValue != "example.com" {
		t.Errorf("match value lost, got %q", byCode["subdomain_of_blocked"].MatchValue)
	}
	if _, ok := byCode["suspicious_entropy"]; !ok {
		t.Errorf("reason without match value not stored, got %+v", got.Reasons)
	}
}

// TestRepo_CreateDomainWithReasons_RollbackOnReasonFailure — негатив: если
// запись reasons падает (имитируем дропом block_list_reasons), вся транзакция
// откатывается и домен в block_lists не остаётся (#95 AC: одна транзакция).
func TestRepo_CreateDomainWithReasons_RollbackOnReasonFailure(t *testing.T) {
	r := newTestRepo(t)
	if err := r.db.Migrator().DropTable(&BlockListReason{}); err != nil {
		t.Fatalf("drop block_list_reasons: %v", err)
	}

	err := r.CreateDomainWithReasons("ads.example.com", "AutoBlocked",
		[]create_domain.Reason{{Code: "subdomain_of_blocked"}})
	if err == nil {
		t.Fatal("expected error when reason table is missing, got nil")
	}

	var count int64
	r.db.Model(&BlockList{}).Where("url = ?", "ads.example.com").Count(&count)
	if count != 0 {
		t.Errorf("block_lists row must be rolled back on reason failure, got %d rows", count)
	}
}

func TestBlockDomainEvent_DomainIdIsIndexed(t *testing.T) {
	r := newTestRepo(t)
	m := r.db.Migrator()
	// Positive: the high-volume events table must be indexed on the column its
	// join (GetEventsByDomain) and prune (DeleteDNSRecordsBySourceNotIn) filter
	// on, or both degrade to full scans as the table grows.
	if !m.HasIndex(&BlockDomainEvent{}, "DomainId") {
		t.Error("expected an index on BlockDomainEvent.DomainId")
	}
	// Negative guard: the migration indexes DomainId specifically, not every
	// column — CreatedAt is deliberately left unindexed.
	if m.HasIndex(&BlockDomainEvent{}, "CreatedAt") {
		t.Error("CreatedAt should not be indexed; only DomainId is")
	}
}
