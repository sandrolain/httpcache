package httpcache

import "context"

// StaleAwareCache is a wrapper that adds stale marking support to any Cache implementation
// that doesn't natively support it. This is useful for backward compatibility with existing
// cache backends.
type StaleAwareCache struct {
	cache       Cache
	staleMarker Cache // Separate cache to store stale markers
}

// NewStaleAwareCache wraps an existing Cache to add stale marking support.
// It uses a second cache instance to track stale entries.
// The staleMarker parameter must not be nil.
func NewStaleAwareCache(cache Cache, staleMarker Cache) *StaleAwareCache {
	return &StaleAwareCache{
		cache:       cache,
		staleMarker: staleMarker,
	}
}

// Get returns the response corresponding to key if present.
func (s *StaleAwareCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return s.cache.Get(ctx, key)
}

// Set saves a response to the cache as key.
func (s *StaleAwareCache) Set(ctx context.Context, key string, responseBytes []byte) error {
	// Clear stale marker when setting new data
	_ = s.staleMarker.Delete(ctx, key) //nolint:errcheck // best effort
	return s.cache.Set(ctx, key, responseBytes)
}

// Delete removes the value associated with the key.
func (s *StaleAwareCache) Delete(ctx context.Context, key string) error {
	// Delete from both caches
	_ = s.staleMarker.Delete(ctx, key) //nolint:errcheck // best effort
	return s.cache.Delete(ctx, key)
}

// MarkStale marks a cached response as stale instead of deleting it.
func (s *StaleAwareCache) MarkStale(ctx context.Context, key string) error {
	// Check if entry exists in main cache
	_, exists, err := s.cache.Get(ctx, key)
	if err != nil || !exists {
		return err
	}
	// Mark as stale in marker cache
	return s.staleMarker.Set(ctx, key, []byte("1"))
}

// IsStale checks if a cached response has been marked as stale.
func (s *StaleAwareCache) IsStale(ctx context.Context, key string) (bool, error) {
	_, exists, err := s.staleMarker.Get(ctx, key)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// GetStale retrieves a stale cached response if it exists.
func (s *StaleAwareCache) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	// Check if marked as stale
	isStale, err := s.IsStale(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if !isStale {
		return nil, false, nil
	}
	// Return the data from main cache
	return s.cache.Get(ctx, key)
}
