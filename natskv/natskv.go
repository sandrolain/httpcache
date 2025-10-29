// Package natskv provides a NATS JetStream Key/Value store interface for http caching.
package natskv

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/sandrolain/httpcache"
)

// cache is an implementation of httpcache.Cache that caches responses in a
// NATS JetStream Key/Value store.
type cache struct {
	kv jetstream.KeyValue
}

// cacheKey modifies an httpcache key for use in NATS K/V. Specifically, it
// prefixes keys to avoid collision with other data stored in the bucket.
// NATS K/V keys must not contain certain characters like ':'.
func cacheKey(key string) string {
	return "httpcache." + key
}

// Get returns the response corresponding to key if present.
func (c cache) Get(key string) (resp []byte, ok bool) {
	entry, err := c.kv.Get(context.Background(), cacheKey(key))
	if err != nil {
		return nil, false
	}
	return entry.Value(), true
}

// Set saves a response to the cache as key.
func (c cache) Set(key string, resp []byte) {
	if _, err := c.kv.Put(context.Background(), cacheKey(key), resp); err != nil {
		httpcache.GetLogger().Warn("failed to write to NATS K/V cache", "key", key, "error", err)
	}
}

// Delete removes the response with key from the cache.
func (c cache) Delete(key string) {
	if err := c.kv.Delete(context.Background(), cacheKey(key)); err != nil {
		httpcache.GetLogger().Warn("failed to delete from NATS K/V cache", "key", key, "error", err)
	}
}

// NewWithKeyValue returns a new Cache with the given NATS JetStream KeyValue store.
func NewWithKeyValue(kv jetstream.KeyValue) httpcache.Cache {
	return cache{kv: kv}
}
