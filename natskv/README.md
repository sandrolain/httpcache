# NATS JetStream Key/Value Cache Backend

This package provides a NATS JetStream Key/Value store implementation for `httpcache`.

## Features

- HTTP response caching using NATS JetStream Key/Value store
- Automatic TTL management for cache entries
- Two construction methods:
  - `New()`: Manages NATS connection internally (recommended)
  - `NewWithKeyValue()`: For manual connection management
- Thread-safe operations
- Configurable bucket settings

## Installation

```bash
go get github.com/sandrolain/httpcache/natskv
```

## Prerequisites

A running NATS server with JetStream enabled. You can start one using Docker:

```bash
docker run -p 4222:4222 nats:latest -js
```

Or install and run NATS locally:

```bash
# Install NATS server
go install github.com/nats-io/nats-server/v2@latest

# Run with JetStream enabled
nats-server -js
```

## Usage

### Basic Usage with New() (Recommended)

The `New()` constructor manages the NATS connection internally and is the recommended approach for most use cases:

```go
package main

import (
    "context"
    "time"
    
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/natskv"
)

func main() {
    ctx := context.Background()
    
    // Create cache with automatic connection management
    cache, err := natskv.New(ctx, natskv.Config{
        NATSUrl:     "nats://localhost:4222", // Optional, defaults to nats.DefaultURL
        Bucket:      "http-cache",
        Description: "HTTP response cache",
        TTL:         24 * time.Hour, // Cache entries expire after 24 hours
    })
    if err != nil {
        panic(err)
    }
    defer cache.(interface{ Close() error }).Close()
    
    // Use the cache with httpcache
    transport := httpcache.NewTransport(cache)
    client := transport.Client()
    
    resp, err := client.Get("https://example.com")
    // ...
}
```

### Advanced Usage with NewWithKeyValue()

For cases where you need more control over the NATS connection:

```go
package main

import (
    "context"
    "time"
    
    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/natskv"
)

func main() {
    // Connect to NATS manually
    nc, err := nats.Connect(nats.DefaultURL)
    if err != nil {
        panic(err)
    }
    defer nc.Close()
    
    // Create JetStream context
    js, err := jetstream.New(nc)
    if err != nil {
        panic(err)
    }
    
    // Create or update K/V bucket
    ctx := context.Background()
    kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
        Bucket:      "http-cache",
        Description: "HTTP response cache",
        TTL:         24 * time.Hour,
    })
    if err != nil {
        panic(err)
    }
    
    // Create cache with existing KeyValue
    cache := natskv.NewWithKeyValue(kv)
    
    // Use the cache
    transport := httpcache.NewTransport(cache)
    client := transport.Client()
    
    resp, err := client.Get("https://example.com")
    // ...
}
```

## Configuration

The `Config` struct accepts the following fields:

- `NATSUrl` (string): URL of the NATS server. Defaults to `nats.DefaultURL` if empty.
- `Bucket` (string): **Required**. Name of the K/V bucket to use.
- `Description` (string): Optional description for the bucket.
- `TTL` (time.Duration): Time-to-live for cache entries. Zero means no expiration.
- `NATSOptions` ([]nats.Option): Additional NATS connection options.

## Integration Tests

To run integration tests, ensure you have a NATS server running with JetStream enabled:

```bash
# Start NATS server
docker run -p 4222:4222 nats:latest -js

# Run tests
go test -v ./natskv/...
```

## Performance Considerations

- NATS JetStream provides high-performance, distributed caching
- Suitable for microservices architectures
- Supports clustering for high availability
- TTL is managed at the bucket level by NATS

## License

See the main [LICENSE.txt](../LICENSE.txt) file in the repository root.
