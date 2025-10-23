package diskcache

import (
	"os"
	"testing"

	"github.com/sandrolain/httpcache/test"
)

func TestDiskCache(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "httpcache")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	test.Cache(t, New(tempDir))
}
