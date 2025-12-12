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
	"context"
	"fmt"

	"github.com/coocood/freecache"
)

// Cache is an implementation of httpcache.Cache that uses freecache for storage.
// It provides zero-GC overhead and automatic LRU eviction when cache is full.
type Cache struct {
	cache *freecache.Cache
}

const stalePrefix = "stale:"

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

// Get returns the cached response bytes and true if present, false if not found.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *Cache) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, err := c.cache.Get([]byte(key))
	if err != nil {
		if err == freecache.ErrNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	return value, true, nil
}

// Set stores the response bytes in the cache with the given key.
// If the cache is full, it will evict the least recently used entry.
// The entry has no expiration time and will only be evicted when cache is full.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *Cache) Set(_ context.Context, key string, value []byte) error {
	// Remove stale marker when setting a fresh value
	c.cache.Del([]byte(stalePrefix + key))

	if err := c.cache.Set([]byte(key), value, 0); err != nil {
		return fmt.Errorf("freecache set failed for key %q: %w", key, err)
	}
	return nil
}

// Delete removes the entry with the given key from the cache.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *Cache) Delete(_ context.Context, key string) error {
	c.cache.Del([]byte(key))
	// Also remove stale marker if present
	c.cache.Del([]byte(stalePrefix + key))
	return nil
}

// MarkStale marks the cached entry as stale without removing it.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *Cache) MarkStale(_ context.Context, key string) error {
	// Check if the key exists
	_, err := c.cache.Get([]byte(key))
	if err != nil {
		if err == freecache.ErrNotFound {
			return nil // Key doesn't exist, nothing to mark
		}
		return fmt.Errorf("freecache check for key %q failed: %w", key, err)
	}

	// Create a stale marker entry with minimal value
	staleKey := stalePrefix + key
	if err := c.cache.Set([]byte(staleKey), []byte{1}, 0); err != nil {
		return fmt.Errorf("freecache mark stale failed for key %q: %w", key, err)
	}
	return nil
}

// IsStale checks if the cached entry is marked as stale.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *Cache) IsStale(_ context.Context, key string) (bool, error) {
	staleKey := stalePrefix + key
	_, err := c.cache.Get([]byte(staleKey))
	if err != nil {
		if err == freecache.ErrNotFound {
			return false, nil // No marker, not stale
		}
		return false, fmt.Errorf("freecache check stale marker for key %q failed: %w", key, err)
	}
	return true, nil
}

// GetStale retrieves a stale entry if it exists and is marked as stale.
// Returns the value and true if the entry is stale, nil and false otherwise.
// The context parameter is accepted for interface compliance but not used for in-memory operations.
func (c *Cache) GetStale(_ context.Context, key string) ([]byte, bool, error) {
	// Check if marked as stale
	staleKey := stalePrefix + key
	_, err := c.cache.Get([]byte(staleKey))
	if err != nil {
		if err == freecache.ErrNotFound {
			return nil, false, nil // Not marked as stale
		}
		return nil, false, fmt.Errorf("freecache check stale marker for key %q failed: %w", key, err)
	}

	// Get the actual entry
	value, err := c.cache.Get([]byte(key))
	if err != nil {
		if err == freecache.ErrNotFound {
			return nil, false, nil // Entry was removed
		}
		return nil, false, fmt.Errorf("freecache get stale for key %q failed: %w", key, err)
	}

	return value, true, nil
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
