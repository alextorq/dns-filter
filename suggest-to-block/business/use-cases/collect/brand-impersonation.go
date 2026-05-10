package collect

import "strings"

// KnownBrands — apex'ы топ-брендов, эксплуатируемых в фишинге через
// typosquatting. Все ключи: lower-case, без trailing dots, ровно 2 лейбла
// (eTLD+1). Список handcrafted; обновлять руками по мере необходимости.
//
// Ограничение: для apex'ов на multi-label public suffix (`bbc.co.uk`)
// использование «последние 2 лейбла» даст `co.uk`, и такие бренды не
// получится покрыть напрямую. В текущем bundle ни одного `*.co.uk` нет —
// FP в сторону пропуска приемлем для Phase 1.
var KnownBrands = map[string]struct{}{
	// --- Глобал-tech ---
	"google.com":    {},
	"microsoft.com": {},
	"amazon.com":    {},
	"apple.com":     {},
	"facebook.com":  {},
	"instagram.com": {},
	"youtube.com":   {},
	"twitter.com":   {},
	"x.com":         {},
	"linkedin.com":  {},

	// --- Mail / messaging ---
	"gmail.com":     {},
	"outlook.com":   {},
	"yahoo.com":     {},
	"whatsapp.com":  {},
	"telegram.org":  {},
	"discord.com":   {},
	"zoom.us":       {},
	"signal.org":    {},

	// --- Соцсети / медиа ---
	"tiktok.com":    {},
	"reddit.com":    {},
	"pinterest.com": {},
	"snapchat.com":  {},
	"twitch.tv":     {},

	// --- Streaming ---
	"netflix.com":  {},
	"spotify.com":  {},
	"disney.com":   {},

	// --- E-commerce / payment ---
	"paypal.com":     {},
	"ebay.com":       {},
	"shopify.com":    {},
	"etsy.com":       {},
	"alibaba.com":    {},
	"aliexpress.com": {},
	"walmart.com":    {},
	"target.com":     {},

	// --- Cloud / dev / SaaS ---
	"github.com":     {},
	"gitlab.com":     {},
	"dropbox.com":    {},
	"slack.com":      {},
	"notion.so":      {},
	"atlassian.com":  {},
	"cloudflare.com": {},
	"openai.com":     {},
	"anthropic.com":  {},
	"icloud.com":     {},

	// --- Crypto ---
	"binance.com":    {},
	"coinbase.com":   {},
	"kraken.com":     {},
	"metamask.io":    {},
	"blockchain.com": {},

	// --- Банки intl ---
	"chase.com":          {},
	"hsbc.com":           {},
	"wellsfargo.com":     {},
	"bankofamerica.com":  {},

	// --- RU: поиск / соцсети / медиа ---
	"yandex.ru":     {},
	"mail.ru":       {},
	"vk.com":        {},
	"ok.ru":         {},
	"dzen.ru":       {},
	"kinopoisk.ru":  {},

	// --- RU: банки ---
	"sberbank.ru":     {},
	"sber.ru":         {},
	"tinkoff.ru":      {},
	"alfabank.ru":     {},
	"vtb.ru":          {},
	"raiffeisen.ru":   {},
	"gazprombank.ru":  {},
	"otkritie.ru":     {},

	// --- RU: госсервисы ---
	"gosuslugi.ru": {},
	"nalog.ru":     {},
	"mos.ru":       {},

	// --- RU: e-commerce ---
	"ozon.ru":        {},
	"wildberries.ru": {},
	"avito.ru":       {},
	"lamoda.ru":      {},
	"citilink.ru":    {},
	"mvideo.ru":      {},
	"eldorado.ru":    {},

	// --- RU: транспорт / job / гео ---
	"rzd.ru":     {},
	"aeroflot.ru": {},
	"s7.ru":      {},
	"pochta.ru":  {},
	"hh.ru":      {},
	"auto.ru":    {},
	"2gis.ru":    {},
	"drom.ru":    {},
}

// BrandSimilarityThreshold — нижняя граница процентного сходства
// (Damerau-Levenshtein) между apex'ом кандидата и известным брендом,
// при которой кандидат считается impersonation'ом. 80% покрывает типовые
// замены 1-2 символов (`goog1e.com`, `paypa1.com`, `arnazon.com`) на
// брендах длиной 7+ рун и не срабатывает на коротких различиях, не
// связанных с typosquat'ом.
const BrandSimilarityThreshold = 80.0

// MinBrandImpersonationLength — минимальная длина (в рунах) и apex'а, и
// бренда, при которой пара участвует в сравнении. На 5-рунных строках
// одна замена даёт ровно 80% similarity, что превращает любой случайный
// 5-рунный домен (`vc.com`, `s8.ru`) в typosquat соседнего короткого
// бренда (`vk.com`, `s7.ru`). Порог 7 убирает этот FP-класс.
const MinBrandImpersonationLength = 7

// IsBrandImpersonation reports whether domain's apex (последние 2 лейбла)
// очень похож на один из KnownBrands по Damerau-Levenshtein, но не равен ему.
// Сравнение case-insensitive; trailing dots нормализуются.
//
// Сравнивается **apex целиком** (включая TLD), не первый лейбл — это сужает
// сигнал до типичного typosquat-сценария «тот же или близкий TLD»
// (`google.com → goog1e.com`) и игнорирует случаи с разным TLD
// (`paypal.com vs paypa1.tk`), которые относятся к другому классу сигналов.
//
// Single-label / пустой / только TLD → false. Apex с любым non-ASCII символом
// или punycode-лейблом (`xn--`) → false: homograph-typosquat покрывается
// отдельным сигналом (Task 4), здесь не дублируется. Apex короче
// MinBrandImpersonationLength рун → false (см. константу): на коротких
// строках процентное сходство теряет различимость и даёт массовые FP.
func IsBrandImpersonation(domain string) bool {
	apex := extractApex(domain)
	if apex == "" {
		return false
	}
	for _, r := range apex {
		if r > 127 {
			return false
		}
	}
	for _, label := range strings.Split(apex, ".") {
		if strings.HasPrefix(label, "xn--") {
			return false
		}
	}
	if len(apex) < MinBrandImpersonationLength {
		return false
	}
	if _, ok := KnownBrands[apex]; ok {
		return false
	}
	for brand := range KnownBrands {
		if len(brand) < MinBrandImpersonationLength {
			continue
		}
		if SimilarityAtLeast(apex, brand, BrandSimilarityThreshold) {
			return true
		}
	}
	return false
}

// extractApex возвращает последние 2 лейбла домена в lower-case, без
// trailing dots. Для single-label / пустого / TLD-only входа возвращает
// пустую строку.
func extractApex(domain string) string {
	d := strings.TrimRight(strings.ToLower(domain), ".")
	if d == "" {
		return ""
	}
	parts := strings.Split(d, ".")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[len(parts)-2:], ".")
}
