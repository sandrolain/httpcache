# Disk Cache Example

This example demonstrates using httpcache with persistent disk storage.

## Features Demonstrated

- Creating a disk-based cache
- Persistent storage across application restarts
- Cache directory management
- Multiple clients sharing the same cache

## Running the Example

From the project root directory:

```bash
go run ./examples/diskcache/main.go
```

Or from the examples/diskcache directory:

```bash
go run main.go
```

## Use Cases

Disk cache is ideal for:

- **Desktop applications** - Cache API responses between runs
- **CLI tools** - Speed up repeated commands
- **Long-running services** - Persist cache across restarts
- **Large datasets** - When memory cache would be too large

## Configuration

```go
cache := diskcache.New("/path/to/cache/dir")
transport := httpcache.NewTransport(cache)
```

The disk cache uses the [diskv](https://github.com/peterbourgon/diskv) library, which provides:

- Efficient key-value storage
- Automatic directory structure
- Built-in compression support (optional)

## Important Notes

- Cache directory must be writable
- Files are stored with MD5-hashed filenames
- No automatic cleanup - consider implementing TTL or size limits
- Thread-safe for concurrent access
