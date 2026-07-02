package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/discovery"
	"github.com/alextorq/dns-filter/clients/use-cases/create"
	"github.com/alextorq/dns-filter/clients/use-cases/update"
	"github.com/alextorq/dns-filter/config"
	"github.com/gin-gonic/gin"
)

type fakeService struct {
	created create.Input
}

func (*fakeService) List() ([]db.Client, error) { return nil, nil }
func (s *fakeService) Create(in create.Input) (*db.Client, error) {
	s.created = in
	return &db.Client{ID: 1, IP: in.IP, Filtered: in.Filtered}, nil
}
func (*fakeService) Update(update.Input) (*db.Client, error) { return nil, nil }
func (*fakeService) ChangeFilter(uint, bool) (*db.Client, error) { return nil, nil }
func (*fakeService) Remove(uint) error                            { return nil }
func (*fakeService) Discover(context.Context, discovery.DiscoverOptions) (*discovery.Result, error) {
	return &discovery.Result{}, nil
}

type noopLogger struct{}

func (noopLogger) Error(error) {}

func TestCreateClient_PreservesExplicitFilteredFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{}
	h := &Handlers{Service: service, Log: noopLogger{}, Mode: config.ModeLAN}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/clients/create", strings.NewReader(`{"ip":"10.0.0.8","filtered":false}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateClient(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if service.created.Filtered {
		t.Fatal("explicit filtered=false was replaced by the default")
	}
	var body ClientResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Client.Filtered {
		t.Fatal("response did not preserve filtered=false")
	}
}

func TestCreateClient_DefaultsFilteredTrueWhenOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{}
	h := &Handlers{Service: service, Log: noopLogger{}, Mode: config.ModeLAN}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/clients/create", strings.NewReader(`{"ip":"10.0.0.8"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateClient(c)

	if w.Code != http.StatusOK || !service.created.Filtered {
		t.Fatalf("omitted filtered must default true: code=%d input=%+v", w.Code, service.created)
	}
}
