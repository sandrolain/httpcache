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
	if err := cache.Set(ctx, benchmarkKey, testData); err != nil {
		b.Fatalf("Set failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = cache.Get(ctx, benchmarkKey)
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
		_ = cache.Set(ctx, benchmarkKey, testData)
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
		_ = cache.Set(ctx, benchmarkKey, testData)
		b.StartTimer()
		_ = cache.Delete(ctx, benchmarkKey)
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
		_ = cache.Set(ctx, benchmarkKey, testData)
		_, _, _ = cache.Get(ctx, benchmarkKey)
		_ = cache.Delete(ctx, benchmarkKey)
	}

	// Clean up
	_, _ = pool.Exec(ctx, queryDropTableIfExists+config.TableName)
}
