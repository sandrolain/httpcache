// Package redis provides a redis interface for http caching.
package redis

import (
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
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

	// DB is the Redis database number to use.
	// Optional - defaults to 0.
	DB int

	// MaxIdle is the maximum number of idle connections in the pool.
	// Optional - defaults to 10.
	MaxIdle int

	// MaxActive is the maximum number of active connections in the pool.
	// Optional - defaults to 100. Set to 0 for unlimited.
	MaxActive int

	// IdleTimeout is the duration after which idle connections are closed.
	// Optional - defaults to 5 minutes.
	IdleTimeout time.Duration

	// ConnectTimeout is the timeout for connecting to Redis.
	// Optional - defaults to 5 seconds.
	ConnectTimeout time.Duration

	// ReadTimeout is the timeout for reading from Redis.
	// Optional - defaults to 5 seconds.
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for writing to Redis.
	// Optional - defaults to 5 seconds.
	WriteTimeout time.Duration
}

// cache is an implementation of httpcache.Cache that caches responses in a
// redis server.
type cache struct {
	pool *redis.Pool
}

// cacheKey modifies an httpcache key for use in redis. Specifically, it
// prefixes keys to avoid collision with other data stored in redis.
func cacheKey(key string) string {
	return "rediscache:" + key
}

// Get returns the response corresponding to key if present.
func (c cache) Get(key string) (resp []byte, ok bool) {
	conn := c.pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			httpcache.GetLogger().Error("failed to close redis connection", "error", err)
		}
	}()

	item, err := redis.Bytes(conn.Do("GET", cacheKey(key)))
	if err != nil {
		return nil, false
	}
	return item, true
}

// Set saves a response to the cache as key.
func (c cache) Set(key string, resp []byte) {
	conn := c.pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			httpcache.GetLogger().Error("failed to close redis connection", "error", err)
		}
	}()

	if _, err := conn.Do("SET", cacheKey(key), resp); err != nil {
		httpcache.GetLogger().Warn("failed to write to redis cache", "key", key, "error", err)
	}
}

// Delete removes the response with key from the cache.
func (c cache) Delete(key string) {
	conn := c.pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			httpcache.GetLogger().Error("failed to close redis connection", "error", err)
		}
	}()

	if _, err := conn.Do("DEL", cacheKey(key)); err != nil {
		httpcache.GetLogger().Warn("failed to delete from redis cache", "key", key, "error", err)
	}
}

// Close closes the connection pool.
// This method should be called when done to properly clean up resources.
func (c cache) Close() error {
	return c.pool.Close()
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxIdle:        10,
		MaxActive:      100,
		IdleTimeout:    5 * time.Minute,
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		DB:             0,
	}
}

// New creates a new Cache with the given configuration.
// It establishes a connection pool to Redis.
// The caller should call Close() on the returned cache when done to clean up resources.
func New(config Config) (httpcache.Cache, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("redis address is required")
	}

	// Apply defaults for zero values
	if config.MaxIdle == 0 {
		config.MaxIdle = DefaultConfig().MaxIdle
	}
	if config.MaxActive == 0 {
		config.MaxActive = DefaultConfig().MaxActive
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = DefaultConfig().IdleTimeout
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = DefaultConfig().ConnectTimeout
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = DefaultConfig().ReadTimeout
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = DefaultConfig().WriteTimeout
	}

	pool := &redis.Pool{
		MaxIdle:     config.MaxIdle,
		MaxActive:   config.MaxActive,
		IdleTimeout: config.IdleTimeout,
		Dial: func() (redis.Conn, error) {
			opts := []redis.DialOption{
				redis.DialConnectTimeout(config.ConnectTimeout),
				redis.DialReadTimeout(config.ReadTimeout),
				redis.DialWriteTimeout(config.WriteTimeout),
				redis.DialDatabase(config.DB),
			}

			if config.Password != "" {
				opts = append(opts, redis.DialPassword(config.Password))
			}

			return redis.Dial("tcp", config.Address, opts...)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}

	// Test connection
	conn := pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			httpcache.GetLogger().Error("failed to close redis test connection", "error", err)
		}
	}()

	if _, err := conn.Do("PING"); err != nil {
		if closeErr := pool.Close(); closeErr != nil {
			httpcache.GetLogger().Error("failed to close redis pool after ping error", "error", closeErr)
		}
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return cache{pool: pool}, nil
}

// NewWithClient returns a new Cache with the given redis connection.
// This constructor is useful for backwards compatibility or when you want
// to manage the connection yourself.
// Note: This creates a single-connection cache. For production use with
// connection pooling, use New() instead.
func NewWithClient(client redis.Conn) httpcache.Cache {
	pool := &redis.Pool{
		MaxIdle:   1,
		MaxActive: 1,
		Dial: func() (redis.Conn, error) {
			return client, nil
		},
	}
	return cache{pool: pool}
}
