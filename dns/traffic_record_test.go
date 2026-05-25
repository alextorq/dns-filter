package dns

import (
	"net"
	"sync"
	"testing"

	"github.com/alextorq/dns-filter/clients/identifier"
	dnsLib "github.com/miekg/dns"
)

// capturedRecord is one call to the fake TrafficRecorder.
type capturedRecord struct {
	kind, value, ip, domain string
	blocked                 bool
}

// fakeTrafficRecorder captures Record calls for assertions.
type fakeTrafficRecorder struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (f *fakeTrafficRecorder) Record(kind, value, ip, domain string, blocked bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, capturedRecord{kind, value, ip, domain, blocked})
}

func (f *fakeTrafficRecorder) snapshot() []capturedRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]capturedRecord, len(f.records))
	copy(out, f.records)
	return out
}

// stubIdentifier returns a fixed lookup, simulating an arpwatcher hit that
// resolves the source IP to a MAC.
type stubIdentifier struct {
	lookup     identifier.Lookup
	identified bool
}

func (s stubIdentifier) Identify(_ identifier.Request) (identifier.Lookup, bool) {
	return s.lookup, s.identified
}

func newRecordingServer(filter func(string) bool, ident identifier.Identifier, rec TrafficRecorder) *DnsServer {
	return &DnsServer{
		Logger:     noopLogger{},
		Cache:      newMemoryCache(),
		Filter:     filter,
		Upstream:   &staticResolver{rcode: dnsLib.RcodeSuccess},
		Metric:     noopMetric{},
		Handlers:   noopHandlers{},
		Identifier: ident,
		Clients:    noopClientStore{},
		Traffic:    rec,
	}
}

func queryFrom(server *DnsServer, remoteIP, qname string) {
	writer := &captureResponseWriter{
		local:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53},
		remote: &net.UDPAddr{IP: net.ParseIP(remoteIP), Port: 50000},
	}
	req := new(dnsLib.Msg)
	req.SetQuestion(qname, dnsLib.TypeA)
	req.Id = 1
	server.handleDNS(writer, req)
}

// TestHandleDNSRecordsBlockedVerdict: a query the filter blocks records
// blocked=true with the resolved MAC identity and the source IP.
func TestHandleDNSRecordsBlockedVerdict(t *testing.T) {
	rec := &fakeTrafficRecorder{}
	ident := stubIdentifier{lookup: identifier.Lookup{Kind: identifier.KindMAC, Value: "aa:bb:cc:dd:ee:ff"}, identified: true}
	server := newRecordingServer(func(string) bool { return true }, ident, rec)

	queryFrom(server, "192.0.2.10", "ads.example.")

	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	r := got[0]
	if !r.blocked {
		t.Errorf("expected blocked=true, got false")
	}
	if r.kind != identifier.KindMAC || r.value != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("expected MAC identity from lookup, got kind=%q value=%q", r.kind, r.value)
	}
	if r.ip != "192.0.2.10" {
		t.Errorf("expected source IP 192.0.2.10, got %q", r.ip)
	}
	if r.domain != "ads.example." {
		t.Errorf("expected domain ads.example., got %q", r.domain)
	}
}

// TestHandleDNSRecordsAllowedVerdict: a query the filter allows records
// blocked=false. Falls back to IP identity when unidentified.
func TestHandleDNSRecordsAllowedVerdict(t *testing.T) {
	rec := &fakeTrafficRecorder{}
	// Unidentified: handler must fall back to Kind=ip / Value=clientIP.
	ident := stubIdentifier{identified: false}
	server := newRecordingServer(func(string) bool { return false }, ident, rec)

	queryFrom(server, "192.0.2.20", "good.example.")

	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	r := got[0]
	if r.blocked {
		t.Errorf("expected blocked=false, got true")
	}
	if r.kind != identifier.KindIP || r.value != "192.0.2.20" {
		t.Errorf("expected IP fallback identity, got kind=%q value=%q", r.kind, r.value)
	}
	if r.ip != "192.0.2.20" {
		t.Errorf("expected source IP 192.0.2.20, got %q", r.ip)
	}
}

// TestHandleDNSSkipsLoopbackAndEmpty: queries from the box itself (loopback /
// empty client) are noise and must NOT be recorded.
func TestHandleDNSSkipsLoopbackAndEmpty(t *testing.T) {
	cases := []string{"127.0.0.1", "::1"}
	for _, ip := range cases {
		t.Run(ip, func(t *testing.T) {
			rec := &fakeTrafficRecorder{}
			ident := stubIdentifier{lookup: identifier.Lookup{Kind: identifier.KindIP, Value: ip}, identified: true}
			server := newRecordingServer(func(string) bool { return false }, ident, rec)

			queryFrom(server, ip, "self.example.")

			if got := rec.snapshot(); len(got) != 0 {
				t.Fatalf("expected loopback query %s to be skipped, got %d records", ip, len(got))
			}
		})
	}
}

// TestHandleDNSNilTrafficRecorderSafe: a server without a Traffic recorder must
// still answer queries (existing tests don't wire one). Guard is a nil check.
func TestHandleDNSNilTrafficRecorderSafe(t *testing.T) {
	server := newRecordingServer(func(string) bool { return false }, identifier.IPIdentifier{}, nil)

	writer := &captureResponseWriter{
		local:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53},
		remote: &net.UDPAddr{IP: net.ParseIP("192.0.2.30"), Port: 50000},
	}
	req := new(dnsLib.Msg)
	req.SetQuestion("nil.example.", dnsLib.TypeA)
	req.Id = 5

	// Must not panic.
	server.handleDNS(writer, req)

	if writer.msg == nil {
		t.Fatal("expected a DNS response even without a traffic recorder")
	}
}
