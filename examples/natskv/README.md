# NATS K/V Cache Example

This example demonstrates how to use the NATS JetStream Key/Value store as a cache backend for httpcache.

## Prerequisites

- NATS server with JetStream enabled

## Running the Example

First, start a NATS server with JetStream:

```bash
docker run -p 4222:4222 nats:latest -js
```

Then run the example:

```bash
go run main.go
```

## What This Example Does

1. Connects to a local NATS server
2. Creates a JetStream Key/Value bucket for caching
3. Makes HTTP requests through the cache
4. Demonstrates cache hits on subsequent requests

## Configuration

The example connects to NATS at `nats://localhost:4222`. You can modify this in the code if your NATS server is running elsewhere.
