package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidatorsMatchFor304(t *testing.T) {
	tests := []struct {
		name    string
		stored  http.Header
		new     http.Header
		matches bool
	}{
		{name: "same strong entity tag", stored: http.Header{"Etag": {`"a"`}}, new: http.Header{"Etag": {`"a"`}}, matches: true},
		{name: "different strong entity tag", stored: http.Header{"Etag": {`"a"`}}, new: http.Header{"Etag": {`"b"`}}},
		{name: "weak tag corresponds to strong stored tag", stored: http.Header{"Etag": {`"a"`}}, new: http.Header{"Etag": {`W/"a"`}}, matches: true},
		{name: "strong tag does not match weak stored tag", stored: http.Header{"Etag": {`W/"a"`}}, new: http.Header{"Etag": {`"a"`}}},
		{name: "matching Last-Modified", stored: http.Header{"Last-Modified": {"Wed, 21 Oct 2015 07:28:00 GMT"}}, new: http.Header{"Last-Modified": {"Wed, 21 Oct 2015 07:28:00 GMT"}}, matches: true},
		{name: "no validators on either response", stored: http.Header{}, new: http.Header{}, matches: true},
		{name: "no new validator but stored has ETag", stored: http.Header{"Etag": {`"a"`}}, new: http.Header{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validatorsMatchFor304(tt.stored, tt.new); got != tt.matches {
				t.Fatalf("validatorsMatchFor304() = %v, want %v", got, tt.matches)
			}
		})
	}
}

func TestAddValidatorsToRequestPreservesClientConditionals(t *testing.T) {
	stored := &http.Response{Header: http.Header{
		"Etag":          {`"stored"`},
		"Last-Modified": {"Wed, 21 Oct 2015 07:28:00 GMT"},
	}}
	request, err := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-None-Match", `"client"`)
	request.Header.Set("If-Modified-Since", "Wed, 21 Oct 2015 07:28:00 GMT")

	got := addValidatorsToRequest(request, stored)
	if got.Header.Get("If-None-Match") != `"client"` {
		t.Fatalf("If-None-Match = %q, want client value", got.Header.Get("If-None-Match"))
	}
	if got.Header.Get("If-Modified-Since") != "Wed, 21 Oct 2015 07:28:00 GMT" {
		t.Fatalf("If-Modified-Since = %q, want client value", got.Header.Get("If-Modified-Since"))
	}
}

func TestMismatched304RetriesWithoutCacheValidator(t *testing.T) {
	var clock fakeClock
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			w.Header().Set("Cache-Control", "max-age=1")
			w.Header().Set("ETag", `"stored"`)
			_, _ = io.WriteString(w, "old representation")
		case 2:
			if r.Header.Get("If-None-Match") != `"stored"` {
				t.Errorf("validation If-None-Match = %q, want stored validator", r.Header.Get("If-None-Match"))
			}
			w.Header().Set("ETag", `"different"`)
			w.WriteHeader(http.StatusNotModified)
		case 3:
			if r.Header.Get("If-None-Match") != "" {
				t.Errorf("retry If-None-Match = %q, want no validator", r.Header.Get("If-None-Match"))
			}
			w.Header().Set("Cache-Control", "max-age=60")
			w.Header().Set("ETag", `"different"`)
			_, _ = io.WriteString(w, "new representation")
		}
	}))
	defer server.Close()

	transport := newMockCacheTransportWithClock(&clock)
	client := &http.Client{Transport: transport}
	first, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, first.Body)
	first.Body.Close()
	clock.elapsed = 2 * time.Second

	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "new representation" {
		t.Fatalf("response = %d %q, want 200 with new representation", response.StatusCode, body)
	}
	if requests != 3 {
		t.Fatalf("origin requests = %d, want validation plus unconditional retry (3 total)", requests)
	}
}

func TestClientValidatorIsForwardedWithoutReplacement(t *testing.T) {
	var clock fakeClock
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("If-None-Match")
		if received == `"client"` {
			w.Header().Set("ETag", `"client"`)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "max-age=1")
		w.Header().Set("ETag", `"stored"`)
		_, _ = io.WriteString(w, "stored body")
	}))
	defer server.Close()

	transport := newMockCacheTransportWithClock(&clock)
	client := &http.Client{Transport: transport}
	first, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, first.Body)
	first.Body.Close()
	clock.elapsed = 2 * time.Second
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-None-Match", `"client"`)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if received != `"client"` {
		t.Fatalf("origin If-None-Match = %q, want client validator", received)
	}
	if response.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304 for client's condition", response.StatusCode)
	}
}
