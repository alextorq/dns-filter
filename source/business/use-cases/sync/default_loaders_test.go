package sync

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	easy_list "github.com/alextorq/dns-filter/source/business/use-cases/sync/easy-list"
	"github.com/alextorq/dns-filter/source/db"
)

type defaultDoerFunc func(*http.Request) (*http.Response, error)

func (f defaultDoerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestNewDefaultLoaders_WiresEveryRemoteSource(t *testing.T) {
	expected := map[db.BlockListSource]string{
		db.SourceEasyList:       easy_list.EasyListURL,
		db.SourceRuAdList:       easy_list.RuAdListURL,
		db.SourceAdGuardRussian: easy_list.AdGuardRussianURL,
		db.SourceStevenBlack:    StevenBlackURL,
		db.SourceHaGeZiMulti:    HaGeZiMultiURL,
	}
	requestedURL := ""
	client := defaultDoerFunc(func(req *http.Request) (*http.Response, error) {
		requestedURL = req.URL.String()
		body := "0.0.0.0 loaded.example.com\n"
		if requestedURL == easy_list.EasyListURL || requestedURL == easy_list.RuAdListURL || requestedURL == easy_list.AdGuardRussianURL {
			body = "||loaded.example.com^\n"
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})

	loaders, err := NewDefaultLoaders(client)
	if err != nil {
		t.Fatalf("NewDefaultLoaders failed: %v", err)
	}
	if len(loaders) != len(expected) {
		t.Fatalf("loader count = %d, want %d", len(loaders), len(expected))
	}
	for source, endpoint := range expected {
		domains, err := loaders[source].Load(context.Background())
		if err != nil {
			t.Fatalf("load %s: %v", source, err)
		}
		if requestedURL != endpoint {
			t.Fatalf("source %s requested %q, want %q", source, requestedURL, endpoint)
		}
		if len(domains) != 1 || domains[0] != "loaded.example.com." {
			t.Fatalf("source %s domains = %v, want parsed response", source, domains)
		}
	}
}

func TestNewDefaultLoaders_RejectsTypedNilHTTPClient(t *testing.T) {
	var client *http.Client
	if _, err := NewDefaultLoaders(client); err == nil {
		t.Fatal("NewDefaultLoaders accepted typed-nil HTTP client")
	}
}
