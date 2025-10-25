package postgresql

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	benchmarkKey            = "benchmark-key"
	benchmarkData           = "benchmark data content"
	benchmarkTableName      = "httpcache_bench"
	errSkipBenchmarkConnect = "skipping benchmark; could not connect to PostgreSQL: %v"
)

func BenchmarkPostgreSQLCacheGet(b *testing.B) {
	ctx := context.Background()
	connString := getTestConnString()

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		b.Skipf(errSkipBenchmarkConnect, err)
	}
	defer pool.Close()

	config := DefaultConfig()
	config.TableName = benchmarkTableName

	cache, err := NewWithPool(pool, config)
	if err != nil {
		b.Fatalf(errNewWithPoolFailed, err)
	}
	defer cache.Close()

	if err := cache.CreateTable(ctx); err != nil {
		b.Fatalf(errCreateTableFailed, err)
	}

	// Setup test data
	testData := []byte(benchmarkData)
	cache.Set(benchmarkKey, testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(benchmarkKey)
	}

	// Clean up
	_, _ = pool.Exec(ctx, queryDropTableIfExists+config.TableName)
}

func BenchmarkPostgreSQLCacheSet(b *testing.B) {
	ctx := context.Background()
	connString := getTestConnString()

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		b.Skipf(errSkipBenchmarkConnect, err)
	}
	defer pool.Close()

	config := DefaultConfig()
	config.TableName = benchmarkTableName

	cache, err := NewWithPool(pool, config)
	if err != nil {
		b.Fatalf(errNewWithPoolFailed, err)
	}
	defer cache.Close()

	if err := cache.CreateTable(ctx); err != nil {
		b.Fatalf(errCreateTableFailed, err)
	}

	testData := []byte(benchmarkData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(benchmarkKey, testData)
	}

	// Clean up
	_, _ = pool.Exec(ctx, queryDropTableIfExists+config.TableName)
}

func BenchmarkPostgreSQLCacheDelete(b *testing.B) {
	ctx := context.Background()
	connString := getTestConnString()

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		b.Skipf(errSkipBenchmarkConnect, err)
	}
	defer pool.Close()

	config := DefaultConfig()
	config.TableName = benchmarkTableName

	cache, err := NewWithPool(pool, config)
	if err != nil {
		b.Fatalf(errNewWithPoolFailed, err)
	}
	defer cache.Close()

	if err := cache.CreateTable(ctx); err != nil {
		b.Fatalf(errCreateTableFailed, err)
	}

	testData := []byte(benchmarkData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		cache.Set(benchmarkKey, testData)
		b.StartTimer()
		cache.Delete(benchmarkKey)
	}

	// Clean up
	_, _ = pool.Exec(ctx, queryDropTableIfExists+config.TableName)
}

func BenchmarkPostgreSQLCacheGetSetDelete(b *testing.B) {
	ctx := context.Background()
	connString := getTestConnString()

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		b.Skipf(errSkipBenchmarkConnect, err)
	}
	defer pool.Close()

	config := DefaultConfig()
	config.TableName = benchmarkTableName

	cache, err := NewWithPool(pool, config)
	if err != nil {
		b.Fatalf(errNewWithPoolFailed, err)
	}
	defer cache.Close()

	if err := cache.CreateTable(ctx); err != nil {
		b.Fatalf(errCreateTableFailed, err)
	}

	testData := []byte(benchmarkData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(benchmarkKey, testData)
		cache.Get(benchmarkKey)
		cache.Delete(benchmarkKey)
	}

	// Clean up
	_, _ = pool.Exec(ctx, queryDropTableIfExists+config.TableName)
}
