// Package postgresql provides a PostgreSQL interface for HTTP caching.
package postgresql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNilPool is returned when a nil pool is provided
	ErrNilPool = errors.New("postgresql: pool cannot be nil")
	// ErrNilConn is returned when a nil connection is provided
	ErrNilConn = errors.New("postgresql: connection cannot be nil")
)

const (
	// DefaultTableName is the default table name for cache storage
	DefaultTableName = "httpcache"
	// DefaultKeyPrefix is the default prefix for cache keys
	DefaultKeyPrefix = "cache:"
)

// Cache is an implementation of httpcache.Cache that stores responses in PostgreSQL.
type Cache struct {
	pool      *pgxpool.Pool
	conn      *pgx.Conn
	tableName string
	keyPrefix string
	timeout   time.Duration
}

// Config holds the configuration for the PostgreSQL cache.
type Config struct {
	// TableName is the name of the table to store cache entries (default: "httpcache")
	TableName string
	// KeyPrefix is the prefix to add to all cache keys (default: "cache:")
	KeyPrefix string
	// Timeout is the maximum time to wait for database operations (default: 5s)
	Timeout time.Duration
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		TableName: DefaultTableName,
		KeyPrefix: DefaultKeyPrefix,
		Timeout:   5 * time.Second,
	}
}

// cacheKey returns the full cache key with prefix.
func (c *Cache) cacheKey(key string) string {
	return c.keyPrefix + key
}

// Get returns the response corresponding to key if present.
// Uses the provided context for timeout and cancellation.
// If the context has a deadline, it will be used; otherwise, the configured timeout is applied.
func (c *Cache) Get(ctx context.Context, key string) (resp []byte, ok bool, err error) {
	// Use provided context with fallback timeout if no deadline is set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var data []byte

	query := `SELECT data FROM ` + c.tableName + ` WHERE key = $1`

	if c.pool != nil {
		err = c.pool.QueryRow(ctx, query, c.cacheKey(key)).Scan(&data)
	} else {
		err = c.conn.QueryRow(ctx, query, c.cacheKey(key)).Scan(&data)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("postgresql cache get failed for key %q: %w", key, err)
	}

	return data, true, nil
}

// Set saves a response to the cache as key.
// Uses the provided context for timeout and cancellation.
// If the context has a deadline, it will be used; otherwise, the configured timeout is applied.
func (c *Cache) Set(ctx context.Context, key string, resp []byte) error {
	// Use provided context with fallback timeout if no deadline is set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	query := `
		INSERT INTO ` + c.tableName + ` (key, data, created_at, stale)
		VALUES ($1, $2, $3, false)
		ON CONFLICT (key) DO UPDATE SET data = $2, created_at = $3, stale = false
	`

	var err error
	if c.pool != nil {
		_, err = c.pool.Exec(ctx, query, c.cacheKey(key), resp, time.Now())
	} else {
		_, err = c.conn.Exec(ctx, query, c.cacheKey(key), resp, time.Now())
	}

	if err != nil {
		return fmt.Errorf("postgresql cache set failed for key %q: %w", key, err)
	}
	return nil
}

// Delete removes the response with key from the cache.
// Uses the provided context for timeout and cancellation.
// If the context has a deadline, it will be used; otherwise, the configured timeout is applied.
func (c *Cache) Delete(ctx context.Context, key string) error {
	// Use provided context with fallback timeout if no deadline is set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	query := `DELETE FROM ` + c.tableName + ` WHERE key = $1`

	var err error
	if c.pool != nil {
		_, err = c.pool.Exec(ctx, query, c.cacheKey(key))
	} else {
		_, err = c.conn.Exec(ctx, query, c.cacheKey(key))
	}

	if err != nil {
		return fmt.Errorf("postgresql cache delete failed for key %q: %w", key, err)
	}
	return nil
}

// MarkStale marks a cached response as stale instead of deleting it.
func (c *Cache) MarkStale(ctx context.Context, key string) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	query := `UPDATE ` + c.tableName + ` SET stale = true WHERE key = $1`

	var err error
	if c.pool != nil {
		_, err = c.pool.Exec(ctx, query, c.cacheKey(key))
	} else {
		_, err = c.conn.Exec(ctx, query, c.cacheKey(key))
	}

	if err != nil {
		return fmt.Errorf("postgresql cache mark stale failed for key %q: %w", key, err)
	}
	return nil
}

// IsStale checks if a cached response has been marked as stale.
func (c *Cache) IsStale(ctx context.Context, key string) (bool, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var stale bool
	query := `SELECT COALESCE(stale, false) FROM ` + c.tableName + ` WHERE key = $1`

	var err error
	if c.pool != nil {
		err = c.pool.QueryRow(ctx, query, c.cacheKey(key)).Scan(&stale)
	} else {
		err = c.conn.QueryRow(ctx, query, c.cacheKey(key)).Scan(&stale)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("postgresql cache is stale check failed for key %q: %w", key, err)
	}

	return stale, nil
}

// GetStale retrieves a stale cached response if it exists.
func (c *Cache) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var data []byte
	var stale bool

	query := `SELECT data, COALESCE(stale, false) FROM ` + c.tableName + ` WHERE key = $1`

	var err error
	if c.pool != nil {
		err = c.pool.QueryRow(ctx, query, c.cacheKey(key)).Scan(&data, &stale)
	} else {
		err = c.conn.QueryRow(ctx, query, c.cacheKey(key)).Scan(&data, &stale)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("postgresql cache get stale failed for key %q: %w", key, err)
	}

	if !stale {
		return nil, false, nil
	}

	return data, true, nil
}

// CreateTable creates the cache table if it doesn't exist.
func (c *Cache) CreateTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS ` + c.tableName + ` (
			key TEXT PRIMARY KEY,
			data BYTEA NOT NULL,
			created_at TIMESTAMP NOT NULL,
			stale BOOLEAN DEFAULT FALSE
		)
	`

	var err error
	if c.pool != nil {
		_, err = c.pool.Exec(ctx, query)
	} else {
		_, err = c.conn.Exec(ctx, query)
	}
	if err != nil {
		return err
	}

	// Ensure the stale column exists for users upgrading from older schemas.
	alter := `ALTER TABLE ` + c.tableName + ` ADD COLUMN IF NOT EXISTS stale BOOLEAN DEFAULT FALSE`
	if c.pool != nil {
		_, err = c.pool.Exec(ctx, alter)
	} else {
		_, err = c.conn.Exec(ctx, alter)
	}
	return err
}

// Close closes the connection pool or connection.
func (c *Cache) Close() {
	if c.pool != nil {
		c.pool.Close()
	} else if c.conn != nil {
		c.conn.Close(context.Background()) //nolint:errcheck // best effort cleanup
	}
}

// NewWithPool returns a new Cache using the provided connection pool.
func NewWithPool(pool *pgxpool.Pool, config *Config) (*Cache, error) {
	if pool == nil {
		return nil, ErrNilPool
	}

	if config == nil {
		config = DefaultConfig()
	}

	return &Cache{
		pool:      pool,
		tableName: config.TableName,
		keyPrefix: config.KeyPrefix,
		timeout:   config.Timeout,
	}, nil
}

// NewWithConn returns a new Cache using the provided connection.
func NewWithConn(conn *pgx.Conn, config *Config) (*Cache, error) {
	if conn == nil {
		return nil, ErrNilConn
	}

	if config == nil {
		config = DefaultConfig()
	}

	return &Cache{
		conn:      conn,
		tableName: config.TableName,
		keyPrefix: config.KeyPrefix,
		timeout:   config.Timeout,
	}, nil
}

// New creates a new Cache with a connection pool from the given connection string.
func New(ctx context.Context, connString string, config *Config) (*Cache, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}

	if config == nil {
		config = DefaultConfig()
	}

	cache := &Cache{
		pool:      pool,
		tableName: config.TableName,
		keyPrefix: config.KeyPrefix,
		timeout:   config.Timeout,
	}

	// Create table if it doesn't exist
	if err := cache.CreateTable(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return cache, nil
}
