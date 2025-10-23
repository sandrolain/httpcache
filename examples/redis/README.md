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

- Connecting to Redis
- Using Redis as a cache backend
- Connection pooling for production use
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
pool := &redis.Pool{
    MaxIdle:     10,
    MaxActive:   100,
    IdleTimeout: 240 * time.Second,
    Wait:        true,
    Dial: func() (redis.Conn, error) {
        c, err := redis.Dial("tcp", "redis:6379")
        if err != nil {
            return nil, err
        }
        // Optional: Authentication
        // _, err = c.Do("AUTH", "password")
        return c, err
    },
    TestOnBorrow: func(c redis.Conn, t time.Time) error {
        if time.Since(t) < time.Minute {
            return nil
        }
        _, err := c.Do("PING")
        return err
    },
}
```

## Important Notes

- Always use a connection pool in production
- Configure appropriate pool size based on your workload
- Enable Redis persistence (RDB/AOF) if you need durability
- Consider Redis Cluster for high availability
- Monitor Redis memory usage and eviction policy
- Cache keys are prefixed with `rediscache:` to avoid collisions
