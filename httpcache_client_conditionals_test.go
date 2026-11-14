package httpcache

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestFreshCacheEvaluatesClientConditionals(t *testing.T) {
	transport := newMockCacheTransport()
	uri, _ := url.Parse("http://example.test/resource")
	lastModified := time.Now().UTC().Add(-time.Hour).Format(http.TimeFormat)
	makeStored := func() *http.Response {
		header := make(http.Header)
		header.Set("Cache-Control", "max-age=3600")
		header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
		header.Set("ETag", `"current"`)
		header.Set("Last-Modified", lastModified)
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        header,
			Body:          http.NoBody,
			ContentLength: 0,
		}
	}
	tests := []struct {
		name    string
		header  string
		value   string
		want304 bool
	}{
		{name: "weak If-None-Match", header: "If-None-Match", value: `W/"current"`, want304: true},
		{name: "If-None-Match list", header: "If-None-Match", value: `"other", "current"`, want304: true},
		{name: "If-None-Match wildcard", header: "If-None-Match", value: "*", want304: true},
		{name: "non-matching If-None-Match", header: "If-None-Match", value: `"other"`},
		{name: "If-Modified-Since", header: "If-Modified-Since", value: time.Now().UTC().Format(http.TimeFormat), want304: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, uri.String(), nil)
			req.Header.Set(tt.header, tt.value)
			stored := makeStored()
			if _, useCache := transport.handleCachedResponse(stored, req, false); !useCache {
				t.Fatal("fresh stored response was not used")
			}
			wantStatus := http.StatusOK
			if tt.want304 {
				wantStatus = http.StatusNotModified
			}
			if stored.StatusCode != wantStatus {
				t.Fatalf("status = %d, want %d", stored.StatusCode, wantStatus)
			}
		})
	}
}
