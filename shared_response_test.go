package httpcache

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"testing"
)

// TestShareableResponseConcurrentAccess verifies that GetReusableResponse is thread-safe
// when multiple goroutines concurrently create clones of the same response
func TestShareableResponseConcurrentAccess(t *testing.T) {
	// Create a response with headers and body
	originalResp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type":  []string{"application/json"},
			"Cache-Control": []string{"max-age=3600"},
			"X-Custom":      []string{"value1", "value2"},
			"Authorization": []string{"Bearer token123"},
			"Accept-Ranges": []string{"bytes"},
		},
		Body: io.NopCloser(bytes.NewBufferString(`{"data": "test response body"}`)),
	}

	// Create shareable response
	shareable := shareHttpResponse(originalResp)

	// Number of concurrent goroutines
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel to collect errors
	errors := make(chan error, numGoroutines)

	// Launch concurrent goroutines that all call GetReusableResponse
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Get a clone of the response
			clone := shareable.GetReusableResponse()

			// Verify the clone has all headers
			if clone == nil {
				errors <- &testError{msg: "clone is nil"}
				return
			}

			if clone.StatusCode != 200 {
				errors <- &testError{msg: "wrong status code"}
				return
			}

			// Verify headers are correctly cloned
			if contentType := clone.Header.Get("Content-Type"); contentType != "application/json" {
				errors <- &testError{msg: "Content-Type header missing or incorrect"}
				return
			}

			if cacheControl := clone.Header.Get("Cache-Control"); cacheControl != "max-age=3600" {
				errors <- &testError{msg: "Cache-Control header missing or incorrect"}
				return
			}

			// Verify multi-value headers
			customValues := clone.Header["X-Custom"]
			if len(customValues) != 2 || customValues[0] != "value1" || customValues[1] != "value2" {
				errors <- &testError{msg: "X-Custom header values incorrect"}
				return
			}

			// Verify body can be read
			if clone.Body != nil {
				body, err := io.ReadAll(clone.Body)
				clone.Body.Close()
				if err != nil {
					errors <- err
					return
				}

				expectedBody := `{"data": "test response body"}`
				if string(body) != expectedBody {
					errors <- &testError{msg: "body content incorrect"}
					return
				}
			}

			// Try modifying the clone's headers (should not affect other clones)
			clone.Header.Set("X-Modified", "by-goroutine")
			clone.Header.Del("Authorization")

		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

// testError is a simple error type for test failures
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestShareableResponseHeaderIsolation verifies that modifications to one clone
// don't affect other clones
func TestShareableResponseHeaderIsolation(t *testing.T) {
	originalResp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type":  []string{"text/html"},
			"Cache-Control": []string{"public, max-age=3600"},
		},
		Body: io.NopCloser(bytes.NewBufferString("test body")),
	}

	shareable := shareHttpResponse(originalResp)

	// Create multiple clones
	clone1 := shareable.GetReusableResponse()
	clone2 := shareable.GetReusableResponse()
	clone3 := shareable.GetReusableResponse()

	// Modify clone1's headers
	clone1.Header.Set("X-Clone-1", "modified")
	clone1.Header.Del("Content-Type")

	// Modify clone2's headers
	clone2.Header.Set("X-Clone-2", "also-modified")
	clone2.Header.Set("Cache-Control", "no-cache")

	// Verify clone3 is unaffected
	if clone3.Header.Get("X-Clone-1") != "" {
		t.Error("clone3 should not have X-Clone-1 header from clone1")
	}

	if clone3.Header.Get("X-Clone-2") != "" {
		t.Error("clone3 should not have X-Clone-2 header from clone2")
	}

	if clone3.Header.Get("Content-Type") != "text/html" {
		t.Error("clone3 Content-Type should be unchanged")
	}

	if clone3.Header.Get("Cache-Control") != "public, max-age=3600" {
		t.Error("clone3 Cache-Control should be unchanged")
	}

	// Verify clone1 and clone2 have their modifications
	if clone1.Header.Get("X-Clone-1") != "modified" {
		t.Error("clone1 should have its X-Clone-1 header")
	}

	if clone2.Header.Get("X-Clone-2") != "also-modified" {
		t.Error("clone2 should have its X-Clone-2 header")
	}
}

// TestShareableResponseBodyIndependence verifies that each clone has an independent body reader
func TestShareableResponseBodyIndependence(t *testing.T) {
	bodyContent := "this is the response body content"
	originalResp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewBufferString(bodyContent)),
	}

	shareable := shareHttpResponse(originalResp)

	// Create multiple clones
	clone1 := shareable.GetReusableResponse()
	clone2 := shareable.GetReusableResponse()

	// Read from clone1
	body1, err := io.ReadAll(clone1.Body)
	if err != nil {
		t.Fatalf("Failed to read clone1 body: %v", err)
	}
	clone1.Body.Close()

	if string(body1) != bodyContent {
		t.Errorf("clone1 body = %q, want %q", string(body1), bodyContent)
	}

	// Read from clone2 (should still work even though clone1 was already read)
	body2, err := io.ReadAll(clone2.Body)
	if err != nil {
		t.Fatalf("Failed to read clone2 body: %v", err)
	}
	clone2.Body.Close()

	if string(body2) != bodyContent {
		t.Errorf("clone2 body = %q, want %q", string(body2), bodyContent)
	}

	// Create another clone after reading previous ones
	clone3 := shareable.GetReusableResponse()
	body3, err := io.ReadAll(clone3.Body)
	if err != nil {
		t.Fatalf("Failed to read clone3 body: %v", err)
	}
	clone3.Body.Close()

	if string(body3) != bodyContent {
		t.Errorf("clone3 body = %q, want %q", string(body3), bodyContent)
	}
}

// TestShareableResponseNilBody verifies correct handling of nil body
func TestShareableResponseNilBody(t *testing.T) {
	originalResp := &http.Response{
		StatusCode: 204, // No Content
		Header:     http.Header{"Cache-Control": []string{"no-cache"}},
		Body:       nil,
	}

	shareable := shareHttpResponse(originalResp)

	clone1 := shareable.GetReusableResponse()
	clone2 := shareable.GetReusableResponse()

	if clone1.Body != nil {
		t.Error("clone1 body should be nil")
	}

	if clone2.Body != nil {
		t.Error("clone2 body should be nil")
	}

	// Verify headers are still correctly cloned
	if clone1.Header.Get("Cache-Control") != "no-cache" {
		t.Error("clone1 should have Cache-Control header")
	}

	if clone2.Header.Get("Cache-Control") != "no-cache" {
		t.Error("clone2 should have Cache-Control header")
	}
}

// TestShareableResponseEmptyBody verifies correct handling of empty body
func TestShareableResponseEmptyBody(t *testing.T) {
	originalResp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Length": []string{"0"}},
		Body:       io.NopCloser(bytes.NewReader([]byte{})),
	}

	shareable := shareHttpResponse(originalResp)

	clone := shareable.GetReusableResponse()

	if clone.Body == nil {
		t.Fatal("clone body should not be nil")
	}

	body, err := io.ReadAll(clone.Body)
	if err != nil {
		t.Fatalf("Failed to read empty body: %v", err)
	}
	clone.Body.Close()

	if len(body) != 0 {
		t.Errorf("Expected empty body, got %d bytes", len(body))
	}
}

// TestShareableResponseGetUnsharedResponse verifies GetUnsharedResponse returns original
func TestShareableResponseGetUnsharedResponse(t *testing.T) {
	originalResp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"X-Original": []string{"true"}},
		Body:       io.NopCloser(bytes.NewBufferString("original body")),
	}

	shareable := shareHttpResponse(originalResp)

	// GetUnsharedResponse should return the original response
	unshared := shareable.GetUnsharedResponse()

	if unshared != originalResp {
		t.Error("GetUnsharedResponse should return the exact original response")
	}

	// Modify the unshared response
	unshared.Header.Set("X-Modified", "yes")

	// This should affect the original (they're the same object)
	if originalResp.Header.Get("X-Modified") != "yes" {
		t.Error("Modifications to unshared should affect original")
	}
}

// TestShareableResponseConcurrentBodyRead verifies that concurrent body reads work correctly
func TestShareableResponseConcurrentBodyRead(t *testing.T) {
	bodyContent := "concurrent body read test content"
	originalResp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewBufferString(bodyContent)),
	}

	shareable := shareHttpResponse(originalResp)

	const numReaders = 50
	var wg sync.WaitGroup
	wg.Add(numReaders)

	errors := make(chan error, numReaders)

	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()

			clone := shareable.GetReusableResponse()
			if clone.Body == nil {
				errors <- &testError{msg: "clone body is nil"}
				return
			}

			body, err := io.ReadAll(clone.Body)
			clone.Body.Close()

			if err != nil {
				errors <- err
				return
			}

			if string(body) != bodyContent {
				errors <- &testError{msg: "body content mismatch"}
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent body read error: %v", err)
	}
}

// TestShareableResponseLargeBody verifies correct handling of large response bodies
func TestShareableResponseLargeBody(t *testing.T) {
	// Create a 1MB body
	largeBody := bytes.Repeat([]byte("x"), 1024*1024)
	originalResp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
		Body:       io.NopCloser(bytes.NewReader(largeBody)),
	}

	shareable := shareHttpResponse(originalResp)

	// Create multiple clones and verify they all read the same large body
	for i := 0; i < 5; i++ {
		clone := shareable.GetReusableResponse()
		body, err := io.ReadAll(clone.Body)
		clone.Body.Close()

		if err != nil {
			t.Fatalf("Failed to read large body (clone %d): %v", i, err)
		}

		if len(body) != len(largeBody) {
			t.Errorf("Clone %d: body length = %d, want %d", i, len(body), len(largeBody))
		}

		if !bytes.Equal(body, largeBody) {
			t.Errorf("Clone %d: body content mismatch", i)
		}
	}
}
