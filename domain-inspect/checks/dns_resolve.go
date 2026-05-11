package checks

import (
	"context"
	"net"
	"strings"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// DNSResolve performs A/AAAA/MX/NS lookups using the system resolver. A domain
// that does not resolve at all is unusable for traffic — typically already
// dead, registrar-suspended, or a typo. We report it but stay non-committal:
// NXDOMAIN does not by itself mean the domain is malicious.
func DNSResolve(ctx context.Context, domain string) domain_inspect.CheckResult {
	r := net.DefaultResolver

	details := map[string]any{}

	ips, err := r.LookupHost(ctx, domain)
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

	mxs, err := r.LookupMX(ctx, domain)
	if err == nil {
		mxNames := make([]string, 0, len(mxs))
		for _, mx := range mxs {
			mxNames = append(mxNames, strings.TrimSuffix(mx.Host, "."))
		}
		details["mx"] = mxNames
	}

	nss, err := r.LookupNS(ctx, domain)
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
