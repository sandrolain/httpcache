package multicache

import (
	"context"
	"fmt"
	"testing"

	httpcache "github.com/sandrolain/httpcache"
)

func BenchmarkGet_SingleTier_Hit(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	mc := New(tier1)

	_ = mc.Set(ctx, "key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = mc.Get(ctx, "key")
		}
	})
}

func BenchmarkGet_SingleTier_Miss(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	mc := New(tier1)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = mc.Get(ctx, "missing")
		}
	})
}

func BenchmarkGet_ThreeTiers_HitInFirst(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	_ = tier1.Set(ctx, "key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = mc.Get(ctx, "key")
		}
	})
}

func BenchmarkGet_ThreeTiers_HitInSecond(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	_ = tier2.Set(ctx, "key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = mc.Get(ctx, "key")
		}
	})
}

func BenchmarkGet_ThreeTiers_HitInThird(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	_ = tier3.Set(ctx, "key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = mc.Get(ctx, "key")
		}
	})
}

func BenchmarkGet_ThreeTiers_Miss(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = mc.Get(ctx, "missing")
		}
	})
}

func BenchmarkSet_SingleTier(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	mc := New(tier1)

	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mc.Set(ctx, "key", value)
		}
	})
}

func BenchmarkSet_ThreeTiers(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mc.Set(ctx, "key", value)
		}
	})
}

func BenchmarkDelete_SingleTier(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	mc := New(tier1)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mc.Delete(ctx, "key")
		}
	})
}

func BenchmarkDelete_ThreeTiers(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mc.Delete(ctx, "key")
		}
	})
}

func BenchmarkSetGetDelete_SingleTier(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	mc := New(tier1)

	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mc.Set(ctx, "key", value)
			_, _, _ = mc.Get(ctx, "key")
			_ = mc.Delete(ctx, "key")
		}
	})
}

func BenchmarkSetGetDelete_ThreeTiers(b *testing.B) {
	ctx := context.Background()
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mc.Set(ctx, "key", value)
			_, _, _ = mc.Get(ctx, "key")
			_ = mc.Delete(ctx, "key")
		}
	})
}

func BenchmarkMultiTiers(b *testing.B) {
	ctx := context.Background()
	for _, numTiers := range []int{1, 2, 3, 5, 10} {
		b.Run(fmt.Sprintf("%d_tiers", numTiers), func(b *testing.B) {
			tiers := make([]httpcache.Cache, numTiers)
			for i := range tiers {
				tiers[i] = newMockCache()
			}

			mc := New(tiers...)
			value := []byte("value")

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = mc.Set(ctx, "key", value)
					_, _, _ = mc.Get(ctx, "key")
				}
			})
		})
	}
}
