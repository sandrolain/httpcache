// Package multicache provides a multi-tiered cache implementation that allows
// cascading through multiple cache backends with automatic fallback and promotion.
// This enables sophisticated caching strategies with different performance and
// persistence characteristics at each tier.
package multicache

import (
	"context"

	httpcache "github.com/sandrolain/httpcache"
)

// MultiCache implements a multi-tiered caching strategy where cache tiers are
// ordered from fastest/smallest (first) to slowest/largest (last). On reads,
// it searches each tier in order and promotes found values to faster tiers.
// On writes, it stores to all tiers. This allows hot data to naturally migrate
// to faster caches while maintaining persistence in slower tiers.
//
// Example use case:
//   - Tier 1: In-memory LRU (fast, small, volatile)
//   - Tier 2: Redis (medium speed, larger, persistent)
//   - Tier 3: PostgreSQL (slower, largest, highly persistent)
type MultiCache struct {
	tiers []httpcache.Cache
}

// New creates a MultiCache with the specified cache tiers.
// Tiers should be ordered from fastest/smallest to slowest/largest.
// At least one tier must be provided, and all tiers must be non-nil and unique.
//
// Returns nil if:
//   - No tiers are provided
//   - Any tier is nil
//   - Duplicate tiers are detected
func New(tiers ...httpcache.Cache) *MultiCache {
	if len(tiers) == 0 {
		return nil
	}

	// Validate all tiers are non-nil and unique
	seen := make(map[httpcache.Cache]bool)
	for _, tier := range tiers {
		if tier == nil {
			return nil
		}
		if seen[tier] {
			return nil
		}
		seen[tier] = true
	}

	return &MultiCache{
		tiers: tiers,
	}
}

// Get returns the cached value for the given key. It searches each tier in order,
// starting with the fastest. When a value is found in a slower tier, it is
// automatically promoted (written) to all faster tiers for subsequent quick access.
//
// Returns the cached value, true if found, and nil error on success.
// Returns nil, false, nil if not found in any tier.
// Returns nil, false, error if any tier returns an error during lookup.
func (c *MultiCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	// Try each tier in order
	for i, tier := range c.tiers {
		value, ok, err := tier.Get(ctx, key)
		if err != nil {
			return nil, false, err
		}
		if ok {
			// Found in this tier - promote to all faster tiers
			// Promotion errors are silently ignored as the value was found successfully
			_ = c.promoteToFasterTiers(ctx, key, value, i) //nolint:errcheck // promotion is best-effort
			return value, true, nil
		}
	}

	return nil, false, nil
}

// Set stores the value in all cache tiers. This ensures consistency across
// all levels and allows each tier to apply its own eviction policies independently.
// Returns an error if any tier fails to store the value.
func (c *MultiCache) Set(ctx context.Context, key string, value []byte) error {
	for _, tier := range c.tiers {
		if err := tier.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the value from all cache tiers to maintain consistency.
// Returns an error if any tier fails to delete the value.
func (c *MultiCache) Delete(ctx context.Context, key string) error {
	for _, tier := range c.tiers {
		if err := tier.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// promoteToFasterTiers writes the value to all tiers faster than the one
// where it was found. This optimizes future reads by moving hot data to
// faster tiers.
func (c *MultiCache) promoteToFasterTiers(ctx context.Context, key string, value []byte, foundAtTier int) error {
	for i := 0; i < foundAtTier; i++ {
		if err := c.tiers[i].Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}
