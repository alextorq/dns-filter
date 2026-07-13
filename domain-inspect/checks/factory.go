package checks

import (
	"net"
	"net/http"
	"time"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

const defaultHTTPTimeout = 10 * time.Second

// CatalogDeps are the dependencies that cross the domain-inspect package
// boundary. The factory owns smaller implementation details such as the HTTP
// client, system resolver, wall clock and provider endpoints.
type CatalogDeps struct {
	Blocks      BlockLookup
	Allowed     AllowLookup
	Credentials *Credentials
	URLScanKey  string
}

// Catalog keeps named checks available both to the HTTP inspection endpoint
// and to consumers that reuse a subset of the same providers.
type Catalog struct {
	LocalStats   domain_inspect.CheckFunc
	DNSResolve   domain_inspect.CheckFunc
	RDAP         domain_inspect.CheckFunc
	CrtSh        domain_inspect.CheckFunc
	URLScan      domain_inspect.CheckFunc
	VirusTotal   domain_inspect.CheckFunc
	SafeBrowsing domain_inspect.CheckFunc
}

// NewDefaultCatalog assembles the production inspection checks. Individual
// constructors remain dependency-injected so tests and alternative factories
// can supply isolated transports, resolvers, clocks and endpoints.
func NewDefaultCatalog(deps CatalogDeps) Catalog {
	requireDependency("block lookup", deps.Blocks)
	requireDependency("allow lookup", deps.Allowed)
	requireDependency("catalog credentials", deps.Credentials)

	client := &http.Client{Timeout: defaultHTTPTimeout}
	clock := ClockFunc(time.Now)

	return Catalog{
		LocalStats:   NewLocalStats(deps.Blocks, deps.Allowed),
		DNSResolve:   NewDNSResolve(net.DefaultResolver),
		RDAP:         NewRDAP(client, DefaultRDAPEndpoint, clock),
		CrtSh:        NewCrtSh(client, DefaultCrtShEndpoint),
		URLScan:      NewURLScan(client, DefaultURLScanEndpoint, deps.URLScanKey),
		VirusTotal:   NewVirusTotal(client, DefaultVirusTotalEndpoint, deps.Credentials),
		SafeBrowsing: NewSafeBrowsing(client, DefaultSafeBrowsingEndpoint, deps.Credentials),
	}
}

// Checks returns a fresh complete map so request-level filtering cannot mutate
// the catalog retained by other consumers.
func (c Catalog) Checks() map[string]domain_inspect.CheckFunc {
	return Default(DefaultDeps{
		LocalStats:   c.LocalStats,
		DNSResolve:   c.DNSResolve,
		RDAP:         c.RDAP,
		CrtSh:        c.CrtSh,
		URLScan:      c.URLScan,
		VirusTotal:   c.VirusTotal,
		SafeBrowsing: c.SafeBrowsing,
	})
}
