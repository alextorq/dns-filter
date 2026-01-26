package collect

import (
	"fmt"
	"strings"
)

type Suggestion struct {
	Domain string
	Reason string
	Score  int
}

const (
	ItemScoreSuspiciousDomain       = 20
	ItemScoreContainsBadKeywords    = 5
	ItemScoreSubdomainOfBlocked     = 20
	ItemScoreSimilarToBlockedDomain = 15
	ThresholdToSuggestBlocking      = 30
)

func CollectSuggest(blockedDomains []string, allowedDomains []string) []Suggestion {
	var result []Suggestion

	for _, allowedDomain := range allowedDomains {
		suggestion := Suggestion{
			Domain: allowedDomain,
			Score:  0,
		}

		if IsDomainSuspicious(allowedDomain) {
			suggestion.Score += ItemScoreSuspiciousDomain
			suggestion.Reason += "\nappears to be suspicious; "
		}

		if CheckForBadKeywords(allowedDomain) {
			suggestion.Score += ItemScoreContainsBadKeywords
			suggestion.Reason += "\ncontains keywords indicating ads or tracking; "
		}

		for _, blockedDomain := range blockedDomains {
			if CheckItIsSubDomain(blockedDomain, allowedDomain) {
				suggestion.Score = +ItemScoreSubdomainOfBlocked
				suggestion.Reason += fmt.Sprintf("\nis subdomain of blocked domain %s; ", blockedDomain)
			}

			if CheckIfBlockSameDomainLevelAndHaveSameBlockedDomain(blockedDomain, allowedDomain) {
				suggestion.Score += ItemScoreSimilarToBlockedDomain
				suggestion.Reason += fmt.Sprintf("\nhas same domain level and similar blocked domain as %s; ", blockedDomain)
			}
		}

		if suggestion.Score > 0 {
			result = append(result, suggestion)
		}

	}

	return result
}

func CheckIfBlockSameDomainLevelAndHaveSameBlockedDomain(blockedDomain string, allowedDomain string) bool {
	const DomainSeparator = "."
	allowedDomainParts := strings.Split(allowedDomain, DomainSeparator)
	if len(allowedDomainParts) < 4 {
		return false
	}

	blockedDomainParts := strings.Split(blockedDomain, DomainSeparator)
	if len(allowedDomainParts) != len(blockedDomainParts) {
		return false
	}

	lastParts := strings.Join(allowedDomainParts[1:], DomainSeparator)
	if lastParts != strings.Join(blockedDomainParts[1:], DomainSeparator) {
		return false
	}

	firstAllowedPart := allowedDomainParts[0]
	firstBlockedPart := blockedDomainParts[0]

	distance := Similarity(firstAllowedPart, firstBlockedPart)
	if distance < 80.0 {
		return false
	}

	return true
}

func CheckItIsSubDomain(parent string, child string) bool {
	// 1. Если домены идентичны, это (обычно) считается вхождением
	if parent == child {
		return true
	}

	// 2. Если дочерний домен короче родительского, он не может быть поддоменом
	if len(child) < len(parent) {
		return false
	}

	// 3. Проверяем, заканчивается ли child на parent
	if !strings.HasSuffix(child, parent) {
		return false
	}

	// 4. ВАЖНО: Проверяем границу домена.
	// Если parent = "google.com", а child = "agoogle.com", HasSuffix даст true,
	// но это не поддомен. Перед parent должна стоять точка.

	// Вычисляем символ, который стоит перед началом parent внутри child
	boundaryIndex := len(child) - len(parent) - 1

	// Проверяем, что там именно точка
	if child[boundaryIndex] == '.' {
		return true
	}

	return false
}
