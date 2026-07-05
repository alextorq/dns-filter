package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	update_dns_record "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/update-dns-record"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// fakeLog ignores everything; handler tests assert behavior via the HTTP
// response, not log output.
type fakeLog struct{}

func (fakeLog) Info(args ...any) {}
func (fakeLog) Error(err error)  {}

type harness struct {
	t        *testing.T
	repo     *blocked_domain_db.Repo
	handlers *Handlers
	refresh  *refreshSpy
}

type refreshSpy struct {
	calls int
	err   error
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
	if err := conn.AutoMigrate(&blocked_domain_db.BlockList{}, &blocked_domain_db.BlockListReason{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := blocked_domain_db.NewRepo(conn)
	spy := &refreshSpy{}
	h := &Handlers{
		Records:       repo,
		Creator:       repo,
		Updater:       repo,
		Log:           fakeLog{},
		RefreshFilter: func() error { spy.calls++; return spy.err },
	}
	return &harness{t: t, repo: repo, handlers: h, refresh: spy}
}

type fakeRecordsRepo struct {
	got    blocked_domain_db.GetAllParams
	result blocked_domain_db.GetRecordsResult
	err    error
}

func (f *fakeRecordsRepo) GetRecordsByFilter(params blocked_domain_db.GetAllParams) (blocked_domain_db.GetRecordsResult, error) {
	f.got = params
	return f.result, f.err
}

type fakeCreateRepo struct {
	createErr error
	domain    string
	source    string
}

func (*fakeCreateRepo) DomainNotExist(string) bool { return true }
func (f *fakeCreateRepo) CreateDomain(domain, source string) error {
	f.domain, f.source = domain, source
	return f.createErr
}
func (f *fakeCreateRepo) CreateDomainWithReasons(string, string, []create_domain.Reason) error {
	return errors.New("unexpected reason path")
}

type fakeUpdateRepo struct {
	record    *blocked_domain_db.BlockList
	getErr    error
	updateErr error
	gotID     uint
	updated   *blocked_domain_db.BlockList
}

func (f *fakeUpdateRepo) GetByID(id uint) (*blocked_domain_db.BlockList, error) {
	f.gotID = id
	return f.record, f.getErr
}
func (f *fakeUpdateRepo) UpdateBlockList(record *blocked_domain_db.BlockList) error {
	f.updated = record
	return f.updateErr
}

func (h *harness) postJSON(path string, fn gin.HandlerFunc, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	r := gin.New()
	r.POST(path, fn)
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Locks in #23 and #26: after POST /api/dns-records/create, the new domain
// must be in the DB and the in-memory filter must be refreshed.
func TestCreateDnsRecords_PersistsAndRefreshes(t *testing.T) {
	h := newHarness(t)
	const domain = "fresh-create.example."

	w := h.postJSON("/api/dns-records/create", h.handlers.CreateDnsRecords,
		create_domain.RequestBody{Domain: domain})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if h.refresh.calls != 1 {
		t.Errorf("expected RefreshFilter called once, got %d", h.refresh.calls)
	}
	if h.repo.DomainNotExist(domain) {
		t.Error("domain must exist in DB after create")
	}
}

// Negative case for create: an empty domain must return 400 (sentinel
// ErrEmptyDomain → BadRequest) and must not refresh the filter.
func TestCreateDnsRecords_RejectsEmpty(t *testing.T) {
	h := newHarness(t)
	w := h.postJSON("/api/dns-records/create", h.handlers.CreateDnsRecords,
		create_domain.RequestBody{Domain: ""})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty domain, got %d (body=%s)", w.Code, w.Body.String())
	}
	if h.refresh.calls != 0 {
		t.Errorf("RefreshFilter must not be called when create fails, got %d", h.refresh.calls)
	}
}

// Locks in #24 and #26: deactivating a record via POST /api/dns-records/update
// must persist Active=false and trigger filter refresh — otherwise bloom/LRU
// keep blocking until process restart.
func TestChangeDnsRecordActive_DeactivatesAndRefreshes(t *testing.T) {
	h := newHarness(t)
	const domain = "toggle.example."
	if err := h.repo.CreateDomain(domain, "test"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := h.repo.GetRecordsByFilter(blocked_domain_db.GetAllParams{Limit: 10})
	if err != nil {
		t.Fatalf("seed lookup: %v", err)
	}
	if len(res.List) != 1 {
		t.Fatalf("expected 1 row seeded, got %d", len(res.List))
	}
	seeded := res.List[0]

	w := h.postJSON("/api/dns-records/update", h.handlers.ChangeDnsRecordActive,
		update_dns_record.UpdateBlockList{ID: seeded.ID, Active: false})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if h.refresh.calls != 1 {
		t.Errorf("expected RefreshFilter called once, got %d", h.refresh.calls)
	}
	got, err := h.repo.GetByID(seeded.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Active {
		t.Error("record must be inactive after update")
	}
}

func TestGetAllDnsRecords_UsesNarrowReaderAndForwardsFilter(t *testing.T) {
	repo := &fakeRecordsRepo{result: blocked_domain_db.GetRecordsResult{
		Total: 1,
		List:  []blocked_domain_db.BlockList{{Url: "ads.example."}},
	}}
	h := &Handlers{Records: repo, Log: fakeLog{}}
	req := GetAllDnsRecordsRequest{Limit: 25, Offset: 10, Filter: "ads", Source: "EasyList"}

	w := (&harness{t: t}).postJSON("/api/dns-records", h.GetAllDnsRecords, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	want := blocked_domain_db.GetAllParams{Limit: 25, Offset: 10, Filter: "ads", Source: "EasyList"}
	if repo.got != want {
		t.Fatalf("filter = %+v, want %+v", repo.got, want)
	}
	var response GetAllDnsRecordsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Total != 1 || len(response.List) != 1 || response.List[0].Url != "ads.example." {
		t.Fatalf("response = %+v", response)
	}
}

func TestGetAllDnsRecords_ReaderErrorReturns500(t *testing.T) {
	h := &Handlers{Records: &fakeRecordsRepo{err: errors.New("db down")}, Log: fakeLog{}}
	w := (&harness{t: t}).postJSON("/api/dns-records", h.GetAllDnsRecords, GetAllDnsRecordsRequest{})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestCreateDnsRecords_StorageErrorSkipsRefresh(t *testing.T) {
	creator := &fakeCreateRepo{createErr: errors.New("db down")}
	refresh := &refreshSpy{}
	h := &Handlers{
		Creator: creator,
		Log:     fakeLog{},
		RefreshFilter: func() error {
			refresh.calls++
			return nil
		},
	}

	w := (&harness{t: t}).postJSON("/api/dns-records/create", h.CreateDnsRecords, create_domain.RequestBody{Domain: "Ads.Example"})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if creator.domain != "ads.example." || creator.source != "User" {
		t.Fatalf("create args = domain %q source %q", creator.domain, creator.source)
	}
	if refresh.calls != 0 {
		t.Fatalf("refresh calls = %d, want 0", refresh.calls)
	}
}

func TestChangeDnsRecordActive_RepoFailuresSkipRefresh(t *testing.T) {
	boom := errors.New("db down")
	for _, tc := range []struct {
		name        string
		repo        *fakeUpdateRepo
		wantUpdated bool
	}{
		{name: "get", repo: &fakeUpdateRepo{getErr: boom}},
		{name: "update", repo: &fakeUpdateRepo{record: &blocked_domain_db.BlockList{}, updateErr: boom}, wantUpdated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refresh := &refreshSpy{}
			h := &Handlers{
				Updater: tc.repo,
				Log:     fakeLog{},
				RefreshFilter: func() error {
					refresh.calls++
					return nil
				},
			}

			w := (&harness{t: t}).postJSON("/api/dns-records/update", h.ChangeDnsRecordActive,
				update_dns_record.UpdateBlockList{ID: 9, Active: true})

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", w.Code)
			}
			if tc.repo.gotID != 9 {
				t.Fatalf("GetByID arg = %d, want 9", tc.repo.gotID)
			}
			if tc.wantUpdated && (tc.repo.updated == nil || !tc.repo.updated.Active) {
				t.Fatalf("updated record = %+v, want Active=true", tc.repo.updated)
			}
			if refresh.calls != 0 {
				t.Fatalf("refresh calls = %d, want 0", refresh.calls)
			}
		})
	}
}
