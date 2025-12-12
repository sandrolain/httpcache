package leveldbcache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandrolain/httpcache/test"
)

func TestDiskCache(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "httpcache")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	cache, err := New(filepath.Join(tempDir, "db"))
	if err != nil {
		t.Fatalf("New leveldb,: %v", err)
	}

	test.Cache(t, cache)
}

func TestDiskCacheStale(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "httpcache")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	cache, err := New(filepath.Join(tempDir, "db"))
	if err != nil {
		t.Fatalf("New leveldb: %v", err)
	}

	test.CacheStale(t, cache)
}
