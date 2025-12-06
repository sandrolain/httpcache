// Package prewarmer provides cache prewarming and prefetching capabilities
// for httpcache. It allows proactive cache population before requests arrive,
// reducing initial latency for known critical resources.
package prewarmer

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Prewarmer provides methods for proactively loading URLs into the cache.
type Prewarmer struct {
	client       *http.Client
	userAgent    string
	timeout      time.Duration
	forceRefresh bool
}

// Config holds configuration options for the Prewarmer.
type Config struct {
	// Client is the HTTP client to use for requests.
	// It should be configured with an httpcache Transport to enable caching.
	// Required.
	Client *http.Client

	// UserAgent is the User-Agent string to use for requests.
	// Optional - defaults to "httpcache-prewarmer/1.0".
	UserAgent string

	// Timeout is the timeout for each individual request.
	// Optional - defaults to 30 seconds.
	Timeout time.Duration

	// ForceRefresh forces cache refresh even if content is already cached.
	// When true, adds Cache-Control: no-cache header to bypass cache.
	// Optional - defaults to false.
	ForceRefresh bool
}

// Result represents the result of a prewarm operation.
type Result struct {
	// URL is the URL that was processed.
	URL string

	// Success indicates whether the prewarm was successful.
	Success bool

	// StatusCode is the HTTP status code returned.
	StatusCode int

	// Duration is how long the request took.
	Duration time.Duration

	// Size is the response body size in bytes.
	Size int64

	// Error is the error if the request failed.
	Error error

	// FromCache indicates if the response came from cache.
	FromCache bool
}

// Stats contains aggregate statistics from a prewarm operation.
type Stats struct {
	// Total is the total number of URLs processed.
	Total int

	// Successful is the number of successful requests.
	Successful int

	// Failed is the number of failed requests.
	Failed int

	// FromCache is the number of responses served from cache.
	FromCache int

	// TotalDuration is the total elapsed time for the operation.
	TotalDuration time.Duration

	// TotalBytes is the total bytes downloaded.
	TotalBytes int64

	// Errors contains all errors encountered.
	Errors []error
}

// ProgressCallback is called after each URL is processed.
type ProgressCallback func(result *Result, completed, total int)

// New creates a new Prewarmer with the given configuration.
func New(config Config) (*Prewarmer, error) {
	if config.Client == nil {
		return nil, errors.New("prewarmer: client is required")
	}

	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = "httpcache-prewarmer/1.0"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Prewarmer{
		client:       config.Client,
		userAgent:    userAgent,
		timeout:      timeout,
		forceRefresh: config.ForceRefresh,
	}, nil
}

// Prewarm loads the given URLs into the cache sequentially.
// It returns aggregate statistics about the operation.
func (p *Prewarmer) Prewarm(ctx context.Context, urls []string) (*Stats, error) {
	return p.PrewarmWithCallback(ctx, urls, nil)
}

// PrewarmWithCallback loads URLs sequentially and calls the callback after each.
func (p *Prewarmer) PrewarmWithCallback(ctx context.Context, urls []string, callback ProgressCallback) (*Stats, error) {
	stats := &Stats{
		Total: len(urls),
	}
	startTime := time.Now()

	for i, url := range urls {
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		default:
		}

		result := p.fetchURL(ctx, url)

		if result.Success {
			stats.Successful++
			stats.TotalBytes += result.Size
			if result.FromCache {
				stats.FromCache++
			}
		} else {
			stats.Failed++
			if result.Error != nil {
				stats.Errors = append(stats.Errors, result.Error)
			}
		}

		if callback != nil {
			callback(result, i+1, len(urls))
		}
	}

	stats.TotalDuration = time.Since(startTime)
	return stats, nil
}

// PrewarmConcurrent loads URLs with controlled concurrency.
// The workers parameter specifies the number of concurrent goroutines.
func (p *Prewarmer) PrewarmConcurrent(ctx context.Context, urls []string, workers int) (*Stats, error) {
	return p.PrewarmConcurrentWithCallback(ctx, urls, workers, nil)
}

// PrewarmConcurrentWithCallback loads URLs concurrently and calls the callback after each.
// The callback is called from multiple goroutines and must be thread-safe.
func (p *Prewarmer) PrewarmConcurrentWithCallback(ctx context.Context, urls []string, workers int, callback ProgressCallback) (*Stats, error) {
	if workers <= 0 {
		workers = 1
	}

	stats := &Stats{
		Total: len(urls),
	}
	startTime := time.Now()

	// Channel for URLs to process
	urlChan := make(chan string, len(urls))
	for _, url := range urls {
		urlChan <- url
	}
	close(urlChan)

	// Channel for results
	resultChan := make(chan *Result, len(urls))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range urlChan {
				select {
				case <-ctx.Done():
					return
				default:
				}
				result := p.fetchURL(ctx, url)
				resultChan <- result
			}
		}()
	}

	// Close results channel when all workers done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var mu sync.Mutex
	var completed int32

	for result := range resultChan {
		mu.Lock()
		if result.Success {
			stats.Successful++
			stats.TotalBytes += result.Size
			if result.FromCache {
				stats.FromCache++
			}
		} else {
			stats.Failed++
			if result.Error != nil {
				stats.Errors = append(stats.Errors, result.Error)
			}
		}
		completed := atomic.AddInt32(&completed, 1)
		mu.Unlock()

		if callback != nil {
			callback(result, int(completed), len(urls))
		}
	}

	stats.TotalDuration = time.Since(startTime)
	return stats, nil
}

// PrewarmFromSitemap parses an XML sitemap and prewarms all URLs found.
func (p *Prewarmer) PrewarmFromSitemap(ctx context.Context, sitemapURL string) (*Stats, error) {
	return p.PrewarmFromSitemapWithCallback(ctx, sitemapURL, 1, nil)
}

// PrewarmFromSitemapConcurrent parses an XML sitemap and prewarms with concurrency.
func (p *Prewarmer) PrewarmFromSitemapConcurrent(ctx context.Context, sitemapURL string, workers int) (*Stats, error) {
	return p.PrewarmFromSitemapWithCallback(ctx, sitemapURL, workers, nil)
}

// PrewarmFromSitemapWithCallback parses a sitemap and prewarms with callback.
func (p *Prewarmer) PrewarmFromSitemapWithCallback(ctx context.Context, sitemapURL string, workers int, callback ProgressCallback) (*Stats, error) {
	urls, err := p.parseSitemap(ctx, sitemapURL)
	if err != nil {
		return nil, fmt.Errorf("prewarmer: failed to parse sitemap: %w", err)
	}

	if workers <= 1 {
		return p.PrewarmWithCallback(ctx, urls, callback)
	}
	return p.PrewarmConcurrentWithCallback(ctx, urls, workers, callback)
}

// fetchURL performs a single HTTP GET request and returns the result.
func (p *Prewarmer) fetchURL(ctx context.Context, url string) *Result {
	result := &Result{
		URL: url,
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	startTime := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = fmt.Errorf("failed to create request: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	req.Header.Set("User-Agent", p.userAgent)

	if p.forceRefresh {
		req.Header.Set("Cache-Control", "no-cache")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("request failed: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}
	defer resp.Body.Close() //nolint:errcheck // best effort cleanup

	// Read body to ensure it gets cached
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Errorf("failed to read body: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	result.Duration = time.Since(startTime)
	result.StatusCode = resp.StatusCode
	result.Size = int64(len(body))
	result.Success = resp.StatusCode >= 200 && resp.StatusCode < 400
	result.FromCache = resp.Header.Get("X-From-Cache") == "1"

	if !result.Success {
		result.Error = fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return result
}

// Sitemap represents an XML sitemap structure.
type Sitemap struct {
	XMLName xml.Name     `xml:"urlset"`
	URLs    []SitemapURL `xml:"url"`
}

// SitemapURL represents a single URL entry in a sitemap.
type SitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

// SitemapIndex represents an XML sitemap index structure.
type SitemapIndex struct {
	XMLName  xml.Name          `xml:"sitemapindex"`
	Sitemaps []SitemapLocation `xml:"sitemap"`
}

// SitemapLocation represents a sitemap reference in a sitemap index.
type SitemapLocation struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

// parseSitemap fetches and parses an XML sitemap, returning all URLs.
// It supports both regular sitemaps and sitemap indexes.
func (p *Prewarmer) parseSitemap(ctx context.Context, sitemapURL string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", p.userAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // best effort cleanup

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sitemap returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Try parsing as sitemap index first
	var sitemapIndex SitemapIndex
	if err := xml.Unmarshal(body, &sitemapIndex); err == nil && len(sitemapIndex.Sitemaps) > 0 {
		// It's a sitemap index - recursively parse all referenced sitemaps
		var allURLs []string
		for _, sm := range sitemapIndex.Sitemaps {
			urls, err := p.parseSitemap(ctx, sm.Loc)
			if err != nil {
				// Log error but continue with other sitemaps
				continue
			}
			allURLs = append(allURLs, urls...)
		}
		return allURLs, nil
	}

	// Parse as regular sitemap
	var sitemap Sitemap
	if err := xml.Unmarshal(body, &sitemap); err != nil {
		return nil, fmt.Errorf("failed to parse sitemap XML: %w", err)
	}

	urls := make([]string, 0, len(sitemap.URLs))
	for _, u := range sitemap.URLs {
		loc := strings.TrimSpace(u.Loc)
		if loc != "" {
			urls = append(urls, loc)
		}
	}

	return urls, nil
}
