// Package natskv provides a NATS JetStream Key/Value store interface for http caching.
package natskv

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sandrolain/httpcache"
)

// Config holds the configuration for creating a NATS K/V cache.
type Config struct {
	// NATSUrl is the URL of the NATS server (e.g., "nats://localhost:4222").
	// If empty, defaults to nats.DefaultURL.
	NATSUrl string

	// Bucket is the name of the K/V bucket to use for caching.
	// Required field.
	Bucket string

	// Description is an optional description for the K/V bucket.
	Description string

	// TTL is the time-to-live for cache entries.
	// If zero, entries don't expire (unless deleted by NATS based on other policies).
	TTL time.Duration

	// NATSOptions are additional options to pass to nats.Connect.
	// Optional.
	NATSOptions []nats.Option
}

// cache is an implementation of httpcache.Cache that caches responses in a
// NATS JetStream Key/Value store.
type cache struct {
	kv jetstream.KeyValue
	nc *nats.Conn
}

// cacheKey modifies an httpcache key for use in NATS K/V. Specifically, it
// prefixes keys to avoid collision with other data stored in the bucket.
// NATS K/V keys must not contain certain characters like ':'.
func cacheKey(key string) string {
	return "httpcache." + key
}

// Get returns the response corresponding to key if present.
// Uses the provided context for cancellation.
func (c cache) Get(ctx context.Context, key string) (resp []byte, ok bool, err error) {
	entry, err := c.kv.Get(ctx, cacheKey(key))
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	return entry.Value(), true, nil
}

// Set saves a response to the cache as key.
// Uses the provided context for cancellation.
func (c cache) Set(ctx context.Context, key string, resp []byte) error {
	if _, err := c.kv.Put(ctx, cacheKey(key), resp); err != nil {
		httpcache.GetLogger().Warn("failed to write to NATS K/V cache", "key", key, "error", err)
		return err
	}
	return nil
}

// Delete removes the response with key from the cache.
// Uses the provided context for cancellation.
func (c cache) Delete(ctx context.Context, key string) error {
	if err := c.kv.Delete(ctx, cacheKey(key)); err != nil {
		if err != jetstream.ErrKeyNotFound {
			httpcache.GetLogger().Warn("failed to delete from NATS K/V cache", "key", key, "error", err)
			return err
		}
	}
	return nil
}

// Close closes the underlying NATS connection if it was created by New().
// This method should be called when using New() to properly clean up resources.
// It's a no-op when using NewWithKeyValue().
func (c cache) Close() error {
	if c.nc != nil {
		c.nc.Close()
	}
	return nil
}

// New creates a new Cache with the given configuration.
// It establishes a connection to NATS, creates a JetStream context,
// and creates or updates the K/V bucket according to the configuration.
// The caller should call Close() on the returned cache when done to clean up resources.
func New(ctx context.Context, config Config) (httpcache.Cache, error) {
	if config.Bucket == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	// Use default URL if not specified
	url := config.NATSUrl
	if url == "" {
		url = nats.DefaultURL
	}

	// Connect to NATS
	nc, err := nats.Connect(url, config.NATSOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	// Create or update K/V bucket
	kvConfig := jetstream.KeyValueConfig{
		Bucket:      config.Bucket,
		Description: config.Description,
		TTL:         config.TTL,
	}

	kv, err := js.CreateOrUpdateKeyValue(ctx, kvConfig)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create or update K/V bucket: %w", err)
	}

	return cache{kv: kv, nc: nc}, nil
}

// NewWithKeyValue returns a new Cache with the given NATS JetStream KeyValue store.
// This constructor is useful when you want to manage the NATS connection yourself.
// The returned cache will not close the NATS connection when Close() is called.
func NewWithKeyValue(kv jetstream.KeyValue) httpcache.Cache {
	return cache{kv: kv, nc: nil}
}
