package checks

import (
	"context"
	"net"
	"strings"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// DNSResolver is the system-DNS port used by the DNS inspection check.
// *net.Resolver satisfies it.
type DNSResolver interface {
	LookupHost(context.Context, string) ([]string, error)
	LookupMX(context.Context, string) ([]*net.MX, error)
	LookupNS(context.Context, string) ([]*net.NS, error)
}

func NewDNSResolve(resolver DNSResolver) domain_inspect.CheckFunc {
	requireDependency("DNS resolver", resolver)
	return func(ctx context.Context, domain string) domain_inspect.CheckResult {
		return dnsResolve(ctx, domain, resolver)
	}
}

// DNSResolve performs A/AAAA/MX/NS lookups using the system resolver. A domain
// that does not resolve at all is unusable for traffic — typically already
// dead, registrar-suspended, or a typo. We report it but stay non-committal:
// NXDOMAIN does not by itself mean the domain is malicious.
func dnsResolve(ctx context.Context, domain string, resolver DNSResolver) domain_inspect.CheckResult {
	details := map[string]any{}

	ips, err := resolver.LookupHost(ctx, domain)
	if err != nil {
		details["resolved"] = false
		details["lookup_error"] = err.Error()
		return domain_inspect.CheckResult{
			Status:  domain_inspect.StatusOK,
			Verdict: domain_inspect.VerdictUnknown,
			Details: details,
		}
	}
	details["resolved"] = true
	details["addresses"] = ips

	mxs, err := resolver.LookupMX(ctx, domain)
	if err == nil {
		mxNames := make([]string, 0, len(mxs))
		for _, mx := range mxs {
			mxNames = append(mxNames, strings.TrimSuffix(mx.Host, "."))
		}
		details["mx"] = mxNames
	}

	nss, err := resolver.LookupNS(ctx, domain)
	if err == nil {
		nsNames := make([]string, 0, len(nss))
		for _, ns := range nss {
			nsNames = append(nsNames, strings.TrimSuffix(ns.Host, "."))
		}
		details["ns"] = nsNames
	}

	return domain_inspect.CheckResult{
		Status:  domain_inspect.StatusOK,
		Verdict: domain_inspect.VerdictUnknown,
		Details: details,
	}
}
