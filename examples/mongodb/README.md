# MongoDB Cache Example

This example demonstrates how to use the MongoDB cache backend with `httpcache`.

## Overview

The MongoDB cache backend provides persistent, distributed caching with automatic expiration support through MongoDB's TTL indexes. It's ideal for scenarios requiring:

- Persistent cache across application restarts
- Shared cache across multiple application instances
- Automatic cache expiration
- High scalability and reliability

## Features

- **Persistent Storage**: Cache survives application restarts
- **Distributed**: Share cache across multiple servers
- **TTL Support**: Automatic expiration using MongoDB TTL indexes
- **Context-Aware**: All operations support context for cancellation and timeouts
- **Configurable**: Full control over connection, database, collection, and timeouts

## Prerequisites

- Go 1.21 or later
- MongoDB 4.0 or later running locally or remotely
- MongoDB Go driver (`go.mongodb.org/mongo-driver/mongo`)

## Setup MongoDB

### Using Docker

```bash
docker run -d \
  --name mongodb \
  -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=root \
  -e MONGO_INITDB_ROOT_PASSWORD=password \
  mongo:8
```

### Using Docker Compose

```yaml
version: '3.8'
services:
  mongodb:
    image: mongo:8
    ports:
      - "27017:27017"
    environment:
      MONGO_INITDB_ROOT_USERNAME: root
      MONGO_INITDB_ROOT_PASSWORD: password
```

### Local Installation

Follow the [MongoDB installation guide](https://docs.mongodb.com/manual/installation/) for your platform.

## Configuration

The MongoDB cache backend uses the `Config` struct for configuration:

```go
config := mongodb.Config{
    URI:        "mongodb://localhost:27017", // MongoDB connection URI
    Database:   "httpcache",                 // Database name
    Collection: "cache",                     // Collection name
    KeyPrefix:  "http:",                     // Optional key prefix
    Timeout:    10 * time.Second,            // Operation timeout
    TTL:        24 * time.Hour,              // Optional TTL for cache entries
}
```

### Configuration Options

- **URI** (required): MongoDB connection string (supports all MongoDB URI options)
- **Database** (required): Name of the database to use
- **Collection** (optional): Collection name (default: "httpcache")
- **KeyPrefix** (optional): Prefix for all cache keys (default: "cache:")
- **Timeout** (optional): Timeout for MongoDB operations (default: 5 seconds)
- **TTL** (optional): Time-to-live for cache entries (MongoDB will auto-expire)

## Running the Example

### With Default Settings (localhost:27017)

```bash
go run main.go
```

### With Custom MongoDB URI

```bash
MONGODB_URI="mongodb://user:pass@localhost:27017/mydb" go run main.go
```

### With Authentication

```bash
MONGODB_URI="mongodb://username:password@localhost:27017/?authSource=admin" go run main.go
```

### With MongoDB Atlas

```bash
MONGODB_URI="mongodb+srv://username:password@cluster.mongodb.net/?retryWrites=true&w=majority" go run main.go
```

## Expected Output

```
Fetching: https://jsonplaceholder.typicode.com/posts/1
Status: 200 OK
Cache: MISS
Body length: 292 bytes

Fetching: https://jsonplaceholder.typicode.com/posts/2
Status: 200 OK
Cache: MISS
Body length: 293 bytes

=== Second round (should hit cache) ===

Fetching: https://jsonplaceholder.typicode.com/posts/1
Status: 200 OK
Cache: HIT
Body length: 292 bytes

Fetching: https://jsonplaceholder.typicode.com/posts/2
Status: 200 OK
Cache: HIT
Body length: 293 bytes
```

## How It Works

1. **Cache Creation**: Creates a MongoDB client and connects to the database
2. **TTL Index**: If TTL is configured, creates a TTL index on the `createdAt` field
3. **Caching**: Stores HTTP responses in MongoDB documents
4. **Retrieval**: Fetches responses from MongoDB using efficient queries
5. **Expiration**: MongoDB automatically removes expired entries based on TTL
6. **Cleanup**: Properly closes the MongoDB connection on shutdown

## MongoDB Document Structure

Cache entries are stored as documents with the following structure:

```json
{
  "_id": "cache:http://example.com/api/data",
  "data": BinData(...),  // Response data as binary
  "createdAt": ISODate("2024-01-01T00:00:00Z")
}
```

## Production Considerations

### Connection Pooling

The MongoDB client automatically manages connection pooling. You can configure pool settings through `ClientOptions`:

```go
clientOptions := options.Client().
    ApplyURI(uri).
    SetMaxPoolSize(100).
    SetMinPoolSize(10)

config := mongodb.Config{
    URI:           uri,
    Database:      "httpcache",
    ClientOptions: clientOptions,
}
```

### Monitoring

Use MongoDB's monitoring and profiling features to track cache performance:

```javascript
### Monitoring

Use MongoDB's monitoring and profiling features to track cache performance:

```javascript
// Enable profiling in MongoDB
db.setProfilingLevel(1, { slowms: 100 })

// View slow operations
db.system.profile.find().sort({ ts: -1 }).limit(10)
```

### Indexes

The MongoDB backend automatically creates necessary indexes:

- Primary index on `_id` (cache key)
- TTL index on `createdAt` (if TTL is configured)

### Scaling

MongoDB's built-in replication and sharding features support horizontal scaling:

- **Replication**: Use replica sets for high availability
- **Sharding**: Shard by cache key for horizontal scaling
- **Read Preference**: Configure read preference for load distribution

## Troubleshooting

### Connection Issues

If you encounter connection problems:

1. Verify MongoDB is running: `mongosh --eval "db.adminCommand('ping')"`
2. Check network connectivity and firewall rules
3. Verify authentication credentials
4. Review MongoDB logs for errors

### Performance Issues

For slow operations:

1. Monitor query performance: `db.cache.explain("executionStats").find({_id: "key"})`
2. Verify indexes are being used
3. Consider adjusting connection pool settings
4. Review MongoDB server resources (CPU, memory, disk)

### TTL Not Working

If TTL expiration isn't working:

1. Verify TTL index exists: `db.cache.getIndexes()`
2. Check MongoDB TTL monitor is running (runs every 60 seconds)
3. Ensure `createdAt` field is a proper ISODate
4. Review MongoDB logs for TTL-related messages

## Advanced Usage

### Custom Client Management

Manage the MongoDB client yourself for fine-grained control:

```go
import (
    "context"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

// Create custom MongoDB client
clientOptions := options.Client().
    ApplyURI("mongodb://localhost:27017").
    SetMaxPoolSize(100)

client, err := mongo.Connect(ctx, clientOptions)
if err != nil {
    log.Fatal(err)
}
defer client.Disconnect(ctx)

// Create cache with custom client
cache := mongodb.NewWithClient(
    client,
    "httpcache",      // database
    "cache",          // collection
    "http:",          // key prefix
    10*time.Second,   // timeout
)
```

### Multiple Databases

Use different databases for different cache purposes:

```go
apiCache, _ := mongodb.New(ctx, mongodb.Config{
    URI:      uri,
    Database: "api_cache",
})

staticCache, _ := mongodb.New(ctx, mongodb.Config{
    URI:      uri,
    Database: "static_cache",
    TTL:      7 * 24 * time.Hour, // 1 week
})
```

## Related Examples

- [Redis Cache](../redis/) - In-memory cache with persistence
- [PostgreSQL Cache](../postgresql/) - SQL-based persistent cache
- [MultiCache](../multicache/) - Multi-tier caching with MongoDB
- [Hazelcast Cache](../hazelcast/) - Distributed in-memory cache

## References

- [MongoDB Go Driver Documentation](https://docs.mongodb.com/drivers/go/current/)
- [MongoDB TTL Indexes](https://docs.mongodb.com/manual/core/index-ttl/)
- [MongoDB Connection String](https://docs.mongodb.com/manual/reference/connection-string/)
- [httpcache Documentation](../../README.md)
