package checks

import (
	"context"
	"reflect"
	"sort"
	"testing"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

func TestNewDefaultCatalog_BuildsCompleteCatalog(t *testing.T) {
	catalog := NewDefaultCatalog(CatalogDeps{
		Blocks:      &fakeBlockLookup{},
		Allowed:     &fakeAllowLookup{allowed: true},
		Credentials: NewCredentials(),
	})

	checks := catalog.Checks()
	gotKeys := make([]string, 0, len(checks))
	for key := range checks {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	wantKeys := []string{"crtsh", "dns_resolve", "local_stats", "rdap", "safe_browsing", "urlscan", "virustotal"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("catalog keys = %v, want %v", gotKeys, wantKeys)
	}

	local := checks["local_stats"](context.Background(), "example.com")
	if got, _ := local.Details["in_allow_list"].(bool); !got {
		t.Fatal("catalog local_stats does not use injected allow lookup")
	}

	for name, check := range map[string]domain_inspect.CheckFunc{
		"rdap":          catalog.RDAP,
		"virustotal":    catalog.VirusTotal,
		"safe_browsing": catalog.SafeBrowsing,
	} {
		if check == nil {
			t.Fatalf("catalog.%s is nil", name)
		}
	}

	for _, name := range []string{"urlscan", "virustotal", "safe_browsing"} {
		if got := checks[name](context.Background(), "example.com").Status; got != domain_inspect.StatusSkipped {
			t.Fatalf("%s without a key: status = %s, want skipped", name, got)
		}
	}
}

func TestNewDefaultCatalog_RejectsMissingExternalDependencies(t *testing.T) {
	blocks := &fakeBlockLookup{}
	allowed := &fakeAllowLookup{}
	credentials := NewCredentials()
	var typedNilBlocks *fakeBlockLookup

	for _, tc := range []struct {
		name string
		deps CatalogDeps
	}{
		{name: "blocks", deps: CatalogDeps{Allowed: allowed, Credentials: credentials}},
		{name: "typed nil blocks", deps: CatalogDeps{Blocks: typedNilBlocks, Allowed: allowed, Credentials: credentials}},
		{name: "allowed", deps: CatalogDeps{Blocks: blocks, Credentials: credentials}},
		{name: "credentials", deps: CatalogDeps{Blocks: blocks, Allowed: allowed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected incomplete catalog wiring to panic")
				}
			}()
			NewDefaultCatalog(tc.deps)
		})
	}
}

func TestCatalog_ChecksReturnsFreshMap(t *testing.T) {
	catalog := NewDefaultCatalog(CatalogDeps{
		Blocks:      &fakeBlockLookup{},
		Allowed:     &fakeAllowLookup{},
		Credentials: NewCredentials(),
	})

	first := catalog.Checks()
	delete(first, "rdap")

	if _, ok := catalog.Checks()["rdap"]; !ok {
		t.Fatal("mutating one catalog map changed subsequent catalog output")
	}
}
