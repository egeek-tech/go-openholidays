// Package openholidays — cacheTransport: the RoundTripper that serves
// previously-fetched response bytes from a Cache backend.
//
// Chain placement (D-89): cacheTransport sits BETWEEN hookTransport
// (outermost, added by Plan 05) and loggingTransport (Phase 2). A cache
// hit short-circuits the chain at this layer — neither loggingTransport
// nor headerTransport sees a cache hit's "request" because no underlying
// round trip occurs. hookTransport above DOES observe cache hits via the
// synthetic *http.Response constructed here, and consumers can detect
// the hit branch by reading CacheHitContextKey from the request context.
//
// Cacheability gate (D-83): only /Countries, /Languages, /Subdivisions
// are cacheable. Non-allowed paths bypass the cache entirely so holiday
// endpoints (/PublicHolidays, /SchoolHolidays) hit the network on every
// call (RESIL-07 — temporal-data trap).
//
// Cache contents (D-83 + Pitfall CACHE-1): raw response bytes are cached
// ONLY when err == nil AND resp.StatusCode == 200. Error and 5xx
// responses are never cached. Bodies are read via
// io.LimitReader(resp.Body, maxResponseBytes+1); the original body is
// drained-and-closed (Pitfall HTTP-3) before being replaced with an
// io.NopCloser over the buffer so the downstream decoder still sees a
// fresh stream.
//
// Cache key encoding (D-82): method + " " + URL.Path + "?" +
// URL.Query().Encode(). Host is intentionally excluded — per-Client cache
// isolation (cache lives on *Client, not globally) is the architectural
// mitigation for Pitfall CACHE-2.
//
// Strict-decoding composition (D-93): the decoder runs in doJSONGet
// AFTER cacheTransport returns. Cached bytes therefore flow through the
// same strict gate as fresh bytes on every read. A strict-mode client
// surfaces a schema-drift response that landed in cache before the
// upstream added a new field — intentional, not a bug.
//
// No init() and no package-level vars EXCEPT the documented exported
// context-key var CacheHitContextKey. The CLIENT-10 AST audit in
// internal_test.go::allowedVars is updated to allow this single
// addition (DEVIATION from CONTEXT.md D-97 step 6 — see plan
// <deviations> for rationale).

package openholidays

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

// cacheHitKeyType is the unexported context-key type backing
// CacheHitContextKey. Defining the type unexported keeps the public
// surface minimal: only the CacheHitContextKey value is exported. This
// is the standard Go context-key idiom (private type, exported var).
type cacheHitKeyType struct{}

// CacheHitContextKey is the context-value key set by cacheTransport when
// a response is served from cache. Consumers can detect cache hits inside
// their WithRequestHook callback via
// req.Context().Value(openholidays.CacheHitContextKey). The value, when
// present, is the untyped boolean true; on cache miss the key is
// absent (Value returns nil).
//
// The signal is one-way — there is no corresponding key for cache misses,
// and the absence of CacheHitContextKey in a request context is the
// documented miss signal (<specifics> 2).
var CacheHitContextKey = cacheHitKeyType{}

// cacheTransport is the cache-layer http.RoundTripper. It consults the
// configured Cache for allowlisted paths and either returns a synthetic
// 200 OK response built from cached bytes (cache hit) or forwards to the
// next RoundTripper and caches the successful response (cache miss).
// Non-allowlisted paths bypass entirely (D-83).
//
// Thread-safety: the Cache implementation is responsible for its own
// internal synchronization (CLIENT-07). cacheTransport itself is
// stateless and safe for concurrent use across goroutines.
type cacheTransport struct {
	cache         Cache
	cacheablePath func(path string) bool
	next          http.RoundTripper
}

// isCacheablePath returns true iff path is in the D-83 exact-match
// allow-list: /Countries, /Languages, /Subdivisions. The OpenHolidays API
// uses query parameters (not subpaths) for variation, so exact equality
// is operationally equivalent to prefix-match while being simpler and
// safer (a future /Countries/PL endpoint would NOT be cached without an
// explicit allow-list update).
func isCacheablePath(path string) bool {
	switch path {
	case "/Countries", "/Languages", "/Subdivisions":
		return true
	}
	return false
}

// cacheKey encodes the request into the D-82 cache key:
// req.Method + " " + req.URL.Path + "?" + req.URL.Query().Encode(). The
// query encoding is deterministic (stdlib sorts keys) so two requests
// with the same logical parameters produce identical cache keys
// regardless of source ordering.
func cacheKey(req *http.Request) string {
	return req.Method + " " + req.URL.Path + "?" + req.URL.Query().Encode()
}

// RoundTrip implements http.RoundTripper. The branches:
//
//  1. Non-allowlisted path → forward to next, return unchanged.
//  2. Cache hit → return a synthetic *http.Response with the cached bytes
//     and CacheHitContextKey == true in resp.Request.Context().
//  3. Cache miss → forward to next; on err != nil or status != 200,
//     return resp/err untouched (Pitfall CACHE-1 — never cache errors).
//     On success, read the body through LimitReader(maxResponseBytes+1),
//     drain-and-close the original body (Pitfall HTTP-3), cache the
//     bytes if within the cap, and replace resp.Body with a NopCloser
//     over the buffer so the downstream decoder sees a fresh stream.
func (t *cacheTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.cacheablePath(req.URL.Path) {
		return t.next.RoundTrip(req)
	}
	key := cacheKey(req)
	if cached, ok := t.cache.Get(key); ok {
		ctxWithHit := context.WithValue(req.Context(), CacheHitContextKey, true)
		synth := &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        make(http.Header),
			Body:          io.NopCloser(bytes.NewReader(cached)),
			ContentLength: int64(len(cached)),
			Request:       req.WithContext(ctxWithHit),
		}
		return synth, nil
	}
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return resp, err
	}
	// Pitfall CACHE-1 / D-83 success path: read body through LimitReader,
	// drain-and-close the original body, then replace with a NopCloser
	// over the buffered bytes so the downstream decoder works unchanged.
	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	buf, readErr := io.ReadAll(limited)
	// Pitfall HTTP-3: drain any remaining bytes past the cap so the
	// underlying connection can return to the keep-alive pool, then
	// close. LimitReader does not advance the underlying reader past
	// its cap — drain defensively.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	// Only cache when within the documented cap. A buf longer than
	// maxResponseBytes (the LimitReader returned maxResponseBytes+1
	// bytes) indicates upstream truncation territory and MUST NOT be
	// cached — the downstream decoder's mid-truncation gate would
	// reject it on every read.
	if len(buf) <= maxResponseBytes {
		t.cache.Put(key, buf)
	}
	resp.Body = io.NopCloser(bytes.NewReader(buf))
	resp.ContentLength = int64(len(buf))
	return resp, nil
}
