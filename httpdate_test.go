package httpcache

import (
	"net/http"
	"testing"
	"time"
)

func TestDateAcceptsAllHTTPDateForms(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "IMF-fixdate", value: "Sun, 06 Nov 1994 08:49:37 GMT"},
		{name: "RFC 850", value: "Sunday, 06-Nov-94 08:49:37 GMT"},
		{name: "asctime", value: "Sun Nov  6 08:49:37 1994"},
		{name: "lowercase GMT", value: "Sun, 06 Nov 1994 08:49:37 gmt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Date(http.Header{"Date": {tt.value}})
			if err != nil {
				t.Fatalf("Date() rejected valid HTTP-date %q: %v", tt.value, err)
			}
			want := time.Date(1994, time.November, 6, 8, 49, 37, 0, time.UTC)
			if !got.Equal(want) {
				t.Fatalf("Date() = %s, want %s", got, want)
			}
		})
	}
}

func TestDateRejectsNonGMTZoneAbbreviations(t *testing.T) {
	for _, value := range []string{
		"Sun, 06 Nov 1994 08:49:37 PST",
		"Sunday, 06-Nov-94 08:49:37 PST",
	} {
		if _, err := Date(http.Header{"Date": {value}}); err == nil {
			t.Errorf("Date() accepted non-GMT timezone in %q", value)
		}
	}
}

func TestExpiresAcceptsObsoleteHTTPDate(t *testing.T) {
	date := time.Date(1994, time.November, 6, 8, 49, 37, 0, time.UTC)
	header := http.Header{"Expires": {"Sunday, 06-Nov-94 09:49:37 GMT"}}
	if got := calculateLifetime(cacheControl{}, header, date, false); got != time.Hour {
		t.Fatalf("calculateLifetime() = %s, want 1h", got)
	}
}
