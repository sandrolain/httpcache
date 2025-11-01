//go:build integration

package blobcache

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/sandrolain/httpcache/test"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	_ "gocloud.dev/blob/s3blob"
)

const (
	minioImage      = "minio/minio:latest"
	minioPort       = "9000/tcp"
	minioAccessKey  = "minioadmin"
	minioSecretKey  = "minioadmin"
	minioBucketName = "test-cache"
	minioRegion     = "us-east-1"
)

// setupMinIOContainer starts a MinIO container and returns the endpoint and cleanup function
func setupMinIOContainer(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        minioImage,
		ExposedPorts: []string{minioPort},
		Env: map[string]string{
			"MINIO_ROOT_USER":     minioAccessKey,
			"MINIO_ROOT_PASSWORD": minioSecretKey,
		},
		Cmd: []string{"server", "/data", "--console-address", ":9001"},
		WaitingFor: wait.ForHTTP("/minio/health/live").
			WithPort("9000/tcp").
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start MinIO container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "9000")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate MinIO container: %v", err)
		}
	}

	// Wait a bit more for MinIO to be fully ready
	time.Sleep(2 * time.Second)

	return endpoint, cleanup
}

// createS3Bucket creates a bucket in MinIO using AWS SDK v1
func createS3Bucket(ctx context.Context, t *testing.T, endpoint, bucketName string) {
	t.Helper()

	sess, err := session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials(minioAccessKey, minioSecretKey, ""),
		Endpoint:         aws.String(endpoint),
		Region:           aws.String(minioRegion),
		DisableSSL:       aws.Bool(true),
		S3ForcePathStyle: aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("failed to create AWS session: %v", err)
	}

	client := s3.New(sess)

	_, err = client.CreateBucketWithContext(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("failed to create bucket: %v", err)
	}

	// Wait for bucket to be available
	err = client.WaitUntilBucketExistsWithContext(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("bucket not available: %v", err)
	}

	t.Logf("S3 bucket '%s' created successfully", bucketName)
}

// TestBlobCacheMinIOIntegration tests the blob cache with MinIO (S3-compatible).
// This is a real integration test that exercises cloud blob storage.
func TestBlobCacheMinIOIntegration(t *testing.T) {
	ctx := context.Background()

	endpoint, cleanup := setupMinIOContainer(ctx, t)
	defer cleanup()

	t.Log("MinIO container started at:", endpoint)

	// Create the bucket
	createS3Bucket(ctx, t, endpoint, minioBucketName)

	// Set AWS credentials for gocloud.dev (uses AWS SDK v1)
	os.Setenv("AWS_ACCESS_KEY_ID", minioAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", minioSecretKey)
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	}()

	// Create S3 bucket URL for MinIO
	// gocloud.dev automatically detects HTTP from endpoint URL
	bucketURL := fmt.Sprintf("s3://%s?endpoint=http://%s&s3ForcePathStyle=true&region=%s",
		minioBucketName, endpoint, minioRegion)

	cache, err := New(ctx, Config{
		BucketURL: bucketURL,
		KeyPrefix: "integration-test/",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create blob cache: %v", err)
	}

	// Ensure cache is closed
	if closer, ok := cache.(interface{ Close() error }); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				t.Errorf("Failed to close cache: %v", err)
			}
		}()
	}

	// Run standard cache tests
	test.Cache(t, cache)

	t.Run("LargeValue", func(t *testing.T) {
		key := "large-key"
		value := make([]byte, 1024*1024) // 1MB
		for i := range value {
			value[i] = byte(i % 256)
		}

		cache.Set(key, value)

		retrieved, ok := cache.Get(key)
		if !ok {
			t.Fatal("Expected to find large key in cache")
		}

		if len(retrieved) != len(value) {
			t.Errorf("Expected value length %d, got %d", len(value), len(retrieved))
		}

		// Verify content
		for i := range value {
			if retrieved[i] != value[i] {
				t.Errorf("Value mismatch at byte %d: expected %d, got %d", i, value[i], retrieved[i])
				break
			}
		}
	})

	t.Run("MultipleKeys", func(t *testing.T) {
		keys := []string{"key1", "key2", "key3", "key4", "key5"}
		values := [][]byte{
			[]byte("value1"),
			[]byte("value2"),
			[]byte("value3"),
			[]byte("value4"),
			[]byte("value5"),
		}

		// Set all keys
		for i, key := range keys {
			cache.Set(key, values[i])
		}

		// Get all keys
		for i, key := range keys {
			retrieved, ok := cache.Get(key)
			if !ok {
				t.Errorf("Expected to find key %s", key)
				continue
			}
			if string(retrieved) != string(values[i]) {
				t.Errorf("Key %s: expected %q, got %q", key, values[i], retrieved)
			}
		}

		// Delete some keys
		cache.Delete(keys[1])
		cache.Delete(keys[3])

		// Verify deletions
		_, ok := cache.Get(keys[1])
		if ok {
			t.Error("Expected key2 to be deleted")
		}
		_, ok = cache.Get(keys[3])
		if ok {
			t.Error("Expected key4 to be deleted")
		}

		// Verify others still exist
		for _, i := range []int{0, 2, 4} {
			_, ok := cache.Get(keys[i])
			if !ok {
				t.Errorf("Expected key %s to still exist", keys[i])
			}
		}
	})
}

// TestBlobCacheMinIOKeyPrefix tests key prefix isolation with MinIO.
func TestBlobCacheMinIOKeyPrefix(t *testing.T) {
	ctx := context.Background()

	endpoint, cleanup := setupMinIOContainer(ctx, t)
	defer cleanup()

	t.Log("MinIO container started at:", endpoint)

	// Create the bucket
	createS3Bucket(ctx, t, endpoint, minioBucketName)

	// Set AWS credentials
	os.Setenv("AWS_ACCESS_KEY_ID", minioAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", minioSecretKey)
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	}()

	bucketURL := fmt.Sprintf("s3://%s?endpoint=http://%s&s3ForcePathStyle=true&region=%s",
		minioBucketName, endpoint, minioRegion)

	// Create two caches with different prefixes
	cache1, err := New(ctx, Config{
		BucketURL: bucketURL,
		KeyPrefix: "prefix1/",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create cache1: %v", err)
	}
	defer func() {
		if closer, ok := cache1.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	cache2, err := New(ctx, Config{
		BucketURL: bucketURL,
		KeyPrefix: "prefix2/",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create cache2: %v", err)
	}
	defer func() {
		if closer, ok := cache2.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	// Same key, different values in different prefixes
	key := "shared-key"
	value1 := []byte("value-from-cache1")
	value2 := []byte("value-from-cache2")

	cache1.Set(key, value1)
	cache2.Set(key, value2)

	// Get from cache1
	retrieved1, ok := cache1.Get(key)
	if !ok {
		t.Fatal("Expected to find key in cache1")
	}
	if string(retrieved1) != string(value1) {
		t.Errorf("Cache1: expected %q, got %q", value1, retrieved1)
	}

	// Get from cache2
	retrieved2, ok := cache2.Get(key)
	if !ok {
		t.Fatal("Expected to find key in cache2")
	}
	if string(retrieved2) != string(value2) {
		t.Errorf("Cache2: expected %q, got %q", value2, retrieved2)
	}

	// Delete from cache1 shouldn't affect cache2
	cache1.Delete(key)

	_, ok = cache1.Get(key)
	if ok {
		t.Error("Expected key to be deleted from cache1")
	}

	retrieved2, ok = cache2.Get(key)
	if !ok {
		t.Error("Expected key to still exist in cache2")
	}
	if string(retrieved2) != string(value2) {
		t.Errorf("Cache2 after cache1 delete: expected %q, got %q", value2, retrieved2)
	}
}
