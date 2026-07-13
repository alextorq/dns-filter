package checks

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
)

type fakeDNSResolver struct {
	hosts   []string
	mxs     []*net.MX
	nss     []*net.NS
	hostErr error
	calls   []string
}

func (r *fakeDNSResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	r.calls = append(r.calls, "host:"+host)
	if r.hostErr != nil {
		return nil, r.hostErr
	}
	return append([]string(nil), r.hosts...), nil
}

func TestDNSResolve_HostLookupFailureReturnsUnknownWithoutSecondaryQueries(t *testing.T) {
	resolver := &fakeDNSResolver{hostErr: errors.New("resolver unavailable")}

	result := NewDNSResolve(resolver)(context.Background(), "example.com")

	if result.Details["resolved"] != false {
		t.Fatalf("resolved = %v, want false", result.Details["resolved"])
	}
	if result.Details["lookup_error"] != "resolver unavailable" {
		t.Fatalf("lookup_error = %v", result.Details["lookup_error"])
	}
	if want := []string{"host:example.com"}; !reflect.DeepEqual(resolver.calls, want) {
		t.Fatalf("calls = %v, want %v", resolver.calls, want)
	}
}

func (r *fakeDNSResolver) LookupMX(_ context.Context, host string) ([]*net.MX, error) {
	r.calls = append(r.calls, "mx:"+host)
	return append([]*net.MX(nil), r.mxs...), nil
}

func (r *fakeDNSResolver) LookupNS(_ context.Context, host string) ([]*net.NS, error) {
	r.calls = append(r.calls, "ns:"+host)
	return append([]*net.NS(nil), r.nss...), nil
}

func TestDNSResolve_UsesInjectedResolver(t *testing.T) {
	resolver := &fakeDNSResolver{
		hosts: []string{"192.0.2.10"},
		mxs:   []*net.MX{{Host: "mail.example.com."}},
		nss:   []*net.NS{{Host: "ns.example.com."}},
	}

	result := NewDNSResolve(resolver)(context.Background(), "example.com")

	if got := result.Details["addresses"]; !reflect.DeepEqual(got, []string{"192.0.2.10"}) {
		t.Fatalf("addresses = %v", got)
	}
	if got := result.Details["mx"]; !reflect.DeepEqual(got, []string{"mail.example.com"}) {
		t.Fatalf("mx = %v", got)
	}
	if got := result.Details["ns"]; !reflect.DeepEqual(got, []string{"ns.example.com"}) {
		t.Fatalf("ns = %v", got)
	}
	if want := []string{"host:example.com", "mx:example.com", "ns:example.com"}; !reflect.DeepEqual(resolver.calls, want) {
		t.Fatalf("calls = %v, want %v", resolver.calls, want)
	}
}

func TestDNSResolve_InstancesAreIndependent(t *testing.T) {
	first := &fakeDNSResolver{hosts: []string{"192.0.2.1"}}
	second := &fakeDNSResolver{hosts: []string{"192.0.2.2"}}

	firstResult := NewDNSResolve(first)(context.Background(), "one.example")
	secondResult := NewDNSResolve(second)(context.Background(), "two.example")

	if got := firstResult.Details["addresses"]; !reflect.DeepEqual(got, []string{"192.0.2.1"}) {
		t.Fatalf("first addresses = %v", got)
	}
	if got := secondResult.Details["addresses"]; !reflect.DeepEqual(got, []string{"192.0.2.2"}) {
		t.Fatalf("second addresses = %v", got)
	}
}
