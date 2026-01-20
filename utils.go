// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// bufferPool is a sync.Pool for reusing bytes.Buffer to reduce allocations and GC pressure.
// Buffers larger than maxPooledBufferSize are not returned to the pool to avoid holding
// large amounts of memory indefinitely.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// defaultMaxPooledBufferSize is the default maximum buffer size to pool.
// This can be overridden per-Transport via MaxPooledBufferSize field.
const defaultMaxPooledBufferSize = 64 * 1024 // 64KB

// getBuffer retrieves a buffer from the pool and resets it for use.
// The returned buffer is ready to use and must be returned to the pool
// with putBuffer when no longer needed.
func getBuffer() *bytes.Buffer {
	//nolint:errcheck // sync.Pool.Get() does not return an error; this is a type assertion
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// putBuffer returns a buffer to the pool for reuse with the default size limit.
// Large buffers (>defaultMaxPooledBufferSize) are not pooled to avoid memory bloat.
// Always call this with defer after getting a buffer to ensure it's returned.
func putBuffer(buf *bytes.Buffer) {
	putBufferWithLimit(buf, defaultMaxPooledBufferSize)
}

// putBufferWithLimit returns a buffer to the pool for reuse with a custom size limit.
// Large buffers (>maxSize) are not pooled to avoid memory bloat.
// This is used internally by Transport to respect the MaxPooledBufferSize configuration.
func putBufferWithLimit(buf *bytes.Buffer, maxSize int64) {
	// Only pool buffers that are not too large
	if buf.Cap() <= int(maxSize) {
		bufferPool.Put(buf)
	}
}

// cloneRequest returns a clone of the provided *http.Request.
// The clone is a shallow copy of the struct and its Header map.
// (This function copyright goauth2 authors: https://code.google.com/p/goauth2)
func cloneRequest(r *http.Request) *http.Request {
	// shallow copy of the struct
	r2 := new(http.Request)
	*r2 = *r
	// deep copy of the Header
	r2.Header = make(http.Header)
	for k, s := range r.Header {
		r2.Header[k] = s
	}
	return r2
}

// headerAllCommaSepValues returns all comma-separated values (each
// with whitespace trimmed) for header name in headers. According to
// Section 4.2 of the HTTP/1.1 spec
// (http://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2),
// values from multiple occurrences of a header should be concatenated, if
// the header's value is a comma-separated list.
func headerAllCommaSepValues(headers http.Header, name string) []string {
	var vals []string
	for _, val := range headers[http.CanonicalHeaderKey(name)] {
		fields := strings.Split(val, ",")
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}
		vals = append(vals, fields...)
	}
	return vals
}

// getEndToEndHeaders returns the list of end-to-end headers from the response
func getEndToEndHeaders(respHeaders http.Header) []string {
	// These headers are always hop-by-hop
	hopByHopHeaders := map[string]struct{}{
		"Connection":          {},
		"Keep-Alive":          {},
		"Proxy-Authenticate":  {},
		"Proxy-Authorization": {},
		"Te":                  {},
		"Trailers":            {},
		"Transfer-Encoding":   {},
		"Upgrade":             {},
	}

	for _, extra := range strings.Split(respHeaders.Get("connection"), ",") {
		// any header listed in connection, if present, is also considered hop-by-hop
		if strings.Trim(extra, " ") != "" {
			hopByHopHeaders[http.CanonicalHeaderKey(extra)] = struct{}{}
		}
	}
	endToEndHeaders := []string{}
	for respHeader := range respHeaders {
		if _, ok := hopByHopHeaders[respHeader]; !ok {
			endToEndHeaders = append(endToEndHeaders, respHeader)
		}
	}
	return endToEndHeaders
}

// newGatewayTimeoutResponse creates a 504 Gateway Timeout response
func newGatewayTimeoutResponse(req *http.Request) *http.Response {
	// Use buffer pool to reduce allocations
	braw := getBuffer()
	defer putBuffer(braw)
	braw.WriteString("HTTP/1.1 504 Gateway Timeout\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(braw), req)
	if err != nil {
		panic(err)
	}
	return resp
}

// cachingReadCloser is a wrapper around ReadCloser R that calls OnEOF
// handler with a full copy of the content read from R when EOF is
// reached. It respects a maximum buffer size to prevent memory exhaustion.
type cachingReadCloser struct {
	// Underlying ReadCloser.
	R io.ReadCloser
	// OnEOF is called with a copy of the content of R when EOF is reached.
	// Only called if the total size didn't exceed maxSize.
	OnEOF func(io.Reader)
	// OnExceeded is called when the response size exceeds maxSize.
	// Provides the total bytes read so far.
	OnExceeded func(totalSize int64)

	buf       bytes.Buffer // buf stores a copy of the content of R.
	maxSize   int64        // Maximum size to buffer (0 = unlimited)
	totalRead int64        // Total bytes read so far
	exceeded  bool         // True if maxSize was exceeded
}

// Read reads the next len(p) bytes from R or until R is drained. The
// return value n is the number of bytes read. If R has no data to
// return, err is io.EOF and OnEOF is called with a full copy of what
// has been read so far (only if size limit wasn't exceeded).
func (r *cachingReadCloser) Read(p []byte) (n int, err error) {
	n, err = r.R.Read(p)

	// Only buffer if we haven't exceeded the limit and there's a limit set
	if !r.exceeded {
		if r.maxSize > 0 && r.totalRead+int64(n) > r.maxSize {
			// Size limit exceeded
			r.exceeded = true
			if r.OnExceeded != nil {
				r.OnExceeded(r.totalRead + int64(n))
			}
			// Clear buffer to free memory
			r.buf.Reset()
		} else {
			// Still within limit, buffer the data
			r.buf.Write(p[:n])
			r.totalRead += int64(n)
		}
	}

	if err == io.EOF || n < len(p) {
		// Only call OnEOF if we didn't exceed the size limit
		if !r.exceeded && r.OnEOF != nil {
			r.OnEOF(bytes.NewReader(r.buf.Bytes()))
		}
	}
	return n, err
}

func (r *cachingReadCloser) Close() error {
	return r.R.Close()
}

const bodyDrainSize = 1 << 15 // 32KB, arbitrary limit for draining

// drainDiscardedBody reads and discards up to drainSize bytes from the body to allow connection reuse.
// It's used when we're discarding a response (e.g., returning stale cache instead of a 500 error).
// Returns an error if draining or closing fails.
func drainDiscardedBody(body io.ReadCloser) error {
	if body == nil {
		return nil
	}

	// Drain the body to allow connection reuse
	if _, err := io.Copy(io.Discard, io.LimitReader(body, bodyDrainSize)); err != nil {
		// Still try to close even if drain failed
		body.Close() //nolint:errcheck // best effort cleanup
		return fmt.Errorf("failed to drain response body: %w", err)
	}

	// Close the body
	if err := body.Close(); err != nil {
		return fmt.Errorf("failed to close response body: %w", err)
	}

	return nil
}
