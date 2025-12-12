//go:build !appengine

// Package memcache provides an implementation of httpcache.Cache that uses
// gomemcache to store cached responses.
//
// When built for Google App Engine, this package will provide an
// implementation that uses App Engine's memcache service.  See the
// appengine.go file in this package for details.
package memcache

import (
	"context"
	"fmt"

	"github.com/bradfitz/gomemcache/memcache"
)

// Cache is an implementation of httpcache.Cache that caches responses in a
// memcache server.
type Cache struct {
	*memcache.Client
}

// cacheKey modifies an httpcache key for use in memcache.  Specifically, it
// prefixes keys to avoid collision with other data stored in memcache.
func cacheKey(key string) string {
	return "httpcache:" + key
}

const stalePrefix = "stale:"

// staleCacheKey returns the key for the stale marker
func staleCacheKey(key string) string {
	return "httpcache:" + stalePrefix + key
}

// Get returns the response corresponding to key if present.
// The context parameter is accepted for interface compliance but not used
// for memcache operations due to library limitations.
func (c *Cache) Get(_ context.Context, key string) (resp []byte, ok bool, err error) {
	item, err := c.Client.Get(cacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return nil, false, nil
		}
		return nil, false, err
	}
	return item.Value, true, nil
}

// Set saves a response to the cache as key.
// The context parameter is accepted for interface compliance but not used
// for memcache operations due to library limitations.
func (c *Cache) Set(_ context.Context, key string, resp []byte) error {
	// Remove stale marker when setting a fresh value
	//nolint:errcheck // Intentionally ignore errors when removing stale marker
	_ = c.Client.Delete(staleCacheKey(key))

	item := &memcache.Item{
		Key:   cacheKey(key),
		Value: resp,
	}
	if err := c.Client.Set(item); err != nil {
		return fmt.Errorf("memcache set failed for key %q: %w", key, err)
	}
	return nil
}

// Delete removes the response with key from the cache.
// The context parameter is accepted for interface compliance but not used
// for memcache operations due to library limitations.
func (c *Cache) Delete(_ context.Context, key string) error {
	// Remove both the entry and its stale marker
	if err := c.Client.Delete(cacheKey(key)); err != nil {
		if err == memcache.ErrCacheMiss {
			return nil
		}
		return fmt.Errorf("memcache delete failed for key %q: %w", key, err)
	}

	// Also remove stale marker if present (ignore errors)
	//nolint:errcheck // Intentionally ignore errors when removing stale marker
	_ = c.Client.Delete(staleCacheKey(key))

	return nil
}

// MarkStale marks the cached entry as stale without removing it.
// The context parameter is accepted for interface compliance but not used
// for memcache operations due to library limitations.
func (c *Cache) MarkStale(_ context.Context, key string) error {
	// Check if the key exists
	_, err := c.Client.Get(cacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return nil // Key doesn't exist, nothing to mark
		}
		return fmt.Errorf("memcache check for key %q failed: %w", key, err)
	}

	// Create a stale marker entry with minimal value
	item := &memcache.Item{
		Key:   staleCacheKey(key),
		Value: []byte{1},
	}
	if err := c.Client.Set(item); err != nil {
		return fmt.Errorf("memcache mark stale failed for key %q: %w", key, err)
	}
	return nil
}

// IsStale checks if the cached entry is marked as stale.
// The context parameter is accepted for interface compliance but not used
// for memcache operations due to library limitations.
func (c *Cache) IsStale(_ context.Context, key string) (bool, error) {
	_, err := c.Client.Get(staleCacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return false, nil // No marker, not stale
		}
		return false, fmt.Errorf("memcache check stale marker for key %q failed: %w", key, err)
	}
	return true, nil
}

// GetStale retrieves a stale entry if it exists and is marked as stale.
// Returns the value and true if the entry is stale, nil and false otherwise.
// The context parameter is accepted for interface compliance but not used
// for memcache operations due to library limitations.
func (c *Cache) GetStale(_ context.Context, key string) ([]byte, bool, error) {
	// Check if marked as stale
	_, err := c.Client.Get(staleCacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return nil, false, nil // Not marked as stale
		}
		return nil, false, fmt.Errorf("memcache check stale marker for key %q failed: %w", key, err)
	}

	// Get the actual entry
	item, err := c.Client.Get(cacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return nil, false, nil // Entry was removed
		}
		return nil, false, fmt.Errorf("memcache get stale for key %q failed: %w", key, err)
	}

	return item.Value, true, nil
}

// New returns a new Cache using the provided memcache server(s) with equal
// weight. If a server is listed multiple times, it gets a proportional amount
// of weight.
func New(server ...string) *Cache {
	return NewWithClient(memcache.New(server...))
}

// NewWithClient returns a new Cache with the given memcache client.
func NewWithClient(client *memcache.Client) *Cache {
	return &Cache{client}
}
