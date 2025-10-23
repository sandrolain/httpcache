// Package diskcache provides an implementation of httpcache.Cache that uses the diskv package
// to supplement an in-memory map with persistent storage
package diskcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/peterbourgon/diskv"
	"github.com/sandrolain/httpcache"
)

// Cache is an implementation of httpcache.Cache that supplements the in-memory map with persistent storage
type Cache struct {
	d *diskv.Diskv
}

// Get returns the response corresponding to key if present
func (c *Cache) Get(key string) (resp []byte, ok bool) {
	key = keyToFilename(key)
	resp, err := c.d.Read(key)
	if err != nil {
		return []byte{}, false
	}
	return resp, true
}

// Set saves a response to the cache as key
func (c *Cache) Set(key string, resp []byte) {
	key = keyToFilename(key)
	if err := c.d.WriteStream(key, bytes.NewReader(resp), true); err != nil {
		httpcache.GetLogger().Warn("failed to write to disk cache", "key", key, "error", err)
	}
}

// Delete removes the response with key from the cache
func (c *Cache) Delete(key string) {
	key = keyToFilename(key)
	if err := c.d.Erase(key); err != nil {
		httpcache.GetLogger().Warn("failed to delete from disk cache", "key", key, "error", err)
	}
}

func keyToFilename(key string) string {
	h := sha256.New()
	// Hash.Write never returns an error according to the interface contract
	//nolint:errcheck // io.WriteString to hash.Hash never fails
	_, _ = io.WriteString(h, key)
	return hex.EncodeToString(h.Sum(nil))
}

// New returns a new Cache that will store files in basePath
func New(basePath string) *Cache {
	return &Cache{
		d: diskv.New(diskv.Options{
			BasePath:     basePath,
			CacheSizeMax: 100 * 1024 * 1024, // 100MB
		}),
	}
}

// NewWithDiskv returns a new Cache using the provided Diskv as underlying
// storage.
func NewWithDiskv(d *diskv.Diskv) *Cache {
	return &Cache{d}
}
