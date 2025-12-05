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

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/sandrolain/httpcache"
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
	item := &memcache.Item{
		Key:   cacheKey(key),
		Value: resp,
	}
	if err := c.Client.Set(item); err != nil {
		httpcache.GetLogger().Warn("failed to write to memcache", "key", key, "error", err)
		return err
	}
	return nil
}

// Delete removes the response with key from the cache.
// The context parameter is accepted for interface compliance but not used
// for memcache operations due to library limitations.
func (c *Cache) Delete(_ context.Context, key string) error {
	if err := c.Client.Delete(cacheKey(key)); err != nil {
		if err == memcache.ErrCacheMiss {
			return nil
		}
		httpcache.GetLogger().Warn("failed to delete from memcache", "key", key, "error", err)
		return err
	}
	return nil
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
