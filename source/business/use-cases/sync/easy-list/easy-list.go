package easy_list

import (
	"bufio"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

const (
	EasyListURL        = "https://easylist.to/easylist/easylist.txt"
	RuAdListURL        = "https://easylist-downloads.adblockplus.org/ruadlist+easylist.txt"
	AdGuardRussianURL  = "https://filters.adtidy.org/extension/ublock/filters/1.txt"
)

func LoadEasyList() ([]string, error) {
	return LoadFromURL(EasyListURL)
}

func LoadFromURL(url string) ([]string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ParseEasyList(resp.Body), nil
}

// IsSafeDNSDomain проверяет, является ли строка валидным доменом для DNS блокировки.
//
// Помимо очевидных проверок (непустой, без wildcard) отбрасывает «голые»
// public suffix вроде "ru", "co.uk", "xyz". В EasyList такие появляются из
// правил уровня "||ru^$third-party" — браузер применяет их с контекстом
// (третья сторона / domain=...), но DNS-фильтр этого контекста не знает и
// положил бы голый TLD в блок-лист. Дальше subdomainAncestors в auto-block
// нашёл бы этот TLD предком ЛЮБОГО *.ru домена и оптом банил бы рунет —
// инцидент 2026-05-14 с 25 авто-блокировками за один прогон Collect()
// случился именно так после добавления RuAdList.
func IsSafeDNSDomain(domain string) bool {
	if strings.Contains(domain, "*") {
		return false
	}
	if len(domain) == 0 {
		return false
	}
	if isPublicSuffix(domain) {
		return false
	}
	return true
}

// isPublicSuffix reports whether domain is itself a public suffix (or has no
// registrable eTLD+1 — same outcome for our purposes: nothing meaningful to
// block). EffectiveTLDPlusOne returns an error for entries like "ru" / "co.uk"
// and for unknown single-label tokens like "localhost"; both are rejected.
func isPublicSuffix(domain string) bool {
	d := strings.TrimSuffix(strings.ToLower(domain), ".")
	if d == "" {
		return true
	}
	_, err := publicsuffix.EffectiveTLDPlusOne(d)
	return err != nil
}

// dnsSafeModifiers — единственные $-модификаторы, при которых blocking-правило
// ||domain^$... всё ещё означает безусловную блокировку ВСЕГО домена и его
// можно уплощать в голую запись DNS-блок-листа:
//   - important — только приоритет правила, область действия не сужает;
//   - all — наоборот, расширяет на все типы запросов (полная блокировка).
//
// Всё остальное делает правило неуплощаемым:
//   - контекстные (domain=, third-party/3p, popup) — браузер применяет их по
//     контексту страницы/стороны, DNS-фильтр этого контекста не знает;
//   - частичные (script, image, document и прочие типы ресурсов) — сужают
//     правило до конкретного типа запроса;
//   - меняющие действие (badfilter отключает другое правило, dnsrewrite
//     подменяет ответ, csp/removeparam/redirect — это не блокировка).
//
// Срезание $... и блокировка голого домена для таких правил — это и есть баг,
// из-за которого ||mail.ru^$domain=dzen.ru заблокировал mail.ru целиком.
// Подход — allowlist (а не denylist): неизвестный будущий модификатор делает
// правило неуплощаемым и оно отбрасывается — fail-safe в сторону недоблока.
var dnsSafeModifiers = map[string]struct{}{
	"important": {},
	"all":       {},
}

// isFlattenableModifierSet reports whether every modifier in the $-options of
// a blocking rule keeps it an unconditional whole-domain block — i.e. the rule
// may be flattened into a bare-domain DNS block. options is the substring
// after the first '$'. Empty / whitespace-only options (no modifiers, or a
// bare trailing '$') are flattenable.
func isFlattenableModifierSet(options string) bool {
	options = strings.ToLower(strings.TrimSpace(options))
	if options == "" {
		return true
	}
	for _, tok := range strings.Split(options, ",") {
		name := strings.TrimSpace(tok)
		if eq := strings.IndexByte(name, '='); eq != -1 {
			name = strings.TrimSpace(name[:eq])
		}
		if name == "" {
			continue // tolerate stray / leading / trailing commas
		}
		if _, ok := dnsSafeModifiers[name]; !ok {
			return false
		}
	}
	return true
}

func MergeLists(blocked []string, allowed []string) []string {
	// Создаем карту для быстрого поиска разрешенных доменов
	allowMap := make(map[string]bool)
	for _, domain := range allowed {
		allowMap[domain] = true
	}

	var finalBlockList []string

	for _, domain := range blocked {
		// Если домен есть в белом списке — пропускаем его (не блокируем)
		if _, exists := allowMap[domain]; exists {
			// Логируем, что мы помиловали этот домен
			// fmt.Printf("Домен %s исключен из блокировки из-за Whitelist\n", domain)
			continue
		}
		finalBlockList = append(finalBlockList, domain)
	}

	return finalBlockList
}

func ParseEasyList(r io.Reader) []string {
	blacklist := make(map[string]struct{}) // используем map для удаления дубликатов
	whitelist := make(map[string]struct{})

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 1. Пропуск комментариев, заголовков и пустых строк
		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
			continue
		}

		// 2. Пропуск правил скрытия элементов (CSS)
		// Они могут начинаться с ## или domain.com##
		if strings.Contains(line, "##") || strings.Contains(line, "#@#") {
			continue
		}

		// 3. Обработка исключений (Whitelist)
		isException := false
		if strings.HasPrefix(line, "@@") {
			isException = true
			line = line[2:] // Удаляем @@
		}

		// 4. Работаем только с правилами, привязанными к домену (||)
		if !strings.HasPrefix(line, "||") {
			continue
		}
		line = line[2:] // Удаляем ||

		// 5. Опции ($).
		if idx := strings.Index(line, "$"); idx != -1 {
			options := line[idx+1:]
			line = line[:idx]
			// Blocking-правило с контекстными/частичными/действие-меняющими
			// модификаторами нельзя свести к безусловной блокировке домена —
			// пропускаем его целиком (см. dnsSafeModifiers). Для exception-
			// правил (@@) опции просто срезаем: расширение whitelist лишь
			// снимает блокировку и ложно заблокировать домен не может.
			if !isException && !isFlattenableModifierSet(options) {
				continue
			}
		}

		// 6. Проверка на наличие пути
		// Если есть '/', значит правило блокирует конкретный URL -> пропускаем
		if strings.Contains(line, "/") {
			continue
		}

		// 7. Удаляем разделитель ^
		line = strings.ReplaceAll(line, "^", "")

		// 8. Удаляем порт, если есть (example.com:8080)
		if idx := strings.Index(line, ":"); idx != -1 {
			line = line[:idx]
		}

		// 9. Нормализация: нижний регистр + срез anchor-символа '|'
		// (||domain| / ||domain|^). Домены регистронезависимы, а '|' в
		// block_lists сделал бы запись невалидной и непробиваемой запросом.
		line = strings.ToLower(strings.Trim(line, "|"))

		// 10. Финальная валидация
		if IsSafeDNSDomain(line) {
			if isException {
				whitelist[line] = struct{}{}
			} else {
				blacklist[line] = struct{}{}
			}
		}
	}

	// Преобразуем map в slice
	var blockListSlice []string
	for k := range blacklist {
		// Если домен есть в белом списке, не добавляем его в блок
		if _, ok := whitelist[k]; !ok {
			blockListSlice = append(blockListSlice, k)
		}
	}

	var allowListSlice []string
	for k := range whitelist {
		allowListSlice = append(allowListSlice, k)
	}

	merge := MergeLists(blockListSlice, allowListSlice)

	withDot := make([]string, 0, len(merge))
	for _, domain := range merge {
		if strings.HasSuffix(domain, ".") {
			withDot = append(withDot, domain)
			continue
		} else {
			withDot = append(withDot, domain+".")
		}
	}
	return withDot
}
