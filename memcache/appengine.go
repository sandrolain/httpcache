//go:build appengine

// Package memcache provides an implementation of httpcache.Cache that uses App
// Engine's memcache package to store cached responses.
//
// When not built for Google App Engine, this package will provide an
// implementation that connects to a specified memcached server.  See the
// memcache.go file in this package for details.
package memcache

import (
	"context"

	"appengine"
	"appengine/memcache"
)

// Cache is an implementation of httpcache.Cache that caches responses in App
// Engine's memcache.
type Cache struct {
	appengine.Context
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
// The ctx parameter is accepted for interface compliance but not used;
// App Engine memcache uses its own context mechanism.
func (c *Cache) Get(_ context.Context, key string) (resp []byte, ok bool, err error) {
	item, err := memcache.Get(c.Context, cacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return nil, false, nil
		}
		c.Context.Errorf("error getting cached response: %v", err)
		return nil, false, err
	}
	return item.Value, true, nil
}

// Set saves a response to the cache as key.
// The ctx parameter is accepted for interface compliance but not used;
// App Engine memcache uses its own context mechanism.
func (c *Cache) Set(_ context.Context, key string, resp []byte) error {
	// Remove stale marker when setting a fresh value
	_ = memcache.Delete(c.Context, staleCacheKey(key)) // Ignore errors

	item := &memcache.Item{
		Key:   cacheKey(key),
		Value: resp,
	}
	if err := memcache.Set(c.Context, item); err != nil {
		c.Context.Errorf("error caching response: %v", err)
		return err
	}
	return nil
}

// Delete removes the response with key from the cache.
// The ctx parameter is accepted for interface compliance but not used;
// App Engine memcache uses its own context mechanism.
func (c *Cache) Delete(_ context.Context, key string) error {
	// Remove both the entry and its stale marker
	if err := memcache.Delete(c.Context, cacheKey(key)); err != nil {
		if err == memcache.ErrCacheMiss {
			return nil // Not an error if key doesn't exist
		}
		c.Context.Errorf("error deleting cached response: %v", err)
		return err
	}

	// Also remove stale marker if present (ignore errors)
	_ = memcache.Delete(c.Context, staleCacheKey(key))

	return nil
}

// MarkStale marks the cached entry as stale without removing it.
// The ctx parameter is accepted for interface compliance but not used;
// App Engine memcache uses its own context mechanism.
func (c *Cache) MarkStale(_ context.Context, key string) error {
	// Check if the key exists
	_, err := memcache.Get(c.Context, cacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return nil // Key doesn't exist, nothing to mark
		}
		c.Context.Errorf("error checking cached response: %v", err)
		return err
	}

	// Create a stale marker entry with minimal value
	item := &memcache.Item{
		Key:   staleCacheKey(key),
		Value: []byte{1},
	}
	if err := memcache.Set(c.Context, item); err != nil {
		c.Context.Errorf("error marking response as stale: %v", err)
		return err
	}
	return nil
}

// IsStale checks if the cached entry is marked as stale.
// The ctx parameter is accepted for interface compliance but not used;
// App Engine memcache uses its own context mechanism.
func (c *Cache) IsStale(_ context.Context, key string) (bool, error) {
	_, err := memcache.Get(c.Context, staleCacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return false, nil // No marker, not stale
		}
		c.Context.Errorf("error checking stale marker: %v", err)
		return false, err
	}
	return true, nil
}

// GetStale retrieves a stale entry if it exists and is marked as stale.
// Returns the value and true if the entry is stale, nil and false otherwise.
// The ctx parameter is accepted for interface compliance but not used;
// App Engine memcache uses its own context mechanism.
func (c *Cache) GetStale(_ context.Context, key string) ([]byte, bool, error) {
	// Check if marked as stale
	_, err := memcache.Get(c.Context, staleCacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return nil, false, nil // Not marked as stale
		}
		c.Context.Errorf("error checking stale marker: %v", err)
		return nil, false, err
	}

	// Get the actual entry
	item, err := memcache.Get(c.Context, cacheKey(key))
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return nil, false, nil // Entry was removed
		}
		c.Context.Errorf("error getting stale response: %v", err)
		return nil, false, err
	}

	return item.Value, true, nil
}

// New returns a new Cache for the given context.
func New(ctx appengine.Context) *Cache {
	return &Cache{ctx}
}
