# Basic Example

This example demonstrates the basic usage of httpcache with in-memory caching.

## Features Demonstrated

- Creating an HTTP client with in-memory cache
- Making cacheable requests
- Verifying cache hits with `X-From-Cache` header
- ETag-based cache validation

## Running the Example

From the project root directory:

```bash
go run ./examples/basic/main.go
```

Or from the examples/basic directory:

```bash
go run main.go
```

## Expected Output

The first request will fetch from the server, and the second request will be served from the cache. You'll see the `X-From-Cache: 1` header on cached responses.

## Key Points

- The in-memory cache is fast but not persistent
- Responses are cached according to HTTP caching headers
- The cache automatically handles ETag and Last-Modified validation
- No configuration needed - works out of the box
