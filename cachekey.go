// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// cacheKey returns the cache key for req.
func cacheKey(req *http.Request) string {
	key := ""
	if req.Method == http.MethodGet {
		key = req.URL.String()
	} else {
		key = req.Method + " " + req.URL.String()
	}

	if host := hostOverrideKey(req); host != "" {
		key += "|host:" + url.QueryEscape(host)
	}
	return key
}

// hostOverrideKey returns a stable authority key only when Request.Host
// overrides the authority in the target URL. An empty Host or an equivalent
// authority needs no additional scope because the URL already scopes the key.
func hostOverrideKey(req *http.Request) string {
	if req == nil || req.URL == nil || req.Host == "" {
		return ""
	}
	if normalizedAuthority(req.Host, req.URL.Scheme) == normalizedAuthority(req.URL.Host, req.URL.Scheme) {
		return ""
	}
	return normalizedAuthority(req.Host, req.URL.Scheme)
}

func normalizedAuthority(authority, scheme string) string {
	u := &url.URL{Host: authority}
	hostname := u.Hostname()
	host := strings.ToLower(hostname)
	if strings.Contains(hostname, ":") {
		// IPv6 address hex digits are case-insensitive, but the zone identifier
		// can be case-sensitive on the local network interface.
		if zone := strings.LastIndexByte(hostname, '%'); zone >= 0 {
			host = strings.ToLower(hostname[:zone]) + hostname[zone:]
		}
	}
	if host == "" {
		return strings.ToLower(authority)
	}
	port := u.Port()
	if port == "" {
		switch strings.ToLower(scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return net.JoinHostPort(host, port)
}

func encodeHeaderValues(prefix string, values []string) string {
	var encoded strings.Builder
	encoded.WriteString(prefix)
	encoded.WriteString(strconv.Itoa(len(values)))
	for _, value := range values {
		encoded.WriteByte(':')
		encoded.WriteString(strconv.Itoa(len(value)))
		encoded.WriteByte(':')
		encoded.WriteString(base64.RawURLEncoding.EncodeToString([]byte(value)))
	}
	return encoded.String()
}

// cacheKeyWithHeaders returns the cache key for req, including specified header values.
// This is used when CacheKeyHeaders is configured to differentiate cache entries
// based on request header values.
// Header values are URL-encoded to prevent collision when values contain the pipe separator.
func cacheKeyWithHeaders(req *http.Request, headers []string) string {
	key := cacheKey(req)

	// Append header values to the key if headers are specified
	if len(headers) > 0 {
		var headerParts []string
		for _, header := range headers {
			canonicalHeader := http.CanonicalHeaderKey(header)
			if canonicalHeader == "Host" {
				// Host is represented by the URL authority, or by hostOverrideKey
				// when Request.Host differs from that authority.
				continue
			}
			values := req.Header.Values(canonicalHeader)
			if len(values) == 1 && values[0] != "" {
				// URL-encode the value to prevent collisions when values contain pipe separator
				encodedValue := url.QueryEscape(values[0])
				headerParts = append(headerParts, canonicalHeader+":"+encodedValue)
			} else if len(values) > 0 {
				// Preserve explicit empty and repeated field values without conflating
				// them with absence, a single combined line, or another line sequence.
				encodedValues := encodeHeaderValues(cacheKeyHeaderValuesPrefix, values)
				headerParts = append(headerParts, canonicalHeader+":"+encodedValues)
			}
		}
		if len(headerParts) > 0 {
			// Sort header parts to ensure consistent key generation
			sort.Strings(headerParts)
			key = key + "|" + strings.Join(headerParts, "|")
		}
	}

	return key
}

const cacheKeyHeaderValuesPrefix = "!header-values-v1!"
