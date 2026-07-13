package checks

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNetworkCheckConstructors_RejectMissingDependencies(t *testing.T) {
	clock := ClockFunc(time.Now)
	keys := NewCredentials()
	cases := []struct {
		name string
		call func()
	}{
		{name: "DNS resolver", call: func() { NewDNSResolve(nil) }},
		{name: "RDAP client", call: func() { NewRDAP(nil, DefaultRDAPEndpoint, clock) }},
		{name: "RDAP endpoint", call: func() { NewRDAP(http.DefaultClient, "", clock) }},
		{name: "RDAP clock", call: func() { NewRDAP(http.DefaultClient, DefaultRDAPEndpoint, nil) }},
		{name: "crt.sh client", call: func() { NewCrtSh(nil, DefaultCrtShEndpoint) }},
		{name: "crt.sh endpoint", call: func() { NewCrtSh(http.DefaultClient, "") }},
		{name: "VirusTotal client", call: func() { NewVirusTotal(nil, DefaultVirusTotalEndpoint, keys) }},
		{name: "VirusTotal endpoint", call: func() { NewVirusTotal(http.DefaultClient, "", keys) }},
		{name: "Safe Browsing client", call: func() { NewSafeBrowsing(nil, DefaultSafeBrowsingEndpoint, keys) }},
		{name: "Safe Browsing endpoint", call: func() { NewSafeBrowsing(http.DefaultClient, "", keys) }},
		{name: "URLScan client", call: func() { NewURLScan(nil, DefaultURLScanEndpoint, "key") }},
		{name: "URLScan endpoint", call: func() { NewURLScan(http.DefaultClient, "", "key") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected constructor to panic")
				}
			}()
			tc.call()
		})
	}
}

func TestNetworkCheckConstructors_RejectTypedNilDependencies(t *testing.T) {
	var client *http.Client
	var resolver *net.Resolver
	var clock ClockFunc

	for _, tc := range []struct {
		name string
		call func()
	}{
		{name: "HTTP client", call: func() { NewCrtSh(client, DefaultCrtShEndpoint) }},
		{name: "DNS resolver", call: func() { NewDNSResolve(resolver) }},
		{name: "clock", call: func() { NewRDAP(http.DefaultClient, DefaultRDAPEndpoint, clock) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected typed-nil dependency to panic")
				}
			}()
			tc.call()
		})
	}
}

func TestNewDNSResolve_AcceptsNetResolver(t *testing.T) {
	_ = NewDNSResolve(&net.Resolver{})
}

func TestHTTPCheckInstancesUseIndependentEndpoints(t *testing.T) {
	server := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
	}
	firstServer := server(`[{"name_value":"one.example","not_before":"2024-01-01"}]`)
	defer firstServer.Close()
	secondServer := server(`[
		{"name_value":"two.example","not_before":"2024-01-01"},
		{"name_value":"www.two.example","not_before":"2024-02-01"}
	]`)
	defer secondServer.Close()

	first := NewCrtSh(firstServer.Client(), firstServer.URL+"/")
	second := NewCrtSh(secondServer.Client(), secondServer.URL+"/")

	if got := first(context.Background(), "one.example").Details["certificates"]; got != 1 {
		t.Fatalf("first certificates = %v", got)
	}
	if got := second(context.Background(), "two.example").Details["certificates"]; got != 2 {
		t.Fatalf("second certificates = %v", got)
	}
}
