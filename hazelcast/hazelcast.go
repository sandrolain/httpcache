// Package hazelcast provides a Hazelcast interface for http caching.
package hazelcast

import (
	"context"

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

// Get returns the response corresponding to key if present.
func (c cache) Get(key string) (resp []byte, ok bool) {
	val, err := c.m.Get(c.ctx, cacheKey(key))
	if err != nil || val == nil {
		return nil, false
	}

	data, ok := val.([]byte)
	if !ok {
		return nil, false
	}

	return data, true
}

// Set saves a response to the cache as key.
func (c cache) Set(key string, resp []byte) {
	if err := c.m.Set(c.ctx, cacheKey(key), resp); err != nil {
		httpcache.GetLogger().Warn("failed to write to Hazelcast cache", "key", key, "error", err)
	}
}

// Delete removes the response with key from the cache.
func (c cache) Delete(key string) {
	if _, err := c.m.Remove(c.ctx, cacheKey(key)); err != nil {
		httpcache.GetLogger().Warn("failed to delete from Hazelcast cache", "key", key, "error", err)
	}
}

// NewWithMap returns a new Cache with the given Hazelcast map.
func NewWithMap(m *hazelcast.Map) httpcache.Cache {
	return cache{m: m, ctx: context.Background()}
}

// NewWithMapAndContext returns a new Cache with the given Hazelcast map and context.
func NewWithMapAndContext(ctx context.Context, m *hazelcast.Map) httpcache.Cache {
	return cache{m: m, ctx: ctx}
}
