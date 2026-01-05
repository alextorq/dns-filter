package collect

import "strings"

// SuspiciousKeywords — список слов-маркеров для поиска рекламы и трекеров.
var SuspiciousKeywords = []string{
	// --- Реклама (Advertising) ---
	"ad",
	"ads",
	"advert",
	"advertising",
	"adserver",
	"adservice",
	"adsystem",
	"banner",
	"banners",
	"campaign",
	"click",
	"clicks",
	"commercial",
	"creative",
	"doubleclick", // Очень часто встречается, стал нарицательным
	"offer",
	"offers",
	"promo",
	"promotion",
	"sponsor",
	"sponsors",
	"sponsorship",
	"syndication",

	// --- Трекинг и Аналитика (Tracking & Analytics) ---
	"analytics",
	"analytic",
	"analyze",
	"beacon", // Часто используется для пикселей слежения
	"collect",
	"collector",
	"count",
	"counter",
	"event",
	"events",
	"fingerprint",
	"insight",
	"insights",
	"log",
	"logger",
	"measure",
	"measurement",
	"metric",
	"metrics",
	"monitor",
	"pixel",
	"report",
	"reporting",
	"stat",
	"stats",
	"statistic",
	"statistics",
	"tag",
	"tags",
	"tagmanager",
	"telemetry",
	"track",
	"tracker",
	"tracking",
	"userback",

	// --- Ad Tech & RTB (Real-Time Bidding) ---
	"bid",
	"bidder",
	"bidding",
	"cdn-ads",
	"delivery",
	"exchange",
	"impression",
	"prebid",
	"rtb",
	"serve",
	"server", // Осторожно: server встречается часто, лучше проверять в комбинации
	"serving",
	"traffic",
	"yield",
}

// CheckForBadKeywords проверяет наличие плохих слов в частях домена.
// Важно: мы не используем strings.Contains для всей строки, чтобы избежать ложных срабатываний
// (например, чтобы "upload.com" не заблокировался из-за "ad").
func CheckForBadKeywords(domain string) bool {
	// Разбиваем домен на токены по разделителям (точка и дефис)
	// Пример: "my-ads-server.com" -> ["my", "ads", "server", "com"]
	tokens := strings.FieldsFunc(domain, func(r rune) bool {
		return r == '.' || r == '-'
	})

	for _, token := range tokens {
		// Приводим к нижнему регистру для сравнения
		lowerToken := strings.ToLower(token)

		for _, badWord := range SuspiciousKeywords {
			// Точное совпадение токена с плохим словом
			// "ads" == "ads" -> TRUE
			// "upload" == "ad" -> FALSE
			if lowerToken == badWord {
				return true
			}

			// Опционально: можно проверять префиксы/суффиксы,
			// но это увеличит риск ложных срабатываний.
			// Например, "adsystem" содержит "ad", но мы его уже добавили в список целиком.
		}
	}
	return false
}
