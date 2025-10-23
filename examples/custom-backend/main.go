package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/sandrolain/httpcache"
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

func (c *StatsCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value, ok := c.underlying.Get(key)
	if ok {
		c.hits++
	} else {
		c.misses++
	}
	return value, ok
}

func (c *StatsCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sets++
	c.underlying.Set(key, value)
}

func (c *StatsCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deletes++
	c.underlying.Delete(key)
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

func (c *TTLCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(item.expiration) {
		return nil, false
	}

	return item.value, true
}

func (c *TTLCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &cacheItem{
		value:      value,
		expiration: time.Now().Add(c.ttl),
	}
}

func (c *TTLCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
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

	// Example 1: Cache with statistics
	fmt.Println("Example 1: Cache with Statistics")
	fmt.Println("---------------------------------")

	// Wrap the memory cache with stats
	baseCache := httpcache.NewMemoryCache()
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
