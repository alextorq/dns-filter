package collect

import (
	"math"
	"strings"
)

// Константы для настройки чувствительности
const (
	// EntropyThreshold: выше этого значения строка считается хаосом.
	// 4.2 - консервативно (меньше ложных срабатываний).
	// 3.8 - агрессивно (ловит больше, но может зацепить легитимные).
	EntropyThreshold = 4.0

	// MinLengthForCheck: слишком короткие строки (например "abc") нет смысла проверять на энтропию.
	MinLengthForCheck = 5

	// MaxConsonantRatio: если согласных больше 78%, это подозрительно (для англ. языка).
	MaxConsonantRatio = 0.78
)

// IsDomainSuspicious — главная функция.
// Принимает домен (можно с точкой в конце: "github.com.").
// Возвращает true, если домен похож на сгенерированный (DGA/мусор).
func IsDomainSuspicious(domain string) bool {
	// 1. Нормализация: убираем точку в конце и приводим к нижнему регистру
	cleanDomain := strings.TrimSuffix(domain, ".")
	cleanDomain = strings.ToLower(cleanDomain)

	// 2. Разбиваем на части
	parts := strings.Split(cleanDomain, ".")

	// Если частей меньше 2 (например "localhost"), считаем нормальным
	if len(parts) < 2 {
		return false
	}

	// 3. Анализируем части домена.
	// Важно: мы пропускаем последнюю часть (TLD), т.е. "com", "net", "org",
	// так как анализировать их на энтропию бессмысленно.
	// Анализируем parts[0] ... parts[len-2]
	partsToCheck := parts[:len(parts)-1]

	for _, part := range partsToCheck {
		// Если хотя бы одна часть домена подозрительная — блокируем весь домен.
		// Например в "bad-hash-x8z7c.google.com" часть "google" ок, но "bad-hash..." подозрительная.
		if isPartSuspicious(part) {
			return true
		}
	}

	return false
}

// isPartSuspicious анализирует конкретную метку (label) домена
func isPartSuspicious(s string) bool {
	// Пропускаем короткие строки
	if len(s) < MinLengthForCheck {
		return false
	}

	// 1. Проверка на Энтропию Шеннона
	entropy := calculateShannonEntropy(s)

	// Если энтропия экстремально высокая — это точно мусор
	if entropy > 4.5 {
		return true
	}

	// 2. Лингвистический анализ (если энтропия средняя)
	// Если энтропия просто "высоковата" (> 3.5), подключаем проверку согласных.
	if entropy > 3.5 {
		cRatio := calculateConsonantRatio(s)
		// Высокая энтропия + почти нет гласных = DGA
		if cRatio > MaxConsonantRatio {
			return true
		}

		// Дополнительно: если энтропия выше порога, возвращаем true
		if entropy > EntropyThreshold {
			return true
		}
	}

	return false
}

// calculateShannonEntropy вычисляет энтропию
func calculateShannonEntropy(s string) float64 {
	counts := make(map[rune]int)
	for _, r := range s {
		counts[r]++
	}

	var entropy float64
	total := float64(len(s))

	for _, count := range counts {
		freq := float64(count) / total
		entropy -= freq * math.Log2(freq)
	}
	return entropy
}

// calculateConsonantRatio считает процент согласных букв
func calculateConsonantRatio(s string) float64 {
	vowels := "aeiouy" // Гласные
	consonants := 0
	letters := 0

	for _, r := range s {
		// Проверяем, буква ли это (a-z)
		if r >= 'a' && r <= 'z' {
			letters++
			// Если не гласная — считаем согласной
			if !strings.ContainsRune(vowels, r) {
				consonants++
			}
		}
	}

	if letters == 0 {
		return 0
	}
	return float64(consonants) / float64(letters)
}
