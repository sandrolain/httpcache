package httpcache

import (
	"net/http"
	"strings"
)

// clientConditionalMatches reports whether a fresh stored representation
// satisfies the request's If-None-Match or If-Modified-Since precondition.
func clientConditionalMatches(req *http.Request, stored *http.Response) bool {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	if values := req.Header.Values("If-None-Match"); len(values) > 0 {
		return ifNoneMatchMatches(values, stored.Header.Values(headerETag))
	}
	if value := req.Header.Get("If-Modified-Since"); value != "" {
		return ifModifiedSinceMatches(value, stored.Header.Get(headerLastModified))
	}
	return false
}

func ifNoneMatchMatches(requestValues, storedValues []string) bool {
	storedTags := validEntityTags(storedValues)
	for _, value := range requestValues {
		for _, candidate := range splitEntityTagList(value) {
			if strings.TrimSpace(candidate) == "*" {
				return true
			}
			for _, requestTag := range validEntityTags([]string{candidate}) {
				for _, storedTag := range storedTags {
					// If-None-Match uses weak comparison for GET and HEAD.
					if requestTag.opaque == storedTag.opaque {
						return true
					}
				}
			}
		}
	}
	return false
}

func ifModifiedSinceMatches(requestValue, storedValue string) bool {
	modifiedSince, err := parseHTTPDate(requestValue)
	if err != nil {
		return false
	}
	lastModified, err := parseHTTPDate(storedValue)
	return err == nil && !lastModified.After(modifiedSince)
}

func splitEntityTagList(value string) []string {
	var tags []string
	start := 0
	quoted := false
	for i := 0; i < len(value); i++ {
		if value[i] == '"' && (i == 0 || value[i-1] != '\\') {
			quoted = !quoted
		} else if value[i] == ',' && !quoted {
			tags = append(tags, value[start:i])
			start = i + 1
		}
	}
	return append(tags, value[start:])
}

// validatorsMatchFor304 reports whether a 304 response can update a stored
// response under RFC 9111 Section 4.3.4.
func validatorsMatchFor304(stored, notModified http.Header) bool {
	newTags := validEntityTags(notModified.Values(headerETag))
	storedTags := validEntityTags(stored.Values(headerETag))

	for _, newTag := range newTags {
		if !newTag.weak {
			for _, storedTag := range storedTags {
				if !storedTag.weak && newTag.opaque == storedTag.opaque {
					return true
				}
			}
		}
	}

	// When the 304 has no strong entity tag, weak validators can identify the
	// stored representation. Last-Modified is also a weak validator.
	hasStrongTag := false
	for _, tag := range newTags {
		hasStrongTag = hasStrongTag || !tag.weak
	}
	if !hasStrongTag {
		for _, newTag := range newTags {
			for _, storedTag := range storedTags {
				if newTag.opaque == storedTag.opaque {
					return true
				}
			}
		}
		if sameHTTPDate(notModified.Get(headerLastModified), stored.Get(headerLastModified)) {
			return true
		}
	}

	// RFC 9111 permits updating without validators only when the stored
	// response also has none (this transport keeps a single selected entry).
	return len(notModified.Values(headerETag)) == 0 && notModified.Get(headerLastModified) == "" &&
		len(stored.Values(headerETag)) == 0 && stored.Get(headerLastModified) == ""
}

type entityTag struct {
	opaque string
	weak   bool
}

func validEntityTags(values []string) []entityTag {
	var tags []entityTag
	for _, value := range values {
		value = strings.TrimSpace(value)
		weak := strings.HasPrefix(value, "W/")
		if weak {
			value = value[2:]
		}
		if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
			continue
		}
		valid := true
		for i := 1; i < len(value)-1; i++ {
			c := value[i]
			if c == 0x21 || c >= 0x23 && c <= 0x7e || c >= 0x80 {
				continue
			}
			if c < 0x80 {
				valid = false
				break
			}
		}
		if valid {
			tags = append(tags, entityTag{opaque: value[1 : len(value)-1], weak: weak})
		}
	}
	return tags
}

func sameHTTPDate(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	parsedA, errA := parseHTTPDate(a)
	parsedB, errB := parseHTTPDate(b)
	return errA == nil && errB == nil && parsedA.Equal(parsedB)
}
