package test_test

import (
	"testing"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/test"
)

func TestMemoryCache(t *testing.T) {
	test.Cache(t, httpcache.NewMemoryCache())
}
