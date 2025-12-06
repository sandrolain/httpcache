# Redis Cache Example

This example demonstrates using httpcache with Redis as a distributed cache backend.

## Prerequisites

You need a running Redis server. The easiest way:

```bash
docker run -d -p 6379:6379 redis
```

Or install Redis locally:

- macOS: `brew install redis && redis-server`
- Linux: `sudo apt-get install redis-server`
- Windows: Use Docker or WSL

## Running the Example

From the project root directory:

```bash
go run ./examples/redis/main.go
```

Or from the examples/redis directory:

```bash
go run main.go
```

## Features Demonstrated

- Connecting to Redis using the official go-redis client
- Using Redis as a cache backend
- Connection pooling with automatic management
- Performance comparison (cache hit vs miss)
- Multiple clients sharing the same cache

## Use Cases

Redis cache is ideal for:

- **Distributed systems** - Multiple instances sharing cache
- **Microservices** - Centralized cache across services
- **High availability** - Redis persistence and replication
- **Scalability** - Redis cluster support
- **TTL support** - Automatic expiration (if configured)

## Production Configuration

```go
import (
    "github.com/redis/go-redis/v9"
    rediscache "github.com/sandrolain/httpcache/redis"
)

// Using the Config-based constructor (recommended)
cache, err := rediscache.New(rediscache.Config{
    Address:      "redis:6379",
    Password:     "password",     // Optional: Redis password
    Username:     "user",         // Optional: Redis 6.0+ ACL username
    DB:           0,              // Database number
    PoolSize:     10,             // Connection pool size
    MinIdleConns: 2,              // Minimum idle connections
    MaxRetries:   3,              // Retry attempts
    DialTimeout:  5 * time.Second,
    ReadTimeout:  3 * time.Second,
    WriteTimeout: 3 * time.Second,
})
if err != nil {
    log.Fatal(err)
}
defer cache.(interface{ Close() error }).Close()

// Or using a custom client
client := redis.NewClient(&redis.Options{
    Addr:     "redis:6379",
    Password: "password",
    DB:       0,
    PoolSize: 100,
})
cache := rediscache.NewWithClient(client)
```

## Important Notes

- The go-redis client includes automatic connection pooling
- Configure appropriate pool size based on your workload
- Enable Redis persistence (RDB/AOF) if you need durability
- Consider Redis Cluster for high availability (use `redis.NewClusterClient`)
- Monitor Redis memory usage and eviction policy
- Cache keys are prefixed with `rediscache:` to avoid collisions
