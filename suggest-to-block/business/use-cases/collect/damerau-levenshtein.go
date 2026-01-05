package collect

import "math"

// Используется алгоритм Optimal String Alignment (OSA).
func DamerauLevenshtein(source, target string) int {
	// Преобразуем строки в руны для корректной работы с Unicode (например, кириллицей)
	s := []rune(source)
	t := []rune(target)

	n := len(s)
	m := len(t)

	// Базовые случаи: если одна из строк пуста, расстояние равно длине другой
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	// Создаем матрицу (n+1) x (m+1)
	matrix := make([][]int, n+1)
	for i := range matrix {
		matrix[i] = make([]int, m+1)
	}

	// Инициализация первой строки и первого столбца
	// (расстояние от пустой строки до подстроки i или j)
	for i := 0; i <= n; i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= m; j++ {
		matrix[0][j] = j
	}

	// Заполнение матрицы
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			// Стоимость замены: 0 если символы равны, 1 если нет
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}

			// Вычисляем минимальное значение из трех основных операций:
			// 1. Удаление (deletion)
			// 2. Вставка (insertion)
			// 3. Замена (substitution)
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // Удаление
				matrix[i][j-1]+1,      // Вставка
				matrix[i-1][j-1]+cost, // Замена
			)

			// 4. Транспозиция (перестановка соседних символов)
			// Проверяем, можно ли переставить символы местами
			if i > 1 && j > 1 && s[i-1] == t[j-2] && s[i-2] == t[j-1] {
				matrix[i][j] = min(
					matrix[i][j],
					matrix[i-2][j-2]+1, // Стоимость транспозиции = 1
				)
			}
		}
	}

	return matrix[n][m]
}

// Similarity возвращает процент сходства от 0.0 до 100.0
func Similarity(source, target string) float64 {
	// 1. Получаем абсолютное расстояние
	distance := DamerauLevenshtein(source, target)

	// 2. Считаем длины в рунах (важно для кириллицы)
	rSource := []rune(source)
	rTarget := []rune(target)
	lenS := len(rSource)
	lenT := len(rTarget)

	// 3. Находим максимальную длину
	maxLen := lenS
	if lenT > maxLen {
		maxLen = lenT
	}

	// Защита от деления на ноль (если обе строки пустые)
	if maxLen == 0 {
		return 100.0
	}

	// 4. Вычисляем процент
	// Формула: (1 - расстояние / макс_длина) * 100
	percentage := (1.0 - float64(distance)/float64(maxLen)) * 100.0

	// Опционально: округляем до 2 знаков после запятой
	return math.Round(percentage*100) / 100
}

// Вспомогательная функция для поиска минимума из вариативного числа аргументов
func min(nums ...int) int {
	if len(nums) == 0 {
		return 0
	}
	m := nums[0]
	for _, v := range nums[1:] {
		if v < m {
			m = v
		}
	}
	return m
}
