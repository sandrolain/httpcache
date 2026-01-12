package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkDeduplicationEnabled measures performance with request deduplication enabled
func BenchmarkDeduplicationEnabled(b *testing.B) {
	counter := atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("/endpoint", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte("benchmark response data"))
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := newMockCache()
	transport := NewTransport(cache)
	transport.EnableDeduplication = true
	client := http.Client{Transport: transport}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		const numConcurrentRequests = 10
		var wg sync.WaitGroup
		wg.Add(numConcurrentRequests)

		for j := 0; j < numConcurrentRequests; j++ {
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest(methodGET, server.URL+"/endpoint", nil)
				resp, err := client.Do(req)
				if err != nil {
					b.Errorf("request failed: %v", err)
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}()
		}

		wg.Wait()
		// Clear cache between iterations to ensure fresh requests
		cache.mu.Lock()
		cache.items = make(map[string][]byte)
		cache.mu.Unlock()
	}
}

// BenchmarkDeduplicationDisabled measures performance without request deduplication
func BenchmarkDeduplicationDisabled(b *testing.B) {
	counter := atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("/endpoint", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte("benchmark response data"))
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := newMockCache()
	transport := NewTransport(cache)
	transport.EnableDeduplication = false
	client := http.Client{Transport: transport}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		const numConcurrentRequests = 10
		var wg sync.WaitGroup
		wg.Add(numConcurrentRequests)

		for j := 0; j < numConcurrentRequests; j++ {
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest(methodGET, server.URL+"/endpoint", nil)
				resp, err := client.Do(req)
				if err != nil {
					b.Errorf("request failed: %v", err)
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}()
		}

		wg.Wait()
		// Clear cache between iterations to ensure fresh requests
		cache.mu.Lock()
		cache.items = make(map[string][]byte)
		cache.mu.Unlock()
	}
}

// BenchmarkDeduplicationVaryingConcurrency measures deduplication impact with different concurrency levels
func BenchmarkDeduplicationVaryingConcurrency(b *testing.B) {
	concurrencyLevels := []int{5, 10, 20, 50}

	for _, concurrency := range concurrencyLevels {
		b.Run("Enabled_Concurrency"+string(rune(concurrency+'0')), func(b *testing.B) {
			benchmarkWithConcurrency(b, true, concurrency)
		})

		b.Run("Disabled_Concurrency"+string(rune(concurrency+'0')), func(b *testing.B) {
			benchmarkWithConcurrency(b, false, concurrency)
		})
	}
}

func benchmarkWithConcurrency(b *testing.B, enableDedup bool, concurrency int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/endpoint", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(5 * time.Millisecond)
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte("benchmark response"))
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := newMockCache()
	transport := NewTransport(cache)
	transport.EnableDeduplication = enableDedup
	client := http.Client{Transport: transport}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(concurrency)

		for j := 0; j < concurrency; j++ {
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest(methodGET, server.URL+"/endpoint", nil)
				resp, err := client.Do(req)
				if err != nil {
					b.Errorf("request failed: %v", err)
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}()
		}

		wg.Wait()
		// Clear cache between iterations
		cache.mu.Lock()
		cache.items = make(map[string][]byte)
		cache.mu.Unlock()
	}
}

// BenchmarkDeduplicationServerLoad measures the server load reduction with deduplication
func BenchmarkDeduplicationServerLoad(b *testing.B) {
	testCases := []struct {
		name       string
		enableDup  bool
		concurrent int
	}{
		{"Enabled_C10", true, 10},
		{"Disabled_C10", false, 10},
		{"Enabled_C50", true, 50},
		{"Disabled_C50", false, 50},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			requestCount := atomic.Int32{}
			mux := http.NewServeMux()
			mux.HandleFunc("/load", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)
				time.Sleep(10 * time.Millisecond)
				w.Header().Set("Cache-Control", "max-age=3600")
				_, _ = w.Write([]byte("load test"))
			}))

			server := httptest.NewServer(mux)
			defer server.Close()

			cache := newMockCache()
			transport := NewTransport(cache)
			transport.EnableDeduplication = tc.enableDup
			client := http.Client{Transport: transport}

			requestCount.Store(0)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				wg.Add(tc.concurrent)

				for j := 0; j < tc.concurrent; j++ {
					go func() {
						defer wg.Done()
						req, _ := http.NewRequest(methodGET, server.URL+"/load", nil)
						resp, err := client.Do(req)
						if err != nil {
							return
						}
						_, _ = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}()
				}

				wg.Wait()
				cache.mu.Lock()
				cache.items = make(map[string][]byte)
				cache.mu.Unlock()
			}

			b.StopTimer()
			b.ReportMetric(float64(requestCount.Load())/float64(b.N*tc.concurrent), "requests/op")
		})
	}
}
