package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	dnsLib "github.com/miekg/dns"
)

const DefaultDoHEndpoint = "https://cloudflare-dns.com/dns-query"

var DefaultDoHBootstrapIPs = []string{"1.1.1.1", "1.0.0.1"}

type UpstreamResolver interface {
	Exchange(ctx context.Context, msg *dnsLib.Msg) (*dnsLib.Msg, error)
}

type DoHResolver struct {
	endpoint string
	client   *http.Client
}

func NewDoHResolver(endpoint string, bootstrapIPs ...string) *DoHResolver {
	if endpoint == "" {
		endpoint = DefaultDoHEndpoint
	}

	endpointURL, _ := url.Parse(endpoint)
	endpointHost := endpointURL.Hostname()
	bootstrapIPs = normalizeBootstrapIPs(endpointHost, bootstrapIPs)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if endpointHost != "" && net.ParseIP(endpointHost) == nil {
		tlsConfig.ServerName = endpointHost
		dialer := &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		transport.DialContext = newBootstrapDialContext(endpointHost, bootstrapIPs, dialer.DialContext)
	}

	transport.TLSClientConfig = tlsConfig

	return &DoHResolver{
		endpoint: endpoint,
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func normalizeBootstrapIPs(endpointHost string, bootstrapIPs []string) []string {
	clean := make([]string, 0, len(bootstrapIPs))
	for _, raw := range bootstrapIPs {
		for _, part := range strings.Split(raw, ",") {
			ip := strings.TrimSpace(part)
			ip = strings.TrimPrefix(strings.TrimSuffix(ip, "]"), "[")
			if net.ParseIP(ip) != nil {
				clean = append(clean, ip)
			}
		}
	}

	if len(clean) > 0 {
		return clean
	}

	if strings.EqualFold(endpointHost, "cloudflare-dns.com") {
		return append([]string(nil), DefaultDoHBootstrapIPs...)
	}

	return nil
}

// EffectiveBootstrapIPs reports the bootstrap IPs NewDoHResolver would actually
// use for the given endpoint and configured IPs: the configured ones if any are
// valid, otherwise the built-in defaults when the endpoint host is Cloudflare,
// otherwise none. Exposed so the settings layer can show the real default for
// doh_bootstrap_ips (e.g. "1.1.1.1,1.0.0.1" on a default deploy) instead of an
// empty string, without duplicating the host-specific fallback rule.
func EffectiveBootstrapIPs(endpoint string, configured []string) []string {
	if endpoint == "" {
		endpoint = DefaultDoHEndpoint
	}
	endpointURL, _ := url.Parse(endpoint)
	return normalizeBootstrapIPs(endpointURL.Hostname(), configured)
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

func newBootstrapDialContext(endpointHost string, bootstrapIPs []string, dial dialContextFunc) dialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("parse doh dial address: %w", err)
		}

		if !strings.EqualFold(host, endpointHost) {
			return nil, fmt.Errorf("refusing to dial %q while bootstrapping DoH endpoint %q", host, endpointHost)
		}

		if len(bootstrapIPs) == 0 {
			return nil, fmt.Errorf("bootstrap IPs are required for DoH endpoint %q; set DNS_FILTER_DOH_BOOTSTRAP_IPS", endpointHost)
		}

		var lastErr error
		for _, ip := range bootstrapIPs {
			conn, err := dial(ctx, network, net.JoinHostPort(ip, port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}

		return nil, fmt.Errorf("dial DoH endpoint %q via bootstrap IPs %v: %w", endpointHost, bootstrapIPs, lastErr)
	}
}

func (r *DoHResolver) Exchange(ctx context.Context, msg *dnsLib.Msg) (*dnsLib.Msg, error) {
	data, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack dns message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create doh request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-message")
	req.Header.Set("Content-Type", "application/dns-message")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send doh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read doh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doh upstream returned %s: %s", resp.Status, string(body))
	}

	in := new(dnsLib.Msg)
	if err := in.Unpack(body); err != nil {
		return nil, fmt.Errorf("unpack doh response: %w", err)
	}

	in.Id = msg.Id
	return in, nil
}

func MakeRequest(domain string) (*dnsLib.Msg, error) {
	m := new(dnsLib.Msg)
	m.SetQuestion(dnsLib.Fqdn(domain), dnsLib.TypeA)

	return NewDoHResolver(DefaultDoHEndpoint).Exchange(context.Background(), m)
}
