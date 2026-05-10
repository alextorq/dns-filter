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
	ItemScoreRiskyTLD               = 5
	ItemScoreNumericRun             = 5
	ItemScoreHexUUID                = 10
	// ItemScoreBrandImpersonation намеренно ниже ThresholdToSuggestBlocking:
	// одно сходство apex'а с брендом не доказывает фишинг (легитимные
	// конкуренты, новые домены), поэтому typosquat должен подтверждаться
	// вторым слабым сигналом — risky-TLD, bad-keyword, subdomain-of-blocked.
	ItemScoreBrandImpersonation = 25
	ThresholdToSuggestBlocking  = 30
)

// Стабильные подстроки, которые CollectSuggest добавляет в Suggestion.Reason
// при срабатывании соответствующего сигнала. Использование констант
// связывает реализацию с тестами и упрощает локализацию/перевод.
const (
	ReasonSuspiciousDomain       = "appears to be suspicious"
	ReasonContainsBadKeywords    = "contains keywords indicating ads or tracking"
	ReasonSubdomainOfBlocked     = "is subdomain of blocked domain"
	ReasonSimilarToBlockedDomain = "has same domain level and similar blocked domain as"
	ReasonRiskyTLD               = "uses a TLD with elevated abuse rate"
	ReasonNumericRun             = "label contains a long run of digits"
	ReasonHexUUIDLabel           = "label looks like a hex hash or UUID"
	ReasonBrandImpersonation     = "resembles a known brand domain but is not it"
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
			suggestion.Reason += "\n" + ReasonSuspiciousDomain + "; "
		}

		if CheckForBadKeywords(allowedDomain) {
			suggestion.Score += ItemScoreContainsBadKeywords
			suggestion.Reason += "\n" + ReasonContainsBadKeywords + "; "
		}

		if IsRiskyTLD(allowedDomain) {
			suggestion.Score += ItemScoreRiskyTLD
			suggestion.Reason += "\n" + ReasonRiskyTLD + "; "
		}

		if HasNumericRun(allowedDomain) {
			suggestion.Score += ItemScoreNumericRun
			suggestion.Reason += "\n" + ReasonNumericRun + "; "
		}

		if HasHexUUIDLabel(allowedDomain) {
			suggestion.Score += ItemScoreHexUUID
			suggestion.Reason += "\n" + ReasonHexUUIDLabel + "; "
		}

		if IsBrandImpersonation(allowedDomain) {
			suggestion.Score += ItemScoreBrandImpersonation
			suggestion.Reason += "\n" + ReasonBrandImpersonation + "; "
		}

		for _, blockedDomain := range blockedDomains {
			if CheckItIsSubDomain(blockedDomain, allowedDomain) {
				suggestion.Score += ItemScoreSubdomainOfBlocked
				suggestion.Reason += fmt.Sprintf("\n%s %s; ", ReasonSubdomainOfBlocked, blockedDomain)
			}

			if CheckIfBlockSameDomainLevelAndHaveSameBlockedDomain(blockedDomain, allowedDomain) {
				suggestion.Score += ItemScoreSimilarToBlockedDomain
				suggestion.Reason += fmt.Sprintf("\n%s %s; ", ReasonSimilarToBlockedDomain, blockedDomain)
			}
		}

		if suggestion.Score >= ThresholdToSuggestBlocking {
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
