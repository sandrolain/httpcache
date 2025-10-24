package postgresql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sandrolain/httpcache/test"
)

func getTestConnString() string {
	connString := os.Getenv("POSTGRESQL_TEST_URL")
	if connString == "" {
		connString = "postgres://postgres:postgres@localhost:5432/httpcache_test?sslmode=disable"
	}
	return connString
}

func TestPostgreSQLCache(t *testing.T) {
	ctx := context.Background()
	connString := getTestConnString()

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		t.Skipf("skipping test; could not connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping test; PostgreSQL not available: %v", err)
	}

	config := DefaultConfig()
	config.TableName = "httpcache_test"

	cache, err := NewWithPool(pool, config)
	if err != nil {
		t.Fatalf("NewWithPool failed: %v", err)
	}
	defer cache.Close()

	// Create table
	if err := cache.CreateTable(ctx); err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Clean up table before tests
	_, err = pool.Exec(ctx, "DELETE FROM "+config.TableName)
	if err != nil {
		t.Fatalf("failed to clean up table: %v", err)
	}

	test.Cache(t, cache)

	// Clean up table after tests
	_, err = pool.Exec(ctx, "DROP TABLE IF EXISTS "+config.TableName)
	if err != nil {
		t.Logf("warning: failed to drop test table: %v", err)
	}
}

func TestPostgreSQLCacheWithConn(t *testing.T) {
	ctx := context.Background()
	connString := getTestConnString()

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		t.Skipf("skipping test; could not connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping test; PostgreSQL not available: %v", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire connection: %v", err)
	}
	defer conn.Release()

	config := DefaultConfig()
	config.TableName = "httpcache_test_conn"

	cache, err := NewWithConn(conn.Conn(), config)
	if err != nil {
		t.Fatalf("NewWithConn failed: %v", err)
	}
	defer cache.Close()

	// Create table
	if err := cache.CreateTable(ctx); err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Clean up table before tests
	_, err = pool.Exec(ctx, "DELETE FROM "+config.TableName)
	if err != nil {
		t.Fatalf("failed to clean up table: %v", err)
	}

	test.Cache(t, cache)

	// Clean up table after tests
	_, err = pool.Exec(ctx, "DROP TABLE IF EXISTS "+config.TableName)
	if err != nil {
		t.Logf("warning: failed to drop test table: %v", err)
	}
}

func TestPostgreSQLCacheNew(t *testing.T) {
	ctx := context.Background()
	connString := getTestConnString()

	config := DefaultConfig()
	config.TableName = "httpcache_test_new"

	cache, err := New(ctx, connString, config)
	if err != nil {
		t.Skipf("skipping test; could not create cache: %v", err)
	}
	defer cache.Close()

	test.Cache(t, cache)

	// Clean up table after tests
	if cache.pool != nil {
		_, err = cache.pool.Exec(ctx, "DROP TABLE IF EXISTS "+config.TableName)
		if err != nil {
			t.Logf("warning: failed to drop test table: %v", err)
		}
	}
}

func TestPostgreSQLCacheConfig(t *testing.T) {
	ctx := context.Background()
	connString := getTestConnString()

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		t.Skipf("skipping test; could not connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	// Test with custom config
	config := &Config{
		TableName: "custom_cache_table",
		KeyPrefix: "custom:",
		Timeout:   10 * time.Second,
	}

	cache, err := NewWithPool(pool, config)
	if err != nil {
		t.Fatalf("NewWithPool failed: %v", err)
	}
	defer cache.Close()

	if cache.tableName != "custom_cache_table" {
		t.Errorf("expected tableName 'custom_cache_table', got '%s'", cache.tableName)
	}

	if cache.keyPrefix != "custom:" {
		t.Errorf("expected keyPrefix 'custom:', got '%s'", cache.keyPrefix)
	}

	if cache.timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", cache.timeout)
	}

	// Test with nil config (should use defaults)
	cache2, err := NewWithPool(pool, nil)
	if err != nil {
		t.Fatalf("NewWithPool with nil config failed: %v", err)
	}
	defer cache2.Close()

	if cache2.tableName != DefaultTableName {
		t.Errorf("expected default tableName '%s', got '%s'", DefaultTableName, cache2.tableName)
	}

	if cache2.keyPrefix != DefaultKeyPrefix {
		t.Errorf("expected default keyPrefix '%s', got '%s'", DefaultKeyPrefix, cache2.keyPrefix)
	}

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+config.TableName)
}

func TestPostgreSQLCacheErrors(t *testing.T) {
	// Test nil pool
	_, err := NewWithPool(nil, nil)
	if err != ErrNilPool {
		t.Errorf("expected ErrNilPool, got %v", err)
	}

	// Test nil conn
	_, err = NewWithConn(nil, nil)
	if err != ErrNilConn {
		t.Errorf("expected ErrNilConn, got %v", err)
	}
}

func TestPostgreSQLCacheKeyPrefix(t *testing.T) {
	ctx := context.Background()
	connString := getTestConnString()

	config := &Config{
		TableName: "httpcache_test_prefix",
		KeyPrefix: "test:",
		Timeout:   5 * time.Second,
	}

	cache, err := New(ctx, connString, config)
	if err != nil {
		t.Skipf("skipping test; could not create cache: %v", err)
	}
	defer cache.Close()

	// Test that key prefix is applied
	testKey := "mykey"
	testData := []byte("test data")

	cache.Set(testKey, testData)

	// Verify the key in database has the prefix
	var key string
	var data []byte
	err = cache.pool.QueryRow(ctx, "SELECT key, data FROM "+config.TableName+" WHERE key = $1", "test:mykey").Scan(&key, &data)
	if err != nil {
		t.Fatalf("failed to query database: %v", err)
	}

	if key != "test:mykey" {
		t.Errorf("expected key 'test:mykey', got '%s'", key)
	}

	if string(data) != string(testData) {
		t.Errorf("expected data '%s', got '%s'", testData, data)
	}

	// Clean up
	_, _ = cache.pool.Exec(ctx, "DROP TABLE IF EXISTS "+config.TableName)
}
