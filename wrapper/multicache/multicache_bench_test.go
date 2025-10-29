package multicache

import (
	"fmt"
	"testing"

	httpcache "github.com/sandrolain/httpcache"
)

func BenchmarkGet_SingleTier_Hit(b *testing.B) {
	tier1 := newMockCache()
	mc := New(tier1)

	mc.Set("key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mc.Get("key")
		}
	})
}

func BenchmarkGet_SingleTier_Miss(b *testing.B) {
	tier1 := newMockCache()
	mc := New(tier1)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mc.Get("missing")
		}
	})
}

func BenchmarkGet_ThreeTiers_HitInFirst(b *testing.B) {
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	tier1.Set("key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mc.Get("key")
		}
	})
}

func BenchmarkGet_ThreeTiers_HitInSecond(b *testing.B) {
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	tier2.Set("key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mc.Get("key")
		}
	})
}

func BenchmarkGet_ThreeTiers_HitInThird(b *testing.B) {
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	tier3.Set("key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mc.Get("key")
		}
	})
}

func BenchmarkGet_ThreeTiers_Miss(b *testing.B) {
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mc.Get("missing")
		}
	})
}

func BenchmarkSet_SingleTier(b *testing.B) {
	tier1 := newMockCache()
	mc := New(tier1)

	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mc.Set("key", value)
		}
	})
}

func BenchmarkSet_ThreeTiers(b *testing.B) {
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mc.Set("key", value)
		}
	})
}

func BenchmarkDelete_SingleTier(b *testing.B) {
	tier1 := newMockCache()
	mc := New(tier1)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mc.Delete("key")
		}
	})
}

func BenchmarkDelete_ThreeTiers(b *testing.B) {
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mc.Delete("key")
		}
	})
}

func BenchmarkSetGetDelete_SingleTier(b *testing.B) {
	tier1 := newMockCache()
	mc := New(tier1)

	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mc.Set("key", value)
			_, _ = mc.Get("key")
			mc.Delete("key")
		}
	})
}

func BenchmarkSetGetDelete_ThreeTiers(b *testing.B) {
	tier1 := newMockCache()
	tier2 := newMockCache()
	tier3 := newMockCache()
	mc := New(tier1, tier2, tier3)

	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mc.Set("key", value)
			_, _ = mc.Get("key")
			mc.Delete("key")
		}
	})
}

func BenchmarkMultiTiers(b *testing.B) {
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
					mc.Set("key", value)
					_, _ = mc.Get("key")
				}
			})
		})
	}
}
