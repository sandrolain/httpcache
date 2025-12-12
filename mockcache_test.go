package httpcache

import (
	"context"
	"sync"
)

// mockCache is a simple in-memory cache implementation for testing purposes.
// It is not exported and should only be used in tests.
type mockCache struct {
	mu     sync.RWMutex
	items  map[string][]byte
	stales map[string]bool // tracks which keys are marked as stale
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
	delete(c.stales, key)
	c.mu.Unlock()
	return nil
}

// MarkStale marks a cached response as stale instead of deleting it.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *mockCache) MarkStale(_ context.Context, key string) error {
	c.mu.Lock()
	if _, exists := c.items[key]; exists {
		c.stales[key] = true
	}
	c.mu.Unlock()
	return nil
}

// IsStale checks if a cached response has been marked as stale.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *mockCache) IsStale(_ context.Context, key string) (bool, error) {
	c.mu.RLock()
	stale := c.stales[key]
	c.mu.RUnlock()
	return stale, nil
}

// GetStale retrieves a stale cached response if it exists.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *mockCache) GetStale(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.stales[key] {
		return nil, false, nil
	}

	data, ok := c.items[key]
	return data, ok, nil
}

// newMockCache returns a new mockCache for testing.
func newMockCache() *mockCache {
	return &mockCache{
		items:  map[string][]byte{},
		stales: map[string]bool{},
	}
}

// newMockCacheTransport returns a new Transport using the mock cache implementation for testing.
func newMockCacheTransport() *Transport {
	c := newMockCache()
	t := NewTransport(c)
	return t
}
