package httpcache

import (
	"net/http"
	"testing"
	"time"
)

// TestCanStaleOnError tests the canStaleOnError function
func TestCanStaleOnError(t *testing.T) {
	resetTest()

	tests := []struct {
		name        string
		respHeaders http.Header
		reqHeaders  http.Header
		want        bool
	}{
		{
			name: "response with stale-if-error no value",
			respHeaders: http.Header{
				"Cache-Control": []string{"stale-if-error"},
				"Date":          []string{time.Now().Format(time.RFC1123)},
			},
			reqHeaders: http.Header{},
			want:       true,
		},
		{
			name: "response with stale-if-error with valid duration",
			respHeaders: http.Header{
				"Cache-Control": []string{"stale-if-error=60"},
				"Date":          []string{time.Now().Format(time.RFC1123)},
			},
			reqHeaders: http.Header{},
			want:       true,
		},
		{
			name: "response with stale-if-error with invalid duration",
			respHeaders: http.Header{
				"Cache-Control": []string{"stale-if-error=invalid"},
				"Date":          []string{time.Now().Format(time.RFC1123)},
			},
			reqHeaders: http.Header{},
			want:       false,
		},
		{
			name: "request with stale-if-error no value",
			respHeaders: http.Header{
				"Date": []string{time.Now().Format(time.RFC1123)},
			},
			reqHeaders: http.Header{
				"Cache-Control": []string{"stale-if-error"},
			},
			want: true,
		},
		{
			name: "request with stale-if-error with valid duration",
			respHeaders: http.Header{
				"Date": []string{time.Now().Format(time.RFC1123)},
			},
			reqHeaders: http.Header{
				"Cache-Control": []string{"stale-if-error=60"},
			},
			want: true,
		},
		{
			name: "request with stale-if-error with invalid duration",
			respHeaders: http.Header{
				"Date": []string{time.Now().Format(time.RFC1123)},
			},
			reqHeaders: http.Header{
				"Cache-Control": []string{"stale-if-error=invalid"},
			},
			want: false,
		},
		{
			name: "stale-if-error expired",
			respHeaders: http.Header{
				"Cache-Control": []string{"stale-if-error=5"},
				"Date":          []string{time.Now().Add(-10 * time.Second).Format(time.RFC1123)},
			},
			reqHeaders: http.Header{},
			want:       false,
		},
		{
			name:        "no stale-if-error",
			respHeaders: http.Header{},
			reqHeaders:  http.Header{},
			want:        false,
		},
		{
			name: "no date header",
			respHeaders: http.Header{
				"Cache-Control": []string{"stale-if-error=60"},
			},
			reqHeaders: http.Header{},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canStaleOnError(tt.respHeaders, tt.reqHeaders)
			if got != tt.want {
				t.Errorf("canStaleOnError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetFreshnessEdgeCases tests edge cases in getFreshness
func TestGetFreshnessEdgeCases(t *testing.T) {
	resetTest()

	t.Run("only-if-cached returns fresh", func(t *testing.T) {
		respHeaders := http.Header{
			"Date": []string{time.Now().Format(time.RFC1123)},
		}
		reqHeaders := http.Header{
			"Cache-Control": []string{"only-if-cached"},
		}
		if got := getFreshness(respHeaders, reqHeaders); got != fresh {
			t.Errorf("getFreshness() = %v, want %v", got, fresh)
		}
	})

	t.Run("invalid max-age returns zero duration", func(t *testing.T) {
		respHeaders := http.Header{
			"Cache-Control": []string{"max-age=invalid"},
			"Date":          []string{time.Now().Format(time.RFC1123)},
		}
		reqHeaders := http.Header{}
		if got := getFreshness(respHeaders, reqHeaders); got != stale {
			t.Errorf("getFreshness() = %v, want %v", got, stale)
		}
	})

	t.Run("invalid expires header", func(t *testing.T) {
		respHeaders := http.Header{
			"Expires": []string{"invalid-date"},
			"Date":    []string{time.Now().Format(time.RFC1123)},
		}
		reqHeaders := http.Header{}
		if got := getFreshness(respHeaders, reqHeaders); got != stale {
			t.Errorf("getFreshness() = %v, want %v", got, stale)
		}
	})

	t.Run("request max-age with invalid value", func(t *testing.T) {
		respHeaders := http.Header{
			"Cache-Control": []string{"max-age=3600"},
			"Date":          []string{time.Now().Format(time.RFC1123)},
		}
		reqHeaders := http.Header{
			"Cache-Control": []string{"max-age=invalid"},
		}
		// RFC 9111: Invalid directive should be ignored, response should be fresh
		// because response has valid max-age=3600
		if got := getFreshness(respHeaders, reqHeaders); got != fresh {
			t.Errorf("getFreshness() = %v, want %v (invalid request max-age ignored)", got, fresh)
		}
	})

	t.Run("min-fresh with invalid value is ignored", func(t *testing.T) {
		respHeaders := http.Header{
			"Cache-Control": []string{"max-age=3600"},
			"Date":          []string{time.Now().Format(time.RFC1123)},
		}
		reqHeaders := http.Header{
			"Cache-Control": []string{"min-fresh=invalid"},
		}
		// Should still be fresh because invalid min-fresh is ignored
		if got := getFreshness(respHeaders, reqHeaders); got != fresh {
			t.Errorf("getFreshness() = %v, want %v", got, fresh)
		}
	})

	t.Run("max-stale with invalid value is ignored", func(t *testing.T) {
		clock = &fakeClock{elapsed: 7200 * time.Second}
		defer func() { clock = &realClock{} }()

		respHeaders := http.Header{
			"Cache-Control": []string{"max-age=3600"},
			"Date":          []string{time.Now().Format(time.RFC1123)},
		}
		reqHeaders := http.Header{
			"Cache-Control": []string{"max-stale=invalid"},
		}
		// Should be stale because invalid max-stale is ignored
		if got := getFreshness(respHeaders, reqHeaders); got != stale {
			t.Errorf("getFreshness() = %v, want %v", got, stale)
		}
	})
}

// TestClientFunction tests the Client helper function
func TestClientFunction(t *testing.T) {
	cache := NewMemoryCache()
	transport := NewTransport(cache)
	transport.MarkCachedResponses = true
	client := transport.Client()

	if client == nil {
		t.Fatal("Client() returned nil")
	}

	if client.Transport != transport {
		t.Fatal("Client transport doesn't match")
	}
}

// TestNewGatewayTimeoutResponse tests the newGatewayTimeoutResponse function
func TestNewGatewayTimeoutResponse(t *testing.T) {
	req, err := http.NewRequest(methodGET, "http://example.com/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp := newGatewayTimeoutResponse(req)

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusGatewayTimeout)
	}

	if resp.Request != req {
		t.Error("Response.Request doesn't match original request")
	}

	if resp.Header == nil {
		t.Error("Response.Header is nil")
	}

	if resp.Body == nil {
		t.Error("Response.Body is nil")
	}

	// Also verify it doesn't panic
	_ = newGatewayTimeoutResponse(req)
}

// TestCloneRequest tests the cloneRequest function
func TestCloneRequest(t *testing.T) {
	original, err := http.NewRequest(methodGET, "http://example.com/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	original.Header.Set("X-Test", "original")

	cloned := cloneRequest(original)

	// Test that it's a different instance
	if cloned == original {
		t.Error("cloneRequest returned the same instance")
	}

	// Test that basic fields are copied
	if cloned.Method != original.Method {
		t.Error("Method not copied correctly")
	}

	if cloned.URL.String() != original.URL.String() {
		t.Error("URL not copied correctly")
	}

	// Test that headers are copied
	if cloned.Header.Get("X-Test") != "original" {
		t.Error("Headers not copied correctly")
	}

	// Test that modifying cloned headers doesn't affect original
	cloned.Header.Set("X-Test", "cloned")
	if original.Header.Get("X-Test") == "cloned" {
		t.Error("Modifying cloned headers affected original")
	}
}
