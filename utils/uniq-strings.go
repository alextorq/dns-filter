package utils

func OnlyUniqString(items []string) []string {
	seen := make(map[string]struct{})
	// pre-allocate memory: длина 0, но емкость (capacity) равна len(items)
	result := make([]string, 0, len(items))

	for _, item := range items {
		if _, exist := seen[item]; !exist {
			result = append(result, item)
			seen[item] = struct{}{}
		}
	}

	return result
}
