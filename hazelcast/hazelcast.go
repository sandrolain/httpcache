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
	if err := c.m.Set(ctx, cacheKey(key), resp); err != nil {
		httpcache.GetLogger().Warn("failed to write to Hazelcast cache", "key", key, "error", err)
		return err
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
	if _, err := c.m.Remove(ctx, cacheKey(key)); err != nil {
		httpcache.GetLogger().Warn("failed to delete from Hazelcast cache", "key", key, "error", err)
		return err
	}
	return nil
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
