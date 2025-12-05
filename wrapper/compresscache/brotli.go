package compresscache

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/sandrolain/httpcache"
)

// BrotliCache wraps a cache with automatic Brotli compression/decompression
type BrotliCache struct {
	*baseCompressCache
	level int
}

// BrotliConfig holds the configuration for Brotli compression
type BrotliConfig struct {
	// Cache is the underlying cache backend (required)
	Cache httpcache.Cache

	// Level is the compression level (0 to 11)
	// Default: 6
	Level int
}

// NewBrotli creates a new BrotliCache with Brotli compression
func NewBrotli(config BrotliConfig) (*BrotliCache, error) {
	if config.Cache == nil {
		return nil, fmt.Errorf("cache cannot be nil")
	}

	// Set defaults
	if config.Level == 0 {
		config.Level = 6 // Default brotli level
	}

	// Validate level (0-11 for brotli)
	if config.Level < 0 || config.Level > 11 {
		return nil, fmt.Errorf("invalid brotli compression level: %d", config.Level)
	}

	return &BrotliCache{
		baseCompressCache: newBaseCompressCache(config.Cache, Brotli),
		level:             config.Level,
	}, nil
}

// compress compresses data using Brotli algorithm
func (c *BrotliCache) compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	w := brotli.NewWriterLevel(&buf, c.level)
	if _, err := w.Write(data); err != nil {
		closeErr := w.Close()
		_ = closeErr // Ignore close error in error path
		return nil, fmt.Errorf("brotli write failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("brotli close failed: %w", err)
	}

	return buf.Bytes(), nil
}

// decompress decompresses data using Brotli algorithm
func (c *BrotliCache) decompress(data []byte) ([]byte, error) {
	r := brotli.NewReader(bytes.NewReader(data))
	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("brotli read failed: %w", err)
	}
	return decompressed, nil
}

// Set compresses and stores a value in the cache.
// Uses the provided context for cache operations.
func (c *BrotliCache) Set(ctx context.Context, key string, value []byte) error {
	return c.set(ctx, key, value, c.compress)
}

// Get retrieves and decompresses a value from the cache.
// Uses the provided context for cache operations.
func (c *BrotliCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return c.get(ctx, key, c.decompress)
}

// Delete removes a value from the cache.
// Uses the provided context for cache operations.
func (c *BrotliCache) Delete(ctx context.Context, key string) error {
	return c.delete(ctx, key)
}

// Stats returns compression statistics
func (c *BrotliCache) Stats() Stats {
	return c.stats()
}
