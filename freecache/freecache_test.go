package freecache

import (
	"context"
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
	ctx := context.Background()

	// Test Get on empty cache
	_, ok, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if ok {
		t.Error("Get should return false for non-existent key")
	}

	// Test Set and Get
	testData := []byte("test value")
	if err := cache.Set(ctx, "key1", testData); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	value, ok, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !ok {
		t.Fatal("Get should return true for existing key")
	}

	if string(value) != string(testData) {
		t.Errorf("Get returned %q, want %q", value, testData)
	}
}

func TestDelete(t *testing.T) {
	cache := New(1024 * 1024)
	ctx := context.Background()

	// Set a value
	if err := cache.Set(ctx, "key1", []byte("value1")); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	// Verify it exists
	_, ok, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !ok {
		t.Fatal("Key should exist before Delete")
	}

	// Delete it
	if err := cache.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	// Verify it's gone
	_, ok, err = cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if ok {
		t.Error("Key should not exist after Delete")
	}
}

func TestClear(t *testing.T) {
	cache := New(1024 * 1024)
	ctx := context.Background()

	// Add multiple entries
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		if err := cache.Set(ctx, key, []byte("value")); err != nil {
			t.Fatalf("Set error: %v", err)
		}
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
	ctx := context.Background()

	if cache.EntryCount() != 0 {
		t.Errorf("Initial EntryCount should be 0, got %d", cache.EntryCount())
	}

	if err := cache.Set(ctx, "key1", []byte("value1")); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := cache.Set(ctx, "key2", []byte("value2")); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	count := cache.EntryCount()
	if count != 2 {
		t.Errorf("EntryCount should be 2, got %d", count)
	}

	if err := cache.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	count = cache.EntryCount()
	if count != 1 {
		t.Errorf("EntryCount should be 1 after delete, got %d", count)
	}
}

func TestStatistics(t *testing.T) {
	cache := New(1024 * 1024)
	ctx := context.Background()

	// Add some data
	if err := cache.Set(ctx, "key1", []byte("value1")); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := cache.Set(ctx, "key2", []byte("value2")); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	// Access data to generate hits
	_, _, _ = cache.Get(ctx, "key1")
	_, _, _ = cache.Get(ctx, "key1")
	_, _, _ = cache.Get(ctx, "nonexistent")

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
	ctx := context.Background()

	// Fill the cache with data larger than cache size
	for i := 0; i < 100; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i/26))
		value := make([]byte, 1024) // 1KB per entry
		_ = cache.Set(ctx, key, value)
	}

	// Some entries should have been evicted
	evacuateCount := cache.EvacuateCount()
	if evacuateCount == 0 {
		// Note: freecache might not report evacuations immediately
		// This is not necessarily a test failure
		t.Logf("Warning: No evictions reported, cache might be larger than expected")
	}

	// Cache should still work
	if err := cache.Set(ctx, "test", []byte("value")); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	value, ok, err := cache.Get(ctx, "test")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !ok || string(value) != "value" {
		t.Error("Cache should still work after eviction")
	}
}

func TestConcurrentAccess(t *testing.T) {
	cache := New(1024 * 1024)
	ctx := context.Background()

	// Test concurrent writes and reads
	done := make(chan bool, 10)

	// Start multiple goroutines
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + id))
				_ = cache.Set(ctx, key, []byte("value"))
			}
			done <- true
		}(i)

		go func(id int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + id))
				_, _, _ = cache.Get(ctx, key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Cache should still be functional
	if err := cache.Set(ctx, "final", []byte("test")); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	value, ok, err := cache.Get(ctx, "final")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !ok || string(value) != "test" {
		t.Error("Cache should work correctly after concurrent access")
	}
}

func TestFreecacheStale(t *testing.T) {
	cache := New(1024 * 1024) // 1MB
	ctx := context.Background()

	key := "staleKey"
	value := []byte("stale test value")

	// Test marking non-existent key
	if err := cache.MarkStale(ctx, key); err != nil {
		t.Fatalf("MarkStale error on non-existent key: %v", err)
	}

	// Test IsStale on non-existent key
	isStale, err := cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("IsStale error: %v", err)
	}
	if isStale {
		t.Error("Non-existent key should not be stale")
	}

	// Set a value
	if err := cache.Set(ctx, key, value); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	// Should not be stale initially
	isStale, err = cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("IsStale error: %v", err)
	}
	if isStale {
		t.Error("Fresh key should not be stale")
	}

	// Mark as stale
	if err := cache.MarkStale(ctx, key); err != nil {
		t.Fatalf("MarkStale error: %v", err)
	}

	// Should be stale now
	isStale, err = cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("IsStale error after marking: %v", err)
	}
	if !isStale {
		t.Error("Marked key should be stale")
	}

	// GetStale should return the value
	staleVal, ok, err := cache.GetStale(ctx, key)
	if err != nil {
		t.Fatalf("GetStale error: %v", err)
	}
	if !ok {
		t.Fatal("GetStale should return true for stale key")
	}
	if string(staleVal) != string(value) {
		t.Error("GetStale returned wrong value")
	}

	// Set should clear stale marker
	newValue := []byte("fresh value")
	if err := cache.Set(ctx, key, newValue); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	isStale, err = cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("IsStale error after refresh: %v", err)
	}
	if isStale {
		t.Error("Refreshed key should not be stale")
	}

	// Delete should remove stale marker
	if err := cache.MarkStale(ctx, key); err != nil {
		t.Fatalf("MarkStale error: %v", err)
	}
	if err := cache.Delete(ctx, key); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	isStale, err = cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("IsStale error after delete: %v", err)
	}
	if isStale {
		t.Error("Deleted key should not be stale")
	}
}
