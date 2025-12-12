package compresscache

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"

	"github.com/sandrolain/httpcache"
)

// GzipCache wraps a cache with automatic Gzip compression/decompression
type GzipCache struct {
	*baseCompressCache
	level int
}

// GzipConfig holds the configuration for Gzip compression
type GzipConfig struct {
	// Cache is the underlying cache backend (required)
	Cache httpcache.Cache

	// Level is the compression level (-2 to 9)
	// Default: gzip.DefaultCompression (-1)
	Level int
}

// NewGzip creates a new GzipCache with Gzip compression
func NewGzip(config GzipConfig) (*GzipCache, error) {
	if config.Cache == nil {
		return nil, fmt.Errorf("cache cannot be nil")
	}

	// Set defaults
	if config.Level == 0 {
		config.Level = gzip.DefaultCompression
	}

	// Validate level
	if config.Level < gzip.HuffmanOnly || config.Level > gzip.BestCompression {
		return nil, fmt.Errorf("invalid gzip compression level: %d", config.Level)
	}

	return &GzipCache{
		baseCompressCache: newBaseCompressCache(config.Cache, Gzip),
		level:             config.Level,
	}, nil
}

// compress compresses data using Gzip algorithm
func (c *GzipCache) compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	w, err := gzip.NewWriterLevel(&buf, c.level)
	if err != nil {
		return nil, fmt.Errorf("gzip writer creation failed: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		closeErr := w.Close()
		_ = closeErr // Ignore close error in error path
		return nil, fmt.Errorf("gzip write failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}

	return buf.Bytes(), nil
}

// decompress decompresses data using Gzip algorithm
func (c *GzipCache) decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader creation failed: %w", err)
	}
	defer func() {
		closeErr := r.Close()
		_ = closeErr // Ignore close error in defer
	}()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gzip read failed: %w", err)
	}
	return decompressed, nil
}

// Set compresses and stores a value in the cache.
// Uses the provided context for cache operations.
func (c *GzipCache) Set(ctx context.Context, key string, value []byte) error {
	return c.set(ctx, key, value, c.compress)
}

// Get retrieves and decompresses a value from the cache.
// Uses the provided context for cache operations.
func (c *GzipCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return c.get(ctx, key, c.decompress)
}

// Delete removes a value from the cache.
// Uses the provided context for cache operations.
func (c *GzipCache) Delete(ctx context.Context, key string) error {
	return c.delete(ctx, key)
}

// MarkStale marks the cached entry as stale without removing it.
// Uses the provided context for cache operations.
func (c *GzipCache) MarkStale(ctx context.Context, key string) error {
	return c.markStale(ctx, key)
}

// IsStale checks if the cached entry is marked as stale.
// Uses the provided context for cache operations.
func (c *GzipCache) IsStale(ctx context.Context, key string) (bool, error) {
	return c.isStale(ctx, key)
}

// GetStale retrieves and decompresses a stale entry if it exists and is marked as stale.
// Uses the provided context for cache operations.
func (c *GzipCache) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	return c.getStale(ctx, key, c.decompress)
}

// Stats returns compression statistics
func (c *GzipCache) Stats() Stats {
	return c.stats()
}
