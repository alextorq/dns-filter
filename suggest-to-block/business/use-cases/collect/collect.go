package collect

import "github.com/alextorq/dns-filter/suggest-to-block/db"

func CollectSuggest(blockedDomains []string, allowedDomains []string) {
	for _, blockedDomain := range blockedDomains {
		for _, allowedDomain := range allowedDomains {
			if DomainsIsLookLikeDomain(blockedDomain, allowedDomain) {

			}
		}
	}
}

func DomainsIsLookLikeDomain(domainA string, domainB string) bool {
	return false
}
