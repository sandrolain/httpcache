// Package prometheus provides Prometheus metrics implementation for httpcache.
// This package is optional and only imported when Prometheus metrics are needed.
package prometheus

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sandrolain/httpcache/metrics"
)

// Collector implements metrics.Collector for Prometheus
type Collector struct {
	cacheRequests    *prometheus.CounterVec
	cacheOpDuration  *prometheus.HistogramVec
	cacheSize        *prometheus.GaugeVec
	cacheEntries     *prometheus.GaugeVec
	httpRequests     *prometheus.CounterVec
	httpDuration     *prometheus.HistogramVec
	httpResponseSize *prometheus.CounterVec
	staleResponses   *prometheus.CounterVec
}

// CollectorConfig provides configuration options for the Prometheus collector
type CollectorConfig struct {
	// Registry is the Prometheus registry to use. If nil, uses prometheus.DefaultRegisterer
	Registry prometheus.Registerer

	// Namespace for metrics (default: "httpcache")
	Namespace string

	// Subsystem for metrics (optional)
	Subsystem string

	// ConstLabels are labels added to all metrics
	ConstLabels prometheus.Labels
}

// NewCollector creates a new Prometheus collector with default registry and configuration
func NewCollector() *Collector {
	return NewCollectorWithConfig(CollectorConfig{})
}

// NewCollectorWithRegistry creates a new Prometheus collector with a custom registry
func NewCollectorWithRegistry(reg prometheus.Registerer) *Collector {
	return NewCollectorWithConfig(CollectorConfig{
		Registry: reg,
	})
}

// NewCollectorWithConfig creates a new Prometheus collector with custom configuration
func NewCollectorWithConfig(config CollectorConfig) *Collector {
	// Set defaults
	if config.Registry == nil {
		config.Registry = prometheus.DefaultRegisterer
	}
	if config.Namespace == "" {
		config.Namespace = "httpcache"
	}

	factory := promauto.With(config.Registry)

	return &Collector{
		cacheRequests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   config.Namespace,
				Subsystem:   config.Subsystem,
				Name:        "cache_requests_total",
				Help:        "Total number of cache operations",
				ConstLabels: config.ConstLabels,
			},
			[]string{"operation", "cache_backend", "result"},
		),
		cacheOpDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   config.Namespace,
				Subsystem:   config.Subsystem,
				Name:        "cache_operation_duration_seconds",
				Help:        "Duration of cache operations in seconds",
				Buckets:     []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1, 5},
				ConstLabels: config.ConstLabels,
			},
			[]string{"operation", "cache_backend"},
		),
		cacheSize: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   config.Namespace,
				Subsystem:   config.Subsystem,
				Name:        "cache_size_bytes",
				Help:        "Current size of cache in bytes",
				ConstLabels: config.ConstLabels,
			},
			[]string{"cache_backend"},
		),
		cacheEntries: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   config.Namespace,
				Subsystem:   config.Subsystem,
				Name:        "cache_entries_total",
				Help:        "Current number of entries in cache",
				ConstLabels: config.ConstLabels,
			},
			[]string{"cache_backend"},
		),
		httpRequests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   config.Namespace,
				Subsystem:   config.Subsystem,
				Name:        "http_requests_total",
				Help:        "Total number of HTTP requests",
				ConstLabels: config.ConstLabels,
			},
			[]string{"method", "cache_status", "status_code"},
		),
		httpDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   config.Namespace,
				Subsystem:   config.Subsystem,
				Name:        "http_request_duration_seconds",
				Help:        "Duration of HTTP requests in seconds",
				Buckets:     []float64{.01, .05, .1, .5, 1, 2, 5, 10, 30},
				ConstLabels: config.ConstLabels,
			},
			[]string{"method", "cache_status"},
		),
		httpResponseSize: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   config.Namespace,
				Subsystem:   config.Subsystem,
				Name:        "http_response_size_bytes_total",
				Help:        "Total size of HTTP responses in bytes",
				ConstLabels: config.ConstLabels,
			},
			[]string{"cache_status"},
		),
		staleResponses: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   config.Namespace,
				Subsystem:   config.Subsystem,
				Name:        "stale_responses_served_total",
				Help:        "Total number of stale responses served on error",
				ConstLabels: config.ConstLabels,
			},
			[]string{"error_type"},
		),
	}
}

// RecordCacheOperation records a cache operation
func (c *Collector) RecordCacheOperation(operation, backend, result string, duration time.Duration) {
	c.cacheRequests.WithLabelValues(operation, backend, result).Inc()
	c.cacheOpDuration.WithLabelValues(operation, backend).Observe(duration.Seconds())
}

// RecordCacheSize records current cache size
func (c *Collector) RecordCacheSize(backend string, sizeBytes int64) {
	c.cacheSize.WithLabelValues(backend).Set(float64(sizeBytes))
}

// RecordCacheEntries records current number of cache entries
func (c *Collector) RecordCacheEntries(backend string, count int64) {
	c.cacheEntries.WithLabelValues(backend).Set(float64(count))
}

// RecordHTTPRequest records an HTTP request
func (c *Collector) RecordHTTPRequest(method, cacheStatus string, statusCode int, duration time.Duration) {
	c.httpRequests.WithLabelValues(method, cacheStatus, strconv.Itoa(statusCode)).Inc()
	c.httpDuration.WithLabelValues(method, cacheStatus).Observe(duration.Seconds())
}

// RecordHTTPResponseSize records HTTP response size
func (c *Collector) RecordHTTPResponseSize(cacheStatus string, sizeBytes int64) {
	c.httpResponseSize.WithLabelValues(cacheStatus).Add(float64(sizeBytes))
}

// RecordStaleResponse records a stale response served on error
func (c *Collector) RecordStaleResponse(errorType string) {
	c.staleResponses.WithLabelValues(errorType).Inc()
}

// Verify interface implementation at compile time
var _ metrics.Collector = (*Collector)(nil)
