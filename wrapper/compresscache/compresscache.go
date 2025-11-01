// Package compresscache provides a cache wrapper that automatically compresses
// cached data to reduce storage requirements and network bandwidth usage.
// Supports multiple compression algorithms: gzip, brotli, and snappy.
package compresscache

import (
	"fmt"
	"sync/atomic"

	"github.com/sandrolain/httpcache"
)

// Algorithm represents the compression algorithm to use
type Algorithm int

const (
	// Gzip uses gzip compression (good balance of compression and speed)
	Gzip Algorithm = iota
	// Brotli uses brotli compression (best compression ratio, slower)
	Brotli
	// Snappy uses snappy compression (fastest, lower compression ratio)
	Snappy
)

// String returns the string representation of the algorithm
func (a Algorithm) String() string {
	switch a {
	case Gzip:
		return "gzip"
	case Brotli:
		return "brotli"
	case Snappy:
		return "snappy"
	default:
		return "unknown"
	}
}

// Stats holds compression statistics
type Stats struct {
	CompressedBytes   int64   // Total bytes after compression
	UncompressedBytes int64   // Total bytes before compression
	CompressedCount   int64   // Number of compressed entries
	UncompressedCount int64   // Number of uncompressed entries (too small)
	CompressionRatio  float64 // Compression ratio (0.0-1.0, lower is better)
	SavingsPercent    float64 // Space savings percentage
}

// CompressCache is a type alias for GzipCache for backward compatibility
// Deprecated: Use GzipCache, BrotliCache, or SnappyCache directly
type CompressCache = GzipCache

// compressFunc is a function type for compression operations
type compressFunc func([]byte) ([]byte, error)

// decompressFunc is a function type for decompression operations
type decompressFunc func([]byte) ([]byte, error)

// baseCompressCache provides common functionality for all compression implementations
type baseCompressCache struct {
	cache     httpcache.Cache
	algorithm Algorithm

	// Statistics
	compressedBytes   atomic.Int64
	uncompressedBytes atomic.Int64
	compressedCount   atomic.Int64
	uncompressedCount atomic.Int64
}

// newBaseCompressCache creates a new base compression cache
func newBaseCompressCache(cache httpcache.Cache, algorithm Algorithm) *baseCompressCache {
	return &baseCompressCache{
		cache:     cache,
		algorithm: algorithm,
	}
}

// get retrieves and decompresses a value from the cache
func (c *baseCompressCache) get(key string, decompressFn decompressFunc) ([]byte, bool) {
	data, ok := c.cache.Get(key)
	if !ok {
		return nil, false
	}

	// Check if data is compressed (has our marker)
	if len(data) < 1 {
		return data, true
	}

	// First byte indicates compression algorithm
	marker := data[0]
	if marker == 0 {
		// Not compressed
		return data[1:], true
	}

	// Get the algorithm from marker
	storedAlgo := Algorithm(marker - 1)

	// Decompress using the appropriate algorithm
	decompressed, err := c.decompressWithAlgorithm(data[1:], storedAlgo, decompressFn)
	if err != nil {
		httpcache.GetLogger().Warn("decompression failed",
			"key", key,
			"algorithm", storedAlgo.String(),
			"error", err)
		return nil, false
	}

	return decompressed, true
}

// decompressWithAlgorithm decompresses data, delegating to the appropriate decompressor
func (c *baseCompressCache) decompressWithAlgorithm(data []byte, algorithm Algorithm, decompressFn decompressFunc) ([]byte, error) {
	// If the stored algorithm matches ours, use our decompressor
	if algorithm == c.algorithm {
		return decompressFn(data)
	}

	// Otherwise, we need to use the appropriate decompressor for the stored algorithm
	// This allows cross-algorithm decompression (as required by the tests)
	return c.decompressAny(data, algorithm)
}

// decompressAny decompresses data using any supported algorithm
// This is needed for cross-algorithm compatibility
func (c *baseCompressCache) decompressAny(data []byte, algorithm Algorithm) ([]byte, error) {
	switch algorithm {
	case Gzip:
		// Create a temporary GzipCache to decompress
		tempCache := &GzipCache{baseCompressCache: c}
		return tempCache.decompress(data)
	case Brotli:
		// Create a temporary BrotliCache to decompress
		tempCache := &BrotliCache{baseCompressCache: c}
		return tempCache.decompress(data)
	case Snappy:
		// Create a temporary SnappyCache to decompress
		tempCache := &SnappyCache{baseCompressCache: c}
		return tempCache.decompress(data)
	default:
		return nil, fmt.Errorf("unsupported decompression algorithm: %v", algorithm)
	}
}

// set compresses and stores a value in the cache
func (c *baseCompressCache) set(key string, value []byte, compressFn compressFunc) {
	// Compress the data
	compressed, err := compressFn(value)
	if err != nil {
		httpcache.GetLogger().Warn("compression failed, storing uncompressed",
			"key", key,
			"algorithm", c.algorithm.String(),
			"error", err)
		// Fallback to uncompressed
		data := make([]byte, len(value)+1)
		data[0] = 0
		copy(data[1:], value)
		c.cache.Set(key, data)
		c.uncompressedCount.Add(1)
		c.uncompressedBytes.Add(int64(len(value)))
		return
	}

	// Prefix with marker (algorithm + 1, so 0 means uncompressed)
	data := make([]byte, len(compressed)+1)
	data[0] = byte(c.algorithm + 1)
	copy(data[1:], compressed)

	c.cache.Set(key, data)
	c.compressedCount.Add(1)
	c.compressedBytes.Add(int64(len(compressed)))
	c.uncompressedBytes.Add(int64(len(value)))
}

// delete removes a value from the cache
func (c *baseCompressCache) delete(key string) {
	c.cache.Delete(key)
}

// stats returns compression statistics
func (c *baseCompressCache) stats() Stats {
	compressed := c.compressedBytes.Load()
	uncompressed := c.uncompressedBytes.Load()

	var ratio, savings float64
	if uncompressed > 0 {
		ratio = float64(compressed) / float64(uncompressed)
		savings = (1.0 - ratio) * 100
	}

	return Stats{
		CompressedBytes:   compressed,
		UncompressedBytes: uncompressed,
		CompressedCount:   c.compressedCount.Load(),
		UncompressedCount: c.uncompressedCount.Load(),
		CompressionRatio:  ratio,
		SavingsPercent:    savings,
	}
}
