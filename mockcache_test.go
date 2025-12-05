package httpcache

import (
	"context"
	"sync"
)

// mockCache is a simple in-memory cache implementation for testing purposes.
// It is not exported and should only be used in tests.
type mockCache struct {
	mu    sync.RWMutex
	items map[string][]byte
}

// Get returns the []byte representation of the response and true if present, false if not.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *mockCache) Get(_ context.Context, key string) (resp []byte, ok bool, err error) {
	c.mu.RLock()
	resp, ok = c.items[key]
	c.mu.RUnlock()
	return resp, ok, nil
}

// Set saves response resp to the cache with key.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *mockCache) Set(_ context.Context, key string, resp []byte) error {
	c.mu.Lock()
	c.items[key] = resp
	c.mu.Unlock()
	return nil
}

// Delete removes key from the cache.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *mockCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
	return nil
}

// newMockCache returns a new mockCache for testing.
func newMockCache() *mockCache {
	return &mockCache{items: map[string][]byte{}}
}

// newMockCacheTransport returns a new Transport using the mock cache implementation for testing.
func newMockCacheTransport() *Transport {
	c := newMockCache()
	t := NewTransport(c)
	return t
}
