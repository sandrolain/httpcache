package prometheus

import (
	"net/http"
	"strconv"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/metrics"
)

// InstrumentedTransport wraps an httpcache.Transport with Prometheus metrics
type InstrumentedTransport struct {
	underlying *httpcache.Transport
	collector  metrics.Collector
}

// NewInstrumentedTransport creates a new instrumented transport that records metrics
// for all HTTP requests.
//
// Parameters:
//   - transport: the underlying httpcache.Transport to wrap
//   - collector: the metrics collector (if nil, uses metrics.DefaultCollector)
//
// Example:
//
//	collector := prometheus.NewCollector()
//	cache := prometheus.NewInstrumentedCache(
//	    httpcache.NewMemoryCache(),
//	    "memory",
//	    collector,
//	)
//	transport := httpcache.NewTransport(cache)
//	instrumentedTransport := prometheus.NewInstrumentedTransport(transport, collector)
//	client := instrumentedTransport.Client()
func NewInstrumentedTransport(transport *httpcache.Transport, collector metrics.Collector) *InstrumentedTransport {
	if collector == nil {
		collector = metrics.DefaultCollector
	}

	return &InstrumentedTransport{
		underlying: transport,
		collector:  collector,
	}
}

// RoundTrip executes an HTTP request with metrics recording
func (t *InstrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.underlying.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		return resp, err
	}

	// Determine cache status
	cacheStatus := "miss"
	if resp.Header.Get(httpcache.XFromCache) == "1" {
		cacheStatus = "hit"
	} else if resp.StatusCode == http.StatusNotModified {
		cacheStatus = "revalidated"
	}

	// Record HTTP request metrics
	t.collector.RecordHTTPRequest(
		req.Method,
		cacheStatus,
		resp.StatusCode,
		duration,
	)

	// Record response size if Content-Length is available
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			t.collector.RecordHTTPResponseSize(cacheStatus, size)
		}
	}

	return resp, nil
}

// Client returns an HTTP client with instrumented transport
func (t *InstrumentedTransport) Client() *http.Client {
	return &http.Client{Transport: t}
}

// Verify interface implementation at compile time
var _ http.RoundTripper = (*InstrumentedTransport)(nil)
