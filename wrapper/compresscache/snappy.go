package compresscache

import (
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

// Set compresses and stores a value in the cache
func (c *SnappyCache) Set(key string, value []byte) {
	c.set(key, value, c.compress)
}

// Get retrieves and decompresses a value from the cache
func (c *SnappyCache) Get(key string) ([]byte, bool) {
	return c.get(key, c.decompress)
}

// Delete removes a value from the cache
func (c *SnappyCache) Delete(key string) {
	c.delete(key)
}

// Stats returns compression statistics
func (c *SnappyCache) Stats() Stats {
	return c.stats()
}
