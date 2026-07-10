package sync

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

type cancelBody struct {
	ctx     context.Context
	started chan struct{}
}

func (b *cancelBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (*cancelBody) Close() error { return nil }

func TestHostsLoader_CancelAbortsRequest(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	loader := NewHostsLoader(doerFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       &cancelBody{ctx: req.Context(), started: started},
			Request:    req,
		}, nil
	}), "https://source.test/hosts")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := loader.Load(ctx)
		done <- err
	}()

	<-started
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("HostsLoader.Load error = %v, want context.Canceled", err)
	}
}

func TestHostsLoader_UsesInjectedEndpointAndParsesResponse(t *testing.T) {
	t.Parallel()
	const endpoint = "https://source.test/custom-hosts"
	var requestedURL string
	loader := NewHostsLoader(doerFunc(func(req *http.Request) (*http.Response, error) {
		requestedURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("0.0.0.0 ads.example.com\n")),
			Request:    req,
		}, nil
	}), endpoint)

	domains, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("HostsLoader.Load failed: %v", err)
	}
	if requestedURL != endpoint {
		t.Fatalf("requested URL = %q, want injected %q", requestedURL, endpoint)
	}
	if len(domains) != 1 || domains[0] != "ads.example.com." {
		t.Fatalf("domains = %v, want parsed injected response", domains)
	}
}

func TestHostsLoader_RejectsTypedNilHTTPClient(t *testing.T) {
	var client *http.Client
	loader := NewHostsLoader(client, "https://source.test/hosts")
	if _, err := loader.Load(context.Background()); err == nil {
		t.Fatal("HostsLoader accepted typed-nil HTTP client")
	}
}

func TestParseIpHostsLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name: "Standard hosts entries with FQDN",
			input: `
0.0.0.0 ads.example.com
0.0.0.0 tracker.example.org
`,
			expected: []string{"ads.example.com.", "tracker.example.org."},
		},
		{
			name: "Comments and blank lines are skipped",
			input: `
# header

0.0.0.0 keep.example.com
`,
			expected: []string{"keep.example.com."},
		},
		{
			// Regression: a malformed source row of "0.0.0.0 ru" would otherwise
			// land "ru" in block_lists and trip the same auto-block cascade as
			// the EasyList ||ru^$... bug. PSL guard must apply here too.
			name: "Drop rows where the host is a public suffix",
			input: `
0.0.0.0 ru
0.0.0.0 co.uk
0.0.0.0 ozone.ru
`,
			expected: []string{"ozone.ru."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseIpHostsLine(strings.NewReader(tt.input))
			sort.Strings(got)
			sort.Strings(tt.expected)
			if len(got) != len(tt.expected) {
				t.Fatalf("ParseIpHostsLine() got %v, want %v", got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Fatalf("ParseIpHostsLine() index %d: got %q want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}
