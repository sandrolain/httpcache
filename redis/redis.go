// Package redis provides a redis interface for http caching.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sandrolain/httpcache"
)

// Config holds the configuration for creating a Redis cache.
type Config struct {
	// Address is the Redis server address (e.g., "localhost:6379").
	// Required field.
	Address string

	// Password is the Redis password for authentication.
	// Optional - leave empty if no authentication is required.
	Password string

	// Username is the Redis username for authentication (Redis 6.0+).
	// Optional - leave empty if no authentication is required.
	Username string

	// DB is the Redis database number to use.
	// Optional - defaults to 0.
	DB int

	// MaxRetries is the maximum number of retries before giving up.
	// Optional - defaults to 3.
	MaxRetries int

	// PoolSize is the maximum number of socket connections in the pool.
	// Optional - defaults to 10.
	PoolSize int

	// MinIdleConns is the minimum number of idle connections.
	// Optional - defaults to 0.
	MinIdleConns int

	// DialTimeout is the timeout for connecting to Redis.
	// Optional - defaults to 5 seconds.
	DialTimeout time.Duration

	// ReadTimeout is the timeout for reading from Redis.
	// Optional - defaults to 5 seconds.
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for writing to Redis.
	// Optional - defaults to 5 seconds.
	WriteTimeout time.Duration

	// PoolTimeout is the timeout for getting a connection from the pool.
	// Optional - defaults to ReadTimeout + 1 second.
	PoolTimeout time.Duration
}

// cache is an implementation of httpcache.Cache that caches responses in a
// redis server.
type cache struct {
	client redis.UniversalClient
}

const stalePrefix = "stale:"

// cacheKey modifies an httpcache key for use in redis. Specifically, it
// prefixes keys to avoid collision with other data stored in redis.
func cacheKey(key string) string {
	return "rediscache:" + key
}

// Get returns the response corresponding to key if present.
func (c cache) Get(ctx context.Context, key string) (resp []byte, ok bool, err error) {
	item, err := c.client.Get(ctx, cacheKey(key)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("redis cache get failed for key %q: %w", key, err)
	}
	return item, true, nil
}

// Set saves a response to the cache as key.
func (c cache) Set(ctx context.Context, key string, resp []byte) error {
	// Use pipeline to set value and remove stale marker atomically
	pipe := c.client.Pipeline()
	pipe.Set(ctx, cacheKey(key), resp, 0)
	pipe.Del(ctx, cacheKey(stalePrefix+key))
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis cache set failed for key %q: %w", key, err)
	}
	return nil
}

// Delete removes the response with key from the cache.
func (c cache) Delete(ctx context.Context, key string) error {
	pipe := c.client.Pipeline()
	pipe.Del(ctx, cacheKey(key))
	pipe.Del(ctx, cacheKey(stalePrefix+key))
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis cache delete failed for key %q: %w", key, err)
	}
	return nil
}

// MarkStale marks a cached response as stale instead of deleting it.
func (c cache) MarkStale(ctx context.Context, key string) error {
	// Check if entry exists
	exists, err := c.client.Exists(ctx, cacheKey(key)).Result()
	if err != nil {
		return fmt.Errorf("redis cache mark stale check failed for key %q: %w", key, err)
	}
	if exists == 0 {
		return nil // Entry doesn't exist, nothing to mark
	}

	// Set a marker key
	if err := c.client.Set(ctx, cacheKey(stalePrefix+key), "1", 0).Err(); err != nil {
		return fmt.Errorf("redis cache mark stale failed for key %q: %w", key, err)
	}
	return nil
}

// IsStale checks if a cached response has been marked as stale.
func (c cache) IsStale(ctx context.Context, key string) (bool, error) {
	exists, err := c.client.Exists(ctx, cacheKey(stalePrefix+key)).Result()
	if err != nil {
		return false, fmt.Errorf("redis cache is stale check failed for key %q: %w", key, err)
	}
	return exists > 0, nil
}

// GetStale retrieves a stale cached response if it exists.
func (c cache) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
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

// Close closes the connection to Redis.
// This method should be called when done to properly clean up resources.
func (c cache) Close() error {
	return c.client.Close()
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxRetries:   3,
		PoolSize:     10,
		MinIdleConns: 0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		DB:           0,
	}
}

// New creates a new Cache with the given configuration.
// It establishes a connection to Redis.
// The caller should call Close() on the returned cache when done to clean up resources.
func New(config Config) (httpcache.Cache, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("redis address is required")
	}

	// Apply defaults for zero values
	if config.MaxRetries == 0 {
		config.MaxRetries = DefaultConfig().MaxRetries
	}
	if config.PoolSize == 0 {
		config.PoolSize = DefaultConfig().PoolSize
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = DefaultConfig().DialTimeout
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = DefaultConfig().ReadTimeout
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = DefaultConfig().WriteTimeout
	}
	if config.PoolTimeout == 0 {
		config.PoolTimeout = config.ReadTimeout + time.Second
	}

	opts := &redis.Options{
		Addr:         config.Address,
		Password:     config.Password,
		Username:     config.Username,
		DB:           config.DB,
		MaxRetries:   config.MaxRetries,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		PoolTimeout:  config.PoolTimeout,
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), config.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close() //nolint:errcheck // best effort cleanup after ping failure
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return cache{client: client}, nil
}

// NewWithClient returns a new Cache with the given redis client.
// This constructor is useful for advanced use cases where you want
// to manage the client yourself or use a custom configuration.
// The passed client will be used directly and its lifecycle is managed
// by the caller (except Close() which can still be called on the cache).
func NewWithClient(client redis.UniversalClient) httpcache.Cache {
	return cache{client: client}
}
