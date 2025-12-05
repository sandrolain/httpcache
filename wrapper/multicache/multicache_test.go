package multicache

import (
	"context"
	"sync"
	"testing"

	httpcache "github.com/sandrolain/httpcache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCache is a simple in-memory cache for testing
type mockCache struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string][]byte),
	}
}

func (m *mockCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.data[key]
	return value, ok, nil
}

func (m *mockCache) Set(_ context.Context, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func TestInterface(t *testing.T) {
	var _ httpcache.Cache = &MultiCache{}
}

func TestNew(t *testing.T) {
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()

	tests := []struct {
		name   string
		tiers  []httpcache.Cache
		expect bool
	}{
		{
			name:   "valid single tier",
			tiers:  []httpcache.Cache{tier1},
			expect: true,
		},
		{
			name:   "valid two tiers",
			tiers:  []httpcache.Cache{tier1, tier2},
			expect: true,
		},
		{
			name:   "valid three tiers",
			tiers:  []httpcache.Cache{tier1, tier2, tier3},
			expect: true,
		},
		{
			name:   "no tiers",
			tiers:  []httpcache.Cache{},
			expect: false,
		},
		{
			name:   "nil tier",
			tiers:  []httpcache.Cache{tier1, nil, tier3},
			expect: false,
		},
		{
			name:   "duplicate tier",
			tiers:  []httpcache.Cache{tier1, tier2, tier1},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := New(tt.tiers...)
			if tt.expect {
				require.NotNil(t, mc)
				assert.Equal(t, len(tt.tiers), len(mc.tiers))
			} else {
				assert.Nil(t, mc)
			}
		})
	}
}

func TestGet_SingleTier(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	mc := New(tier1)
	require.NotNil(t, mc)

	// Cache miss
	value, ok, _ := mc.Get(ctx, "missing")
	assert.False(t, ok)
	assert.Nil(t, value)

	// Add to tier and retrieve
	_ = tier1.Set(ctx, "key1", []byte("value1"))
	value, ok, _ = mc.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)
}

func TestGet_MultipleTiers_FoundInFirst(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)
	require.NotNil(t, mc)

	// Add to first tier only
	_ = tier1.Set(ctx, "key1", []byte("value1"))

	value, ok, _ := mc.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)

	// Should not be promoted (already in fastest tier)
	_, ok, _ = tier2.Get(ctx, "key1")
	assert.False(t, ok)
	_, ok, _ = tier3.Get(ctx, "key1")
	assert.False(t, ok)
}

func TestGet_MultipleTiers_FoundInMiddle(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)
	require.NotNil(t, mc)

	// Add to second tier only
	_ = tier2.Set(ctx, "key1", []byte("value1"))

	value, ok, _ := mc.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)

	// Should be promoted to first tier
	value, ok, _ = tier1.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)

	// Should not be in third tier
	_, ok, _ = tier3.Get(ctx, "key1")
	assert.False(t, ok)
}

func TestGet_MultipleTiers_FoundInLast(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)
	require.NotNil(t, mc)

	// Add to last tier only
	_ = tier3.Set(ctx, "key1", []byte("value1"))

	value, ok, _ := mc.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)

	// Should be promoted to all faster tiers
	value, ok, _ = tier1.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)

	value, ok, _ = tier2.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)
}

func TestGet_NotFound(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)
	require.NotNil(t, mc)

	value, ok, _ := mc.Get(ctx, "missing")
	assert.False(t, ok)
	assert.Nil(t, value)
}

func TestSet_SingleTier(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	mc := New(tier1)
	require.NotNil(t, mc)

	_ = mc.Set(ctx, "key1", []byte("value1"))

	value, ok, _ := tier1.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)
}

func TestSet_MultipleTiers(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)
	require.NotNil(t, mc)

	_ = mc.Set(ctx, "key1", []byte("value1"))

	// Should be set in all tiers
	value, ok, _ := tier1.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)

	value, ok, _ = tier2.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)

	value, ok, _ = tier3.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value1"), value)
}

func TestSet_Overwrite(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	mc := New(tier1, tier2)
	require.NotNil(t, mc)

	_ = mc.Set(ctx, "key1", []byte("value1"))
	_ = mc.Set(ctx, "key1", []byte("value2"))

	// Should be overwritten in all tiers
	value, ok, _ := tier1.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value2"), value)

	value, ok, _ = tier2.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, []byte("value2"), value)
}

func TestDelete_SingleTier(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	mc := New(tier1)
	require.NotNil(t, mc)

	_ = tier1.Set(ctx, "key1", []byte("value1"))
	_ = mc.Delete(ctx, "key1")

	_, ok, _ := tier1.Get(ctx, "key1")
	assert.False(t, ok)
}

func TestDelete_MultipleTiers(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)
	require.NotNil(t, mc)

	// Set in all tiers
	_ = tier1.Set(ctx, "key1", []byte("value1"))
	_ = tier2.Set(ctx, "key1", []byte("value1"))
	_ = tier3.Set(ctx, "key1", []byte("value1"))

	_ = mc.Delete(ctx, "key1")

	// Should be deleted from all tiers
	_, ok, _ := tier1.Get(ctx, "key1")
	assert.False(t, ok)

	_, ok, _ = tier2.Get(ctx, "key1")
	assert.False(t, ok)

	_, ok, _ = tier3.Get(ctx, "key1")
	assert.False(t, ok)
}

func TestDelete_NotFound(t *testing.T) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	mc := New(tier1, tier2)
	require.NotNil(t, mc)

	// Should not panic
	_ = mc.Delete(ctx, "missing")
}

func TestPromotion_Scenario(t *testing.T) {
	ctx := context.Background()
	// Simulate a realistic scenario:
	// - Tier 1: Fast LRU with limited capacity
	// - Tier 2: Medium speed cache with more capacity
	// - Tier 3: Slow persistent cache with unlimited capacity

	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)
	require.NotNil(t, mc)

	// Initially store in all tiers
	_ = mc.Set(ctx, "hot-key", []byte("hot-value"))

	// Simulate tier 1 eviction (e.g., LRU evicted the entry)
	_ = tier1.Delete(ctx, "hot-key")

	// First access after eviction should find in tier 2 and promote to tier 1
	value, ok, _ := mc.Get(ctx, "hot-key")
	assert.True(t, ok)
	assert.Equal(t, []byte("hot-value"), value)

	// Now should be back in tier 1
	value, ok, _ = tier1.Get(ctx, "hot-key")
	assert.True(t, ok)
	assert.Equal(t, []byte("hot-value"), value)

	// Simulate both tier 1 and tier 2 evictions
	_ = tier1.Delete(ctx, "hot-key")
	_ = tier2.Delete(ctx, "hot-key")

	// Access should find in tier 3 and promote to all faster tiers
	value, ok, _ = mc.Get(ctx, "hot-key")
	assert.True(t, ok)
	assert.Equal(t, []byte("hot-value"), value)

	// Now should be in all tiers again
	value, ok, _ = tier1.Get(ctx, "hot-key")
	assert.True(t, ok)
	assert.Equal(t, []byte("hot-value"), value)

	value, ok, _ = tier2.Get(ctx, "hot-key")
	assert.True(t, ok)
	assert.Equal(t, []byte("hot-value"), value)
}

func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	// Basic concurrency test to ensure no races
	tier1 := newMockCache()
	tier2 := newMockCache()
	mc := New(tier1, tier2)
	require.NotNil(t, mc)

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = mc.Set(ctx, "key", []byte("value"))
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_, _, _ = mc.Get(ctx, "key")
		}
		done <- true
	}()

	// Deleter goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = mc.Delete(ctx, "key")
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
}
