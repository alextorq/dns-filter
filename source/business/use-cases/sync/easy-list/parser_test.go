package easy_list

import (
	"sort"
	"strings"
	"testing"
)

func TestParseEasyList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Empty input",
			input:    "",
			expected: []string{}, // Ожидаем пустой слайс (или nil)
		},
		{
			name: "Basic blocking",
			input: `
||example.com^
||test.org^
`,
			expected: []string{"example.com", "test.org"},
		},
		{
			name: "Whitelist logic (Merge check)",
			input: `
||bad.com^
||good.com^
@@||good.com^
`,
			expected: []string{"bad.com"}, // good.com должен исчезнуть
		},
		{
			name: "Ignore paths and files",
			input: `
||site.com/ads/
||domain.com/banner.jpg
||clean-domain.com^
`,
			expected: []string{"clean-domain.com"},
		},
		{
			name: "Strip options and ports",
			input: `
||options.com^$third-party,popup
||port-test.com:8080^
`,
			expected: []string{"options.com", "port-test.com"},
		},
		{
			name: "Ignore CSS rules and wildcards",
			input: `
##div.ad
example.com##.banner
||wildcard.*^
||valid.com^
`,
			expected: []string{"valid.com"},
		},
		{
			name: "Mixed complex case",
			input: `
! Comments
[Adblock Plus 2.0]
||blocked.com^
@@||friend.com^
||friend.com^$third-party
||trash.com/file
||weird-port.com:999^
`,
			expected: []string{"blocked.com", "weird-port.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем Reader из строки
			reader := strings.NewReader(tt.input)

			got := ParseEasyList(reader)

			// ВАЖНО: Сортируем полученный и ожидаемый списки,
			// так как порядок итерации по map в Go не гарантирован.
			sort.Strings(got)
			sort.Strings(tt.expected)

			// Сравниваем длины
			if len(got) != len(tt.expected) {
				t.Errorf("ParseEasyList() length = %v, want %v. \nGot: %v\nWant: %v", len(got), len(tt.expected), got, tt.expected)
				return
			}

			// Сравниваем элементы
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("ParseEasyList() mismatch at index %d \nGot: %s\nWant: %s", i, got[i], tt.expected[i])
				}
			}
		})
	}
}
