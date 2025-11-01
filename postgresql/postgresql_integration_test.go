//go:build integration

package postgresql

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sandrolain/httpcache/test"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	postgresImage    = "postgres:18.0-alpine3.22"
	cockroachImage   = "cockroachdb/cockroach:v25.2.7"
	postgresPassword = "testpassword"
	postgresUser     = "testuser"
	postgresDB       = "testdb"
)

// setupPostgreSQLContainer starts a PostgreSQL container and returns the connection string
func setupPostgreSQLContainer(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        postgresImage,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": postgresPassword,
			"POSTGRES_USER":     postgresUser,
			"POSTGRES_DB":       postgresDB,
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start PostgreSQL container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		postgresUser, postgresPassword, host, port.Port(), postgresDB)

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", err)
		}
	}

	return connString, cleanup
}

// setupCockroachDBContainer starts a CockroachDB container and returns the connection string
func setupCockroachDBContainer(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        cockroachImage,
		ExposedPorts: []string{"26257/tcp"},
		Cmd:          []string{"start-single-node", "--insecure"},
		WaitingFor: wait.ForLog("CockroachDB node starting").
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start CockroachDB container: %v", err)
	}

	// Wait a bit for CockroachDB to be fully ready
	time.Sleep(2 * time.Second)

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "26257")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	connString := fmt.Sprintf("postgres://root@%s:%s/defaultdb?sslmode=disable",
		host, port.Port())

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate CockroachDB container: %v", err)
		}
	}

	return connString, cleanup
}

// waitForDatabase waits for the database to be ready
func waitForDatabase(ctx context.Context, t *testing.T, connString string, maxRetries int, retryDelay time.Duration) *pgxpool.Pool {
	t.Helper()

	var pool *pgxpool.Pool
	var err error
	for i := 0; i < maxRetries; i++ {
		pool, err = pgxpool.New(ctx, connString)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool
			}
			pool.Close()
		}
		time.Sleep(retryDelay)
	}
	t.Fatalf("failed to connect to database after %d retries: %v", maxRetries, err)
	return nil
}

// setupTestCache creates and initializes a test cache
func setupTestCache(ctx context.Context, t *testing.T, pool *pgxpool.Pool, tableName string) *Cache {
	t.Helper()

	config := DefaultConfig()
	config.TableName = tableName

	cache, err := NewWithPool(pool, config)
	if err != nil {
		t.Fatalf(errNewWithPoolFailed, err)
	}

	if err := cache.CreateTable(ctx); err != nil {
		t.Fatalf(errCreateTableFailed, err)
	}

	// Clean up before tests
	_, _ = pool.Exec(ctx, "DELETE FROM "+config.TableName)

	return cache
}

// cleanupTestTable drops the test table
func cleanupTestTable(ctx context.Context, pool *pgxpool.Pool, tableName string) {
	_, _ = pool.Exec(ctx, queryDropTableIfExists+tableName)
}

func TestPostgreSQLCacheIntegration(t *testing.T) {
	ctx := context.Background()

	connString, cleanup := setupPostgreSQLContainer(ctx, t)
	defer cleanup()

	t.Log("PostgreSQL container started, connection string:", connString)

	pool := waitForDatabase(ctx, t, connString, 10, 1*time.Second)
	defer pool.Close()

	t.Log("Successfully connected to PostgreSQL")

	// Run tests with pool
	t.Run("WithPool", func(t *testing.T) {
		cache := setupTestCache(ctx, t, pool, "httpcache_integration_test")
		// Don't close the cache here as it would close the shared pool

		test.Cache(t, cache)

		cleanupTestTable(ctx, pool, "httpcache_integration_test")
	})

	// Run tests with New()
	t.Run("WithNew", func(t *testing.T) {
		config := DefaultConfig()
		config.TableName = "httpcache_integration_new"

		cache, err := New(ctx, connString, config)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		defer cache.Close()

		test.Cache(t, cache)

		cleanupTestTable(ctx, cache.pool, config.TableName)
	})

	testConcurrentAccess(ctx, t, pool)
}

// testConcurrentAccess tests concurrent cache operations
func testConcurrentAccess(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	t.Run("ConcurrentAccess", func(t *testing.T) {
		cache := setupTestCache(ctx, t, pool, "httpcache_concurrent")
		// Don't close the cache here as it would close the shared pool

		// Run concurrent operations
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(n int) {
				key := fmt.Sprintf("key-%d", n)
				data := []byte(fmt.Sprintf("data-%d", n))

				// Set
				cache.Set(key, data)

				// Get
				retrieved, ok := cache.Get(key)
				if !ok {
					t.Errorf("failed to get key %s", key)
				}
				if string(retrieved) != string(data) {
					t.Errorf("data mismatch for key %s", key)
				}

				// Delete
				cache.Delete(key)

				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		cleanupTestTable(ctx, pool, "httpcache_concurrent")
	})
}

func TestCockroachDBCacheIntegration(t *testing.T) {
	ctx := context.Background()

	connString, cleanup := setupCockroachDBContainer(ctx, t)
	defer cleanup()

	t.Log("CockroachDB container started, connection string:", connString)

	pool := waitForDatabase(ctx, t, connString, 15, 2*time.Second)
	defer pool.Close()

	t.Log("Successfully connected to CockroachDB")

	// Run tests with CockroachDB
	t.Run("WithPool", func(t *testing.T) {
		cache := setupTestCache(ctx, t, pool, "httpcache_cockroach_test")
		// Don't close the cache here as it would close the shared pool

		test.Cache(t, cache)

		cleanupTestTable(ctx, pool, "httpcache_cockroach_test")
	})

	testUpsertBehavior(ctx, t, pool)
	testDistributedTransactions(ctx, t, pool)
}

// testUpsertBehavior tests UPSERT functionality (important for CockroachDB)
func testUpsertBehavior(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	t.Run("UpsertBehavior", func(t *testing.T) {
		cache := setupTestCache(ctx, t, pool, "httpcache_upsert_test")
		// Don't close the cache here as it would close the shared pool

		key := "upsert-test-key"
		data1 := []byte("original data")
		data2 := []byte("updated data")

		// First insert
		cache.Set(key, data1)

		retrieved, ok := cache.Get(key)
		if !ok {
			t.Fatal("failed to get key after first insert")
		}
		if string(retrieved) != string(data1) {
			t.Errorf("expected '%s', got '%s'", data1, retrieved)
		}

		// Update (should use UPSERT)
		cache.Set(key, data2)

		retrieved, ok = cache.Get(key)
		if !ok {
			t.Fatal("failed to get key after update")
		}
		if string(retrieved) != string(data2) {
			t.Errorf("expected '%s', got '%s'", data2, retrieved)
		}

		// Verify only one row exists
		var count int
		err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM httpcache_upsert_test WHERE key = $1", "cache:"+key).Scan(&count)
		if err != nil {
			t.Fatalf("failed to count rows: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 row, got %d", count)
		}

		cleanupTestTable(ctx, pool, "httpcache_upsert_test")
	})
}

// testDistributedTransactions tests concurrent operations (CockroachDB specialty)
func testDistributedTransactions(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	t.Run("DistributedTransactions", func(t *testing.T) {
		cache := setupTestCache(ctx, t, pool, "httpcache_distributed")
		// Don't close the cache here as it would close the shared pool

		// Simulate concurrent writes from different "nodes"
		done := make(chan bool)
		errors := make(chan error, 5)

		for i := 0; i < 5; i++ {
			go func(n int) {
				key := fmt.Sprintf("distributed-key-%d", n)
				data := []byte(fmt.Sprintf("distributed-data-%d", n))

				// Multiple updates to the same key
				for j := 0; j < 10; j++ {
					cache.Set(key, append(data, byte(j)))
					time.Sleep(10 * time.Millisecond)
				}

				// Verify we can read the data
				_, ok := cache.Get(key)
				if !ok {
					errors <- fmt.Errorf("failed to get key %s", key)
				}

				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 5; i++ {
			<-done
		}

		// Check for errors
		close(errors)
		for err := range errors {
			t.Error(err)
		}

		cleanupTestTable(ctx, pool, "httpcache_distributed")
	})
}

func TestIntegrationCacheKeyPrefix(t *testing.T) {
	ctx := context.Background()

	connString, cleanup := setupPostgreSQLContainer(ctx, t)
	defer cleanup()

	pool := waitForDatabase(ctx, t, connString, 10, 1*time.Second)
	defer pool.Close()

	config := &Config{
		TableName: "httpcache_prefix_test",
		KeyPrefix: "integration:",
		Timeout:   5 * time.Second,
	}

	cache, err := NewWithPool(pool, config)
	if err != nil {
		t.Fatalf(errNewWithPoolFailed, err)
	}
	defer cache.Close()

	if err := cache.CreateTable(ctx); err != nil {
		t.Fatalf(errCreateTableFailed, err)
	}

	// Test key prefix
	testKey := "mykey"
	testData := []byte("test data")

	cache.Set(testKey, testData)

	// Verify the key in database has the prefix
	var key string
	var data []byte
	err = pool.QueryRow(ctx, "SELECT key, data FROM "+config.TableName+" WHERE key = $1", "integration:mykey").Scan(&key, &data)
	if err != nil {
		t.Fatalf("failed to query database: %v", err)
	}

	if key != "integration:mykey" {
		t.Errorf("expected key 'integration:mykey', got '%s'", key)
	}

	if string(data) != string(testData) {
		t.Errorf("expected data '%s', got '%s'", testData, data)
	}

	cleanupTestTable(ctx, pool, config.TableName)
}
