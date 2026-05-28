// Package openholidays — tests for the cacheTransport RoundTripper.
//
// One TestXxx per concern (Gold Rule 3 applies to exported prod functions;
// cacheTransport.RoundTrip is one method whose multiple invariants — path
// allow-list, miss-then-hit, raw-bytes key, context-key signal — are split
// across named TestXxx funcs because each maps to a distinct contract
// derived from a different decision identifier (D-82, D-83, D-88,
// <specifics> 2).
//
// roundTripperFunc is declared in transport_header_test.go (same-package
// visibility, D-50). Do NOT redeclare it.

package openholidays

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCacheTransport wires a fresh MemoryCache + counting next-handler
// for the per-test invariant under inspection.
func newTestCacheTransport(t *testing.T, body []byte, status int) (*cacheTransport, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	next := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		hits.Add(1)
		return &http.Response{
			StatusCode:    status,
			Status:        http.StatusText(status),
			Header:        make(http.Header),
			Body:          io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)),
		}, nil
	})
	nc := NewMemoryCache(time.Hour)
	t.Cleanup(func() { _ = nc.Close() })
	return &cacheTransport{
		cache:         nc,
		cacheablePath: isCacheablePath,
		next:          next,
	}, &hits
}

// newTestRequest builds an http.Request with the given path + query for the
// cacheTransport tests. Host is irrelevant because the cache key excludes
// it (D-82).
func newTestRequest(t *testing.T, path string, query url.Values) *http.Request {
	t.Helper()
	u := &url.URL{Scheme: "http", Host: "example.test", Path: path, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	require.NoError(t, err)
	return req
}

// TestCacheTransport_PathAllowlist locks the D-83 cacheability gate: only
// /Countries, /Languages, /Subdivisions are cacheable. Everything else
// bypasses the cache entirely.
func TestCacheTransport_PathAllowlist(t *testing.T) {
	t.Parallel()

	type tc struct {
		name string
		path string
		// wantHitsAfterTwoCalls expresses the contract: 1 for cacheable
		// (second call served from cache), 2 for bypass (both calls hit
		// the next handler).
		wantHitsAfterTwoCalls int32
	}
	cases := []tc{
		{name: "/Countries is cacheable", path: "/Countries", wantHitsAfterTwoCalls: 1},
		{name: "/Languages is cacheable", path: "/Languages", wantHitsAfterTwoCalls: 1},
		{name: "/Subdivisions is cacheable", path: "/Subdivisions", wantHitsAfterTwoCalls: 1},
		{name: "/PublicHolidays bypasses cache", path: "/PublicHolidays", wantHitsAfterTwoCalls: 2},
		{name: "/SchoolHolidays bypasses cache", path: "/SchoolHolidays", wantHitsAfterTwoCalls: 2},
		{
			// Exact-match policy per Plan 04 action: subpaths are NOT
			// cacheable because the OpenHolidays API only uses query
			// parameters for variation. A future API addition of
			// /Countries/PL would still bypass and could be promoted
			// in a later phase.
			name:                  "/Countries/PL is NOT cacheable (exact-match policy)",
			path:                  "/Countries/PL",
			wantHitsAfterTwoCalls: 2,
		},
		{name: "/ is NOT cacheable", path: "/", wantHitsAfterTwoCalls: 2},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tr, hits := newTestCacheTransport(t, []byte("{}"), http.StatusOK)

			req := newTestRequest(t, c.path, nil)

			resp1, err := tr.RoundTrip(req)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp1.Body)
			require.NoError(t, resp1.Body.Close())

			resp2, err := tr.RoundTrip(req)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp2.Body)
			require.NoError(t, resp2.Body.Close())

			assert.Equal(t, c.wantHitsAfterTwoCalls, hits.Load(),
				"path %s: expected %d next-handler hits across two calls", c.path, c.wantHitsAfterTwoCalls)
		})
	}
}

// TestCacheTransport_HolidayPathsBypass is the explicit RESIL-07 lock:
// holiday endpoints are NEVER cached by default. This duplicates one row
// from TestCacheTransport_PathAllowlist intentionally — duplication
// documents the RESIL-07 requirement so a `go test -run
// TestCacheTransport_HolidayPathsBypass` produces a named pass.
func TestCacheTransport_HolidayPathsBypass(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/PublicHolidays", "/SchoolHolidays"} {
		path := path
		t.Run(path+" two calls produce two next-handler hits", func(t *testing.T) {
			t.Parallel()
			tr, hits := newTestCacheTransport(t, []byte("[]"), http.StatusOK)

			for i := 0; i < 2; i++ {
				req := newTestRequest(t, path, nil)
				resp, err := tr.RoundTrip(req)
				require.NoError(t, err)
				_, _ = io.Copy(io.Discard, resp.Body)
				require.NoError(t, resp.Body.Close())
			}

			assert.Equal(t, int32(2), hits.Load(),
				"holiday endpoint %s must hit the next handler on every call (RESIL-07)", path)
		})
	}
}

// TestCacheTransport_HitMissBehavior locks the miss-then-hit cycle: first
// call is a cache miss (forwards to next, caches the bytes); second call
// is a cache hit (next NOT invoked, returns cached bytes).
//
// Also locks Pitfall CACHE-1 / D-83: 5xx responses are NEVER cached, so a
// 503-then-503 pattern produces two next-handler hits.
func TestCacheTransport_HitMissBehavior(t *testing.T) {
	t.Parallel()

	t.Run("miss-then-hit returns identical bytes and skips the next handler", func(t *testing.T) {
		t.Parallel()
		tr, hits := newTestCacheTransport(t, []byte(`[{"isoCode":"PL"}]`), http.StatusOK)

		req := newTestRequest(t, "/Countries", nil)

		resp1, err := tr.RoundTrip(req)
		require.NoError(t, err)
		body1, err := io.ReadAll(resp1.Body)
		require.NoError(t, err)
		require.NoError(t, resp1.Body.Close())

		resp2, err := tr.RoundTrip(req)
		require.NoError(t, err)
		body2, err := io.ReadAll(resp2.Body)
		require.NoError(t, err)
		require.NoError(t, resp2.Body.Close())

		assert.Equal(t, int32(1), hits.Load(),
			"second call must be served from cache (no second next-handler invocation)")
		assert.Equal(t, body1, body2,
			"cache hit must return byte-identical payload to the original miss")
		assert.Equal(t, int64(len(body1)), resp2.ContentLength,
			"synthetic cache-hit response must set ContentLength to len(cachedBytes) for OBS-02 bytes_in")
	})

	t.Run("503 response is NEVER cached (Pitfall CACHE-1 / D-83)", func(t *testing.T) {
		t.Parallel()
		tr, hits := newTestCacheTransport(t, []byte(`{"error":"unavailable"}`), http.StatusServiceUnavailable)

		req := newTestRequest(t, "/Countries", nil)

		for i := 0; i < 2; i++ {
			resp, err := tr.RoundTrip(req)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())
		}

		assert.Equal(t, int32(2), hits.Load(),
			"503 responses must NOT be cached — both calls must hit the next handler (Pitfall CACHE-1)")
	})
}

// TestCacheTransport_RawBytesKey locks D-82: cache key is
// method + " " + URL.Path + "?" + URL.Query().Encode(). Two requests with
// the same path/query but different Host produce identical keys (per-Client
// isolation is the architectural mitigation — Pitfall CACHE-2). Two
// requests with different query produce distinct keys.
func TestCacheTransport_RawBytesKey(t *testing.T) {
	t.Parallel()

	t.Run("different query params produce distinct cache entries", func(t *testing.T) {
		t.Parallel()
		tr, hits := newTestCacheTransport(t, []byte(`[]`), http.StatusOK)

		reqPL := newTestRequest(t, "/Countries", url.Values{"country": []string{"PL"}})
		reqDE := newTestRequest(t, "/Countries", url.Values{"country": []string{"DE"}})

		for _, r := range []*http.Request{reqPL, reqDE} {
			resp, err := tr.RoundTrip(r)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())
		}

		assert.Equal(t, int32(2), hits.Load(),
			"distinct query params must produce distinct cache entries (D-82 — key includes query)")
	})

	t.Run("same path+query different Host hits the same cache key (per-Client isolation contract)", func(t *testing.T) {
		t.Parallel()
		tr, hits := newTestCacheTransport(t, []byte(`[]`), http.StatusOK)

		// Two requests with same path/query but different URL.Host: key is
		// identical because D-82 excludes Host from the encoding. The
		// per-Client isolation contract (cache lives on *Client) is what
		// prevents host collisions in real usage — exercised end-to-end
		// in client_test.go::TestCache_PerClientIsolation.
		uA := &url.URL{Scheme: "http", Host: "host-a.test", Path: "/Countries"}
		uB := &url.URL{Scheme: "http", Host: "host-b.test", Path: "/Countries"}
		reqA, err := http.NewRequestWithContext(context.Background(), http.MethodGet, uA.String(), nil)
		require.NoError(t, err)
		reqB, err := http.NewRequestWithContext(context.Background(), http.MethodGet, uB.String(), nil)
		require.NoError(t, err)

		respA, err := tr.RoundTrip(reqA)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, respA.Body)
		require.NoError(t, respA.Body.Close())

		respB, err := tr.RoundTrip(reqB)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, respB.Body)
		require.NoError(t, respB.Body.Close())

		assert.Equal(t, int32(1), hits.Load(),
			"D-82: cache key excludes Host — reqA caches, reqB hits the cached entry (per-Client isolation handles real-world separation)")
	})
}

// TestCacheHitContextKey_OnHit locks <specifics> 2: synthetic cache-hit
// responses carry CacheHitContextKey == true in
// resp.Request.Context().Value(...). Cache miss responses do NOT carry the
// key.
func TestCacheHitContextKey_OnHit(t *testing.T) {
	t.Parallel()

	t.Run("miss response carries no CacheHitContextKey", func(t *testing.T) {
		t.Parallel()
		tr, _ := newTestCacheTransport(t, []byte(`[]`), http.StatusOK)
		req := newTestRequest(t, "/Countries", nil)

		resp, err := tr.RoundTrip(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		// On a cache miss the response's req.Context() is the next
		// handler's untouched context — it does NOT carry the key.
		// (resp.Request may be nil on the miss path because the synthetic
		// handler did not set it; treat nil as "no key present".)
		if resp.Request != nil {
			v := resp.Request.Context().Value(CacheHitContextKey)
			assert.Nil(t, v, "cache-miss response must NOT carry CacheHitContextKey")
		}
	})

	t.Run("hit response carries CacheHitContextKey == true", func(t *testing.T) {
		t.Parallel()
		tr, _ := newTestCacheTransport(t, []byte(`[]`), http.StatusOK)
		req := newTestRequest(t, "/Countries", nil)

		// Populate the cache.
		respMiss, err := tr.RoundTrip(req)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, respMiss.Body)
		require.NoError(t, respMiss.Body.Close())

		// Second call is a cache hit.
		respHit, err := tr.RoundTrip(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = respHit.Body.Close() })

		require.NotNil(t, respHit.Request, "cache-hit synthetic response must populate resp.Request")
		v := respHit.Request.Context().Value(CacheHitContextKey)
		assert.Equal(t, true, v,
			"cache-hit response must carry CacheHitContextKey == true (<specifics> 2)")
	})
}
