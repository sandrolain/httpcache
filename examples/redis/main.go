package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/sandrolain/httpcache"
	rediscache "github.com/sandrolain/httpcache/redis"
)

func main() {
	fmt.Println("Redis Cache Example")
	fmt.Println("===================")

	// Connect to Redis
	conn, err := redis.Dial("tcp", "localhost:6379")
	if err != nil {
		log.Printf("Failed to connect to Redis: %v\n", err)
		log.Println("Make sure Redis is running on localhost:6379")
		log.Println("You can start Redis with: docker run -d -p 6379:6379 redis")
		return
	}
	defer conn.Close()

	// Test connection
	_, err = conn.Do("PING")
	if err != nil {
		log.Fatal("Redis connection failed:", err)
	}

	fmt.Println("✓ Connected to Redis")

	// Clear any existing cache for this example
	conn.Do("FLUSHDB")
	fmt.Println("✓ Cleared Redis cache")

	// Create Redis cache
	cache := rediscache.NewWithClient(conn)

	// Create HTTP transport with Redis cache
	transport := httpcache.NewTransport(cache)
	client := &http.Client{Transport: transport}

	url := "https://httpbin.org/cache/300"

	// First request
	fmt.Println("Making first request...")
	start := time.Now()
	resp1, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp1.Body.Close()

	body1, _ := io.ReadAll(resp1.Body)
	elapsed1 := time.Since(start)

	fmt.Printf("Status: %s\n", resp1.Status)
	fmt.Printf("From cache: %s\n", resp1.Header.Get(httpcache.XFromCache))
	fmt.Printf("Time: %v\n", elapsed1)
	fmt.Printf("Response length: %d bytes\n\n", len(body1))

	// Second request - should be from Redis cache
	fmt.Println("Making second request (should be cached in Redis)...")
	start = time.Now()
	resp2, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	elapsed2 := time.Since(start)

	fmt.Printf("Status: %s\n", resp2.Status)
	fmt.Printf("From cache: %s\n", resp2.Header.Get(httpcache.XFromCache))
	fmt.Printf("Time: %v\n", elapsed2)
	fmt.Printf("Response length: %d bytes\n\n", len(body2))

	if resp2.Header.Get(httpcache.XFromCache) == "1" {
		fmt.Printf("✓ Redis cache is working!\n")
		fmt.Printf("  First request: %v\n", elapsed1)
		fmt.Printf("  Cached request: %v\n", elapsed2)
		if elapsed2 < elapsed1 {
			fmt.Printf("  Speed improvement: %.1fx faster\n\n", float64(elapsed1)/float64(elapsed2))
		}
	}

	// Check Redis keys
	keys, _ := redis.Strings(conn.Do("KEYS", "rediscache:*"))
	fmt.Printf("Redis has %d cached response(s)\n", len(keys))

	// Example with connection pool (recommended for production)
	fmt.Println("\nExample: Using connection pool (production setup)")
	fmt.Println("=================================================")

	pool := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", "localhost:6379")
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	defer pool.Close()

	// Get a connection from the pool
	poolConn := pool.Get()
	defer poolConn.Close()

	cache2 := rediscache.NewWithClient(poolConn)
	transport2 := httpcache.NewTransport(cache2)
	client2 := &http.Client{Transport: transport2}

	fmt.Println("Making request with pooled connection...")
	resp3, err := client2.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp3.Body.Close()
	io.Copy(io.Discard, resp3.Body)

	fmt.Printf("From cache: %s\n", resp3.Header.Get(httpcache.XFromCache))

	if resp3.Header.Get(httpcache.XFromCache) == "1" {
		fmt.Println("\n✓ Connection pool is working correctly!")
	}

	fmt.Println("\nExample completed successfully!")
}
