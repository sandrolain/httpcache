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
	if err := memcache.Delete(c.Context, cacheKey(key)); err != nil {
		if err == memcache.ErrCacheMiss {
			return nil // Not an error if key doesn't exist
		}
		c.Context.Errorf("error deleting cached response: %v", err)
		return err
	}
	return nil
}

// New returns a new Cache for the given context.
func New(ctx appengine.Context) *Cache {
	return &Cache{ctx}
}
