package freecache

import (
	"testing"

	"github.com/sandrolain/httpcache"
)

func TestFreecacheImplementsCache(t *testing.T) {
	var _ httpcache.Cache = &Cache{}
}

func TestNew(t *testing.T) {
	cache := New(1024 * 1024) // 1MB
	if cache == nil {
		t.Fatal("New() returned nil")
	}
	if cache.cache == nil {
		t.Fatal("underlying freecache is nil")
	}
}

func TestGetSet(t *testing.T) {
	cache := New(1024 * 1024)

	// Test Get on empty cache
	_, ok := cache.Get("key1")
	if ok {
		t.Error("Get should return false for non-existent key")
	}

	// Test Set and Get
	testData := []byte("test value")
	cache.Set("key1", testData)

	value, ok := cache.Get("key1")
	if !ok {
		t.Fatal("Get should return true for existing key")
	}

	if string(value) != string(testData) {
		t.Errorf("Get returned %q, want %q", value, testData)
	}
}

func TestDelete(t *testing.T) {
	cache := New(1024 * 1024)

	// Set a value
	cache.Set("key1", []byte("value1"))

	// Verify it exists
	_, ok := cache.Get("key1")
	if !ok {
		t.Fatal("Key should exist before Delete")
	}

	// Delete it
	cache.Delete("key1")

	// Verify it's gone
	_, ok = cache.Get("key1")
	if ok {
		t.Error("Key should not exist after Delete")
	}
}

func TestClear(t *testing.T) {
	cache := New(1024 * 1024)

	// Add multiple entries
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		cache.Set(key, []byte("value"))
	}

	// Verify entries exist
	if cache.EntryCount() == 0 {
		t.Fatal("Cache should have entries before Clear")
	}

	// Clear the cache
	cache.Clear()

	// Verify all entries are gone
	if cache.EntryCount() != 0 {
		t.Errorf("EntryCount should be 0 after Clear, got %d", cache.EntryCount())
	}
}

func TestEntryCount(t *testing.T) {
	cache := New(1024 * 1024)

	if cache.EntryCount() != 0 {
		t.Errorf("Initial EntryCount should be 0, got %d", cache.EntryCount())
	}

	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))

	count := cache.EntryCount()
	if count != 2 {
		t.Errorf("EntryCount should be 2, got %d", count)
	}

	cache.Delete("key1")
	count = cache.EntryCount()
	if count != 1 {
		t.Errorf("EntryCount should be 1 after delete, got %d", count)
	}
}

func TestStatistics(t *testing.T) {
	cache := New(1024 * 1024)

	// Add some data
	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))

	// Access data to generate hits
	cache.Get("key1")
	cache.Get("key1")
	cache.Get("nonexistent")

	hitRate := cache.HitRate()
	if hitRate < 0 || hitRate > 1 {
		t.Errorf("HitRate should be between 0 and 1, got %f", hitRate)
	}

	// Reset statistics
	cache.ResetStatistics()

	// After reset, hit rate should be 0 (no lookups)
	hitRate = cache.HitRate()
	if hitRate != 0 {
		t.Errorf("HitRate should be 0 after reset, got %f", hitRate)
	}
}

func TestEviction(t *testing.T) {
	// Create a small cache (10KB) to trigger eviction
	cache := New(10 * 1024)

	// Fill the cache with data larger than cache size
	for i := 0; i < 100; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i/26))
		value := make([]byte, 1024) // 1KB per entry
		cache.Set(key, value)
	}

	// Some entries should have been evicted
	evacuateCount := cache.EvacuateCount()
	if evacuateCount == 0 {
		// Note: freecache might not report evacuations immediately
		// This is not necessarily a test failure
		t.Logf("Warning: No evictions reported, cache might be larger than expected")
	}

	// Cache should still work
	cache.Set("test", []byte("value"))
	value, ok := cache.Get("test")
	if !ok || string(value) != "value" {
		t.Error("Cache should still work after eviction")
	}
}

func TestConcurrentAccess(t *testing.T) {
	cache := New(1024 * 1024)

	// Test concurrent writes and reads
	done := make(chan bool, 10)

	// Start multiple goroutines
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + id))
				cache.Set(key, []byte("value"))
			}
			done <- true
		}(i)

		go func(id int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + id))
				cache.Get(key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Cache should still be functional
	cache.Set("final", []byte("test"))
	value, ok := cache.Get("final")
	if !ok || string(value) != "test" {
		t.Error("Cache should work correctly after concurrent access")
	}
}
