package httpcache

import (
	"bytes"
	"io"
	"net/http"
	"sync"
)

// readerResult holds the result of reading a response body
type readerResult struct {
	content []byte
	err     error
}

// shareableResponse wraps an http.Response to make it safely shareable across multiple goroutines.
// It handles the challenge that http.Response.Body can only be read once by buffering the body
// and providing independent copies to each consumer.
type shareableResponse struct {
	resp           *http.Response
	readBodyResult func() readerResult // sync.OnceValue to read body only once
	mu             sync.RWMutex        // Protects concurrent access to resp.Header during cloning
}

// shareHttpResponse creates a shareableResponse from an http.Response.
// It sets up lazy reading of the body using sync.OnceValue to ensure the body
// is read exactly once, even when shared across multiple goroutines.
func shareHttpResponse(resp *http.Response) *shareableResponse {
	sharable := &shareableResponse{resp: resp}

	if resp != nil && resp.Body != nil {
		// sync.OnceValue ensures the body is read exactly once
		sharable.readBodyResult = sync.OnceValue(func() readerResult {
			content, readErr := io.ReadAll(resp.Body)
			if closeErr := resp.Body.Close(); closeErr != nil && readErr == nil {
				// If read succeeded but close failed, report close error
				readErr = closeErr
			}
			return readerResult{content, readErr}
		})
	}

	return sharable
}

// GetUnsharedResponse returns the original response without any modifications.
// This should only be called by the first goroutine (when singleflight.shared == false)
// as it returns the original response with the original body.
func (r *shareableResponse) GetUnsharedResponse() *http.Response {
	return r.resp
}

// GetReusableResponse returns a clone of the original response with a new body that can be read independently.
// This is used for goroutines that receive a shared response from singleflight.
// Each clone gets its own lazy reader that reads from the buffered content.
// Thread-safe: uses RWMutex to protect concurrent access to the original response headers.
func (r *shareableResponse) GetReusableResponse() *http.Response {
	// Lock for reading to prevent concurrent modifications during header copy
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a shallow copy of the response
	clone := new(http.Response)
	*clone = *r.resp

	// Deep copy the Header map for goroutine safety
	clone.Header = make(http.Header, len(r.resp.Header))
	for k, v := range r.resp.Header {
		clone.Header[k] = append([]string(nil), v...)
	}

	// Create a new lazy reader for the body
	clone.Body = nil
	if r.readBodyResult != nil {
		clone.Body = io.NopCloser(newLazyReader(r.readBodyResult))
	}

	return clone
}

// lazyReader defers reading content until the first Read call.
// This is important for performance: the body is only read if actually consumed.
type lazyReader struct {
	getReader func() (io.Reader, error) // sync.OnceValues to create reader only once
}

// newLazyReader creates a lazyReader that gets its content from the provided function.
// The function will be called at most once, even across multiple Read calls.
func newLazyReader(getContent func() readerResult) *lazyReader {
	return &lazyReader{
		getReader: sync.OnceValues(func() (io.Reader, error) {
			content := getContent()
			if content.err != nil {
				return nil, content.err
			}
			return bytes.NewReader(content.content), nil
		}),
	}
}

// Read implements io.Reader. On the first call, it initializes the underlying reader
// by calling getReader, then delegates all Read calls to it.
func (d *lazyReader) Read(p []byte) (int, error) {
	reader, err := d.getReader()
	if err != nil {
		return 0, err
	}
	return reader.Read(p)
}
