# PostgreSQL Cache Backend

This package provides a PostgreSQL implementation of the `httpcache.Cache` interface using the `pgx/v5` driver.

## Features

- ✅ **ACID Compliance** - Uses PostgreSQL transactions for reliability
- ✅ **Thread-Safe** - Safe for concurrent access from multiple goroutines
- ✅ **Configurable** - Customizable table name, key prefix, and timeout
- ✅ **Connection Pooling** - Efficient resource usage with pgxpool
- ✅ **Atomic Updates** - UPSERT operations using `ON CONFLICT`
- ✅ **Auto-initialization** - Automatic table creation
- ✅ **Error Logging** - Centralized error logging via httpcache logger

## Installation

```bash
go get github.com/sandrolain/httpcache/postgresql
```

## Quick Start

```go
import (
    "context"
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/postgresql"
)

ctx := context.Background()

// Create cache with default configuration
cache, err := postgresql.New(ctx, "postgres://user:pass@localhost/mydb", nil)
if err != nil {
    log.Fatal(err)
}
defer cache.Close()

// Use with HTTP transport
transport := httpcache.NewTransport(cache)
client := transport.Client()
```

## Configuration

### Default Configuration

```go
config := postgresql.DefaultConfig()
// TableName: "httpcache"
// KeyPrefix: "cache:"
// Timeout:   5 * time.Second
```

### Custom Configuration

```go
config := &postgresql.Config{
    TableName: "my_http_cache",
    KeyPrefix: "api:",
    Timeout:   10 * time.Second,
}

cache, err := postgresql.New(ctx, connString, config)
```

## Usage Examples

### With Automatic Connection Pool

```go
ctx := context.Background()
connString := "postgres://user:pass@localhost:5432/mydb?sslmode=disable"

cache, err := postgresql.New(ctx, connString, nil)
if err != nil {
    log.Fatal(err)
}
defer cache.Close()
```

### With Existing Connection Pool

```go
import "github.com/jackc/pgx/v5/pgxpool"

pool, err := pgxpool.New(ctx, connString)
if err != nil {
    log.Fatal(err)
}

config := postgresql.DefaultConfig()
cache, err := postgresql.NewWithPool(pool, config)
if err != nil {
    log.Fatal(err)
}
```

### With Single Connection

```go
import "github.com/jackc/pgx/v5"

conn, err := pgx.Connect(ctx, connString)
if err != nil {
    log.Fatal(err)
}

cache, err := postgresql.NewWithConn(conn, nil)
if err != nil {
    log.Fatal(err)
}
```

## Database Schema

The cache automatically creates the following table:

```sql
CREATE TABLE IF NOT EXISTS httpcache (
    key TEXT PRIMARY KEY,
    data BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL
)
```

You can customize the table name via configuration.

## Connection String Format

```text
postgres://username:password@host:port/database?options
```

Common options:

- `sslmode=disable` - Disable SSL (for local development)
- `sslmode=require` - Require SSL (for production)
- `pool_max_conns=10` - Maximum pool connections
- `pool_min_conns=2` - Minimum pool connections

Examples:

```go
// Local development
"postgres://postgres:postgres@localhost:5432/httpcache?sslmode=disable"

// Production with SSL
"postgres://user:pass@db.example.com:5432/mydb?sslmode=require"

// With connection pool settings
"postgres://user:pass@localhost/db?pool_max_conns=20&pool_min_conns=5"
```

## Performance Considerations

### Connection Pooling

For production use, the connection pool is automatically configured. You can tune it via the connection string:

```go
connString := "postgres://user:pass@host/db?pool_max_conns=20&pool_min_conns=5"
```

### Timeouts

Operations have a default timeout of 5 seconds. Adjust via configuration:

```go
config := &postgresql.Config{
    Timeout: 30 * time.Second, // For slow networks
}
```

### Indexes

For better performance with large caches, consider adding indexes:

```sql
-- If you query by created_at
CREATE INDEX idx_httpcache_created_at ON httpcache(created_at);
```

## Error Handling

All errors are logged using the centralized httpcache logger:

```go
import "log/slog"

// Set custom logger
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
httpcache.SetLogger(logger)
```

## Thread Safety

The implementation is fully thread-safe and can be used concurrently from multiple goroutines.

## Testing

Tests require a running PostgreSQL server:

```bash
# Set connection string (optional)
export POSTGRESQL_TEST_URL="postgres://postgres:postgres@localhost:5432/httpcache_test?sslmode=disable"

# Run tests
go test -v

# Run benchmarks
go test -bench=.
```

Tests will be skipped if PostgreSQL is not available.

### Integration Tests

Integration tests use Testcontainers to automatically spin up PostgreSQL and CockroachDB instances:

```bash
# Run integration tests (requires Docker)
go test -v -tags=integration

# Run specific integration test
go test -v -tags=integration -run TestPostgreSQLCacheIntegration
```

See [INTEGRATION_TESTS.md](./INTEGRATION_TESTS.md) for more details.

## Cleanup

### Manual Table Cleanup

```go
// Delete all cache entries
_, err := pool.Exec(ctx, "DELETE FROM httpcache")

// Drop the table
_, err := pool.Exec(ctx, "DROP TABLE IF EXISTS httpcache")
```

### TTL Support (Future)

Currently, the cache does not automatically expire old entries. Consider implementing a cleanup job:

```sql
-- Delete entries older than 7 days
DELETE FROM httpcache WHERE created_at < NOW() - INTERVAL '7 days';
```

## Production Recommendations

1. **Use SSL**: Always use `sslmode=require` in production
2. **Connection Pooling**: Let the driver manage the pool automatically
3. **Monitoring**: Monitor cache hit rates and database performance
4. **Cleanup**: Implement periodic cleanup of old cache entries
5. **Backups**: Include cache table in backup strategy (or exclude if ephemeral)
6. **Indexing**: Add indexes based on your access patterns

## Limitations

- No automatic TTL/expiration (implement via cleanup job)
- No built-in cache eviction policy
- Network latency affects performance
- Requires PostgreSQL 9.5+ (for `ON CONFLICT` support)

## See Also

- [Complete Example](../examples/postgresql/)
- [Main Documentation](../)
- [pgx Documentation](https://github.com/jackc/pgx)

## License

MIT License - See [LICENSE.txt](../LICENSE.txt)
