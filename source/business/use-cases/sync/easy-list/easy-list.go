package easy_list

import (
	"bufio"
	"io"
	"net/http"
	"strings"
)

func LoadEasyList() ([]string, error) {
	resp, err := http.Get("https://easylist.to/easylist/easylist.txt")
	if err == nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	result := ParseEasyList(resp.Body)
	return result, nil
}

// IsSafeDNSDomain проверяет, является ли строка валидным доменом для DNS блокировки
func IsSafeDNSDomain(domain string) bool {
	// Домен не должен содержать звездочек (если ваш DNS не поддерживает regex)
	if strings.Contains(domain, "*") {
		return false
	}
	// Домен не должен быть пустым
	if len(domain) == 0 {
		return false
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

		// 5. Очистка опций ($)
		if idx := strings.Index(line, "$"); idx != -1 {
			// Тут можно добавить логику проверки опций, например:
			// options := line[idx+1:]
			// if strings.Contains(options, "third-party") { ... }

			line = line[:idx]
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

		// 9. Финальная валидация
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
