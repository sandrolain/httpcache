// Package hazelcast provides a Hazelcast interface for http caching.
package hazelcast

import (
	"context"
	"fmt"

	"github.com/hazelcast/hazelcast-go-client"
	"github.com/sandrolain/httpcache"
)

// cache is an implementation of httpcache.Cache that caches responses in a
// Hazelcast cluster.
type cache struct {
	m   *hazelcast.Map
	ctx context.Context
}

// cacheKey modifies an httpcache key for use in Hazelcast. Specifically, it
// prefixes keys to avoid collision with other data stored in the map.
func cacheKey(key string) string {
	return "httpcache:" + key
}

const stalePrefix = "stale:"

// staleCacheKey returns the key for the stale marker
func staleCacheKey(key string) string {
	return "httpcache:" + stalePrefix + key
}

// Get returns the response corresponding to key if present.
// Uses the provided context for cancellation. If ctx is non-nil, it takes precedence
// over the context stored in the cache instance.
func (c cache) Get(ctx context.Context, key string) (resp []byte, ok bool, err error) {
	if ctx == nil {
		ctx = c.ctx
	}
	val, err := c.m.Get(ctx, cacheKey(key))
	if err != nil {
		return nil, false, err
	}
	if val == nil {
		return nil, false, nil
	}

	data, ok := val.([]byte)
	if !ok {
		return nil, false, nil
	}

	return data, true, nil
}

// Set saves a response to the cache as key.
// Uses the provided context for cancellation. If ctx is non-nil, it takes precedence
// over the context stored in the cache instance.
func (c cache) Set(ctx context.Context, key string, resp []byte) error {
	if ctx == nil {
		ctx = c.ctx
	}

	// Remove stale marker when setting a fresh value
	//nolint:staticcheck // Intentionally ignore errors when removing stale marker
	if _, err := c.m.Remove(ctx, staleCacheKey(key)); err != nil {
		// Ignore errors when removing stale marker (it may not exist)
	}

	if err := c.m.Set(ctx, cacheKey(key), resp); err != nil {
		return fmt.Errorf("hazelcast cache set failed for key %q: %w", key, err)
	}
	return nil
}

// Delete removes the response with key from the cache.
// Uses the provided context for cancellation. If ctx is non-nil, it takes precedence
// over the context stored in the cache instance.
func (c cache) Delete(ctx context.Context, key string) error {
	if ctx == nil {
		ctx = c.ctx
	}

	// Remove both the entry and its stale marker
	if _, err := c.m.Remove(ctx, cacheKey(key)); err != nil {
		return fmt.Errorf("hazelcast cache delete failed for key %q: %w", key, err)
	}

	// Also remove stale marker if present (ignore errors if it doesn't exist)
	//nolint:staticcheck // Intentionally ignore errors when removing stale marker
	if _, err := c.m.Remove(ctx, staleCacheKey(key)); err != nil {
		// Ignore errors when removing stale marker (it may not exist)
	}

	return nil
}

// MarkStale marks the cached entry as stale without removing it.
// Uses the provided context for cancellation. If ctx is non-nil, it takes precedence
// over the context stored in the cache instance.
func (c cache) MarkStale(ctx context.Context, key string) error {
	if ctx == nil {
		ctx = c.ctx
	}

	// Check if the key exists
	val, err := c.m.Get(ctx, cacheKey(key))
	if err != nil {
		return fmt.Errorf("hazelcast cache check for key %q failed: %w", key, err)
	}
	if val == nil {
		return nil // Key doesn't exist, nothing to mark
	}

	// Create a stale marker entry with minimal value
	if err := c.m.Set(ctx, staleCacheKey(key), []byte{1}); err != nil {
		return fmt.Errorf("hazelcast cache mark stale failed for key %q: %w", key, err)
	}
	return nil
}

// IsStale checks if the cached entry is marked as stale.
// Uses the provided context for cancellation. If ctx is non-nil, it takes precedence
// over the context stored in the cache instance.
func (c cache) IsStale(ctx context.Context, key string) (bool, error) {
	if ctx == nil {
		ctx = c.ctx
	}

	val, err := c.m.Get(ctx, staleCacheKey(key))
	if err != nil {
		return false, fmt.Errorf("hazelcast cache check stale marker for key %q failed: %w", key, err)
	}

	return val != nil, nil
}

// GetStale retrieves a stale entry if it exists and is marked as stale.
// Returns the value and true if the entry is stale, nil and false otherwise.
// Uses the provided context for cancellation. If ctx is non-nil, it takes precedence
// over the context stored in the cache instance.
func (c cache) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	if ctx == nil {
		ctx = c.ctx
	}

	// Check if marked as stale
	staleVal, err := c.m.Get(ctx, staleCacheKey(key))
	if err != nil {
		return nil, false, fmt.Errorf("hazelcast cache check stale marker for key %q failed: %w", key, err)
	}
	if staleVal == nil {
		return nil, false, nil // Not marked as stale
	}

	// Get the actual entry
	val, err := c.m.Get(ctx, cacheKey(key))
	if err != nil {
		return nil, false, fmt.Errorf("hazelcast cache get stale for key %q failed: %w", key, err)
	}
	if val == nil {
		return nil, false, nil // Entry was removed
	}

	data, ok := val.([]byte)
	if !ok {
		return nil, false, nil
	}

	return data, true, nil
}

// NewWithMap returns a new Cache with the given Hazelcast map.
func NewWithMap(m *hazelcast.Map) httpcache.Cache {
	return cache{m: m, ctx: context.Background()}
}

// NewWithMapAndContext returns a new Cache with the given Hazelcast map and context.
// Note: The provided context is used as a fallback; contexts passed to Get/Set/Delete
// take precedence.
func NewWithMapAndContext(ctx context.Context, m *hazelcast.Map) httpcache.Cache {
	return cache{m: m, ctx: ctx}
}
