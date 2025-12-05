// Package leveldbcache provides an implementation of httpcache.Cache that
// uses github.com/syndtr/goleveldb/leveldb
package leveldbcache

import (
	"context"

	"github.com/sandrolain/httpcache"
	"github.com/syndtr/goleveldb/leveldb"
)

// Cache is an implementation of httpcache.Cache with leveldb storage
type Cache struct {
	db *leveldb.DB
}

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
	if err := c.db.Put([]byte(key), resp, nil); err != nil {
		httpcache.GetLogger().Warn("failed to write to leveldb cache", "key", key, "error", err)
		return err
	}
	return nil
}

// Delete removes the response with key from the cache.
// The context parameter is accepted for interface compliance but not used for LevelDB operations.
func (c *Cache) Delete(_ context.Context, key string) error {
	if err := c.db.Delete([]byte(key), nil); err != nil {
		httpcache.GetLogger().Warn("failed to delete from leveldb cache", "key", key, "error", err)
		return err
	}
	return nil
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
