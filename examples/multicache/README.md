# MultiCache Example

This example demonstrates how to use the `multicache` wrapper to create a multi-tiered caching strategy with automatic fallback and promotion.

## Scenario

In this example, we set up a three-tier cache system:

1. **Tier 1 - Memory Cache**: Fast, small capacity, volatile
2. **Tier 2 - Disk Cache**: Medium speed, larger capacity, persistent across restarts
3. **Tier 3 - Redis Cache**: Network-based, largest capacity, shared across instances

## How It Works

- **GET operations**: Search from fastest to slowest tier. When found in a slower tier, automatically promote to all faster tiers.
- **SET operations**: Write to all tiers simultaneously.
- **DELETE operations**: Remove from all tiers to maintain consistency.

## Benefits

- **Performance**: Hot data naturally migrates to faster tiers
- **Resilience**: Data persists in slower tiers even if faster caches are cleared
- **Scalability**: Different tiers can have different eviction policies
- **Flexibility**: Add or remove tiers based on your needs

## Running the Example

```bash
# Make sure Redis is running (if you want to test with Redis tier)
docker run -d -p 6379:6379 redis:alpine

# Run the example
go run main.go
```

## Expected Output

You'll see:

1. Initial write to all tiers
2. Fast reads from tier 1 (memory)
3. Automatic promotion when tier 1 is cleared
4. Fallback to tier 3 when tier 1 and 2 are cleared
