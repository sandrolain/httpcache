# PostgreSQL Cache Example

This example demonstrates how to use the PostgreSQL backend for HTTP caching.

## Prerequisites

- Go 1.18 or later
- PostgreSQL server running and accessible
- Database created for the cache

## Setup

1. Start PostgreSQL server (if not already running):

   ```bash
   # Using Docker
   docker run --name postgres-cache -e POSTGRES_PASSWORD=postgres -p 5432:5432 -d postgres
   
   # Or using Homebrew on macOS
   brew services start postgresql
   ```

2. Create a database for the cache:

   ```bash
   createdb httpcache
   
   # Or using psql
   psql -U postgres -c "CREATE DATABASE httpcache;"
   ```

## Running the Example

```bash
go run main.go
```

The example will:

1. Connect to PostgreSQL using the connection string
2. Create the cache table automatically
3. Make an HTTP request to GitHub API
4. Cache the response in PostgreSQL
5. Make the same request again (served from cache)

## Configuration

You can customize the cache configuration:

```go
config := &postgresql.Config{
    TableName: "my_http_cache",  // Custom table name
    KeyPrefix: "api:",            // Prefix for cache keys
    Timeout:   10 * time.Second,  // Database operation timeout
}

cache, err := postgresql.New(ctx, connString, config)
```

## Connection String Format

```text
postgres://username:password@host:port/database?sslmode=disable
```

Examples:

- Local: `postgres://postgres:postgres@localhost:5432/httpcache?sslmode=disable`
- Remote: `postgres://user:pass@example.com:5432/mydb?sslmode=require`

## Using with Connection Pool

For production use, you can create your own connection pool:

```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/sandrolain/httpcache/postgresql"
)

pool, err := pgxpool.New(ctx, connString)
if err != nil {
    log.Fatal(err)
}
defer pool.Close()

config := postgresql.DefaultConfig()
cache, err := postgresql.NewWithPool(pool, config)
if err != nil {
    log.Fatal(err)
}
defer cache.Close()
```

## Cleaning Up

To remove the cache table:

```bash
psql -U postgres -d httpcache -c "DROP TABLE IF EXISTS my_http_cache;"
```

Or from within the application, you can delete individual cache entries using the `Delete` method.
