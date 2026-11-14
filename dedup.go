package httpcache

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// dedupGroup coalesces equivalent requests while allowing each caller to stop
// waiting independently. The origin operation is canceled after its last
// participant leaves.
type dedupGroup struct {
	mu      sync.Mutex
	flights map[string]*dedupFlight
}

type dedupFlight struct {
	key           string
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	refs          int
	finished      bool
	bodyHandedOff bool
	creator       *struct{ id byte }
	response      *http.Response
	body          []byte
	err           error
	oversized     bool
	noShare       bool
}

// do runs fn once for concurrent requests with the same key. A bounded probe
// makes small bodies replayable; oversized bodies remain streaming for the
// creator while joining callers retry independently.
func (g *dedupGroup) do(req *http.Request, key string, maxSize int64, fn func(context.Context, *http.Request) (*http.Response, error)) (*http.Response, bool, error) {
	if err := req.Context().Err(); err != nil {
		return nil, false, err
	}
	participant := &struct{ id byte }{}

	g.mu.Lock()
	if g.flights == nil {
		g.flights = make(map[string]*dedupFlight)
	}
	flight := g.flights[key]
	creator := flight == nil
	if creator {
		baseCtx := context.WithoutCancel(req.Context())
		flightCtx, cancel := context.WithCancel(baseCtx)
		flight = &dedupFlight{key: key, ctx: flightCtx, cancel: cancel, done: make(chan struct{}), creator: participant, refs: 1}
		g.flights[key] = flight
	} else {
		flight.refs++
	}
	g.mu.Unlock()

	if creator {
		request := req.Clone(flight.ctx)
		go g.run(key, flight, request, maxSize, fn)
	}

	select {
	case <-req.Context().Done():
		g.release(flight)
		return nil, false, req.Context().Err()
	case <-flight.done:
		if err := req.Context().Err(); err != nil {
			g.release(flight)
			return nil, false, err
		}
	}

	if flight.err != nil {
		g.release(flight)
		return nil, false, flight.err
	}
	if flight.oversized || flight.noShare {
		if participant != flight.creator {
			g.release(flight)
			return nil, false, errDedupRetry
		}
		resp := flight.response
		if resp == nil || resp.Body == nil {
			g.release(flight)
			return resp, false, nil
		}
		g.mu.Lock()
		flight.bodyHandedOff = true
		g.mu.Unlock()
		ownedResp := new(http.Response)
		*ownedResp = *resp
		ownedResp.Header = resp.Header.Clone()
		// The original Trailer map is populated as the streaming body reaches EOF.
		ownedResp.Trailer = resp.Trailer
		ownedResp.Request = req
		ownedResp.Body = newDedupLeaseBody(resp.Body, req.Context(), func() { g.release(flight) })
		return ownedResp, false, nil
	}

	g.release(flight)
	return cloneDedupResponse(flight.response, req), !creator, nil
}

var errDedupRetry = dedupRetryError{}

type dedupRetryError struct{}

func (dedupRetryError) Error() string { return "deduplication requires an independent request" }

func (g *dedupGroup) run(key string, flight *dedupFlight, req *http.Request, maxSize int64, fn func(context.Context, *http.Request) (*http.Response, error)) {
	resp, err := fn(flight.ctx, req)
	if err == nil && resp != nil && flight.ctx.Err() != nil {
		closeDedupBody(resp.Body)
		resp = nil
		err = flight.ctx.Err()
	}
	if err == nil && resp != nil {
		resp, flight.body, flight.oversized, flight.noShare, err = probeDedupResponse(resp, maxSize)
	}

	g.mu.Lock()
	flight.response, flight.err = resp, err
	flight.finished = true
	if g.flights[key] == flight {
		delete(g.flights, key)
	}
	close(flight.done)
	refs := flight.refs
	g.mu.Unlock()

	if err != nil || !flight.oversized {
		flight.cancel()
	}
	if err != nil && resp != nil && resp.Body != nil {
		closeDedupBody(resp.Body)
	}
	if err == nil && refs == 0 && resp != nil && resp.Body != nil {
		closeDedupBody(resp.Body)
		flight.cancel()
	}
}

func (g *dedupGroup) release(flight *dedupFlight) {
	g.mu.Lock()
	flight.refs--
	last := flight.refs == 0
	resp := flight.response
	finished := flight.finished
	if last && g.flights[flight.key] == flight {
		delete(g.flights, flight.key)
	}
	g.mu.Unlock()
	if last {
		flight.cancel()
		if finished && flight.oversized && !flight.bodyHandedOff && resp != nil && resp.Body != nil {
			closeDedupBody(resp.Body)
		}
	}
}

// probeDedupResponse buffers at most maxSize+1 bytes. A zero limit explicitly
// retains the existing unlimited-sharing behavior.
func probeDedupResponse(resp *http.Response, maxSize int64) (*http.Response, []byte, bool, bool, error) {
	if resp.Body == nil || resp.Body == http.NoBody {
		resp.Body = http.NoBody
		return resp, nil, false, hasVaryStar(resp.Header), nil
	}
	if maxSize > 0 && resp.ContentLength > maxSize {
		return resp, nil, true, false, nil
	}
	var body []byte
	var err error
	if maxSize > 0 {
		limit := maxSize
		if maxSize < int64(^uint64(0)>>1) {
			limit++
		}
		body, err = io.ReadAll(io.LimitReader(resp.Body, limit))
	} else {
		body, err = io.ReadAll(resp.Body)
	}
	if err != nil {
		closeDedupBody(resp.Body)
		return nil, nil, false, false, err
	}
	if maxSize > 0 && int64(len(body)) > maxSize {
		resp.Body = &dedupPrefixBody{Reader: io.MultiReader(bytes.NewReader(body), resp.Body), closer: resp.Body}
		return resp, nil, true, false, nil
	}
	closeDedupBody(resp.Body)
	resp.Body = &dedupBufferedBody{Reader: bytes.NewReader(body), data: body}
	return resp, body, false, hasVaryStar(resp.Header), nil
}

func hasVaryStar(header http.Header) bool {
	for _, value := range header.Values("Vary") {
		for _, field := range strings.Split(value, ",") {
			if strings.TrimSpace(field) == "*" {
				return true
			}
		}
	}
	return false
}

func cloneDedupResponse(resp *http.Response, req *http.Request) *http.Response {
	if resp == nil {
		return nil
	}
	clone := new(http.Response)
	*clone = *resp
	clone.Request = req
	clone.Header = resp.Header.Clone()
	clone.Trailer = resp.Trailer.Clone()
	if resp.Body != nil {
		clone.Body = io.NopCloser(bytes.NewReader(dedupBodyBytes(resp.Body)))
	}
	return clone
}

func dedupBodyBytes(body io.ReadCloser) []byte {
	if buffered, ok := body.(*dedupBufferedBody); ok {
		return buffered.data
	}
	return nil
}

type dedupBufferedBody struct {
	*bytes.Reader
	data []byte
}

func (b *dedupBufferedBody) Close() error { return nil }

type dedupPrefixBody struct {
	io.Reader
	closer io.Closer
}

func (b *dedupPrefixBody) Close() error { return b.closer.Close() }

func closeDedupBody(body io.Closer) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil {
		slog.Default().Warn("failed to close discarded deduplicated response body", "error", err)
	}
}

type dedupLeaseBody struct {
	io.ReadCloser
	release    func()
	finishOnce sync.Once
	closeOnce  sync.Once
	closeErr   error
	stop       chan struct{}
}

func newDedupLeaseBody(body io.ReadCloser, ctx context.Context, release func()) *dedupLeaseBody {
	leased := &dedupLeaseBody{ReadCloser: body, release: release, stop: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			if err := leased.Close(); err != nil {
				slog.Default().Warn("failed to close canceled deduplicated response body", "error", err)
			}
		case <-leased.stop:
		}
	}()
	return leased
}

func (b *dedupLeaseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		closeErr := b.Close()
		if err == io.EOF && closeErr != nil {
			return n, closeErr
		}
	}
	return n, err
}

func (b *dedupLeaseBody) Close() error {
	b.closeOnce.Do(func() {
		b.closeErr = b.ReadCloser.Close()
		b.finish()
	})
	return b.closeErr
}

func (b *dedupLeaseBody) finish() {
	b.finishOnce.Do(func() {
		close(b.stop)
		b.release()
	})
}

// deduplicationKey serializes request identity without relying on cache Vary
// metadata, which is only available after a response arrives. Length prefixes
// make the encoding unambiguous even when values contain separators.
func deduplicationKey(cacheKey string, req *http.Request) string {
	var b bytes.Buffer
	writeDedupPart(&b, cacheKey)
	writeDedupPart(&b, req.Method)
	writeDedupPart(&b, req.Host)
	names := make([]string, 0, len(req.Header))
	for name := range req.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	writeDedupPart(&b, string(binary.AppendUvarint(nil, uint64(len(names)))))
	for _, name := range names {
		values := req.Header[name]
		writeDedupPart(&b, name)
		writeDedupPart(&b, string(binary.AppendUvarint(nil, uint64(len(values)))))
		for _, value := range values {
			writeDedupPart(&b, value)
		}
	}
	return b.String()
}

func writeDedupPart(b *bytes.Buffer, value string) {
	var size [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(size[:], uint64(len(value)))
	b.Write(size[:n])
	b.WriteString(value)
}

func requestHasBody(req *http.Request) bool {
	return req.Body != nil && req.Body != http.NoBody
}
