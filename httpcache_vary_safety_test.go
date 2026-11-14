package httpcache

import (
	"net/http"
	"testing"
)

func TestVaryQuotedWhitespaceRemainsSignificant(t *testing.T) {
	storedReq, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	storedReq.Header.Set("X-Mode", `"wide  view"`)
	response := &http.Response{Header: http.Header{"Vary": []string{"X-Mode"}}}
	storeVaryHeaders(response, storedReq)

	lookup, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	lookup.Header.Set("X-Mode", `"wide view"`)
	if varyMatches(response, lookup) {
		t.Fatal("quoted whitespace difference must not match a stored Vary value")
	}
	if got := response.Header.Get(headerXVariedPrefix + "X-Mode"); got != `"wide  view"` {
		t.Fatalf("stored quoted value = %q, want original whitespace preserved", got)
	}
	if normalizeHeaderValue(`"a,  b"`) != `"a,  b"` {
		t.Fatal("whitespace around commas inside quoted strings must be preserved")
	}
	if normalizeHeaderValue(`foo "a,  b"`) != `foo "a,  b"` {
		t.Fatal("whitespace before quoted strings must not be removed because of an internal comma")
	}
}

func TestVaryAbsentAndExplicitEmptyAreDistinct(t *testing.T) {
	absent, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	empty, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	empty.Header["X-Mode"] = []string{""}

	responseFromAbsent := &http.Response{Header: http.Header{"Vary": []string{"X-Mode"}}}
	storeVaryHeaders(responseFromAbsent, absent)
	if !varyMatches(responseFromAbsent, absent) {
		t.Fatal("absent field should match another absent field")
	}
	if varyMatches(responseFromAbsent, empty) {
		t.Fatal("absent field must not match an explicitly empty field")
	}

	responseFromEmpty := &http.Response{Header: http.Header{"Vary": []string{"X-Mode"}}}
	storeVaryHeaders(responseFromEmpty, empty)
	if !varyMatches(responseFromEmpty, empty) {
		t.Fatal("explicitly empty field should match another explicitly empty field")
	}
	if varyMatches(responseFromEmpty, absent) {
		t.Fatal("explicitly empty field must not match an absent field")
	}

	if cacheKeyWithVary(absent, []string{"X-Mode"}) == cacheKeyWithVary(empty, []string{"X-Mode"}) {
		t.Fatal("variant keys must distinguish absent and explicitly empty fields")
	}
}

func TestVaryLegacyEmptyPresenceIsConservative(t *testing.T) {
	legacy := &http.Response{Header: http.Header{
		"Vary":                         []string{"X-Mode"},
		headerXVariedPrefix + "X-Mode": []string{""},
	}}
	lookup, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	lookup.Header["X-Mode"] = []string{""}
	if varyMatches(legacy, lookup) {
		t.Fatal("legacy empty value without presence metadata must not be reused")
	}
}

func TestVaryUnknownFieldValuesRemainExact(t *testing.T) {
	tests := []struct {
		name    string
		stored  []string
		request []string
	}{
		{name: "unquoted whitespace", stored: []string{"alpha  beta"}, request: []string{"alpha beta"}},
		{name: "multiple lines versus one comma line", stored: []string{"a", "b"}, request: []string{"a, b"}},
		{name: "different multiple line boundaries", stored: []string{"a", "bc"}, request: []string{"ab", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storedReq, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
			storedReq.Header["X-Opaque"] = append([]string(nil), tt.stored...)
			response := &http.Response{Header: http.Header{"Vary": []string{"X-Opaque"}}}
			storeVaryHeaders(response, storedReq)

			lookup, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
			lookup.Header["X-Opaque"] = append([]string(nil), tt.request...)
			if varyMatches(response, lookup) {
				t.Fatal("unknown field values with different bytes or field-line boundaries must not match")
			}
			if cacheKeyWithVary(storedReq, []string{"X-Opaque"}) == cacheKeyWithVary(lookup, []string{"X-Opaque"}) {
				t.Fatal("variant keys must preserve unknown field values and line boundaries")
			}
		})
	}
}

func TestVaryLegacyUnknownFieldIsConservative(t *testing.T) {
	legacy := &http.Response{Header: http.Header{
		"Vary":                           []string{"X-Opaque"},
		headerXVariedPrefix + "X-Opaque": []string{"alpha beta"},
	}}
	lookup, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	lookup.Header.Set("X-Opaque", "alpha beta")
	if varyMatches(legacy, lookup) {
		t.Fatal("legacy unknown-field value without versioned metadata must not be reused")
	}
}

func TestVaryAllowMethodsRemainCaseSensitive(t *testing.T) {
	storedReq, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	storedReq.Header.Set("Allow", "GET")
	response := &http.Response{Header: http.Header{"Vary": []string{"Allow"}}}
	storeVaryHeaders(response, storedReq)

	lookup, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	lookup.Header.Set("Allow", "get")
	if varyMatches(response, lookup) {
		t.Fatal("Allow method values are case-sensitive")
	}
}
