# LevelDB Cache Example

This example demonstrates using httpcache with LevelDB for high-performance persistent storage.

## Features Demonstrated

- Creating a LevelDB-based cache
- Fast persistent storage
- Caching multiple URLs
- Cache size monitoring

## Running the Example

From the project root directory:

```bash
go run ./examples/leveldb/main.go
```

Or from the examples/leveldb directory:

```bash
go run main.go
```

## Use Cases

LevelDB cache is ideal for:

- **High-performance applications** - Faster than regular disk cache
- **Embedded systems** - No external dependencies
- **Desktop applications** - Local persistent cache
- **CLI tools** - Quick startup with cached data
- **Offline-first apps** - Reliable local storage

## Advantages

- **Fast**: Much faster than disk-based cache
- **Persistent**: Survives application restarts
- **Embedded**: No external server needed
- **Compact**: Efficient storage with compression
- **Reliable**: Battle-tested database engine

## Configuration

```go
cache, err := leveldbcache.New("/path/to/leveldb")
if err != nil {
    log.Fatal(err)
}
transport := httpcache.NewTransport(cache)
```

## Performance Characteristics

- Read latency: ~100µs (much faster than disk cache ~1ms)
- Write latency: Similar to disk cache
- Storage: Compressed, efficient space usage
- Concurrent access: Thread-safe, good concurrent performance

## Important Notes

- LevelDB directory must be exclusive to one process
- Not suitable for multiple processes accessing same DB
- Consider periodic compaction for long-running applications
- Database is closed when cache is garbage collected
