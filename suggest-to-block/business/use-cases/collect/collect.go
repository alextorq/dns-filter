package collect

import (
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Reason is a single signal that contributed to a suggestion. Code identifies
// the signal; Match optionally carries the related blocked-domain for signals
// that compare against the blocklist (subdomain / similar). Stored verbatim
// in the suggest_block_reasons table — no human-readable text on the backend.
// Reason carries a signal code and an optional related value. The json tags
// are load-bearing: reasons are snapshotted into inspect_candidate.reasons_json
// and round-tripped by the worker, and the canonical wire form is lowercase
// {"code":...,"match":...} — matching SuggestBlockReason's API shape. Without
// tags json.Marshal would emit PascalCase keys and an always-present empty
// "match", diverging from that contract.
type Reason struct {
	Code  string `json:"code"`
	Match string `json:"match,omitempty"`
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
	// ThresholdToAutoBlock — score, above which Collect() promotes a suggestion
	// straight into the blocklist without manual review. Two strong signals
	// (e.g. brand-impersonation + similar-to-blocked, или subdomain + entropy
	// + similar) must independently agree on the verdict.
	ThresholdToAutoBlock = 60
	// MinInspectCandidateScore is the lower bound of the lexical band that is
	// too weak to surface in the UI on its own (< ThresholdToSuggestBlocking)
	// but strong enough to be worth a (rate-limited, opt-in) reputation check.
	// Domains scoring in [MinInspectCandidateScore, ThresholdToSuggestBlocking)
	// are routed to the inspect queue instead of the suggest list.
	MinInspectCandidateScore = 10
)

// ShouldAutoBlock reports whether a collected suggestion qualifies for
// auto-promotion to the blocklist. Two independent gates:
//   - score >= ThresholdToAutoBlock — accumulated heuristic confidence is high
//     enough that two strong signals must independently agree;
//   - any reason has CodeSubdomainOfBlocked — the parent is already blocked,
//     so the subdomain is almost certainly part of the same family and is the
//     most deterministic signal we have, regardless of the total score.
//
// Lives on the use-case package (not the orchestrator) so the rule is unit-
// testable without touching the DB or the filter singletons.
func ShouldAutoBlock(s Suggestion) bool {
	if s.Score >= ThresholdToAutoBlock {
		return true
	}
	for _, r := range s.Reasons {
		if r.Code == CodeSubdomainOfBlocked {
			return true
		}
	}
	return false
}

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

// Reason codes produced by the reputation-enrichment worker (not the lexical
// pass). They MUST all carry the "inspect_" prefix: the worker's upsert path
// (UpsertWithInspect) refreshes only inspect-derived reasons by matching that
// prefix, so no lexical code above may ever start with it. They are defined
// here so both the adapter and the signal catalog share one source of truth
// without an import cycle.
const (
	CodeInspectVTMalicious   = "inspect_vt_malicious"
	CodeInspectSafeBrowsing  = "inspect_safe_browsing"
	CodeInspectRDAPYoung     = "inspect_rdap_young"
	CodeInspectCleanEndorsed = "inspect_clean_endorsed"
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
		Code:        CodeInspectVTMalicious,
		Label:       "VirusTotal malicious",
		Description: "Multiple antivirus engines on VirusTotal flag the domain as malicious.",
	},
	{
		Code:        CodeInspectSafeBrowsing,
		Label:       "Safe Browsing hit",
		Description: "Google Safe Browsing lists the domain as malware, phishing, or unwanted software.",
	},
	{
		Code:        CodeInspectRDAPYoung,
		Label:       "Recently registered",
		Description: "Domain was registered very recently (RDAP) — freshly registered names dominate phishing.",
	},
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
	{
		Code:        CodeInspectCleanEndorsed,
		Label:       "Reputation clean",
		Description: "Reputation checks (VirusTotal / Safe Browsing / domain age) actively endorse the domain as clean.",
	},
}

// Catalog returns a defensive copy of the signal catalog. Returning a clone
// avoids leaking mutability of the package-level slice to callers.
func Catalog() []SignalDescriptor {
	return slices.Clone(signalCatalog)
}

// ScoreCandidates runs the lexical heuristics once over every allowed domain
// and returns each one that triggered at least one signal (Score > 0), with NO
// policy threshold applied. Building the blocked index is the expensive part,
// so callers run this once and bucket by score themselves: CollectSuggest keeps
// the UI-worthy band (>= ThresholdToSuggestBlocking) and the inspect queue
// takes the weaker [MinInspectCandidateScore, ThresholdToSuggestBlocking) band.
func ScoreCandidates(blockedDomains []string, allowedDomains []string) []Suggestion {
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

		if suggestion.Score > 0 {
			result = append(result, suggestion)
		}

	}

	return result
}

// CollectSuggest returns the candidates worth surfacing in the UI on lexical
// signal alone — those at or above ThresholdToSuggestBlocking. It is a thin
// policy filter over ScoreCandidates (the scoring pass runs once); the weaker
// band below the threshold is consumed separately by the inspect queue.
func CollectSuggest(blockedDomains []string, allowedDomains []string) []Suggestion {
	var result []Suggestion
	for _, s := range ScoreCandidates(blockedDomains, allowedDomains) {
		if s.Score >= ThresholdToSuggestBlocking {
			result = append(result, s)
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
		// Skip entries that are themselves a public suffix (e.g. "ru",
		// "co.uk"). They should never have ended up in the blocklist (the
		// source parser now filters them via easy_list.IsSafeDNSDomain), but
		// historical poisoned rows from RuAdList still live in block_lists.
		// If we kept them in subdomainSet, subdomainAncestors would match
		// every *.ru domain as a "subdomain of blocked" and ShouldAutoBlock
		// would mass-promote them — exactly the 2026-05-14 incident.
		if isPublicSuffix(b) {
			continue
		}
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

// isPublicSuffix reports whether domain is itself a public suffix or has no
// registrable eTLD+1. Mirrors the parser-side guard so that even if a poisoned
// row sneaks through (legacy data, future source bugs), auto-block stays safe.
func isPublicSuffix(domain string) bool {
	if domain == "" {
		return true
	}
	_, err := publicsuffix.EffectiveTLDPlusOne(domain)
	return err != nil
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
