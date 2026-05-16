package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	source_db "github.com/alextorq/dns-filter/source/db"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	"github.com/alextorq/dns-filter/utils"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type silentLog struct{}

func (silentLog) Info(args ...any) {}
func (silentLog) Error(err error)  {}

// callLog records the strict order of cross-component calls so order-of-
// operations tests can assert "X happened before Y" rather than just "both
// were called". Mutex-guarded so concurrent handler invocations stay safe.
type callLog struct {
	mu  sync.Mutex
	log []string
}

func (c *callLog) record(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = append(c.log, name)
}

func (c *callLog) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.log))
	copy(out, c.log)
	return out
}

type refreshSpy struct {
	events *callLog
	calls  int
	err    error
}

func (s *refreshSpy) UpdateFromDb() error {
	s.calls++
	s.events.record("filter")
	return s.err
}

// recordingSuggestRepo wraps the real Repo so tests can both observe the
// order of writes and inject errors (UpdateActive) without standing up
// extra fixtures.
type recordingSuggestRepo struct {
	inner           SuggestRepo
	events          *callLog
	updateActiveErr error
}

func (r *recordingSuggestRepo) GetByFilter(p suggest_to_block_db.GetAllParams) (*suggest_to_block_db.GetAllResult, error) {
	return r.inner.GetByFilter(p)
}

func (r *recordingSuggestRepo) UpdateActive(id uint, active bool) error {
	if r.updateActiveErr != nil {
		return r.updateActiveErr
	}
	err := r.inner.UpdateActive(id, active)
	if err == nil {
		r.events.record("update-active")
	}
	return err
}

type harness struct {
	t        *testing.T
	conn     *gorm.DB
	handlers *Handlers
	refresh  *refreshSpy
	suggest  *recordingSuggestRepo
	events   *callLog
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlConn, err := conn.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(
		&blocked_domain_db.BlockList{},
		&blocked_domain_db.BlockDomainEvent{},
		&suggest_to_block_db.SuggestBlock{},
		&suggest_to_block_db.SuggestBlockReason{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	blockRepo := blocked_domain_db.NewRepo(conn)
	suggestRepo := suggest_to_block_db.NewRepo(conn)

	events := &callLog{}
	recRepo := &recordingSuggestRepo{inner: suggestRepo, events: events}
	spy := &refreshSpy{events: events}
	h := &Handlers{
		Repo:      recRepo,
		BlockRepo: blockRepo,
		Filter:    spy,
		Log:       silentLog{},
	}
	return &harness{t: t, conn: conn, handlers: h, refresh: spy, suggest: recRepo, events: events}
}

func (h *harness) seedSuggestion(domain string) uint {
	h.t.Helper()
	s := suggest_to_block_db.SuggestBlock{Domain: domain, Active: true}
	if err := h.conn.Create(&s).Error; err != nil {
		h.t.Fatalf("seed suggestion: %v", err)
	}
	return s.ID
}

func (h *harness) seedBlocklist(domain string) {
	h.t.Helper()
	b := blocked_domain_db.BlockList{
		// block_lists всегда хранит домены в канонической FQDN-форме (#30).
		Url:    utils.CanonicalDomain(domain),
		Active: true,
		Source: source_db.SourceSuggestedToBlock.String(),
	}
	if err := h.conn.Create(&b).Error; err != nil {
		h.t.Fatalf("seed blocklist: %v", err)
	}
}

func (h *harness) callAddToBlock(body any) *httptest.ResponseRecorder {
	h.t.Helper()
	r := gin.New()
	r.POST("/api/suggest-to-block/add-to-block", h.handlers.AddToBlock)

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatalf("encode body: %v", err)
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
	h := newHarness(t)
	const domain = "fresh-add.example"
	id := h.seedSuggestion(domain)

	w := h.callAddToBlock(AddToBlockRequest{ID: id, Domain: domain})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	if blocked_domain_db.NewRepo(h.conn).DomainNotExist(utils.CanonicalDomain(domain)) {
		t.Fatal("domain should be present in blocklist")
	}
	if h.refresh.calls != 1 {
		t.Errorf("expected RefreshFilter called once, got %d", h.refresh.calls)
	}

	var s suggest_to_block_db.SuggestBlock
	if err := h.conn.First(&s, id).Error; err != nil {
		t.Fatalf("load suggestion: %v", err)
	}
	if s.Active {
		t.Fatal("suggestion should be deactivated after promotion")
	}
}

// Regression: when the proposed domain is already in the blocklist, the
// handler must respond 200, deactivate the suggestion, and NOT trigger a
// (pointless) bloom rebuild.
func TestAddToBlock_DomainAlreadyInBlocklist_Returns200AndDeactivates(t *testing.T) {
	h := newHarness(t)
	const domain = "already-blocked.example"
	h.seedBlocklist(domain)
	id := h.seedSuggestion(domain)

	w := h.callAddToBlock(AddToBlockRequest{ID: id, Domain: domain})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	msg := decodeMessage(t, w)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	var count int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Where("url = ?", utils.CanonicalDomain(domain)).Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly one blocklist entry for %s, got %d", domain, count)
	}

	var s suggest_to_block_db.SuggestBlock
	if err := h.conn.First(&s, id).Error; err != nil {
		t.Fatalf("load suggestion: %v", err)
	}
	if s.Active {
		t.Fatal("suggestion should be deactivated even when blocklist already had the domain")
	}
	if h.refresh.calls != 0 {
		t.Errorf("RefreshFilter must not be called when nothing was added, got %d", h.refresh.calls)
	}
}

// TestGetSignalCodes pins the wire contract: handler returns a non-empty
// list and every entry exposes code + label.
func TestGetSignalCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &Handlers{}
	r.GET("/api/suggest-to-block/codes", h.GetSignalCodes)

	req := httptest.NewRequest(http.MethodGet, "/api/suggest-to-block/codes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	var resp GetSignalCodesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.List) == 0 {
		t.Fatal("expected non-empty signal catalog")
	}
	for i, s := range resp.List {
		if s.Code == "" {
			t.Errorf("entry %d has empty code", i)
		}
		if s.Label == "" {
			t.Errorf("entry %d (%s) has empty label", i, s.Code)
		}
	}
}

func TestAddToBlock_BadJSON_Returns400(t *testing.T) {
	h := newHarness(t)

	r := gin.New()
	r.POST("/api/suggest-to-block/add-to-block", h.handlers.AddToBlock)
	req := httptest.NewRequest(http.MethodPost, "/api/suggest-to-block/add-to-block",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// AddToBlock must deactivate the suggestion BEFORE rebuilding the bloom.
// Otherwise, a crash between the two would leave the suggestion active in
// the UI while bloom already reflects the new entry — operator sees the
// suggestion still "pending" yet the domain is already blocked.
func TestAddToBlock_DeactivatesBeforeRefreshingFilter(t *testing.T) {
	h := newHarness(t)
	const domain = "order-test.example"
	id := h.seedSuggestion(domain)

	w := h.callAddToBlock(AddToBlockRequest{ID: id, Domain: domain})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	got := h.events.snapshot()
	want := []string{"update-active", "filter"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected order %v, got %v", want, got)
	}
}

// Negative path: filter rebuild fails after the suggestion is already
// deactivated. Handler must surface 500 — operator needs to know the bloom
// is now out of sync with the DB.
func TestAddToBlock_FilterRefreshError_Returns500(t *testing.T) {
	h := newHarness(t)
	h.refresh.err = errors.New("filter rebuild failed")
	const domain = "refresh-fail.example"
	id := h.seedSuggestion(domain)

	w := h.callAddToBlock(AddToBlockRequest{ID: id, Domain: domain})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// Negative path: writing the new domain succeeded but UpdateActive on the
// suggest row failed. Handler must surface 500 and MUST NOT trigger the
// bloom rebuild — the suggest row still says "pending", so a stale UI is
// preferable to a half-applied state.
func TestAddToBlock_UpdateActiveError_Returns500AndSkipsRefresh(t *testing.T) {
	h := newHarness(t)
	h.suggest.updateActiveErr = errors.New("update failed")
	const domain = "update-fail.example"
	id := h.seedSuggestion(domain)

	w := h.callAddToBlock(AddToBlockRequest{ID: id, Domain: domain})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d (body=%s)", w.Code, w.Body.String())
	}
	if h.refresh.calls != 0 {
		t.Errorf("filter rebuild must NOT run when UpdateActive failed, got %d calls", h.refresh.calls)
	}
}

// Same negative path for ChangeActiveStatus.
func TestChangeActiveStatus_UpdateError_Returns500(t *testing.T) {
	h := newHarness(t)
	h.suggest.updateActiveErr = errors.New("update failed")

	r := gin.New()
	r.POST("/api/suggest-to-block/change-status", h.handlers.ChangeActiveStatus)
	body, _ := json.Marshal(ChangeSuggestStatusRequest{ID: 1, Active: true})
	req := httptest.NewRequest(http.MethodPost, "/api/suggest-to-block/change-status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestChangeActiveStatus_BadJSON_Returns400(t *testing.T) {
	h := newHarness(t)

	r := gin.New()
	r.POST("/api/suggest-to-block/change-status", h.handlers.ChangeActiveStatus)
	req := httptest.NewRequest(http.MethodPost, "/api/suggest-to-block/change-status",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}
