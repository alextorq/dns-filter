package dns

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	dnsLib "github.com/miekg/dns"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoHResolverExchange(t *testing.T) {
	resolver := &DoHResolver{
		endpoint: "https://doh.example.test/dns-query",
		client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", r.Method)
			}

			if got := r.Header.Get("Content-Type"); got != "application/dns-message" {
				t.Fatalf("unexpected content type: %s", got)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}

			query := new(dnsLib.Msg)
			if err := query.Unpack(body); err != nil {
				t.Fatalf("unpack dns request: %v", err)
			}

			if len(query.Question) != 1 || query.Question[0].Name != "example.com." {
				t.Fatalf("unexpected query: %+v", query.Question)
			}

			reply := new(dnsLib.Msg)
			reply.SetReply(query)
			rr, err := dnsLib.NewRR("example.com. 60 IN A 1.2.3.4")
			if err != nil {
				t.Fatalf("create rr: %v", err)
			}
			reply.Answer = []dnsLib.RR{rr}

			wire, err := reply.Pack()
			if err != nil {
				t.Fatalf("pack reply: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/dns-message"}},
				Body:       io.NopCloser(bytes.NewReader(wire)),
				Request:    r,
			}, nil
		})},
	}

	msg := new(dnsLib.Msg)
	msg.SetQuestion("example.com.", dnsLib.TypeA)
	msg.Id = 1234

	resp, err := resolver.Exchange(context.Background(), msg)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}

	if resp.Id != msg.Id {
		t.Fatalf("expected response id %d, got %d", msg.Id, resp.Id)
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("expected one answer, got %d", len(resp.Answer))
	}
}

func TestDoHResolverExchangeStatusError(t *testing.T) {
	resolver := &DoHResolver{
		endpoint: "https://doh.example.test/dns-query",
		client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				Body:       io.NopCloser(strings.NewReader("bad upstream")),
				Request:    r,
			}, nil
		})},
	}

	msg := new(dnsLib.Msg)
	msg.SetQuestion("example.com.", dnsLib.TypeA)

	_, err := resolver.Exchange(context.Background(), msg)
	if err == nil {
		t.Fatal("expected status error")
	}

	if !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeBootstrapIPsUsesCloudflareDefaults(t *testing.T) {
	ips := normalizeBootstrapIPs("cloudflare-dns.com", nil)
	if len(ips) == 0 {
		t.Fatal("expected default Cloudflare bootstrap IPs")
	}

	if ips[0] != "1.1.1.1" {
		t.Fatalf("unexpected first bootstrap IP: %s", ips[0])
	}
}

func TestBootstrapDialContextDialsBootstrapIP(t *testing.T) {
	var dialed []string
	dial := newBootstrapDialContext("cloudflare-dns.com", []string{"1.1.1.1"}, func(_ context.Context, _ string, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		return nil, errors.New("stop")
	})

	_, err := dial(context.Background(), "tcp", "cloudflare-dns.com:443")
	if err == nil {
		t.Fatal("expected fake dial error")
	}

	if len(dialed) != 1 || dialed[0] != "1.1.1.1:443" {
		t.Fatalf("expected bootstrap dial to 1.1.1.1:443, got %v", dialed)
	}
}

func TestBootstrapDialContextRequiresBootstrapIPs(t *testing.T) {
	dial := newBootstrapDialContext("doh.example.test", nil, func(_ context.Context, _ string, _ string) (net.Conn, error) {
		t.Fatal("dial should not be called without bootstrap IPs")
		return nil, nil
	})

	_, err := dial(context.Background(), "tcp", "doh.example.test:443")
	if err == nil {
		t.Fatal("expected missing bootstrap IP error")
	}

	if !strings.Contains(err.Error(), "DNS_FILTER_DOH_BOOTSTRAP_IPS") {
		t.Fatalf("unexpected error: %v", err)
	}
}
