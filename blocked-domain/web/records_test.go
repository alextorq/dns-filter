package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	update_dns_record "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/update-dns-record"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	app_db "github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/filter"
	filtercache "github.com/alextorq/dns-filter/filter/cache"
	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "blocked-domain-web-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}
	conn := app_db.GetConnection()
	if err := conn.AutoMigrate(
		&blocked_domain_db.BlockList{},
		&blocked_domain_db.BlockDomainEvent{},
	); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}
	gin.SetMode(gin.TestMode)
	if err := filter.UpdateFilterFromDb(); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func resetTables(t *testing.T) {
	t.Helper()
	conn := app_db.GetConnection()
	conn.Exec("DELETE FROM block_lists")
	conn.Exec("DELETE FROM block_domain_events")
	if err := filter.UpdateFilterFromDb(); err != nil {
		t.Fatalf("refresh filter: %v", err)
	}
}

func postJSON(t *testing.T, path string, h gin.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.POST(path, h)
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Locks in #23 and #26: after POST /api/dns-records/create, the new domain
// must be blockable on the DNS hot path immediately (without restart).
func TestCreateDnsRecords_RefreshesFilterAndCache(t *testing.T) {
	resetTables(t)
	const domain = "fresh-create.example."

	if filter.CheckExist(domain) {
		t.Fatal("precondition: domain must not be in filter before create")
	}

	w := postJSON(t, "/api/dns-records/create", CreateDnsRecords,
		create_domain.RequestBody{Domain: domain})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	if !filter.CheckExist(domain) {
		t.Fatal("domain must be blockable immediately after create — bloom/cache not refreshed")
	}
}

// Locks in #24 and #26: deactivating a record via POST /api/dns-records/update
// must stop blocking immediately. The previous code skipped UpdateFilterFromDb,
// so the bloom and the LRU cache kept the stale verdict until process restart.
func TestChangeDnsRecordActive_DeactivationLiftsBlock(t *testing.T) {
	resetTables(t)
	const domain = "toggle.example."

	rec := blocked_domain_db.BlockList{Url: domain, Active: true, Source: "test"}
	if err := app_db.GetConnection().Create(&rec).Error; err != nil {
		t.Fatalf("seed record: %v", err)
	}
	if err := filter.UpdateFilterFromDb(); err != nil {
		t.Fatalf("seed filter: %v", err)
	}

	if !filter.CheckExist(domain) {
		t.Fatal("precondition: domain must be blocked before deactivation")
	}

	// Prime the LRU cache with the stale `true` verdict to cover #26 directly.
	filtercache.GetCache().Add(domain, true)

	w := postJSON(t, "/api/dns-records/update", ChangeDnsRecordActive,
		update_dns_record.UpdateBlockList{ID: rec.ID, Active: false})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	if _, ok := filtercache.GetCache().Get(domain); ok {
		t.Fatal("LRU cache must be cleared after update — stale verdict survives")
	}
	if filter.CheckExist(domain) {
		t.Fatal("domain must no longer be blocked after deactivation")
	}
}
