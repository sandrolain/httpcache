// Package prometheus provides Prometheus metrics exporter for httpcache internal metrics.
// This package exports httpcache.TransportMetrics as Prometheus gauges.
package prometheus

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sandrolain/httpcache"
)

// Collector exports httpcache.TransportMetrics as Prometheus metrics.
// This is the recommended way to expose cache metrics to Prometheus in v2.
type Collector struct {
	metrics *httpcache.TransportMetrics

	// Prometheus metrics
	cacheHits     prometheus.Gauge
	cacheMisses   prometheus.Gauge
	cacheErrors   prometheus.Gauge
	staleServed   prometheus.Gauge
	deduplication prometheus.Gauge
	cacheHitRate  prometheus.Gauge
	cachedBytes   prometheus.Gauge
	totalRequests prometheus.Gauge

	updateInterval time.Duration
}

// CollectorConfig configures the Prometheus exporter for TransportMetrics
type CollectorConfig struct {
	// Metrics is the TransportMetrics instance to export (required)
	Metrics *httpcache.TransportMetrics

	// Registry is the Prometheus registry to use. If nil, uses prometheus.DefaultRegisterer
	Registry prometheus.Registerer

	// Namespace for metrics (default: "httpcache")
	Namespace string

	// Subsystem for metrics (optional)
	Subsystem string

	// ConstLabels are labels added to all metrics
	ConstLabels prometheus.Labels

	// UpdateInterval is how often to update metrics from TransportMetrics
	// Default: 10 seconds. Set to 0 to disable automatic updates (call Update() manually)
	UpdateInterval time.Duration
}

// NewCollector creates a Prometheus collector that exports httpcache.TransportMetrics.
// The collector registers Prometheus gauges that reflect the current state of the TransportMetrics.
//
// Example:
//
//	metrics := httpcache.NewTransportMetrics()
//	transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))
//
//	collector := prometheus.NewCollector(prometheus.CollectorConfig{
//	    Metrics: metrics,
//	})
//	defer collector.Start(context.Background())()
//
// To expose metrics via HTTP:
//
//	import "github.com/prometheus/client_golang/prometheus/promhttp"
//	http.Handle("/metrics", promhttp.Handler())
func NewCollector(config CollectorConfig) *Collector {
	if config.Metrics == nil {
		panic("TransportMetrics cannot be nil")
	}

	// Set defaults
	if config.Registry == nil {
		config.Registry = prometheus.DefaultRegisterer
	}
	if config.Namespace == "" {
		config.Namespace = "httpcache"
	}
	if config.UpdateInterval == 0 {
		config.UpdateInterval = 10 * time.Second
	}

	factory := promauto.With(config.Registry)

	c := &Collector{
		metrics:        config.Metrics,
		updateInterval: config.UpdateInterval,
		cacheHits: factory.NewGauge(prometheus.GaugeOpts{
			Namespace:   config.Namespace,
			Subsystem:   config.Subsystem,
			Name:        "cache_hits_total",
			Help:        "Total number of cache hits",
			ConstLabels: config.ConstLabels,
		}),
		cacheMisses: factory.NewGauge(prometheus.GaugeOpts{
			Namespace:   config.Namespace,
			Subsystem:   config.Subsystem,
			Name:        "cache_misses_total",
			Help:        "Total number of cache misses",
			ConstLabels: config.ConstLabels,
		}),
		cacheErrors: factory.NewGauge(prometheus.GaugeOpts{
			Namespace:   config.Namespace,
			Subsystem:   config.Subsystem,
			Name:        "cache_errors_total",
			Help:        "Total number of cache operation errors",
			ConstLabels: config.ConstLabels,
		}),
		staleServed: factory.NewGauge(prometheus.GaugeOpts{
			Namespace:   config.Namespace,
			Subsystem:   config.Subsystem,
			Name:        "stale_served_total",
			Help:        "Total number of stale responses served",
			ConstLabels: config.ConstLabels,
		}),
		deduplication: factory.NewGauge(prometheus.GaugeOpts{
			Namespace:   config.Namespace,
			Subsystem:   config.Subsystem,
			Name:        "deduplication_total",
			Help:        "Total number of requests deduplicated via singleflight",
			ConstLabels: config.ConstLabels,
		}),
		cacheHitRate: factory.NewGauge(prometheus.GaugeOpts{
			Namespace:   config.Namespace,
			Subsystem:   config.Subsystem,
			Name:        "cache_hit_rate",
			Help:        "Cache hit rate (0-1)",
			ConstLabels: config.ConstLabels,
		}),
		cachedBytes: factory.NewGauge(prometheus.GaugeOpts{
			Namespace:   config.Namespace,
			Subsystem:   config.Subsystem,
			Name:        "cached_bytes",
			Help:        "Approximate number of bytes currently cached",
			ConstLabels: config.ConstLabels,
		}),
		totalRequests: factory.NewGauge(prometheus.GaugeOpts{
			Namespace:   config.Namespace,
			Subsystem:   config.Subsystem,
			Name:        "total_requests",
			Help:        "Total number of cache requests (hits + misses)",
			ConstLabels: config.ConstLabels,
		}),
	}

	// Initial update
	c.Update()

	return c
}

// NewCollectorWithRegistry creates a collector with a custom registry.
// This is a convenience function for NewCollector with a custom registry.
func NewCollectorWithRegistry(registry prometheus.Registerer, metrics *httpcache.TransportMetrics) *Collector {
	return NewCollector(CollectorConfig{
		Metrics:  metrics,
		Registry: registry,
	})
}

// Update synchronizes Prometheus metrics with the current state of TransportMetrics.
// This method reads the current snapshot and updates all gauge values.
func (c *Collector) Update() {
	snapshot := c.metrics.Snapshot()

	c.cacheHits.Set(float64(snapshot.CacheHits))
	c.cacheMisses.Set(float64(snapshot.CacheMisses))
	c.cacheErrors.Set(float64(snapshot.CacheErrors))
	c.staleServed.Set(float64(snapshot.StaleServed))
	c.deduplication.Set(float64(snapshot.Deduplication))
	c.cacheHitRate.Set(snapshot.HitRate)
	c.cachedBytes.Set(float64(snapshot.CachedBytes))
	c.totalRequests.Set(float64(snapshot.TotalRequests))
}

// Start begins periodically updating Prometheus metrics from TransportMetrics.
// This runs in a goroutine and returns a stop function that should be called to
// stop the updates.
//
// Example:
//
//	stop := collector.Start(context.Background())
//	defer stop()
//
// Or with context cancellation:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	collector.Start(ctx)
func (c *Collector) Start(ctx context.Context) func() {
	ticker := time.NewTicker(c.updateInterval)
	done := make(chan struct{})
	stopped := false
	var stopMutex sync.Mutex

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				c.Update()
			}
		}
	}()

	return func() {
		stopMutex.Lock()
		defer stopMutex.Unlock()
		if !stopped {
			close(done)
			stopped = true
		}
	}
}
