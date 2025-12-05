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
| **[MongoDB](../mongodb)** | ⚡⚡ Fast | ✅ Yes | ✅ Yes | Document-based systems, MongoDB infrastructure, TTL support |
| **[Memcache](../memcache)** | ⚡⚡ Fast | ❌ No | ✅ Yes | Distributed systems, App Engine |
| **[NATS K/V](../natskv)** | ⚡⚡ Fast | ✅ Configurable | ✅ Yes | NATS-based microservices, JetStream |
| **[Hazelcast](../hazelcast)** | ⚡⚡ Fast | ✅ Yes | ✅ Yes | Enterprise distributed systems, in-memory data grids |
| **[FreeCache](../freecache)** | ⚡⚡⚡ Fastest | ❌ No | ❌ No | High-performance in-memory with zero GC overhead |
| **[BlobCache](../blobcache)** | ⚡ Medium | ✅ Yes | ✅ Yes | Cloud storage (S3, GCS, Azure), multi-cloud deployments |

## Third-Party Backends

- [`sourcegraph.com/sourcegraph/s3cache`](https://sourcegraph.com/github.com/sourcegraph/s3cache) - Amazon S3 storage
- [`github.com/die-net/lrucache`](https://github.com/die-net/lrucache) - In-memory with LRU eviction
- [`github.com/die-net/lrucache/twotier`](https://github.com/die-net/lrucache/tree/master/twotier) - Multi-tier caching (e.g., memory + disk)
- [`github.com/birkelund/boltdbcache`](https://github.com/birkelund/boltdbcache) - BoltDB implementation

## Cache Wrappers

### MultiCache - Multi-Tiered Caching

The [`multicache`](../wrapper/multicache/README.md) wrapper allows you to combine multiple cache backends with automatic fallback and promotion:

```go
import "github.com/sandrolain/httpcache/wrapper/multicache"

// Tier 1: Fast in-memory cache
memCache := freecache.New(10 * 1024 * 1024)  // 10 MB

// Tier 2: Medium-speed disk cache
diskCache := diskcache.New("/tmp/cache")

// Tier 3: Persistent distributed cache
redisCache, _ := redis.New("localhost:6379")

// Combine into multi-tier cache
mc := multicache.New(
    memCache,   // Fastest, checked first
    diskCache,  // Medium speed
    redisCache, // Slowest, checked last
)

transport := httpcache.NewTransport(mc)
client := &http.Client{Transport: transport}
```

**How it works:**

- **GET**: Searches tiers in order (fast → slow), promotes found data to faster tiers
- **SET**: Writes to all tiers simultaneously
- **DELETE**: Removes from all tiers for consistency

**Use cases:**

- Performance + Persistence: Memory → Disk → Database
- Local + Distributed: Memory → Redis → PostgreSQL
- CDN-like: Edge → Regional → Origin

See the [MultiCache documentation](../wrapper/multicache/README.md) for details.

### SecureCache - Encryption Wrapper

The [`securecache`](../wrapper/securecache/README.md) wrapper adds security features:

- **Key hashing**: SHA-256 hashing of cache keys (always enabled)
- **Data encryption**: Optional AES-256-GCM encryption with passphrase

See [Security Considerations](./security.md#secure-cache-wrapper) for details.

## Related Projects

- [`github.com/moul/hcfilters`](https://github.com/moul/hcfilters) - HTTP cache middleware and filters for advanced cache control

## Usage Examples

### Disk Cache

```go
import "github.com/sandrolain/httpcache/diskcache"

cache := diskcache.New("/tmp/my-cache")
transport := httpcache.NewTransport(cache)
client := transport.Client()
```

**Best for**: Desktop applications, CLI tools, development

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

### MongoDB Cache

```go
import "github.com/sandrolain/httpcache/mongodb"

ctx := context.Background()
config := mongodb.Config{
    URI:      "mongodb://localhost:27017",
    Database: "httpcache",
    TTL:      24 * time.Hour, // Optional: automatic expiration
}
cache, _ := mongodb.New(ctx, config)
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Document-based systems, MongoDB infrastructure, applications requiring TTL support

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

### BlobCache - Cloud Storage

```go
import (
    "github.com/sandrolain/httpcache/blobcache"
    _ "gocloud.dev/blob/s3blob"      // For AWS S3
    // _ "gocloud.dev/blob/gcsblob"  // For Google Cloud Storage
    // _ "gocloud.dev/blob/azureblob" // For Azure Blob Storage
)

ctx := context.Background()

// AWS S3
cache, _ := blobcache.New(ctx, blobcache.Config{
    BucketURL: "s3://my-bucket?region=us-east-1",
    KeyPrefix: "httpcache/",
    Timeout:   30 * time.Second,
})

// Google Cloud Storage
// cache, _ := blobcache.New(ctx, blobcache.Config{
//     BucketURL: "gs://my-bucket",
//     KeyPrefix: "httpcache/",
// })

// Azure Blob Storage
// cache, _ := blobcache.New(ctx, blobcache.Config{
//     BucketURL: "azblob://my-container",
//     KeyPrefix: "httpcache/",
// })

defer cache.(interface{ Close() error }).Close()

transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Cloud-native applications, multi-cloud deployments, serverless functions, long-term cache storage

**Features**:

- ✓ **Cloud-Agnostic** - Works with AWS S3, Google Cloud Storage, Azure Blob Storage
- ✓ **S3-Compatible** - Supports MinIO, Ceph, SeaweedFS, and other S3-compatible services
- ✓ **SHA-256 Key Hashing** - Ensures compatibility with cloud storage naming restrictions
- ✓ **Local Storage** - Supports `file://` and `mem://` URLs for development/testing

**Authentication**:

Set credentials via environment variables:

```bash
# AWS S3
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key

# Google Cloud Storage
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json

# Azure Blob Storage
export AZURE_STORAGE_ACCOUNT=youraccount
export AZURE_STORAGE_KEY=your-key
```

See [BlobCache Integration Tests](../blobcache/INTEGRATION_TESTS.md) for more examples.

### Secure Cache Wrapper

Add security to any cache backend with SHA-256 key hashing and optional AES-256-GCM encryption:

```go
import (
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/wrapper/securecache"
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

See [`securecache/README.md`](../wrapper/securecache/README.md) for details.

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
