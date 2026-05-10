package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	app_db "github.com/alextorq/dns-filter/db"
	source_db "github.com/alextorq/dns-filter/source/db"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	// config singleton is already initialized via app_db's package-level var,
	// so we chdir to a temp directory to redirect the default ./filter.sqlite path.
	tmp, err := os.MkdirTemp("", "suggest-web-test-*")
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
		&suggest_to_block_db.SuggestBlock{},
	); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	gin.SetMode(gin.TestMode)

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func resetTables(t *testing.T) {
	t.Helper()
	conn := app_db.GetConnection()
	conn.Exec("DELETE FROM block_lists")
	conn.Exec("DELETE FROM block_domain_events")
	conn.Exec("DELETE FROM suggest_blocks")
}

func seedSuggestion(t *testing.T, domain string) uint {
	t.Helper()
	s := suggest_to_block_db.SuggestBlock{Domain: domain, Active: true}
	if err := app_db.GetConnection().Create(&s).Error; err != nil {
		t.Fatalf("seed suggestion: %v", err)
	}
	return s.ID
}

func seedBlocklist(t *testing.T, domain string) {
	t.Helper()
	b := blocked_domain_db.BlockList{
		Url:    domain,
		Active: true,
		Source: source_db.SourceSuggestedToBlock.String(),
	}
	if err := app_db.GetConnection().Create(&b).Error; err != nil {
		t.Fatalf("seed blocklist: %v", err)
	}
}

func callAddToBlock(t *testing.T, body any) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.POST("/api/suggest-to-block/add-to-block", AddToBlock)

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/suggest-to-block/add-to-block", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.Message
}

func TestAddToBlock_NewDomain_AddsAndDeactivates(t *testing.T) {
	resetTables(t)
	const domain = "fresh-add.example"
	id := seedSuggestion(t, domain)

	w := callAddToBlock(t, AddToBlockRequest{ID: id, Domain: domain})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	if blocked_domain_db.DomainNotExist(domain) {
		t.Fatal("domain should be present in blocklist")
	}

	var s suggest_to_block_db.SuggestBlock
	if err := app_db.GetConnection().First(&s, id).Error; err != nil {
		t.Fatalf("load suggestion: %v", err)
	}
	if s.Active {
		t.Fatal("suggestion should be deactivated after promotion")
	}
}

// Regression test for the 500 the user reported: when the proposed domain is
// already in the blocklist, the handler must respond 200 and just deactivate
// the suggestion.
func TestAddToBlock_DomainAlreadyInBlocklist_Returns200AndDeactivates(t *testing.T) {
	resetTables(t)
	const domain = "already-blocked.example"
	seedBlocklist(t, domain)
	id := seedSuggestion(t, domain)

	w := callAddToBlock(t, AddToBlockRequest{ID: id, Domain: domain})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	msg := decodeMessage(t, w)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	var count int64
	app_db.GetConnection().Model(&blocked_domain_db.BlockList{}).
		Where("url = ?", domain).Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly one blocklist entry for %s, got %d", domain, count)
	}

	var s suggest_to_block_db.SuggestBlock
	if err := app_db.GetConnection().First(&s, id).Error; err != nil {
		t.Fatalf("load suggestion: %v", err)
	}
	if s.Active {
		t.Fatal("suggestion should be deactivated even when blocklist already had the domain")
	}
}

func TestAddToBlock_BadJSON_Returns400(t *testing.T) {
	resetTables(t)

	r := gin.New()
	r.POST("/api/suggest-to-block/add-to-block", AddToBlock)
	req := httptest.NewRequest(http.MethodPost, "/api/suggest-to-block/add-to-block",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}
