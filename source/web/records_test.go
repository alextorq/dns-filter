package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	source_db "github.com/alextorq/dns-filter/source/db"
	"github.com/gin-gonic/gin"
)

type fakeSourceRepo struct {
	list       []source_db.Source
	total      int64
	getAllErr  error
	source     *source_db.Source
	getByIDErr error
	updateErr  error
	calls      *[]string
	gotID      uint
	updated    *source_db.Source
}

func (f *fakeSourceRepo) GetAll(source_db.GetAllParams) ([]source_db.Source, error) {
	return f.list, f.getAllErr
}
func (f *fakeSourceRepo) Amount() int64 { return f.total }
func (f *fakeSourceRepo) GetByID(id uint) (*source_db.Source, error) {
	f.gotID = id
	return f.source, f.getByIDErr
}
func (f *fakeSourceRepo) Update(source *source_db.Source) error {
	f.updated = source
	if f.calls != nil {
		*f.calls = append(*f.calls, "source-update")
	}
	return f.updateErr
}

type fakeBlockRepo struct {
	err    error
	calls  *[]string
	source string
	active bool
}

func (f *fakeBlockRepo) ChangeRecordStatusBySource(source string, active bool) error {
	f.source = source
	f.active = active
	if f.calls != nil {
		*f.calls = append(*f.calls, "block-update")
	}
	return f.err
}

type fakeFilter struct {
	err   error
	calls *[]string
}

func (f *fakeFilter) UpdateFromDb() error {
	if f.calls != nil {
		*f.calls = append(*f.calls, "filter-refresh")
	}
	return f.err
}

type fakeLogger struct{}

func (fakeLogger) Info(...any) {}
func (fakeLogger) Error(error) {}

func performSourceRequest(h *Handlers, method, path string, body []byte) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/sources", h.GetAllSources)
	r.POST("/sources/change-status", h.ChangeSourceActive)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGetAllSources_ReturnsRepoListAndTotal(t *testing.T) {
	repo := &fakeSourceRepo{
		list:  []source_db.Source{{ID: 1, Name: source_db.SourceEasyList, Active: true}},
		total: 1,
	}
	h := &Handlers{Repo: repo, Log: fakeLogger{}}

	w := performSourceRequest(h, http.MethodPost, "/sources", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var got GetAllSourcesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 1 || len(got.List) != 1 || got.List[0].Name != source_db.SourceEasyList {
		t.Fatalf("response = %+v", got)
	}
}

func TestGetAllSources_RepoErrorReturns500(t *testing.T) {
	h := &Handlers{Repo: &fakeSourceRepo{getAllErr: errors.New("db down")}, Log: fakeLogger{}}
	w := performSourceRequest(h, http.MethodPost, "/sources", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestChangeSourceActive_UpdatesInRequiredOrder(t *testing.T) {
	calls := []string{}
	source := &source_db.Source{ID: 7, Name: source_db.SourceEasyList, Active: false}
	repo := &fakeSourceRepo{source: source, calls: &calls}
	blocks := &fakeBlockRepo{calls: &calls}
	h := &Handlers{
		Repo:      repo,
		BlockRepo: blocks,
		Filter:    &fakeFilter{calls: &calls},
		Log:       fakeLogger{},
	}

	w := performSourceRequest(h, http.MethodPost, "/sources/change-status", []byte(`{"id":7,"active":true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if !source.Active {
		t.Fatal("source active flag was not updated")
	}
	if repo.gotID != 7 || repo.updated != source {
		t.Fatalf("repo args: id=%d updated=%p, want id=7 source=%p", repo.gotID, repo.updated, source)
	}
	if blocks.source != source_db.SourceEasyList.String() || !blocks.active {
		t.Fatalf("block args: source=%q active=%v", blocks.source, blocks.active)
	}
	want := []string{"source-update", "block-update", "filter-refresh"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestChangeSourceActive_FailuresStopPipeline(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name       string
		body       []byte
		repo       *fakeSourceRepo
		block      *fakeBlockRepo
		filter     *fakeFilter
		wantStatus int
		wantCalls  []string
	}{
		{name: "invalid json", body: []byte(`{`), repo: &fakeSourceRepo{}, block: &fakeBlockRepo{}, filter: &fakeFilter{}, wantStatus: 400},
		{name: "get source", body: []byte(`{"id":1}`), repo: &fakeSourceRepo{getByIDErr: boom}, block: &fakeBlockRepo{}, filter: &fakeFilter{}, wantStatus: 500},
		{name: "update source", body: []byte(`{"id":1}`), repo: &fakeSourceRepo{source: &source_db.Source{Name: source_db.SourceEasyList}, updateErr: boom}, block: &fakeBlockRepo{}, filter: &fakeFilter{}, wantStatus: 500, wantCalls: []string{"source-update"}},
		{name: "update block rows", body: []byte(`{"id":1}`), repo: &fakeSourceRepo{source: &source_db.Source{Name: source_db.SourceEasyList}}, block: &fakeBlockRepo{err: boom}, filter: &fakeFilter{}, wantStatus: 500, wantCalls: []string{"source-update", "block-update"}},
		{name: "refresh filter", body: []byte(`{"id":1}`), repo: &fakeSourceRepo{source: &source_db.Source{Name: source_db.SourceEasyList}}, block: &fakeBlockRepo{}, filter: &fakeFilter{err: boom}, wantStatus: 500, wantCalls: []string{"source-update", "block-update", "filter-refresh"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			tc.repo.calls = &calls
			tc.block.calls = &calls
			tc.filter.calls = &calls
			h := &Handlers{Repo: tc.repo, BlockRepo: tc.block, Filter: tc.filter, Log: fakeLogger{}}

			w := performSourceRequest(h, http.MethodPost, "/sources/change-status", tc.body)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, tc.wantCalls)
			}
		})
	}
}
