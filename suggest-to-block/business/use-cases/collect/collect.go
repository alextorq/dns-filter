package collect

import (
	"slices"
	"strconv"
	"strings"
)

// Reason is a single signal that contributed to a suggestion. Code identifies
// the signal; Match optionally carries the related blocked-domain for signals
// that compare against the blocklist (subdomain / similar). Stored verbatim
// in the suggest_block_reasons table — no human-readable text on the backend.
type Reason struct {
	Code  string
	Match string
}

type Suggestion struct {
	Domain  string
	Reasons []Reason
	Score   int
}

const (
	ItemScoreSuspiciousDomain       = 20
	ItemScoreContainsBadKeywords    = 5
	ItemScoreSubdomainOfBlocked     = 20
	ItemScoreSimilarToBlockedDomain = 15
	ItemScoreRiskyTLD               = 5
	ItemScoreNumericRun             = 5
	ItemScoreHexUUID                = 10
	ItemScoreHomograph              = 10
	// ItemScoreBrandImpersonation намеренно ниже ThresholdToSuggestBlocking:
	// одно сходство apex'а с брендом не доказывает фишинг (легитимные
	// конкуренты, новые домены), поэтому typosquat должен подтверждаться
	// вторым слабым сигналом — risky-TLD, bad-keyword, subdomain-of-blocked.
	ItemScoreBrandImpersonation = 25
	ThresholdToSuggestBlocking  = 30
)

// Стабильные коды сигналов. Хранятся в БД и в API в неизменном виде —
// при переименовании ломается история и фронт-маппинг лейблов.
const (
	CodeSuspiciousEntropy  = "suspicious_entropy"
	CodeBadKeywords        = "bad_keywords"
	CodeSubdomainOfBlocked = "subdomain_of_blocked"
	CodeSimilarToBlocked   = "similar_to_blocked"
	CodeRiskyTLD           = "risky_tld"
	CodeNumericRun         = "numeric_run"
	CodeHexUUID            = "hex_uuid"
	CodeHomograph          = "homograph"
	CodeBrandImpersonation = "brand_impersonation"
)

// SignalDescriptor — публичное описание одного сигнала. Бек отдаёт каталог
// на /api/suggest-to-block/codes, фронт использует его и для человеческих
// лейблов в таблице, и для опций мульти-селекта фильтра.
type SignalDescriptor struct {
	Code        string `json:"code"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// signalCatalog — единственный источник правды для лейблов сигналов.
// Порядок здесь = порядок в UI (от самых сильных сигналов к слабым).
// Доступ только через Catalog() — это защищает от случайной мутации
// глобального слайса вызывающим кодом или тестами.
var signalCatalog = []SignalDescriptor{
	{
		Code:        CodeBrandImpersonation,
		Label:       "Brand impersonation",
		Description: "Apex domain looks like a typosquat of a known brand (paypa1, goog1e).",
	},
	{
		Code:        CodeSuspiciousEntropy,
		Label:       "Suspicious entropy",
		Description: "Label has high entropy or all-consonant ratio — looks machine-generated.",
	},
	{
		Code:        CodeSubdomainOfBlocked,
		Label:       "Subdomain of blocked",
		Description: "Domain is a subdomain of a domain already on the blocklist.",
	},
	{
		Code:        CodeSimilarToBlocked,
		Label:       "Similar to blocked",
		Description: "Same domain depth and ≥80% Damerau-Levenshtein similarity to a blocked domain.",
	},
	{
		Code:        CodeHomograph,
		Label:       "Homograph",
		Description: "Label contains mixed-script characters (Cyrillic/Latin lookalikes, IDN typosquat).",
	},
	{
		Code:        CodeHexUUID,
		Label:       "Hex/UUID label",
		Description: "Label looks like a hex hash or UUID — common for tracker/CDN endpoints.",
	},
	{
		Code:        CodeRiskyTLD,
		Label:       "Risky TLD",
		Description: "TLD has elevated abuse rate (.tk, .xyz, .work, .click, …).",
	},
	{
		Code:        CodeNumericRun,
		Label:       "Numeric run",
		Description: "Label contains a long run of digits — common in throwaway hostnames.",
	},
	{
		Code:        CodeBadKeywords,
		Label:       "Ad/tracker keywords",
		Description: "Contains tokens commonly used by ad/tracking infrastructure (ad, ads, tracker, pixel, …).",
	},
}

// Catalog returns a defensive copy of the signal catalog. Returning a clone
// avoids leaking mutability of the package-level slice to callers.
func Catalog() []SignalDescriptor {
	return slices.Clone(signalCatalog)
}

func CollectSuggest(blockedDomains []string, allowedDomains []string) []Suggestion {
	// Inputs из miekg/dns (q.Name) приходят с trailing dot, blocklist-источники
	// (HaGeZi и т.д.) — без. Без нормализации depth-гейт в similar-ветке считает
	// domain. за depth+1 (фантомный пустой лейбл от trailing dot), а Match-поле
	// в одной выдаче смешивает обе формы. Нормализуем один раз на входе, чтобы
	// и сравнения, и пользовательский вывод были консистентны.
	blockedDomains = trimTrailingDots(blockedDomains)
	allowedDomains = trimTrailingDots(allowedDomains)

	idx := buildBlockedIndex(blockedDomains)
	var result []Suggestion

	for _, allowedDomain := range allowedDomains {
		suggestion := Suggestion{
			Domain: allowedDomain,
			Score:  0,
		}

		if IsDomainSuspicious(allowedDomain) {
			suggestion.Score += ItemScoreSuspiciousDomain
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeSuspiciousEntropy})
		}

		if CheckForBadKeywords(allowedDomain) {
			suggestion.Score += ItemScoreContainsBadKeywords
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeBadKeywords})
		}

		if IsRiskyTLD(allowedDomain) {
			suggestion.Score += ItemScoreRiskyTLD
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeRiskyTLD})
		}

		if HasNumericRun(allowedDomain) {
			suggestion.Score += ItemScoreNumericRun
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeNumericRun})
		}

		if HasHexUUIDLabel(allowedDomain) {
			suggestion.Score += ItemScoreHexUUID
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeHexUUID})
		}

		if HasHomographLabel(allowedDomain) {
			suggestion.Score += ItemScoreHomograph
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeHomograph})
		}

		if IsBrandImpersonation(allowedDomain) {
			suggestion.Score += ItemScoreBrandImpersonation
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeBrandImpersonation})
		}

		// Scoring matches the prior O(A×B) loop: каждое совпадение
		// (под-домен или similar) добавляет очки и Reason. Индекс просто
		// убирает перебор всех blocked.
		for _, parent := range idx.subdomainAncestors(allowedDomain) {
			suggestion.Score += ItemScoreSubdomainOfBlocked
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeSubdomainOfBlocked, Match: parent})
		}
		for _, match := range idx.similarMatches(allowedDomain) {
			suggestion.Score += ItemScoreSimilarToBlockedDomain
			suggestion.Reasons = append(suggestion.Reasons, Reason{Code: CodeSimilarToBlocked, Match: match})
		}

		if suggestion.Score >= ThresholdToSuggestBlocking {
			result = append(result, suggestion)
		}

	}

	return result
}

// blockedIndex precomputes lookups so CollectSuggest can avoid the A×B inner
// loop. На реальных размерах (657k blocked × 1.4k allowed) полный перебор
// давал ≈10⁹ вызовов DamerauLevenshtein с аллокацией O(L²) матрицы — collect
// шёл 5 минут на ядро. С индексом тот же результат получаем за O(B + A·k),
// где k — размер бакета (обычно 1-10 кандидатов).
type blockedIndex struct {
	// subdomainSet — все blocked-домены целиком, для O(L) проверки
	// «является ли allowed под-доменом одного из blocked». Эквивалент
	// CheckItIsSubDomain.
	subdomainSet map[string]struct{}
	// similarBuckets — blocked-домены, сгруппированные по depth+parent-suffix.
	// Ключ = "<depth>|<parts[1:].join('.')>". Эквивалент пред-условий
	// CheckIfBlockSameDomainLevelAndHaveSameBlockedDomain: same-depth (≥4) и
	// same parent сразу выполнены, остаётся только DL по first-label.
	similarBuckets map[string][]similarEntry
}

type similarEntry struct {
	firstLabel string
	full       string
}

// trimTrailingDots returns a copy of in with each entry stripped of trailing
// dots. Cheap (O(N) с одной аллокацией под результат и без аллокаций под
// сами строки, т.к. TrimRight возвращает sub-slice исходной строки).
func trimTrailingDots(in []string) []string {
	out := make([]string, len(in))
	for i, d := range in {
		out[i] = strings.TrimRight(d, ".")
	}
	return out
}

func buildBlockedIndex(blocked []string) *blockedIndex {
	idx := &blockedIndex{
		subdomainSet:   make(map[string]struct{}, len(blocked)),
		similarBuckets: make(map[string][]similarEntry),
	}
	for _, b := range blocked {
		idx.subdomainSet[b] = struct{}{}
		parts := strings.Split(b, ".")
		if len(parts) < 4 {
			continue
		}
		key := strconv.Itoa(len(parts)) + "|" + strings.Join(parts[1:], ".")
		idx.similarBuckets[key] = append(idx.similarBuckets[key], similarEntry{
			firstLabel: parts[0],
			full:       b,
		})
	}
	return idx
}

// subdomainAncestors returns blocked entries that contain domain as
// (sub-)domain — domain itself or any of its dot-trimmed suffixes that
// appears in the set. Поведение совпадает с CheckItIsSubDomain, прогнанным
// по всем blocked, но без перебора blocked.
func (idx *blockedIndex) subdomainAncestors(domain string) []string {
	if len(idx.subdomainSet) == 0 {
		return nil
	}
	var matches []string
	if _, ok := idx.subdomainSet[domain]; ok {
		matches = append(matches, domain)
	}
	rest := domain
	for {
		i := strings.Index(rest, ".")
		if i < 0 {
			break
		}
		rest = rest[i+1:]
		if rest == "" {
			break
		}
		if _, ok := idx.subdomainSet[rest]; ok {
			matches = append(matches, rest)
		}
	}
	return matches
}

// similarMatches returns blocked entries with same depth + same parent
// suffix as allowed and ≥80% Damerau-Levenshtein similarity on first label.
// Семантика — как у CheckIfBlockSameDomainLevelAndHaveSameBlockedDomain,
// прогнанной по всем blocked.
func (idx *blockedIndex) similarMatches(allowed string) []string {
	parts := strings.Split(allowed, ".")
	if len(parts) < 4 {
		return nil
	}
	key := strconv.Itoa(len(parts)) + "|" + strings.Join(parts[1:], ".")
	bucket, ok := idx.similarBuckets[key]
	if !ok {
		return nil
	}
	first := parts[0]
	var matches []string
	for _, e := range bucket {
		if SimilarityAtLeast(first, e.firstLabel, 80.0) {
			matches = append(matches, e.full)
		}
	}
	return matches
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

	return SimilarityAtLeast(firstAllowedPart, firstBlockedPart, 80.0)
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
