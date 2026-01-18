package httpcache

import (
	"net/http"
	"testing"
)

// BenchmarkNormalizeHeaderValue benchmarks the optimized single-pass implementation
func BenchmarkNormalizeHeaderValue(b *testing.B) {
	tests := []struct {
		name  string
		value string
	}{
		{"simple", "gzip"},
		{"comma_with_spaces", "en, fr, it"},
		{"comma_without_spaces", "en,fr,it"},
		{"complex_accept_language", "en-US, en;q=0.9, fr;q=0.8, it;q=0.7"},
		{"accept_encoding", "gzip, deflate, br;q=1.0, *;q=0.5"},
		{"with_whitespace", "  gzip, deflate  "},
		{"tabs_and_newlines", "en,\tfr,\nit"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = normalizeHeaderValue(tt.value)
			}
		})
	}
}

// BenchmarkNormalizedHeaderValuesMatch benchmarks the fast path optimization
func BenchmarkNormalizedHeaderValuesMatch(b *testing.B) {
	tests := []struct {
		name   string
		value1 string
		value2 string
	}{
		{"exact_match", "gzip", "gzip"},
		{"whitespace_diff", "en, fr", "en,fr"},
		{"different_values", "gzip", "deflate"},
		{"both_empty", "", ""},
		{"complex_match", "gzip, deflate, br;q=1.0", "gzip,deflate,br;q=1.0"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = normalizedHeaderValuesMatch(tt.value1, tt.value2)
			}
		})
	}
}

// BenchmarkVaryMatches benchmarks the single-pass varyMatches implementation
func BenchmarkVaryMatches(b *testing.B) {
	// Setup cached response with Vary headers
	cachedResp := &http.Response{
		Header: http.Header{
			"Vary":                     []string{"Accept-Encoding, Accept-Language"},
			"X-Varied-Accept-Encoding": []string{"gzip,deflate"},
			"X-Varied-Accept-Language": []string{"en-US,en;q=0.9"},
		},
	}

	tests := []struct {
		name        string
		req         *http.Request
		description string
	}{
		{
			name: "matching_headers",
			req: &http.Request{
				Header: http.Header{
					"Accept-Encoding": []string{"gzip, deflate"},
					"Accept-Language": []string{"en-US, en;q=0.9"},
				},
			},
			description: "Headers that match (with whitespace normalization)",
		},
		{
			name: "non_matching_headers",
			req: &http.Request{
				Header: http.Header{
					"Accept-Encoding": []string{"br"},
					"Accept-Language": []string{"fr"},
				},
			},
			description: "Headers that don't match",
		},
		{
			name: "exact_match",
			req: &http.Request{
				Header: http.Header{
					"Accept-Encoding": []string{"gzip,deflate"},
					"Accept-Language": []string{"en-US,en;q=0.9"},
				},
			},
			description: "Exact match (fast path)",
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = varyMatches(cachedResp, tt.req)
			}
		})
	}
}

// BenchmarkVaryMatchesWithStar benchmarks the early return for Vary: *
func BenchmarkVaryMatchesWithStar(b *testing.B) {
	cachedResp := &http.Response{
		Header: http.Header{
			"Vary": []string{"*"},
		},
	}

	req := &http.Request{
		Header: http.Header{
			"Accept-Encoding": []string{"gzip"},
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = varyMatches(cachedResp, req)
	}
}

// BenchmarkVaryMatchesNoVary benchmarks the fast path for no Vary headers
func BenchmarkVaryMatchesNoVary(b *testing.B) {
	cachedResp := &http.Response{
		Header: http.Header{},
	}

	req := &http.Request{
		Header: http.Header{
			"Accept-Encoding": []string{"gzip"},
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = varyMatches(cachedResp, req)
	}
}

// BenchmarkVaryMatchesMultipleHeaders benchmarks with many vary headers
func BenchmarkVaryMatchesMultipleHeaders(b *testing.B) {
	cachedResp := &http.Response{
		Header: http.Header{
			"Vary":                     []string{"Accept-Encoding, Accept-Language, User-Agent, Accept, Origin"},
			"X-Varied-Accept-Encoding": []string{"gzip,deflate"},
			"X-Varied-Accept-Language": []string{"en-US"},
			"X-Varied-User-Agent":      []string{"Mozilla/5.0"},
			"X-Varied-Accept":          []string{"text/html"},
			"X-Varied-Origin":          []string{"https://example.com"},
		},
	}

	req := &http.Request{
		Header: http.Header{
			"Accept-Encoding": []string{"gzip, deflate"},
			"Accept-Language": []string{"en-US"},
			"User-Agent":      []string{"Mozilla/5.0"},
			"Accept":          []string{"text/html"},
			"Origin":          []string{"https://example.com"},
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = varyMatches(cachedResp, req)
	}
}
