package dns

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	dnsLib "github.com/miekg/dns"
)

// dohTestServer stands up a minimal DoH endpoint that answers every query with
// a single A record pointing at ip. The httptest host is 127.0.0.1 (an IP), so
// the resolver skips bootstrap dialing — no bootstrap IPs needed.
func dohTestServer(t *testing.T, ip string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", http.StatusBadRequest)
			return
		}
		req := new(dnsLib.Msg)
		if err := req.Unpack(body); err != nil {
			http.Error(w, "unpack", http.StatusBadRequest)
			return
		}
		resp := new(dnsLib.Msg)
		resp.SetReply(req)
		resp.Answer = append(resp.Answer, &dnsLib.A{
			Hdr: dnsLib.RR_Header{Name: req.Question[0].Name, Rrtype: dnsLib.TypeA, Class: dnsLib.ClassINET, Ttl: 60},
			A:   net.ParseIP(ip),
		})
		packed, err := resp.Pack()
		if err != nil {
			http.Error(w, "pack", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(packed)
	}))
}

func query(name string) *dnsLib.Msg {
	m := new(dnsLib.Msg)
	m.SetQuestion(dnsLib.Fqdn(name), dnsLib.TypeA)
	return m
}

func answerIP(t *testing.T, m *dnsLib.Msg) string {
	t.Helper()
	if len(m.Answer) == 0 {
		t.Fatal("response had no answer records")
	}
	a, ok := m.Answer[0].(*dnsLib.A)
	if !ok {
		t.Fatalf("expected A record, got %T", m.Answer[0])
	}
	return a.A.String()
}

func TestReloadableResolver_ExchangeDelegates(t *testing.T) {
	srv := dohTestServer(t, "1.2.3.4")
	defer srv.Close()

	r := NewReloadableResolver(srv.URL)
	out, err := r.Exchange(context.Background(), query("example.com"))
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if got := answerIP(t, out); got != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", got)
	}
}

// SetEndpoint must repoint the resolver so the next Exchange hits the new
// upstream — the whole point of making the upstream runtime-configurable.
func TestReloadableResolver_SetEndpointSwitchesUpstream(t *testing.T) {
	first := dohTestServer(t, "1.1.1.1")
	defer first.Close()
	second := dohTestServer(t, "2.2.2.2")
	defer second.Close()

	r := NewReloadableResolver(first.URL)

	out, err := r.Exchange(context.Background(), query("example.com"))
	if err != nil {
		t.Fatalf("exchange before swap: %v", err)
	}
	if got := answerIP(t, out); got != "1.1.1.1" {
		t.Fatalf("expected first upstream 1.1.1.1, got %s", got)
	}

	r.SetEndpoint(second.URL)

	out, err = r.Exchange(context.Background(), query("example.com"))
	if err != nil {
		t.Fatalf("exchange after swap: %v", err)
	}
	if got := answerIP(t, out); got != "2.2.2.2" {
		t.Errorf("expected second upstream 2.2.2.2 after swap, got %s", got)
	}
}

// White-box: a swap rebuilds the underlying resolver, and SetBootstrapIPs
// keeps the current endpoint.
func TestReloadableResolver_RebuildsKeepingSibling(t *testing.T) {
	r := NewReloadableResolver("https://a.example/dns-query")
	if got := r.inner.Load().endpoint; got != "https://a.example/dns-query" {
		t.Fatalf("initial endpoint %q", got)
	}

	r.SetEndpoint("https://b.example/dns-query")
	if got := r.inner.Load().endpoint; got != "https://b.example/dns-query" {
		t.Errorf("after SetEndpoint, endpoint = %q", got)
	}

	before := r.inner.Load()
	r.SetBootstrapIPs([]string{"9.9.9.9"})
	if got := r.inner.Load().endpoint; got != "https://b.example/dns-query" {
		t.Errorf("SetBootstrapIPs must preserve endpoint, got %q", got)
	}
	if r.inner.Load() == before {
		t.Error("SetBootstrapIPs must rebuild the underlying resolver")
	}
}

// Negative: an upstream error must surface rather than be masked.
func TestReloadableResolver_UpstreamErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewReloadableResolver(srv.URL)
	if _, err := r.Exchange(context.Background(), query("example.com")); err == nil {
		t.Error("expected error from a 500 upstream")
	}
}

func TestEffectiveBootstrapIPs(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		configured []string
		want       []string
	}{
		{
			name:       "cloudflare default with no configured IPs uses built-in defaults",
			endpoint:   "https://cloudflare-dns.com/dns-query",
			configured: nil,
			want:       []string{"1.1.1.1", "1.0.0.1"},
		},
		{
			name:       "empty endpoint falls back to the default Cloudflare endpoint",
			endpoint:   "",
			configured: nil,
			want:       []string{"1.1.1.1", "1.0.0.1"},
		},
		{
			name:       "configured IPs win even for Cloudflare",
			endpoint:   "https://cloudflare-dns.com/dns-query",
			configured: []string{"9.9.9.9"},
			want:       []string{"9.9.9.9"},
		},
		{
			name:       "custom upstream without configured IPs has no bootstrap defaults",
			endpoint:   "https://dns.google/dns-query",
			configured: nil,
			want:       nil,
		},
		{
			name:       "custom upstream uses its configured IPs",
			endpoint:   "https://dns.google/dns-query",
			configured: []string{"8.8.8.8", "8.8.4.4"},
			want:       []string{"8.8.8.8", "8.8.4.4"},
		},
		{
			name:       "invalid IPs are dropped",
			endpoint:   "https://dns.google/dns-query",
			configured: []string{"not-an-ip", "8.8.8.8"},
			want:       []string{"8.8.8.8"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveBootstrapIPs(tt.endpoint, tt.configured)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// Concurrent swaps and exchanges must be race-free — run under `go test -race`.
func TestReloadableResolver_ConcurrentSetAndExchangeRaceFree(t *testing.T) {
	a := dohTestServer(t, "1.1.1.1")
	defer a.Close()
	b := dohTestServer(t, "2.2.2.2")
	defer b.Close()

	r := NewReloadableResolver(a.URL)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := range 200 {
			if i%2 == 0 {
				r.SetEndpoint(a.URL)
			} else {
				r.SetEndpoint(b.URL)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			r.SetBootstrapIPs([]string{"9.9.9.9"})
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			_, _ = r.Exchange(context.Background(), query("example.com"))
		}
	}()
	wg.Wait()
}
