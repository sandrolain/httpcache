// Package freecache provides a high-performance, zero-GC overhead implementation of httpcache.Cache
// using github.com/coocood/freecache as the underlying storage.
//
// This backend is suitable for applications that need to cache millions of entries
// with minimal GC overhead and automatic memory management with LRU eviction.
//
// Example usage:
//
//	cache := freecache.New(100 * 1024 * 1024) // 100MB cache
//	transport := httpcache.NewTransport(cache)
//	client := transport.Client()
package freecache

import (
	"github.com/coocood/freecache"
	"github.com/sandrolain/httpcache"
)

// Cache is an implementation of httpcache.Cache that uses freecache for storage.
// It provides zero-GC overhead and automatic LRU eviction when cache is full.
type Cache struct {
	cache *freecache.Cache
}

// New creates a new Cache with the specified size in bytes.
// The cache size will be set to 512KB at minimum.
//
// For large cache sizes, you may want to call debug.SetGCPercent()
// with a lower value to reduce GC overhead.
//
// Example:
//
//	import "runtime/debug"
//	cache := freecache.New(100 * 1024 * 1024) // 100MB
//	debug.SetGCPercent(20)
func New(size int) *Cache {
	return &Cache{
		cache: freecache.NewCache(size),
	}
}

// Get returns the cached response bytes and true if present, false if not found
func (c *Cache) Get(key string) ([]byte, bool) {
	value, err := c.cache.Get([]byte(key))
	if err != nil {
		return nil, false
	}
	return value, true
}

// Set stores the response bytes in the cache with the given key.
// If the cache is full, it will evict the least recently used entry.
// The entry has no expiration time and will only be evicted when cache is full.
func (c *Cache) Set(key string, value []byte) {
	if err := c.cache.Set([]byte(key), value, 0); err != nil {
		// Log error but don't fail - cache operations should be best-effort
		// Errors can occur if key/value are too large
		httpcache.GetLogger().Warn("failed to set cache value", "key", key, "error", err)
	}
}

// Delete removes the entry with the given key from the cache
func (c *Cache) Delete(key string) {
	c.cache.Del([]byte(key))
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.cache.Clear()
}

// EntryCount returns the number of entries currently in the cache
func (c *Cache) EntryCount() int64 {
	return c.cache.EntryCount()
}

// HitRate returns the ratio of cache hits to total lookups
func (c *Cache) HitRate() float64 {
	return c.cache.HitRate()
}

// EvacuateCount returns the number of times entries were evicted due to cache being full
func (c *Cache) EvacuateCount() int64 {
	return c.cache.EvacuateCount()
}

// ExpiredCount returns the number of times entries expired
func (c *Cache) ExpiredCount() int64 {
	return c.cache.ExpiredCount()
}

// ResetStatistics resets all statistics counters (hit rate, evictions, etc.)
func (c *Cache) ResetStatistics() {
	c.cache.ResetStatistics()
}
