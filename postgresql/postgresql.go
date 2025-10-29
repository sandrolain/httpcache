// Package postgresql provides a PostgreSQL interface for HTTP caching.
package postgresql

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sandrolain/httpcache"
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
func (c *Cache) Get(key string) (resp []byte, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	var data []byte
	var err error

	query := `SELECT data FROM ` + c.tableName + ` WHERE key = $1`

	if c.pool != nil {
		err = c.pool.QueryRow(ctx, query, c.cacheKey(key)).Scan(&data)
	} else {
		err = c.conn.QueryRow(ctx, query, c.cacheKey(key)).Scan(&data)
	}

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			httpcache.GetLogger().Warn("failed to read from postgresql cache", "key", key, "error", err)
		}
		return nil, false
	}

	return data, true
}

// Set saves a response to the cache as key.
func (c *Cache) Set(key string, resp []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	query := `
		INSERT INTO ` + c.tableName + ` (key, data, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET data = $2, created_at = $3
	`

	var err error
	if c.pool != nil {
		_, err = c.pool.Exec(ctx, query, c.cacheKey(key), resp, time.Now())
	} else {
		_, err = c.conn.Exec(ctx, query, c.cacheKey(key), resp, time.Now())
	}

	if err != nil {
		httpcache.GetLogger().Warn("failed to write to postgresql cache", "key", key, "error", err)
	}
}

// Delete removes the response with key from the cache.
func (c *Cache) Delete(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	query := `DELETE FROM ` + c.tableName + ` WHERE key = $1`

	var err error
	if c.pool != nil {
		_, err = c.pool.Exec(ctx, query, c.cacheKey(key))
	} else {
		_, err = c.conn.Exec(ctx, query, c.cacheKey(key))
	}

	if err != nil {
		httpcache.GetLogger().Warn("failed to delete from postgresql cache", "key", key, "error", err)
	}
}

// CreateTable creates the cache table if it doesn't exist.
func (c *Cache) CreateTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS ` + c.tableName + ` (
			key TEXT PRIMARY KEY,
			data BYTEA NOT NULL,
			created_at TIMESTAMP NOT NULL
		)
	`

	var err error
	if c.pool != nil {
		_, err = c.pool.Exec(ctx, query)
	} else {
		_, err = c.conn.Exec(ctx, query)
	}

	return err
}

// Close closes the connection pool or connection.
func (c *Cache) Close() {
	if c.pool != nil {
		c.pool.Close()
	} else if c.conn != nil {
		if err := c.conn.Close(context.Background()); err != nil {
			httpcache.GetLogger().Warn("failed to close postgresql connection", "error", err)
		}
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
