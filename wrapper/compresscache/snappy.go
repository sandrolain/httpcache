package compresscache

import (
	"context"
	"fmt"

	"github.com/golang/snappy"
	"github.com/sandrolain/httpcache"
)

// SnappyCache wraps a cache with automatic Snappy compression/decompression
type SnappyCache struct {
	*baseCompressCache
}

// SnappyConfig holds the configuration for Snappy compression
type SnappyConfig struct {
	// Cache is the underlying cache backend (required)
	Cache httpcache.Cache
}

// NewSnappy creates a new SnappyCache with Snappy compression
func NewSnappy(config SnappyConfig) (*SnappyCache, error) {
	if config.Cache == nil {
		return nil, fmt.Errorf("cache cannot be nil")
	}

	return &SnappyCache{
		baseCompressCache: newBaseCompressCache(config.Cache, Snappy),
	}, nil
}

// compress compresses data using Snappy algorithm
func (c *SnappyCache) compress(data []byte) ([]byte, error) {
	compressed := snappy.Encode(nil, data)
	return compressed, nil
}

// decompress decompresses data using Snappy algorithm
func (c *SnappyCache) decompress(data []byte) ([]byte, error) {
	decompressed, err := snappy.Decode(nil, data)
	if err != nil {
		return nil, fmt.Errorf("snappy decode failed: %w", err)
	}
	return decompressed, nil
}

// Set compresses and stores a value in the cache.
// Uses the provided context for cache operations.
func (c *SnappyCache) Set(ctx context.Context, key string, value []byte) error {
	return c.set(ctx, key, value, c.compress)
}

// Get retrieves and decompresses a value from the cache.
// Uses the provided context for cache operations.
func (c *SnappyCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return c.get(ctx, key, c.decompress)
}

// Delete removes a value from the cache.
// Uses the provided context for cache operations.
func (c *SnappyCache) Delete(ctx context.Context, key string) error {
	return c.delete(ctx, key)
}

// Stats returns compression statistics
func (c *SnappyCache) Stats() Stats {
	return c.stats()
}
