# BlobCache Example

This example demonstrates how to use BlobCache with cloud blob storage (AWS S3, Google Cloud Storage, Azure Blob Storage) or S3-compatible services (MinIO, Ceph, SeaweedFS).

## Prerequisites

Choose one of the following:

### Option 1: AWS S3 (Real Cloud)

- AWS account with S3 access
- Credentials set via environment variables or `~/.aws/credentials`

### Option 2: MinIO (Local S3-Compatible)

- Docker installed
- MinIO running locally

### Option 3: In-Memory (Development/Testing)

- No external dependencies required

## Quick Start

### Using In-Memory Storage (Development)

```bash
cd examples/blobcache
go run main.go
```

This will use `mem://` URL which is perfect for testing without any setup.

### Using MinIO (Local S3-Compatible)

1. Start MinIO with Docker:

```bash
docker run -p 9000:9000 -p 9001:9001 \
  -e "MINIO_ROOT_USER=minioadmin" \
  -e "MINIO_ROOT_PASSWORD=minioadmin" \
  minio/minio server /data --console-address ":9001"
```

2. Create a bucket:

```bash
# Using MinIO Client (mc)
mc alias set local http://localhost:9000 minioadmin minioadmin
mc mb local/http-cache

# Or via web console at http://localhost:9001
```

3. Set environment variables and run:

```bash
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export BUCKET_URL="s3://http-cache?endpoint=http://localhost:9000&s3ForcePathStyle=true&region=us-east-1"
go run main.go
```

### Using Real AWS S3

1. Create an S3 bucket:

```bash
aws s3 mb s3://my-httpcache-bucket --region us-east-1
```

2. Set environment variables and run:

```bash
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key
export BUCKET_URL="s3://my-httpcache-bucket?region=us-east-1"
go run main.go
```

### Using Google Cloud Storage

1. Create a GCS bucket:

```bash
gsutil mb gs://my-httpcache-bucket
```

2. Set service account credentials and run:

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
export BUCKET_URL="gs://my-httpcache-bucket"
go run main.go
```

### Using Azure Blob Storage

1. Create an Azure storage account:

```bash
az storage account create --name myhttpcache --resource-group mygroup
```

2. Set credentials and run:

```bash
export AZURE_STORAGE_ACCOUNT=myhttpcache
export AZURE_STORAGE_KEY=your-storage-key
export BUCKET_URL="azblob://my-container"
go run main.go
```

## What This Example Does

1. Creates a BlobCache instance with the specified storage backend
2. Makes multiple HTTP requests to httpbin.org
3. Demonstrates cache hits and misses
4. Shows cache key prefix usage
5. Properly closes the bucket connection

## Code Walkthrough

```go
import (
    "github.com/sandrolain/httpcache/blobcache"
    _ "gocloud.dev/blob/s3blob"      // For AWS S3
    // _ "gocloud.dev/blob/gcsblob"  // For Google Cloud Storage
    // _ "gocloud.dev/blob/azureblob" // For Azure Blob Storage
    // _ "gocloud.dev/blob/memblob"  // For in-memory (testing)
)

ctx := context.Background()

// Create cache
cache, err := blobcache.New(ctx, blobcache.Config{
    BucketURL: bucketURL,      // Cloud storage URL
    KeyPrefix: "httpcache/",   // Optional prefix for cache keys
    Timeout:   30 * time.Second, // Operation timeout
})
if err != nil {
    log.Fatal(err)
}
defer cache.Close()

// Use with HTTP client
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

## Configuration Options

### BucketURL Formats

```go
// AWS S3
"s3://bucket-name?region=us-east-1"

// AWS S3 with custom endpoint (MinIO, etc.)
"s3://bucket-name?endpoint=http://localhost:9000&s3ForcePathStyle=true&region=us-east-1"

// Google Cloud Storage
"gs://bucket-name"

// Azure Blob Storage
"azblob://container-name"

// In-memory (development/testing)
"mem://"

// Local filesystem (development/testing)
"file:///path/to/cache/directory"
```

### Config Parameters

```go
type Config struct {
    BucketURL string        // Required: Cloud storage URL
    KeyPrefix string        // Optional: Prefix for cache keys (default: "cache/")
    Timeout   time.Duration // Optional: Operation timeout (default: 30s)
    Bucket    *blob.Bucket  // Optional: Pre-opened bucket
}
```

## Features

### Cloud-Agnostic

BlobCache uses [Go Cloud Development Kit](https://gocloud.dev/howto/blob/) to provide a unified API across:

- ✅ AWS S3
- ✅ Google Cloud Storage
- ✅ Azure Blob Storage
- ✅ S3-compatible (MinIO, Ceph, SeaweedFS)
- ✅ In-memory (`mem://`)
- ✅ Filesystem (`file://`)

### Key Hashing

All cache keys are hashed using SHA-256 to:

- Avoid special character issues in cloud storage
- Ensure consistent key format across providers
- Prevent key enumeration attacks

### Timeout Control

Each operation (Get, Set, Delete) uses the configured timeout to prevent hanging operations.

### Graceful Cleanup

The `Close()` method properly closes the bucket connection, but only if the cache owns the bucket (created via `BucketURL`).

## Performance Considerations

- **Latency**: Cloud storage operations are slower than in-memory or local caches (typically 50-200ms)
- **Cost**: Cloud storage has per-operation costs - consider using multi-tier caching
- **Throughput**: Good for infrequent requests, less suitable for high-traffic scenarios
- **Best Practice**: Use as a persistent tier in a MultiCache setup:

```go
import "github.com/sandrolain/httpcache/wrapper/multicache"

localCache := diskcache.New("/tmp/cache")
blobCache, _ := blobcache.New(ctx, config)

multiCache := multicache.New(
    localCache,  // Fast tier (checked first)
    blobCache,   // Persistent tier (fallback)
)
```

## Use Cases

### ✅ Good Use Cases

- Serverless functions (AWS Lambda, Cloud Functions) - shared cache across invocations
- Multi-region deployments - centralized cache storage
- Long-term caching - persist data for days/weeks
- Backup/archive tier - part of multi-tier cache strategy
- CI/CD pipelines - cache between builds
- Multi-cloud applications - vendor-independent storage

### ❌ Not Recommended

- High-frequency requests - latency too high, consider Redis or in-memory
- Real-time applications - use faster backends
- Single-instance apps - disk cache is simpler and faster
- Cost-sensitive scenarios - per-operation costs can add up

## Integration Tests

BlobCache includes comprehensive integration tests using MinIO:

```bash
# Run integration tests
go test -v -tags=integration ./blobcache/... -timeout 5m
```

See [BlobCache Integration Tests](../../blobcache/INTEGRATION_TESTS.md) for more details.

## Troubleshooting

### "unknown query parameter" Error

```
failed to open bucket: unknown query parameter "disableSSL"
```

**Solution**: Use HTTP endpoint without `disableSSL` parameter:

```go
// ❌ Wrong
"s3://bucket?endpoint=http://localhost:9000&disableSSL=true&region=us-east-1"

// ✅ Correct
"s3://bucket?endpoint=http://localhost:9000&s3ForcePathStyle=true&region=us-east-1"
```

### "NoSuchBucket" Error

```
api error NoSuchBucket: The specified bucket does not exist
```

**Solution**: Create the bucket first using AWS CLI, MinIO Client, or cloud console.

### Permission Denied

```
operation error S3: PutObject, AccessDenied
```

**Solution**: Ensure your credentials have read/write permissions on the bucket.

### Slow Performance

**Solution**: BlobCache is inherently slower than in-memory or local caches. Consider:

1. Using MultiCache with memory as the first tier
2. Increasing cache timeouts
3. Using a regional bucket closer to your application
4. Implementing request batching

## Related Examples

- [Basic Example](../basic/) - Simple in-memory caching
- [MultiCache Example](../multicache/) - Multi-tier caching strategy
- [Security Example](../security-best-practices/) - Secure cache wrapper

## Further Reading

- [Go Cloud Development Kit - Blob Storage](https://gocloud.dev/howto/blob/)
- [BlobCache Integration Tests](../../blobcache/INTEGRATION_TESTS.md)
- [Cache Backends Documentation](../../docs/backends.md#blobcache---cloud-storage)
- [AWS S3 Documentation](https://docs.aws.amazon.com/s3/)
- [Google Cloud Storage Documentation](https://cloud.google.com/storage/docs)
- [Azure Blob Storage Documentation](https://docs.microsoft.com/en-us/azure/storage/blobs/)
- [MinIO Documentation](https://min.io/docs/minio/linux/index.html)
