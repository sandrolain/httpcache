# Hazelcast Cache Example

This example demonstrates how to use Hazelcast as a cache backend for httpcache.

## Prerequisites

- Hazelcast server running on localhost:5701

## Running the Example

First, start a Hazelcast server:

```bash
docker run -p 5701:5701 hazelcast/hazelcast:5.5
```

Then run the example:

```bash
go run main.go
```

## What This Example Does

1. Connects to a local Hazelcast server
2. Creates a distributed map for caching
3. Makes HTTP requests through the cache
4. Demonstrates cache hits on subsequent requests

## Configuration

The example connects to Hazelcast at `localhost:5701`. You can modify this in the code if your Hazelcast server is running elsewhere.

## Features

- **Distributed caching**: Share cache across multiple application instances
- **In-memory performance**: Fast access to cached data
- **Scalability**: Hazelcast automatically distributes data across cluster nodes
- **High availability**: Data is replicated for fault tolerance
