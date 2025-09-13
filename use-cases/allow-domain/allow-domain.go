package allow_domain

import (
	dnsLib "github.com/miekg/dns"
)

func AllowDomain(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	go func() {
		first := r.Question[0]
		domain := first.Name
		SendEventAboutAllowDomain(domain)
	}()
}
