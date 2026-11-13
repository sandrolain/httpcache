package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInvalidationCoversCacheKeyHeadersAndVaryVariants(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.Header().Set("Vary", "Accept")
			_, _ = io.WriteString(w, r.Header.Get("Authorization")+"/"+r.Header.Get("Accept"))
		case http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	transport := newMockCacheTransport()
	transport.CacheKeyHeaders = []string{"Authorization"}
	transport.EnableVarySeparation = true
	client := &http.Client{Transport: transport}

	for _, auth := range []string{"Bearer one", "Bearer two"} {
		request, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		request.Header.Set("Authorization", auth)
		request.Header.Set("Accept", "application/json")
		resp, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if requestCount != 2 {
		t.Fatalf("initial origin requests = %d, want 2", requestCount)
	}
	firstVariant, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	firstVariant.Header.Set("Authorization", "Bearer one")
	firstVariant.Header.Set("Accept", "application/json")
	resp, err := client.Do(firstVariant)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Bearer one/application/json" || resp.Header.Get(XFromCache) != "1" {
		t.Fatalf("cached variant = %q from-cache=%q, want Bearer one variant from cache", body, resp.Header.Get(XFromCache))
	}

	post, _ := http.NewRequest(http.MethodPost, server.URL, nil)
	post.Header.Set("Authorization", "Bearer one")
	resp, err = client.Do(post)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	// A mutation for one Authorization variant invalidates every cached
	// representation for the URI, including another header and Vary variant.
	get, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	get.Header.Set("Authorization", "Bearer two")
	get.Header.Set("Accept", "application/json")
	resp, err = client.Do(get)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if requestCount != 4 || resp.Header.Get(XFromCache) == "1" {
		t.Fatalf("requests=%d from-cache=%q, want invalidated origin fetch", requestCount, resp.Header.Get(XFromCache))
	}
}

func TestRangeGETDoesNotDeleteCachedFullResponse(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", "bytes 0-1/4")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "ab")
			return
		}
		_, _ = io.WriteString(w, "abcd")
	}))
	defer server.Close()

	client := &http.Client{Transport: newMockCacheTransport()}
	do := func(rangeHeader string) *http.Response {
		t.Helper()
		request, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		if rangeHeader != "" {
			request.Header.Set("Range", rangeHeader)
		}
		resp, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp
	}

	do("")
	rangeResponse := do("bytes=0-1")
	if rangeResponse.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", rangeResponse.StatusCode)
	}
	fullResponse := do("")
	if requestCount != 2 || fullResponse.Header.Get(XFromCache) != "1" {
		t.Fatalf("requests=%d from-cache=%q, want full response retained after Range GET", requestCount, fullResponse.Header.Get(XFromCache))
	}
}

func TestUnknownMethodInvalidatesAndOptionsDoesNot(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		wantOriginAfter int
		wantCachedAfter bool
	}{
		{name: "unknown method is unsafe", method: "PURGE", wantOriginAfter: 3},
		{name: "OPTIONS is safe", method: http.MethodOptions, wantOriginAfter: 2, wantCachedAfter: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				if r.Method == http.MethodGet {
					w.Header().Set("Cache-Control", "max-age=3600")
					_, _ = io.WriteString(w, "cached")
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			client := &http.Client{Transport: newMockCacheTransport()}
			get := func() *http.Response {
				t.Helper()
				resp, err := client.Get(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				return resp
			}
			get()
			request, _ := http.NewRequest(tt.method, server.URL, nil)
			resp, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			last := get()
			if requestCount != tt.wantOriginAfter || (last.Header.Get(XFromCache) == "1") != tt.wantCachedAfter {
				t.Fatalf("requests=%d from-cache=%q", requestCount, last.Header.Get(XFromCache))
			}
		})
	}
}
