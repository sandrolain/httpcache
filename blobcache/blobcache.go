// Package blobcache provides an httpcache.Cache implementation that uses
// Go Cloud Development Kit (CDK) blob storage for cloud-agnostic cache storage.
//
// Supports multiple cloud providers:
//   - Amazon S3
//   - Google Cloud Storage
//   - Azure Blob Storage
//   - In-memory (for testing)
//   - Local filesystem
//
// Example usage with S3:
//
//	import (
//	    "context"
//	    _ "gocloud.dev/blob/s3blob"
//	    "github.com/sandrolain/httpcache/blobcache"
//	)
//
//	ctx := context.Background()
//	cache, err := blobcache.New(ctx, blobcache.Config{
//	    BucketURL: "s3://my-bucket?region=us-west-2",
//	    KeyPrefix: "httpcache/",
//	})
package blobcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/sandrolain/httpcache"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
)

// Config holds the configuration for the blob cache.
type Config struct {
	// BucketURL is the Go Cloud blob URL (e.g., "s3://bucket?region=us-west-2")
	BucketURL string

	// KeyPrefix is prepended to all cache keys (default: "cache/")
	KeyPrefix string

	// Timeout for blob operations (default: 30s)
	Timeout time.Duration

	// Bucket is an optional pre-opened bucket (if nil, BucketURL is used)
	Bucket *blob.Bucket
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		KeyPrefix: "cache/",
		Timeout:   30 * time.Second,
	}
}

// cache implements httpcache.Cache using Go Cloud blob storage.
type cache struct {
	bucket     *blob.Bucket
	keyPrefix  string
	timeout    time.Duration
	ownsBucket bool // true if we opened the bucket (should close it)
}

const stalePrefix = "stale_"

// New creates a new blob cache with the given configuration.
// The bucket is opened using the BucketURL.
// Call Close() to clean up resources when done.
func New(ctx context.Context, config Config) (httpcache.Cache, error) {
	if config.BucketURL == "" && config.Bucket == nil {
		return nil, fmt.Errorf("either BucketURL or Bucket must be provided")
	}

	if config.KeyPrefix == "" {
		config.KeyPrefix = DefaultConfig().KeyPrefix
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultConfig().Timeout
	}

	var bucket *blob.Bucket
	var ownsBucket bool
	var err error

	if config.Bucket != nil {
		bucket = config.Bucket
		ownsBucket = false
	} else {
		bucket, err = blob.OpenBucket(ctx, config.BucketURL)
		if err != nil {
			return nil, fmt.Errorf("failed to open bucket: %w", err)
		}
		ownsBucket = true
	}

	return &cache{
		bucket:     bucket,
		keyPrefix:  config.KeyPrefix,
		timeout:    config.Timeout,
		ownsBucket: ownsBucket,
	}, nil
}

// NewWithBucket creates a cache using an already-opened bucket.
// The caller is responsible for closing the bucket.
func NewWithBucket(bucket *blob.Bucket, keyPrefix string, timeout time.Duration) httpcache.Cache {
	if keyPrefix == "" {
		keyPrefix = DefaultConfig().KeyPrefix
	}
	if timeout == 0 {
		timeout = DefaultConfig().Timeout
	}

	return &cache{
		bucket:     bucket,
		keyPrefix:  keyPrefix,
		timeout:    timeout,
		ownsBucket: false,
	}
}

// cacheKey generates a blob key from a cache key.
// Uses SHA-256 hash to avoid issues with special characters in cloud storage.
func (c *cache) cacheKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return c.keyPrefix + hex.EncodeToString(hash[:])
}

// Get returns the response corresponding to key if present.
// Uses the provided context for timeout and cancellation.
// If the context has a deadline, it will be used; otherwise, the configured timeout is applied.
func (c *cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	// Use provided context with fallback timeout if no deadline is set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	blobKey := c.cacheKey(key)

	reader, err := c.bucket.NewReader(ctx, blobKey, nil)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("blobcache get failed for key %q: %w", key, err)
	}
	defer reader.Close() //nolint:errcheck // best effort cleanup, error already handled

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, false, fmt.Errorf("blobcache read failed for key %q: %w", key, err)
	}

	return data, true, nil
}

// Set saves a response to the cache as key.
// Uses the provided context for timeout and cancellation.
// If the context has a deadline, it will be used; otherwise, the configured timeout is applied.
func (c *cache) Set(ctx context.Context, key string, data []byte) error {
	// Use provided context with fallback timeout if no deadline is set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	// Remove stale marker when setting a fresh value
	staleKey := c.cacheKey(stalePrefix + key)
	_ = c.bucket.Delete(ctx, staleKey) //nolint:errcheck // blob not found is acceptable

	blobKey := c.cacheKey(key)

	writer, err := c.bucket.NewWriter(ctx, blobKey, nil)
	if err != nil {
		return fmt.Errorf("blobcache set failed to create writer for key %q: %w", key, err)
	}

	_, writeErr := writer.Write(data)
	closeErr := writer.Close()

	if writeErr != nil {
		return fmt.Errorf("blobcache set failed to write for key %q: %w", key, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("blobcache set failed to close writer for key %q: %w", key, closeErr)
	}
	return nil
}

// Delete removes the response with key from the cache.
// Uses the provided context for timeout and cancellation.
// If the context has a deadline, it will be used; otherwise, the configured timeout is applied.
func (c *cache) Delete(ctx context.Context, key string) error {
	// Use provided context with fallback timeout if no deadline is set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	blobKey := c.cacheKey(key)
	staleKey := c.cacheKey(stalePrefix + key)

	// Delete both the main entry and stale marker
	err := c.bucket.Delete(ctx, blobKey)
	if err != nil && gcerrors.Code(err) != gcerrors.NotFound {
		return fmt.Errorf("blobcache delete failed for key %q: %w", key, err)
	}

	// Also delete stale marker if it exists (ignore not found errors)
	_ = c.bucket.Delete(ctx, staleKey) //nolint:errcheck // not found is acceptable
	return nil
}

// MarkStale marks a cached response as stale instead of deleting it.
// Uses the provided context for timeout and cancellation.
func (c *cache) MarkStale(ctx context.Context, key string) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	blobKey := c.cacheKey(key)
	staleKey := c.cacheKey(stalePrefix + key)

	// Check if entry exists
	exists, err := c.bucket.Exists(ctx, blobKey)
	if err != nil {
		return fmt.Errorf("blobcache mark stale failed to check existence for key %q: %w", key, err)
	}
	if !exists {
		return nil // Entry doesn't exist, nothing to mark
	}

	// Create a marker blob
	writer, err := c.bucket.NewWriter(ctx, staleKey, nil)
	if err != nil {
		return fmt.Errorf("blobcache mark stale failed to create writer for key %q: %w", key, err)
	}
	_, writeErr := writer.Write([]byte("1"))
	closeErr := writer.Close()
	if writeErr != nil {
		return fmt.Errorf("blobcache mark stale failed to write for key %q: %w", key, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("blobcache mark stale failed to close writer for key %q: %w", key, closeErr)
	}
	return nil
}

// IsStale checks if a cached response has been marked as stale.
// Uses the provided context for timeout and cancellation.
func (c *cache) IsStale(ctx context.Context, key string) (bool, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	staleKey := c.cacheKey(stalePrefix + key)
	exists, err := c.bucket.Exists(ctx, staleKey)
	if err != nil {
		return false, fmt.Errorf("blobcache is stale check failed for key %q: %w", key, err)
	}
	return exists, nil
}

// GetStale retrieves a stale cached response if it exists.
// Uses the provided context for timeout and cancellation.
func (c *cache) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	// Check if marked as stale
	isStale, err := c.IsStale(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if !isStale {
		return nil, false, nil
	}

	// Retrieve the actual data
	return c.Get(ctx, key)
}

// Close closes the bucket if it was opened by New().
// If the bucket was provided via NewWithBucket(), it's not closed.
func (c *cache) Close() error {
	if c.ownsBucket {
		if err := c.bucket.Close(); err != nil {
			return fmt.Errorf("failed to close blob bucket: %w", err)
		}
	}
	return nil
}
