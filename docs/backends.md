# Cache Backends

httpcache supports multiple storage backends. Choose the one that fits your use case:

## Built-in Backends

| Backend | Speed | Persistence | Distributed | Use Case |
|---------|-------|-------------|-------------|----------|
| **Memory** | ⚡⚡⚡ Fastest | ❌ No | ❌ No | Development, testing, single-instance apps |
| **[Disk](../diskcache)** | ⚡ Slow | ✅ Yes | ❌ No | Desktop apps, CLI tools |
| **[LevelDB](../leveldbcache)** | ⚡⚡ Fast | ✅ Yes | ❌ No | High-performance local cache |
| **[Redis](../redis)** | ⚡⚡ Fast | ✅ Configurable | ✅ Yes | Microservices, distributed systems |
| **[PostgreSQL](../postgresql)** | ⚡⚡ Fast | ✅ Yes | ✅ Yes | Existing PostgreSQL infrastructure, SQL-based systems |
| **[Memcache](../memcache)** | ⚡⚡ Fast | ❌ No | ✅ Yes | Distributed systems, App Engine |
| **[NATS K/V](../natskv)** | ⚡⚡ Fast | ✅ Configurable | ✅ Yes | NATS-based microservices, JetStream |
| **[Hazelcast](../hazelcast)** | ⚡⚡ Fast | ✅ Yes | ✅ Yes | Enterprise distributed systems, in-memory data grids |
| **[FreeCache](../freecache)** | ⚡⚡⚡ Fastest | ❌ No | ❌ No | High-performance in-memory with zero GC overhead |

## Third-Party Backends

- [`sourcegraph.com/sourcegraph/s3cache`](https://sourcegraph.com/github.com/sourcegraph/s3cache) - Amazon S3 storage
- [`github.com/die-net/lrucache`](https://github.com/die-net/lrucache) - In-memory with LRU eviction
- [`github.com/die-net/lrucache/twotier`](https://github.com/die-net/lrucache/tree/master/twotier) - Multi-tier caching (e.g., memory + disk)
- [`github.com/birkelund/boltdbcache`](https://github.com/birkelund/boltdbcache) - BoltDB implementation

## Related Projects

- [`github.com/moul/hcfilters`](https://github.com/moul/hcfilters) - HTTP cache middleware and filters for advanced cache control

## Usage Examples

### Memory Cache (Default)

```go
transport := httpcache.NewMemoryCacheTransport()
client := transport.Client()
```

**Best for**: Testing, development, single-instance applications

### Disk Cache

```go
import "github.com/sandrolain/httpcache/diskcache"

cache := diskcache.New("/tmp/my-cache")
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Desktop applications, CLI tools that run repeatedly

> ⚠️ **Breaking Change**: The disk cache hashing algorithm has been changed from MD5 to SHA-256 for security reasons. Existing caches created with the original fork (gregjones/httpcache) are **not compatible** and will need to be regenerated.

### Redis Cache

```go
import (
    "github.com/gomodule/redigo/redis"
    rediscache "github.com/sandrolain/httpcache/redis"
)

conn, _ := redis.Dial("tcp", "localhost:6379")
cache := rediscache.NewWithClient(conn)
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Microservices, distributed systems, high availability

### LevelDB Cache

```go
import "github.com/sandrolain/httpcache/leveldbcache"

cache, _ := leveldbcache.New("/path/to/cache")
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: High-performance local caching with persistence

### PostgreSQL Cache

```go
import "github.com/sandrolain/httpcache/postgresql"

ctx := context.Background()
cache, _ := postgresql.New(ctx, "postgres://user:pass@localhost/dbname", nil)
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Applications with existing PostgreSQL infrastructure, SQL-based systems

### NATS K/V Cache

```go
import "github.com/sandrolain/httpcache/natskv"

ctx := context.Background()
cache, _ := natskv.New(ctx, natskv.Config{
    NATSUrl: "nats://localhost:4222",
    Bucket:  "http-cache",
    TTL:     24 * time.Hour,
})
defer cache.(interface{ Close() error }).Close()

transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: NATS-based microservices, JetStream infrastructure, distributed systems with built-in TTL

### Hazelcast Cache

```go
import (
    "github.com/hazelcast/hazelcast-go-client"
    hzcache "github.com/sandrolain/httpcache/hazelcast"
)

ctx := context.Background()
config := hazelcast.Config{}
config.Cluster.Network.SetAddresses("localhost:5701")
client, _ := hazelcast.StartNewClientWithConfig(ctx, config)
defer client.Shutdown(ctx)

cache := hzcache.New(client, "http-cache")
transport := httpcache.NewTransport(cache)
httpClient := &http.Client{Transport: transport}
```

**Best for**: Enterprise distributed systems, in-memory data grids, high availability clusters

### FreeCache

```go
import (
    "github.com/coocood/freecache"
    fcache "github.com/sandrolain/httpcache/freecache"
)

// Create FreeCache with 100MB size
fc := freecache.NewCache(100 * 1024 * 1024)
cache := fcache.NewWithClient(fc)
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: High-performance in-memory caching with zero GC overhead, memory-constrained environments

### Secure Cache Wrapper

Add security to any cache backend with SHA-256 key hashing and optional AES-256-GCM encryption:

```go
import (
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/securecache"
    "github.com/sandrolain/httpcache/redis"
)

// Wrap any backend with security layer
redisCache := redis.NewWithClient(redisConn)
secureCache, _ := securecache.New(securecache.Config{
    Cache:      redisCache,
    Passphrase: "your-secret-passphrase-from-env",
})

transport := httpcache.NewTransport(secureCache)
client := &http.Client{Transport: transport}
```

**Security Features**:

- ✓ **SHA-256 Key Hashing** (always enabled) - Prevents key enumeration
- ✓ **AES-256-GCM Encryption** (optional) - Encrypts cached data when passphrase is provided
- ✓ **Authenticated Encryption** - GCM mode provides both confidentiality and integrity
- ✓ **scrypt Key Derivation** - Strong key derivation from passphrase

**Best for**: User-specific data, PII, authentication tokens, GDPR/CCPA compliance, HIPAA-regulated data, PCI DSS requirements

See [`securecache/README.md`](../securecache/README.md) for details.

### Custom Transport Configuration

```go
// Use a custom underlying transport
transport := httpcache.NewTransport(cache)
transport.Transport = &http.Transport{
    MaxIdleConns:        100,
    IdleConnTimeout:     90 * time.Second,
    DisableCompression:  false,
}
transport.MarkCachedResponses = true // Add X-From-Cache header

client := &http.Client{
    Transport: transport,
    Timeout:   30 * time.Second,
}
```
