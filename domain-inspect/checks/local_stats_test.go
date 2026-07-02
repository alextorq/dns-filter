package checks

import (
	"context"
	"errors"
	"testing"

	blocked_db "github.com/alextorq/dns-filter/blocked-domain/db"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

type fakeBlockLookup struct {
	record *blocked_db.BlockList
	found  bool
	err    error
	got    string
}

func (f *fakeBlockLookup) LookupByDomain(domain string) (*blocked_db.BlockList, bool, error) {
	f.got = domain
	return f.record, f.found, f.err
}

type fakeAllowLookup struct {
	allowed bool
	err     error
	got     string
}

func (f *fakeAllowLookup) IsAllowed(domain string) (bool, error) {
	f.got = domain
	return f.allowed, f.err
}

func TestNewLocalStats_RejectsMissingDependencies(t *testing.T) {
	cases := []struct {
		name  string
		block BlockLookup
		allow AllowLookup
	}{
		{name: "missing block lookup", allow: &fakeAllowLookup{}},
		{name: "missing allow lookup", block: &fakeBlockLookup{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected incomplete LocalStats wiring to panic")
				}
			}()
			NewLocalStats(tc.block, tc.allow)
		})
	}
}

func TestLocalStats_UnknownDomain(t *testing.T) {
	block := &fakeBlockLookup{}
	allow := &fakeAllowLookup{}
	check := NewLocalStats(block, allow)

	res := check(context.Background(), "unknown.example")

	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("expected OK, got %s", res.Status)
	}
	if got, _ := res.Details["in_block_list"].(bool); got {
		t.Error("unknown domain must not be in block list")
	}
	if got, _ := res.Details["in_allow_list"].(bool); got {
		t.Error("unknown domain must not be in allow list")
	}
	if block.got != "unknown.example." || allow.got != "unknown.example." {
		t.Errorf("domain was not forwarded to both lookups: block=%q allow=%q", block.got, allow.got)
	}
}

func TestLocalStats_CanonicalizesLocalLookupDomain(t *testing.T) {
	block := &fakeBlockLookup{
		record: &blocked_db.BlockList{Url: "example.com.", Active: true, Source: "test"},
		found:  true,
	}
	allow := &fakeAllowLookup{allowed: true}
	check := NewLocalStats(block, allow)

	res := check(context.Background(), " Example.COM.. ")

	if block.got != "example.com." || allow.got != "example.com." {
		t.Fatalf("lookups must receive canonical FQDN: block=%q allow=%q", block.got, allow.got)
	}
	if got, _ := res.Details["in_block_list"].(bool); !got {
		t.Error("canonical block-list lookup was not reflected in result")
	}
	if got, _ := res.Details["in_allow_list"].(bool); !got {
		t.Error("canonical traffic lookup was not reflected in result")
	}
}

func TestLocalStats_BlockedDomain(t *testing.T) {
	block := &fakeBlockLookup{
		record: &blocked_db.BlockList{Url: "blocked.example", Active: true, Source: "test"},
		found:  true,
	}
	check := NewLocalStats(block, &fakeAllowLookup{})

	res := check(context.Background(), "blocked.example")

	if got, _ := res.Details["in_block_list"].(bool); !got {
		t.Error("expected in_block_list=true")
	}
	if got, _ := res.Details["block_list_active"].(bool); !got {
		t.Error("expected block_list_active=true")
	}
	if got, _ := res.Details["block_list_source"].(string); got != "test" {
		t.Errorf("source: got %q, want test", got)
	}
	if _, present := res.Details["block_events_total"]; present {
		t.Error("block_events_total must not be emitted")
	}
}

func TestLocalStats_AllowedDomain(t *testing.T) {
	check := NewLocalStats(&fakeBlockLookup{}, &fakeAllowLookup{allowed: true})

	res := check(context.Background(), "allowed.example")

	if got, _ := res.Details["in_allow_list"].(bool); !got {
		t.Error("expected in_allow_list=true")
	}
	if got, _ := res.Details["allow_list_active"].(bool); !got {
		t.Error("expected allow_list_active=true")
	}
}

func TestLocalStats_LookupErrorsFailOpen(t *testing.T) {
	check := NewLocalStats(
		&fakeBlockLookup{err: errors.New("block db down")},
		&fakeAllowLookup{err: errors.New("traffic db down")},
	)

	res := check(context.Background(), "whatever.example")

	if res.Status != domain_inspect.StatusOK {
		t.Fatalf("LocalStats must stay OK on lookup errors, got %s", res.Status)
	}
	if got, _ := res.Details["in_block_list"].(bool); got {
		t.Error("expected in_block_list=false when block lookup errors")
	}
	if got, _ := res.Details["in_allow_list"].(bool); got {
		t.Error("expected in_allow_list=false when allow lookup errors")
	}
}

func TestLocalStats_InstancesAreIndependent(t *testing.T) {
	allowed := NewLocalStats(&fakeBlockLookup{}, &fakeAllowLookup{allowed: true})
	unknown := NewLocalStats(&fakeBlockLookup{}, &fakeAllowLookup{})

	if got, _ := allowed(context.Background(), "same.example").Details["in_allow_list"].(bool); !got {
		t.Error("first instance must use its own allow lookup")
	}
	if got, _ := unknown(context.Background(), "same.example").Details["in_allow_list"].(bool); got {
		t.Error("second instance must not inherit the first instance's allow lookup")
	}
}
