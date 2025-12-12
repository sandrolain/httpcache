package httpcache

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStaleMarkingSystem(t *testing.T) {
	t.Run("mark entry as stale", func(t *testing.T) {
		cache := newMockCache()
		key := "test-key"
		data := []byte("test data")

		// Set data
		err := cache.Set(context.Background(), key, data)
		if err != nil {
			t.Fatalf("Failed to set cache: %v", err)
		}

		// Mark as stale
		err = cache.MarkStale(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to mark as stale: %v", err)
		}

		// Verify it's marked as stale
		isStale, err := cache.IsStale(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to check stale: %v", err)
		}
		if !isStale {
			t.Error("Expected entry to be marked as stale")
		}
	})

	t.Run("get stale entry", func(t *testing.T) {
		cache := newMockCache()
		key := "test-key"
		data := []byte("test data")

		// Set and mark as stale
		_ = cache.Set(context.Background(), key, data)
		_ = cache.MarkStale(context.Background(), key)

		// Get stale
		staleData, ok, err := cache.GetStale(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to get stale: %v", err)
		}
		if !ok {
			t.Error("Expected stale entry to exist")
		}
		if string(staleData) != string(data) {
			t.Errorf("Expected %q, got %q", data, staleData)
		}
	})

	t.Run("delete removes stale marker", func(t *testing.T) {
		cache := newMockCache()
		key := "test-key"
		data := []byte("test data")

		// Set, mark as stale, then delete
		_ = cache.Set(context.Background(), key, data)
		_ = cache.MarkStale(context.Background(), key)
		err := cache.Delete(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to delete: %v", err)
		}

		// Verify stale marker is also gone
		isStale, err := cache.IsStale(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to check stale: %v", err)
		}
		if isStale {
			t.Error("Expected stale marker to be removed")
		}
	})

	t.Run("mark non-existent entry does not error", func(t *testing.T) {
		cache := newMockCache()
		err := cache.MarkStale(context.Background(), "non-existent")
		if err != nil {
			t.Errorf("Expected no error marking non-existent entry, got: %v", err)
		}
	})
}

func TestStaleMarkingWithTransport(t *testing.T) {
	t.Run("forces revalidation for marked-stale entries and serves stale on server error", func(t *testing.T) {
		hitCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitCount++
			if hitCount == 1 {
				// Cacheable response that explicitly allows stale-if-error.
				w.Header().Set("Cache-Control", "max-age=3600, stale-if-error=60")
				w.Header().Set("ETag", "v1")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("original"))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		cache := newMockCache()
		transport := NewTransport(cache, WithEnableStaleMarking(true))

		req1, _ := http.NewRequest("GET", server.URL, nil)
		resp1, err := transport.RoundTrip(req1)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		_, _ = io.ReadAll(resp1.Body)
		resp1.Body.Close()

		// Explicitly mark the entry as stale (simulates invalidation).
		k := cacheKeyWithHeaders(req1, nil)
		if err := transport.cacheMarkStale(context.Background(), k); err != nil {
			t.Fatalf("Failed to mark as stale: %v", err)
		}

		req2, _ := http.NewRequest("GET", server.URL, nil)
		resp2, err := transport.RoundTrip(req2)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		defer resp2.Body.Close()

		// Marked-stale entries must not be served directly even if they are fresh.
		// The transport should revalidate, then fall back to the cached response when the
		// origin returns a server error and stale-if-error is in effect.
		if hitCount != 2 {
			t.Fatalf("Expected 2 origin hits, got %d", hitCount)
		}
		if resp2.StatusCode != http.StatusOK {
			t.Fatalf("Expected stale cached response (200), got %d", resp2.StatusCode)
		}
		if got := resp2.Header.Get(XStale); got != "1" {
			t.Fatalf("Expected %q header to be set on stale response, got %q", XStale, got)
		}
	})

	t.Run("deletes entry on error when disabled", func(t *testing.T) {
		hitCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitCount++
			if hitCount == 1 {
				w.Header().Set("Cache-Control", "max-age=1")
				w.Header().Set("ETag", "v1")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("original"))
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		defer server.Close()

		cache := newMockCache()
		transport := NewTransport(cache, WithEnableStaleMarking(false))

		// First request
		req1, _ := http.NewRequest("GET", server.URL, nil)
		resp1, _ := transport.RoundTrip(req1)
		resp1.Body.Close()

		// Wait for expiry
		time.Sleep(2 * time.Second)

		// Second request - server error
		req2, _ := http.NewRequest("GET", server.URL, nil)
		resp2, err := transport.RoundTrip(req2)

		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp2.Body.Close()

		// With stale marking disabled, we get the error
		if resp2.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected error response (500), got %d", resp2.StatusCode)
		}

		// Verify entry is deleted
		cacheKey := cacheKey(req1)
		_, exists, _ := cache.Get(context.Background(), hashKey(cacheKey))
		if exists {
			t.Error("Expected cache entry to be deleted with stale marking disabled")
		}
	})

	t.Run("simple stale marking verification", func(t *testing.T) {
		cache := newMockCache()
		transport := NewTransport(cache, WithEnableStaleMarking(true))

		// Manually insert a cache entry
		key := "test-key"
		_ = transport.cacheSet(context.Background(), key, []byte("data"))

		// Mark it as stale
		err := transport.cacheMarkStale(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to mark as stale: %v", err)
		}

		// Verify it's marked
		isStale, err := cache.IsStale(context.Background(), hashKey(key))
		if err != nil {
			t.Fatalf("Failed to check stale: %v", err)
		}
		if !isStale {
			t.Error("Expected entry to be marked as stale")
		}
	})
}

func TestStaleAwareCache(t *testing.T) {
	t.Run("wraps cache with stale support", func(t *testing.T) {
		innerCache := newMockCache()
		staleMarker := newMockCache()
		wrapped := NewStaleAwareCache(innerCache, staleMarker)

		key := "test-key"
		data := []byte("test data")

		// Set data
		err := wrapped.Set(context.Background(), key, data)
		if err != nil {
			t.Fatalf("Failed to set: %v", err)
		}

		// Mark as stale
		err = wrapped.MarkStale(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to mark as stale: %v", err)
		}

		// Verify stale
		isStale, err := wrapped.IsStale(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to check stale: %v", err)
		}
		if !isStale {
			t.Error("Expected entry to be stale")
		}

		// Get stale
		staleData, ok, err := wrapped.GetStale(context.Background(), key)
		if err != nil {
			t.Fatalf("Failed to get stale: %v", err)
		}
		if !ok {
			t.Error("Expected stale entry to exist")
		}
		if string(staleData) != string(data) {
			t.Errorf("Expected %q, got %q", data, staleData)
		}

		// Set new data clears stale marker
		newData := []byte("new data")
		_ = wrapped.Set(context.Background(), key, newData)
		isStale, _ = wrapped.IsStale(context.Background(), key)
		if isStale {
			t.Error("Expected stale marker to be cleared on set")
		}
	})
}
