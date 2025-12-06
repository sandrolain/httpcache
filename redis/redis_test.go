package redis

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/sandrolain/httpcache/test"
)

func TestRedisCache(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Check if Redis is available
	if err := client.Ping(ctx).Err(); err != nil {
		// TODO: rather than skip the test, fall back to a faked redis server
		t.Skipf("skipping test; no server running at localhost:6379")
	}
	_ = client.FlushAll(ctx)

	test.Cache(t, NewWithClient(client))
}
