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
