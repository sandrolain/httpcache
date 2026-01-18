package httpcache

import (
	"testing"
)

// TestNormalizeHeaderValueOptimized tests the optimized normalizeHeaderValue function
func TestNormalizeHeaderValueOptimized(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
		{
			name:     "simple value",
			input:    "gzip",
			expected: "gzip",
		},
		{
			name:     "comma-separated with spaces",
			input:    "en, fr, it",
			expected: "en,fr,it",
		},
		{
			name:     "comma-separated without spaces",
			input:    "en,fr,it",
			expected: "en,fr,it",
		},
		{
			name:     "multiple spaces",
			input:    "en,  fr,   it",
			expected: "en,fr,it",
		},
		{
			name:     "tabs and newlines",
			input:    "en,\tfr,\nit",
			expected: "en,fr,it",
		},
		{
			name:     "leading and trailing whitespace",
			input:    "  gzip, deflate  ",
			expected: "gzip,deflate",
		},
		{
			name:     "mixed whitespace",
			input:    "  en , fr , it  ",
			expected: "en,fr,it",
		},
		{
			name:     "internal spaces preserved (not near comma)",
			input:    "text html",
			expected: "text html",
		},
		{
			name:     "space before comma removed",
			input:    "en ,fr",
			expected: "en,fr",
		},
		{
			name:     "space after comma removed",
			input:    "en, fr",
			expected: "en,fr",
		},
		{
			name:     "multiple consecutive spaces",
			input:    "en    fr",
			expected: "en fr",
		},
		{
			name:     "complex Accept-Language",
			input:    "en-US, en;q=0.9, fr;q=0.8",
			expected: "en-US,en;q=0.9,fr;q=0.8",
		},
		{
			name:     "Accept-Encoding with weights",
			input:    "gzip, deflate, br;q=1.0, *;q=0.5",
			expected: "gzip,deflate,br;q=1.0,*;q=0.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeHeaderValue(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeHeaderValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNormalizeHeaderValueConsistency verifies that equivalent inputs produce same output
func TestNormalizeHeaderValueConsistency(t *testing.T) {
	equivalentPairs := []struct {
		name   string
		value1 string
		value2 string
	}{
		{
			name:   "comma with and without space",
			value1: "en, fr",
			value2: "en,fr",
		},
		{
			name:   "multiple spaces",
			value1: "en,  fr",
			value2: "en,fr",
		},
		{
			name:   "tabs vs spaces",
			value1: "en,\tfr",
			value2: "en, fr",
		},
		{
			name:   "leading/trailing whitespace",
			value1: "  gzip, deflate  ",
			value2: "gzip,deflate",
		},
		{
			name:   "mixed whitespace types",
			value1: "en ,\t fr \n, it",
			value2: "en,fr,it",
		},
	}

	for _, tt := range equivalentPairs {
		t.Run(tt.name, func(t *testing.T) {
			norm1 := normalizeHeaderValue(tt.value1)
			norm2 := normalizeHeaderValue(tt.value2)

			if norm1 != norm2 {
				t.Errorf("normalized values should match:\n  normalizeHeaderValue(%q) = %q\n  normalizeHeaderValue(%q) = %q",
					tt.value1, norm1, tt.value2, norm2)
			}
		})
	}
}

// TestNormalizedHeaderValuesMatchOptimized tests the fast path and normalized matching
func TestNormalizedHeaderValuesMatchOptimized(t *testing.T) {
	tests := []struct {
		name     string
		value1   string
		value2   string
		expected bool
	}{
		{
			name:     "exact match (fast path)",
			value1:   "gzip",
			value2:   "gzip",
			expected: true,
		},
		{
			name:     "different values",
			value1:   "gzip",
			value2:   "deflate",
			expected: false,
		},
		{
			name:     "whitespace difference",
			value1:   "en, fr",
			value2:   "en,fr",
			expected: true,
		},
		{
			name:     "both empty",
			value1:   "",
			value2:   "",
			expected: true,
		},
		{
			name:     "one empty, one not",
			value1:   "gzip",
			value2:   "",
			expected: false,
		},
		{
			name:     "whitespace-only vs empty",
			value1:   "   ",
			value2:   "",
			expected: true,
		},
		{
			name:     "complex headers match",
			value1:   "gzip, deflate, br;q=1.0",
			value2:   "gzip,deflate,br;q=1.0",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizedHeaderValuesMatch(tt.value1, tt.value2)
			if result != tt.expected {
				t.Errorf("normalizedHeaderValuesMatch(%q, %q) = %v, want %v",
					tt.value1, tt.value2, result, tt.expected)
			}
		})
	}
}
