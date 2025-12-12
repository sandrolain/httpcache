package test

import (
	"bytes"
	"context"
	"testing"

	"github.com/sandrolain/httpcache"
)

// Cache excercises a httpcache.Cache implementation.
func Cache(t *testing.T, cache httpcache.Cache) {
	ctx := context.Background()
	key := "testKey"
	_, ok, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("error getting key: %v", err)
	}
	if ok {
		t.Fatal("retrieved key before adding it")
	}

	val := []byte("some bytes")
	if err := cache.Set(ctx, key, val); err != nil {
		t.Fatalf("error setting key: %v", err)
	}

	retVal, ok, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("error getting key: %v", err)
	}
	if !ok {
		t.Fatal("could not retrieve an element we just added")
	}
	if !bytes.Equal(retVal, val) {
		t.Fatal("retrieved a different value than what we put in")
	}

	if err := cache.Delete(ctx, key); err != nil {
		t.Fatalf("error deleting key: %v", err)
	}

	_, ok, err = cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("error getting key: %v", err)
	}
	if ok {
		t.Fatal("deleted key still present")
	}
}

// CacheStale exercises the stale marking functionality of a httpcache.Cache implementation.
func CacheStale(t *testing.T, cache httpcache.Cache) {
	ctx := context.Background()
	key := "testStaleKey"
	val := []byte("stale test value")

	// Test 1: Mark non-existent key as stale (should not error)
	if err := cache.MarkStale(ctx, key); err != nil {
		t.Fatalf("error marking non-existent key as stale: %v", err)
	}

	// Test 2: Check that non-existent key is not stale
	isStale, err := cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("error checking if non-existent key is stale: %v", err)
	}
	if isStale {
		t.Fatal("non-existent key reported as stale")
	}

	// Test 3: Set a value
	if err := cache.Set(ctx, key, val); err != nil {
		t.Fatalf("error setting key: %v", err)
	}

	// Test 4: Verify it's not stale initially
	isStale, err = cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("error checking if key is stale: %v", err)
	}
	if isStale {
		t.Fatal("fresh key reported as stale")
	}

	// Test 5: Mark it as stale
	if err := cache.MarkStale(ctx, key); err != nil {
		t.Fatalf("error marking key as stale: %v", err)
	}

	// Test 6: Verify it's now stale
	isStale, err = cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("error checking if key is stale after marking: %v", err)
	}
	if !isStale {
		t.Fatal("marked key not reported as stale")
	}

	// Test 7: GetStale should return the value
	staleVal, ok, err := cache.GetStale(ctx, key)
	if err != nil {
		t.Fatalf("error getting stale value: %v", err)
	}
	if !ok {
		t.Fatal("could not retrieve stale value")
	}
	if !bytes.Equal(staleVal, val) {
		t.Fatal("stale value differs from original")
	}

	// Test 8: Regular Get should still work
	regularVal, ok, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("error getting stale key with Get: %v", err)
	}
	if !ok {
		t.Fatal("could not retrieve stale key with Get")
	}
	if !bytes.Equal(regularVal, val) {
		t.Fatal("Get value differs from original for stale key")
	}

	// Test 9: Set should clear stale marker
	newVal := []byte("fresh value")
	if err := cache.Set(ctx, key, newVal); err != nil {
		t.Fatalf("error setting key again: %v", err)
	}

	isStale, err = cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("error checking if key is stale after refresh: %v", err)
	}
	if isStale {
		t.Fatal("refreshed key still reported as stale")
	}

	// Test 10: GetStale should not return non-stale value
	_, ok, err = cache.GetStale(ctx, key)
	if err != nil {
		t.Fatalf("error calling GetStale on fresh key: %v", err)
	}
	if ok {
		t.Fatal("GetStale returned value for non-stale key")
	}

	// Test 11: Delete should remove both value and stale marker
	if err := cache.MarkStale(ctx, key); err != nil {
		t.Fatalf("error marking key as stale before delete: %v", err)
	}

	if err := cache.Delete(ctx, key); err != nil {
		t.Fatalf("error deleting stale key: %v", err)
	}

	isStale, err = cache.IsStale(ctx, key)
	if err != nil {
		t.Fatalf("error checking if deleted key is stale: %v", err)
	}
	if isStale {
		t.Fatal("deleted key still reported as stale")
	}

	_, ok, err = cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("error getting deleted key: %v", err)
	}
	if ok {
		t.Fatal("deleted key still retrievable")
	}
}
