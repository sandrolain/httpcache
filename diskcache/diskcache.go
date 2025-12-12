// Package diskcache provides an implementation of httpcache.Cache that uses the diskv package
// to supplement an in-memory map with persistent storage
package diskcache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/peterbourgon/diskv"
)

// Cache is an implementation of httpcache.Cache that supplements the in-memory map with persistent storage
type Cache struct {
	d *diskv.Diskv
}

const stalePrefix = "stale_"

// Get returns the response corresponding to key if present.
// The context parameter is accepted for interface compliance but not used for disk operations.
func (c *Cache) Get(_ context.Context, key string) (resp []byte, ok bool, err error) {
	key = keyToFilename(key)
	resp, err = c.d.Read(key)
	if err != nil {
		return nil, false, nil // File not found is not an error, just missing
	}
	return resp, true, nil
}

// Set saves a response to the cache as key.
// The context parameter is accepted for interface compliance but not used for disk operations.
func (c *Cache) Set(_ context.Context, key string, resp []byte) error {
	key = keyToFilename(key)
	// Remove stale marker when setting a fresh value
	_ = c.d.Erase(stalePrefix + key) //nolint:errcheck // file not found is acceptable

	if err := c.d.WriteStream(key, bytes.NewReader(resp), true); err != nil {
		return fmt.Errorf("diskcache set failed for key: %w", err)
	}
	return nil
}

// Delete removes the response with key from the cache.
// The context parameter is accepted for interface compliance but not used for disk operations.
func (c *Cache) Delete(_ context.Context, key string) error {
	key = keyToFilename(key)
	// Erase errors when file doesn't exist are not real errors, so we ignore them
	_ = c.d.Erase(key) //nolint:errcheck // file not found is acceptable
	// Also remove stale marker if it exists
	_ = c.d.Erase(stalePrefix + key) //nolint:errcheck // file not found is acceptable
	return nil
}

// MarkStale marks a cached response as stale instead of deleting it.
// The context parameter is accepted for interface compliance but not used for disk operations.
func (c *Cache) MarkStale(_ context.Context, key string) error {
	key = keyToFilename(key)
	// Check if the entry exists
	if _, err := c.d.Read(key); err != nil {
		return nil // Entry doesn't exist, nothing to mark
	}
	// Create a marker file to indicate staleness
	if err := c.d.WriteStream(stalePrefix+key, bytes.NewReader([]byte("1")), true); err != nil {
		return fmt.Errorf("diskcache mark stale failed for key: %w", err)
	}
	return nil
}

// IsStale checks if a cached response has been marked as stale.
// The context parameter is accepted for interface compliance but not used for disk operations.
func (c *Cache) IsStale(_ context.Context, key string) (bool, error) {
	key = keyToFilename(key)
	_, err := c.d.Read(stalePrefix + key)
	if err != nil {
		return false, nil // Marker doesn't exist
	}
	return true, nil
}

// GetStale retrieves a stale cached response if it exists.
// The context parameter is accepted for interface compliance but not used for disk operations.
func (c *Cache) GetStale(_ context.Context, key string) ([]byte, bool, error) {
	key = keyToFilename(key)
	// Check if marked as stale
	_, err := c.d.Read(stalePrefix + key)
	if err != nil {
		return nil, false, nil // Not marked as stale
	}
	// Retrieve the actual data
	resp, err := c.d.Read(key)
	if err != nil {
		return nil, false, nil // Data doesn't exist
	}
	return resp, true, nil
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
