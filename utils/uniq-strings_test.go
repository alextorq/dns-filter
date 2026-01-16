package utils

import (
	"reflect"
	"testing"
)

func TestOnlyUniqString(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single element",
			input:    []string{"a"},
			expected: []string{"a"},
		},
		{
			name:     "all unique elements",
			input:    []string{"a", "b", "c", "d"},
			expected: []string{"a", "b", "c", "d"},
		},
		{
			name:     "with duplicates",
			input:    []string{"a", "b", "a", "c", "b", "d"},
			expected: []string{"a", "b", "c", "d"},
		},
		{
			name:     "all same elements",
			input:    []string{"a", "a", "a", "a"},
			expected: []string{"a"},
		},
		{
			name:     "preserves order",
			input:    []string{"z", "a", "m", "z", "a"},
			expected: []string{"z", "a", "m"},
		},
		{
			name:     "empty strings",
			input:    []string{"", "a", "", "b"},
			expected: []string{"", "a", "b"},
		},
		{
			name:     "strings with spaces",
			input:    []string{"hello", "world", "hello", "  world"},
			expected: []string{"hello", "world", "  world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OnlyUniqString(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("OnlyUniqString(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func BenchmarkOnlyUniqString(b *testing.B) {
	input := []string{"a", "b", "c", "d", "e", "a", "b", "c", "f", "g"}
	for i := 0; i < b.N; i++ {
		OnlyUniqString(input)
	}
}
