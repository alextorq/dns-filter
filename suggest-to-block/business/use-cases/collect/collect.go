package collect

import (
	"fmt"
	"strings"
)

const DomainSeparator = "."

func CollectSuggest(blockedDomains []string, allowedDomains []string) {
	for _, blockedDomain := range blockedDomains {
		for _, allowedDomain := range allowedDomains {
			if DomainsIsLookLikeDomain(blockedDomain, allowedDomain) {
				fmt.Println("Suggest to block domain:", allowedDomain)
			}
		}
	}
}

func ItIsSubdomain(blockedDomains string, allowedDomain string) bool {
	parts := strings.Split(allowedDomain, DomainSeparator)
	if len(parts) < 2 {
		return false
	}
	domain := strings.Join(parts[len(parts)-2:], DomainSeparator)
	if strings.HasSuffix(blockedDomains, domain) {
		return true
	}
	return false
}

func DomainsIsLookLikeDomain(domainA string, domainB string) bool {
	return ItIsSubdomain(domainA, domainB)
}
