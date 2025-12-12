// Package leveldbcache provides an implementation of httpcache.Cache that
// uses github.com/syndtr/goleveldb/leveldb
package leveldbcache

import (
	"context"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
)

// Cache is an implementation of httpcache.Cache with leveldb storage
type Cache struct {
	db *leveldb.DB
}

const stalePrefix = "stale:"

// Get returns the response corresponding to key if present.
// The context parameter is accepted for interface compliance but not used for LevelDB operations.
func (c *Cache) Get(_ context.Context, key string) (resp []byte, ok bool, err error) {
	resp, err = c.db.Get([]byte(key), nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	return resp, true, nil
}

// Set saves a response to the cache as key.
// The context parameter is accepted for interface compliance but not used for LevelDB operations.
func (c *Cache) Set(_ context.Context, key string, resp []byte) error {
	// Remove stale marker when setting a fresh value
	batch := new(leveldb.Batch)
	batch.Put([]byte(key), resp)
	batch.Delete([]byte(stalePrefix + key))

	if err := c.db.Write(batch, nil); err != nil {
		return fmt.Errorf("leveldb cache set failed for key %q: %w", key, err)
	}
	return nil
}

// Delete removes the response with key from the cache.
// The context parameter is accepted for interface compliance but not used for LevelDB operations.
func (c *Cache) Delete(_ context.Context, key string) error {
	batch := new(leveldb.Batch)
	batch.Delete([]byte(key))
	batch.Delete([]byte(stalePrefix + key))
	if err := c.db.Write(batch, nil); err != nil {
		return fmt.Errorf("leveldb cache delete failed for key %q: %w", key, err)
	}
	return nil
}

// MarkStale marks a cached response as stale instead of deleting it.
// The context parameter is accepted for interface compliance but not used for LevelDB operations.
func (c *Cache) MarkStale(_ context.Context, key string) error {
	// Check if entry exists
	_, err := c.db.Get([]byte(key), nil)
	if err == leveldb.ErrNotFound {
		return nil // Entry doesn't exist, nothing to mark
	}
	if err != nil {
		return fmt.Errorf("leveldb cache mark stale check failed for key %q: %w", key, err)
	}

	// Set marker
	if err := c.db.Put([]byte(stalePrefix+key), []byte("1"), nil); err != nil {
		return fmt.Errorf("leveldb cache mark stale failed for key %q: %w", key, err)
	}
	return nil
}

// IsStale checks if a cached response has been marked as stale.
// The context parameter is accepted for interface compliance but not used for LevelDB operations.
func (c *Cache) IsStale(_ context.Context, key string) (bool, error) {
	_, err := c.db.Get([]byte(stalePrefix+key), nil)
	if err == leveldb.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("leveldb cache is stale check failed for key %q: %w", key, err)
	}
	return true, nil
}

// GetStale retrieves a stale cached response if it exists.
// The context parameter is accepted for interface compliance but not used for LevelDB operations.
func (c *Cache) GetStale(_ context.Context, key string) ([]byte, bool, error) {
	// Check if marked as stale
	isStale, err := c.IsStale(context.Background(), key)
	if err != nil {
		return nil, false, err
	}
	if !isStale {
		return nil, false, nil
	}

	// Retrieve the actual data
	return c.Get(context.Background(), key)
}

// New returns a new Cache that will store leveldb in path
func New(path string) (*Cache, error) {
	cache := &Cache{}

	var err error
	cache.db, err = leveldb.OpenFile(path, nil)

	if err != nil {
		return nil, err
	}
	return cache, nil
}

// NewWithDB returns a new Cache using the provided leveldb as underlying
// storage.
func NewWithDB(db *leveldb.DB) *Cache {
	return &Cache{db}
}
