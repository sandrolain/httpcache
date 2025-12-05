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

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	"github.com/sandrolain/httpcache"
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
		httpcache.GetLogger().Error("failed to read from blob cache", "key", key, "error", err)
		return nil, false, err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			httpcache.GetLogger().Error("failed to close blob reader", "key", key, "error", closeErr)
		}
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		httpcache.GetLogger().Error("failed to read blob data", "key", key, "error", err)
		return nil, false, err
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

	blobKey := c.cacheKey(key)

	writer, err := c.bucket.NewWriter(ctx, blobKey, nil)
	if err != nil {
		httpcache.GetLogger().Error("failed to create blob writer", "key", key, "error", err)
		return err
	}

	_, writeErr := writer.Write(data)
	closeErr := writer.Close()

	if writeErr != nil {
		httpcache.GetLogger().Error("failed to write to blob cache", "key", key, "error", writeErr)
		return writeErr
	}
	if closeErr != nil {
		httpcache.GetLogger().Error("failed to close blob writer", "key", key, "error", closeErr)
		return closeErr
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

	err := c.bucket.Delete(ctx, blobKey)
	if err != nil && gcerrors.Code(err) != gcerrors.NotFound {
		httpcache.GetLogger().Error("failed to delete from blob cache", "key", key, "error", err)
		return err
	}
	return nil
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
