package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSharedCacheAuthorizationReuse(t *testing.T) {
	tests := []struct {
		name               string
		isPublicCache      bool
		cacheControl       string
		wantOriginRequests int
		wantBody           string
		wantCacheHit       bool
	}{
		{
			name:               "shared cache rejects anonymous response without permission",
			isPublicCache:      true,
			cacheControl:       "max-age=3600",
			wantOriginRequests: 2,
			wantBody:           "authorized response",
		},
		{
			name:               "public permits reuse",
			isPublicCache:      true,
			cacheControl:       "public, max-age=3600",
			wantOriginRequests: 1,
			wantBody:           "anonymous response",
			wantCacheHit:       true,
		},
		{
			name:               "must-revalidate permits reuse while fresh",
			isPublicCache:      true,
			cacheControl:       "must-revalidate, max-age=3600",
			wantOriginRequests: 1,
			wantBody:           "anonymous response",
			wantCacheHit:       true,
		},
		{
			name:               "s-maxage permits reuse",
			isPublicCache:      true,
			cacheControl:       "s-maxage=3600",
			wantOriginRequests: 1,
			wantBody:           "anonymous response",
			wantCacheHit:       true,
		},
		{
			name:               "private cache behavior is unchanged",
			isPublicCache:      false,
			cacheControl:       "max-age=3600",
			wantOriginRequests: 1,
			wantBody:           "anonymous response",
			wantCacheHit:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originRequests++
				w.Header().Set("Cache-Control", tt.cacheControl)
				if r.Header.Get("Authorization") != "" {
					_, _ = io.WriteString(w, "authorized response")
					return
				}
				_, _ = io.WriteString(w, "anonymous response")
			}))
			defer server.Close()

			transport := newMockCacheTransport()
			transport.IsPublicCache = tt.isPublicCache
			transport.MarkCachedResponses = true
			client := transport.Client()

			request, err := http.NewRequest(http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()

			request, err = http.NewRequest(http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer user-token")
			response, err = client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}

			if originRequests != tt.wantOriginRequests {
				t.Fatalf("origin request count = %d, want %d", originRequests, tt.wantOriginRequests)
			}
			if string(body) != tt.wantBody {
				t.Fatalf("response body = %q, want %q", body, tt.wantBody)
			}
			gotCacheHit := response.Header.Get(XFromCache) == "1"
			if gotCacheHit != tt.wantCacheHit {
				t.Fatalf("cache hit = %v, want %v", gotCacheHit, tt.wantCacheHit)
			}
		})
	}
}

func TestSharedCacheAuthorizationDoesNotReuseStaleAnonymousResponse(t *testing.T) {
	tests := []struct {
		name                string
		cacheControl        string
		onAuthorizedRequest func(http.ResponseWriter)
		requestCacheControl string
		wantOriginRequests  int
		wantStatus          int
		wantBody            string
	}{
		{
			name:         "stale-while-revalidate response is bypassed",
			cacheControl: "max-age=0, stale-while-revalidate=60",
			onAuthorizedRequest: func(w http.ResponseWriter) {
				w.Header().Set("Cache-Control", "max-age=3600")
				_, _ = io.WriteString(w, "authorized response")
			},
			wantOriginRequests: 2,
			wantStatus:         http.StatusOK,
			wantBody:           "authorized response",
		},
		{
			name:         "stale-if-error response is not served on origin error",
			cacheControl: "max-age=0, stale-if-error=60",
			onAuthorizedRequest: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "origin error")
			},
			wantOriginRequests: 2,
			wantStatus:         http.StatusServiceUnavailable,
			wantBody:           "origin error",
		},
		{
			name:                "only-if-cached does not return disallowed entry",
			cacheControl:        "max-age=0, stale-if-error=60, stale-while-revalidate=60",
			requestCacheControl: "only-if-cached",
			wantOriginRequests:  1,
			wantStatus:          http.StatusGatewayTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originRequests++
				if r.Header.Get("Authorization") == "" {
					w.Header().Set("Cache-Control", tt.cacheControl)
					_, _ = io.WriteString(w, "anonymous response")
					return
				}
				if tt.onAuthorizedRequest != nil {
					tt.onAuthorizedRequest(w)
				}
			}))
			defer server.Close()

			transport := newMockCacheTransport()
			transport.IsPublicCache = true
			transport.MarkCachedResponses = true
			client := transport.Client()

			request, err := http.NewRequest(http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()

			request, err = http.NewRequest(http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer user-token")
			if tt.requestCacheControl != "" {
				request.Header.Set("Cache-Control", tt.requestCacheControl)
			}
			response, err = client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}

			if originRequests != tt.wantOriginRequests {
				t.Fatalf("origin request count = %d, want %d", originRequests, tt.wantOriginRequests)
			}
			if response.StatusCode != tt.wantStatus {
				t.Fatalf("response status = %d, want %d", response.StatusCode, tt.wantStatus)
			}
			if string(body) != tt.wantBody {
				t.Fatalf("response body = %q, want %q", body, tt.wantBody)
			}
			if response.Header.Get(XStale) == "1" {
				t.Fatal("disallowed anonymous cached response was served stale")
			}
		})
	}
}

func TestSharedCacheAuthorizationPresenceIncludesEmptyValue(t *testing.T) {
	t.Run("empty Authorization request bypasses anonymous cache entry", func(t *testing.T) {
		originRequests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			originRequests++
			w.Header().Set("Cache-Control", "max-age=3600")
			_, _ = io.WriteString(w, "origin response")
		}))
		defer server.Close()

		transport := newMockCacheTransport()
		transport.IsPublicCache = true
		transport.MarkCachedResponses = true
		client := transport.Client()

		request, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()

		request, err = http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header["Authorization"] = []string{""}
		response, err = client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()

		if originRequests != 2 {
			t.Fatalf("origin request count = %d, want 2", originRequests)
		}
		if response.Header.Get(XFromCache) == "1" {
			t.Fatal("empty Authorization request reused anonymous cache entry")
		}
	})

	t.Run("empty Authorization request does not store response", func(t *testing.T) {
		originRequests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			originRequests++
			w.Header().Set("Cache-Control", "max-age=3600")
			_, _ = io.WriteString(w, "origin response")
		}))
		defer server.Close()

		transport := newMockCacheTransport()
		transport.IsPublicCache = true
		transport.MarkCachedResponses = true
		client := transport.Client()

		for i := 0; i < 2; i++ {
			request, err := http.NewRequest(http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header["Authorization"] = []string{""}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.Header.Get(XFromCache) == "1" {
				t.Fatalf("request %d unexpectedly used a cached response", i+1)
			}
		}

		if originRequests != 2 {
			t.Fatalf("origin request count = %d, want 2", originRequests)
		}
	})
}
