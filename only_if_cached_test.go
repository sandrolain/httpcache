package httpcache

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestOnlyIfCachedDoesNotForwardWhenEntryCannotSatisfy(t *testing.T) {
	tests := []struct {
		name            string
		responseControl string
		requestControl  string
	}{
		{name: "stale entry", responseControl: "max-age=0"},
		{name: "must-revalidate entry", responseControl: "max-age=0, must-revalidate"},
		{name: "request no-cache", responseControl: "max-age=3600", requestControl: "no-cache"},
		{name: "stale-if-error is not an origin failure", responseControl: "max-age=0, stale-if-error=600"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Cache-Control", tt.responseControl)
				_, _ = io.WriteString(w, "cached body")
			}))
			defer server.Close()

			transport := newMockCacheTransport()
			client := &http.Client{Transport: transport}
			first, err := client.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, first.Body)
			_ = first.Body.Close()

			request, err := http.NewRequest(http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			control := "only-if-cached"
			if tt.requestControl != "" {
				control += ", " + tt.requestControl
			}
			request.Header.Set("Cache-Control", control)
			resp, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusGatewayTimeout {
				t.Fatalf("status = %d, want 504", resp.StatusCode)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("origin requests = %d, want 1; only-if-cached forwarded", got)
			}
		})
	}
}

func TestOnlyIfCachedVaryMismatchReturnsGatewayTimeout(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Vary", "Accept")
		_, _ = io.WriteString(w, r.Header.Get("Accept"))
	}))
	defer server.Close()

	transport := newMockCacheTransport()
	transport.EnableVarySeparation = true
	client := &http.Client{Transport: transport}
	first, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	first.Header.Set("Accept", "application/json")
	resp, err := client.Do(first)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	second, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	second.Header.Set("Accept", "text/plain")
	second.Header.Set("Cache-Control", "only-if-cached")
	resp, err = client.Do(second)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 for a Vary mismatch", resp.StatusCode)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("origin requests = %d, want 1; only-if-cached forwarded", got)
	}
}

func TestOnlyIfCachedMarkedStaleReturnsGatewayTimeout(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = io.WriteString(w, "cached body")
	}))
	defer server.Close()

	transport := newMockCacheTransport()
	transport.EnableStaleMarking = true
	client := &http.Client{Transport: transport}
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, first.Body)
	_ = first.Body.Close()
	if err := transport.cacheMarkStale(context.Background(), cacheKey(request)); err != nil {
		t.Fatal(err)
	}

	request2, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request2.Header.Set("Cache-Control", "only-if-cached")
	resp, err := client.Do(request2)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 for a marked-stale entry", resp.StatusCode)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("origin requests = %d, want 1; only-if-cached forwarded", got)
	}
}
