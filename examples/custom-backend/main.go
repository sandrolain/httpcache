package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

// StatsCache wraps any cache implementation and adds statistics
type StatsCache struct {
	underlying httpcache.Cache
	mu         sync.RWMutex
	hits       int64
	misses     int64
	sets       int64
	deletes    int64
}

// NewStatsCache creates a new cache with statistics tracking
func NewStatsCache(underlying httpcache.Cache) *StatsCache {
	return &StatsCache{
		underlying: underlying,
	}
}

func (c *StatsCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value, ok, err := c.underlying.Get(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if ok {
		c.hits++
	} else {
		c.misses++
	}
	return value, ok, nil
}

func (c *StatsCache) Set(ctx context.Context, key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sets++
	return c.underlying.Set(ctx, key, value)
}

func (c *StatsCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deletes++
	return c.underlying.Delete(ctx, key)
}

func (c *StatsCache) MarkStale(ctx context.Context, key string) error {
	return c.underlying.MarkStale(ctx, key)
}

func (c *StatsCache) IsStale(ctx context.Context, key string) (bool, error) {
	return c.underlying.IsStale(ctx, key)
}

func (c *StatsCache) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	return c.underlying.GetStale(ctx, key)
}

// Stats returns current cache statistics
func (c *StatsCache) Stats() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]int64{
		"hits":    c.hits,
		"misses":  c.misses,
		"sets":    c.sets,
		"deletes": c.deletes,
	}
}

// HitRate returns the cache hit rate as a percentage
func (c *StatsCache) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total) * 100
}

// TTLCache implements a cache with automatic expiration
type TTLCache struct {
	mu      sync.RWMutex
	items   map[string]*cacheItem
	ttl     time.Duration
	cleanup *time.Ticker
}

type cacheItem struct {
	value      []byte
	expiration time.Time
	stale      bool
}

// NewTTLCache creates a new cache with TTL support
func NewTTLCache(ttl time.Duration) *TTLCache {
	c := &TTLCache{
		items:   make(map[string]*cacheItem),
		ttl:     ttl,
		cleanup: time.NewTicker(ttl),
	}

	// Start cleanup goroutine
	go c.cleanupExpired()

	return c
}

func (c *TTLCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, false, nil
	}

	// Check if expired
	if time.Now().After(item.expiration) {
		return nil, false, nil
	}

	return item.value, true, nil
}

func (c *TTLCache) Set(_ context.Context, key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &cacheItem{
		value:      value,
		expiration: time.Now().Add(c.ttl),
	}
	return nil
}

func (c *TTLCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
	return nil
}

func (c *TTLCache) MarkStale(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, ok := c.items[key]; ok {
		item.stale = true
	}
	return nil
}

func (c *TTLCache) IsStale(_ context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if item, ok := c.items[key]; ok {
		return item.stale, nil
	}
	return false, nil
}

func (c *TTLCache) GetStale(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok || !item.stale {
		return nil, false, nil
	}

	// Check if expired
	if time.Now().After(item.expiration) {
		return nil, false, nil
	}

	return item.value, true, nil
}

func (c *TTLCache) cleanupExpired() {
	for range c.cleanup.C {
		c.mu.Lock()
		now := time.Now()
		for key, item := range c.items {
			if now.After(item.expiration) {
				delete(c.items, key)
			}
		}
		c.mu.Unlock()
	}
}

func (c *TTLCache) Stop() {
	c.cleanup.Stop()
}

func main() {
	fmt.Println("Custom Cache Backends Example")
	fmt.Println("==============================")

	// Create a temporary directory for disk cache
	tmpDir, err := os.MkdirTemp("", "httpcache-custom-backend-*")
	if err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Example 1: Cache with statistics
	fmt.Println("Example 1: Cache with Statistics")
	fmt.Println("---------------------------------")

	// Create disk cache as the underlying cache
	baseCache := diskcache.New(tmpDir)

	// Wrap with stats
	statsCache := NewStatsCache(baseCache)

	transport1 := httpcache.NewTransport(statsCache)
	client1 := &http.Client{Transport: transport1}

	urls := []string{
		"https://httpbin.org/cache/300",
		"https://httpbin.org/cache/300", // Same URL - should be cached
		"https://httpbin.org/delay/1",
		"https://httpbin.org/cache/300", // Same as first - should be cached
	}

	for i, url := range urls {
		fmt.Printf("%d. Fetching %s\n", i+1, url)
		resp, err := client1.Get(url)
		if err != nil {
			log.Printf("Error: %v\n", err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.Header.Get(httpcache.XFromCache) == "1" {
			fmt.Println("   ↳ Cache HIT")
		} else {
			fmt.Println("   ↳ Cache MISS")
		}
	}

	// Print statistics
	stats := statsCache.Stats()
	fmt.Println("\nCache Statistics:")
	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(statsJSON))
	fmt.Printf("Hit Rate: %.1f%%\n", statsCache.HitRate())

	// Example 2: Cache with TTL
	fmt.Println("\n\nExample 2: Cache with TTL (Time-To-Live)")
	fmt.Println("-----------------------------------------")

	ttlCache := NewTTLCache(3 * time.Second)
	defer ttlCache.Stop()

	transport2 := httpcache.NewTransport(ttlCache)
	client2 := &http.Client{Transport: transport2}

	url := "https://httpbin.org/uuid"

	// First request
	fmt.Println("Making first request...")
	resp1, _ := client2.Get(url)
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	fmt.Printf("UUID: %s\n", string(body1))

	// Immediate second request - should be cached
	fmt.Println("\nImmediate second request (should be cached)...")
	resp2, _ := client2.Get(url)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("UUID: %s\n", string(body2))
	fmt.Printf("From cache: %s\n", resp2.Header.Get(httpcache.XFromCache))

	// Wait for TTL to expire
	fmt.Println("\nWaiting 4 seconds for TTL to expire...")
	time.Sleep(4 * time.Second)

	// Third request - cache expired, should fetch new
	fmt.Println("Request after TTL expiration (should be fresh)...")
	resp3, _ := client2.Get(url)
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	fmt.Printf("UUID: %s\n", string(body3))
	fmt.Printf("From cache: %s\n", resp3.Header.Get(httpcache.XFromCache))

	if string(body1) == string(body2) && string(body1) != string(body3) {
		fmt.Println("\n✓ TTL cache is working correctly!")
		fmt.Println("  - Second request returned cached value")
		fmt.Println("  - Third request (after TTL) returned new value")
	}

	fmt.Println("\nExample completed successfully!")
	fmt.Println("\nNote: You can implement any custom cache backend by")
	fmt.Println("implementing the httpcache.Cache interface (Get, Set, Delete).")
}
