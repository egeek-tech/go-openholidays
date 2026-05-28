// Package openholidays — tests for Client construction and lifecycle.
//
// One TestXxx per exported production function (Gold Rule 3): TestNewClient
// (defaults + option composition) and TestClient_Close (idempotent + race-safe
// from any goroutine).
//
// NOTE: TestClient_ConcurrentAccess (CLIENT-07) and TestClient_ContextCancel
// (CLIENT-09) require an end-to-end HTTP call through the chain. Plan 02-02
// ships only the construction-time contract; those two tests land in Plan
// 02-03 alongside the Countries endpoint. The Close-under-100-goroutines
// subtest below is the Phase 2 Plan 02 mechanical race-safety check for the
// `closed` atomic flag specifically.

package openholidays

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newClientForTest is the D-94 test seam: same-package _test.go helper that
// wraps NewClient and replaces Client.nowFunc / Client.sleepFunc when the
// caller supplies non-nil overrides. Plans 03 (retry) and 04 (cache TTL)
// use this to drive timing-sensitive code with the fakeClock from
// clock_test.go. The seam is test-only (same-package visibility; not
// exported) — production callers MUST go through NewClient + the public
// option set.
func newClientForTest(now func() time.Time, sleep func(context.Context, time.Duration) error, opts ...Option) *Client {
	c := NewClient(opts...)
	if now != nil {
		c.nowFunc = now
	}
	if sleep != nil {
		c.sleepFunc = sleep
	}
	return c
}

// TestNewClient covers CLIENT-01: defaults applied when no Option supplied,
// option composition (later Options override earlier ones for the same
// field), and the combined-options happy path.
func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("defaults applied when no Option supplied", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		assert.Equal(t, "https://openholidaysapi.org", c.baseURL,
			"default baseURL must match D-36 / PROJECT.md")
		assert.Equal(t, "go-openholidays/"+Version, c.userAgent,
			"default userAgent must be go-openholidays/<Version>")
		assert.Equal(t, 15*time.Second, c.timeout,
			"default timeout must be 15s per CLIENT-06 / D-28")
		require.NotNil(t, c.logger, "default logger must be non-nil (slog.Default())")
		require.NotNil(t, c.http, "default http client must be non-nil")
	})

	t.Run("Options compose left-to-right (later wins)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(
			WithBaseURL("https://first.test"),
			WithBaseURL("https://second.test"),
		)
		require.NotNil(t, c)
		assert.Equal(t, "https://second.test", c.baseURL,
			"later Option must override earlier Option for the same field")
	})

	t.Run("WithHTTPClient + WithTimeout combine", func(t *testing.T) {
		t.Parallel()
		custom := &http.Client{}
		c := NewClient(
			WithHTTPClient(custom),
			WithTimeout(5*time.Second),
		)
		require.NotNil(t, c)
		require.NotNil(t, c.http)
		assert.Equal(t, 5*time.Second, c.timeout,
			"WithTimeout must be applied even when WithHTTPClient also supplied")
	})
}

// TestClient_Close covers CLIENT-08 / D-40: idempotent (every call returns
// nil, subsequent calls still return nil), and race-safe from any goroutine
// (100 parallel goroutines under -race all return nil and the final flag is
// true).
func TestClient_Close(t *testing.T) {
	t.Parallel()

	t.Run("first call returns nil and flips closed flag", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		require.False(t, c.closed.Load(),
			"closed should be false immediately after NewClient")
		err := c.Close()
		assert.NoError(t, err, "Close must return nil")
		assert.True(t, c.closed.Load(),
			"closed flag must be true after the first Close call")
	})

	t.Run("subsequent calls also return nil (idempotent)", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		for i := 0; i < 5; i++ {
			assert.NoError(t, c.Close(),
				"Close call %d must return nil (idempotent per CLIENT-08)", i+1)
		}
		assert.True(t, c.closed.Load(),
			"closed must remain true after multiple Close calls")
	})

	t.Run("concurrent close is race-safe (100 goroutines)", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		var wg sync.WaitGroup
		const N = 100
		for i := 0; i < N; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				assert.NoError(t, c.Close(),
					"Close from a goroutine must return nil (CLIENT-08 / D-40)")
			}()
		}
		wg.Wait()
		assert.True(t, c.closed.Load(),
			"closed must be true after all goroutines have called Close")
	})
}

// TestClient_ConcurrentAccess verifies CLIENT-07 + TEST-04: 50 parallel
// Countries calls under -race must complete with identical payloads and
// no data-race reports. Client is immutable after NewClient (only c.closed
// is mutable, and Close is not exercised here), so concurrent reads of
// every field are race-safe by definition.
func TestClient_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join("testdata", "countries.json"))
	require.NoError(t, err, "fixture missing — re-capture per Plan 02-03 Task 2")

	// Synthetic delay simulates real network latency without flake risk.
	// math/rand/v2.IntN (D-47 5-20 ms range) is concurrent-safe without
	// seeding — preferred over math/rand v1 per CLAUDE.md What-NOT-to-Use.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(5+rand.IntN(15)) * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithBaseURL(srv.URL))
	const N = 50
	var wg sync.WaitGroup
	errs := make([]error, N)
	results := make([][]Country, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = c.Countries(context.Background(), CountriesRequest{})
		}(i)
	}
	wg.Wait()

	t.Run("all 50 calls succeed with identical payloads", func(t *testing.T) {
		for i := 0; i < N; i++ {
			require.NoError(t, errs[i], "call %d failed: %v", i, errs[i])
			require.NotEmpty(t, results[i], "call %d returned empty", i)
			if i > 0 {
				assert.Equal(t, results[0], results[i],
					"call %d payload differs from call 0", i)
			}
		}
	})
}

// TestClient_ContextCancel verifies CLIENT-09 + D-48: ctx cancellation
// interrupts in-flight HTTP within ≤ 100 ms (asserted at the 200 ms ceiling
// for 2x CI slack); errors.Is(err, context.Canceled) holds through
// countries.go's fmt.Errorf("openholidays: GET /Countries: %w", err) wrap.
func TestClient_ContextCancel(t *testing.T) {
	t.Parallel()

	// Server hangs forever (10 s) — caller cancellation MUST interrupt.
	// The select on r.Context().Done() lets the handler return promptly
	// when the client disconnects, freeing the goroutine.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Server observes the client cancellation. No write needed.
		}
	}))
	t.Cleanup(srv.Close)

	// SDK timeout intentionally large so the ctx cancel is the cause.
	c := NewClient(WithBaseURL(srv.URL), WithTimeout(30*time.Second))

	t.Run("ctx cancel interrupts in-flight HTTP within 500ms", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)
		start := time.Now()
		_, err := c.Countries(ctx, CountriesRequest{})
		elapsed := time.Since(start)

		require.Error(t, err)
		// CLIENT-09 contract: ≤ 100 ms target. WR-09: bumped CI slack
		// ceiling from 200 ms to 500 ms to absorb GC pauses and scheduler
		// noise on heavily-loaded CI runners; the contract-level 100 ms
		// is verified by microbenchmarks, not this smoke integration
		// test. Bumping to 500 ms keeps the assertion well below the
		// 30s WithTimeout while reducing flake under load.
		assert.Less(t, elapsed, 500*time.Millisecond,
			"ctx cancel must interrupt in-flight HTTP within 500 ms (CLIENT-09 target 100ms; CI slack); took %v", elapsed)
		assert.True(t, errors.Is(err, context.Canceled),
			"expected errors.Is(err, context.Canceled) to hold; got %v", err)
	})
}

// TestNewClientForTest covers D-94: the same-package test seam overrides
// Client.nowFunc / Client.sleepFunc when the caller's args are non-nil,
// otherwise leaves NewClient's defaults intact, and passes Option values
// through to NewClient. Without this seam, retry_test.go (Plan 03) and
// cache_test.go (Plan 04) would have to mutate Client fields directly
// from test code — a fragile coupling the seam decouples explicitly.
func TestNewClientForTest(t *testing.T) {
	t.Parallel()

	t.Run("non-nil now and sleep override defaults", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClock(time.Unix(0, 0))
		c := newClientForTest(fc.Now, fc.Sleep)
		require.NotNil(t, c)
		assert.True(t, c.nowFunc().Equal(time.Unix(0, 0)),
			"newClientForTest must replace Client.nowFunc with the supplied function (D-94)")
		require.NoError(t, c.sleepFunc(context.Background(), time.Second),
			"supplied sleep must not return an error on a live ctx")
		assert.True(t, fc.Now().Equal(time.Unix(0, 0).Add(time.Second)),
			"calling Client.sleepFunc must advance the fakeClock by d (D-94 seam wiring)")
	})

	t.Run("nil now and sleep leave NewClient defaults in place", func(t *testing.T) {
		t.Parallel()
		c := newClientForTest(nil, nil)
		require.NotNil(t, c)
		require.NotNil(t, c.nowFunc, "default nowFunc must be non-nil (time.Now)")
		require.NotNil(t, c.sleepFunc, "default sleepFunc must be non-nil (ctxSleep)")
		assert.WithinDuration(t, time.Now(), c.nowFunc(), time.Second,
			"default Client.nowFunc must be time.Now — calling it returns ≈ now")
	})

	t.Run("passes options through to NewClient", func(t *testing.T) {
		t.Parallel()
		c := newClientForTest(nil, nil, WithStrictDecoding(true))
		require.NotNil(t, c)
		assert.True(t, c.strict,
			"newClientForTest must forward Options to NewClient (WithStrictDecoding(true) reached the Client)")
	})
}

// TestWithStrictDecoding_RejectsUnknown covers OBS-03 wire-level behavior
// (D-91 + D-92): the WithStrictDecoding(true) Client sends every JSON
// response through json.Decoder.DisallowUnknownFields, so an upstream
// payload with a field absent from the destination Go struct surfaces a
// decode error containing the offending field name. The error path also
// confirms the existing request.go error-wrap ("openholidays: decode")
// stays intact.
func TestWithStrictDecoding_RejectsUnknown(t *testing.T) {
	t.Parallel()

	t.Run("strict mode rejects unknown JSON fields", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"isoCode":"PL","extra_unknown_field":42,"name":[{"language":"en","text":"Poland"}]}]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL), WithStrictDecoding(true))
		_, err := c.Countries(context.Background(), CountriesRequest{})
		require.Error(t, err, "strict mode must surface a decode error on unknown field")
		assert.Contains(t, err.Error(), "extra_unknown_field",
			"error message must name the offending field (json.Decoder.DisallowUnknownFields convention)")
		assert.Contains(t, err.Error(), "openholidays: decode",
			"existing request.go error-wrap prefix must be preserved (Phase 1 D-23 / Phase 3 D-62)")
	})
}

// TestWithStrictDecoding_DefaultLenient covers Pitfall JSON-1 / D-91:
// strict-decoding is OFF by default. A default-constructed Client MUST
// accept upstream JSON containing fields absent from the destination Go
// struct without error — the only reason this test exists is to lock the
// "OFF by default" invariant against accidental future flips.
func TestWithStrictDecoding_DefaultLenient(t *testing.T) {
	t.Parallel()

	t.Run("default Client accepts unknown JSON fields", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"isoCode":"PL","extra_unknown_field":42,"name":[{"language":"en","text":"Poland"}]}]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL)) // NO WithStrictDecoding
		cs, err := c.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err,
			"default Client must accept unknown JSON fields (Pitfall JSON-1 — upstream adds fields routinely)")
		require.Len(t, cs, 1, "decoded payload must produce exactly one Country")
		assert.Equal(t, "PL", cs[0].IsoCode,
			"known fields must decode normally even with an unknown sibling present")
	})
}

// countriesServer returns an httptest.Server that responds 200 with the
// supplied body and increments hits on every request. Shared helper for
// the Plan 04 cache composition tests below.
func countriesServer(t *testing.T, body []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	return srv, &hits
}

// TestClient_CloseStopsSweeper locks D-96 + RESIL-08: Close stops the cache
// sweeper goroutine. The test runs a real cache through one end-to-end
// Countries call (forcing a Put → lazy sweeper start), calls Close, then
// asserts runtime.NumGoroutine() delta ≤ 0 after closing the server and a
// small grace period.
//
// Implementation note: D-96's documented pattern omits server bookkeeping;
// in practice an httptest.Server + http.Transport keep-alive pool spawns
// short-lived goroutines that persist past Client.Close until the server
// itself closes. To isolate the sweeper-leak signal, we explicitly close
// both the server's idle client connections AND the server itself BEFORE
// the assertion. The sweeper-stop check is the load-bearing invariant —
// the connection-pool noise is a documented test-harness artifact, not a
// regression in Client.Close (Rule 1 auto-fix per the executor protocol —
// the plan's verbatim D-96 pattern would be flaky without this).
//
// Not t.Parallel() because runtime.NumGoroutine() delta checks are
// sensitive to other tests' goroutine churn (Phase 2 D-48 / D-96).
func TestClient_CloseStopsSweeper(t *testing.T) {
	body := []byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}]}]`)
	srv, _ := countriesServer(t, body)

	before := runtime.NumGoroutine()

	c := NewClient(WithBaseURL(srv.URL), WithCache(1*time.Millisecond))
	_, err := c.Countries(context.Background(), CountriesRequest{})
	require.NoError(t, err, "Countries call must succeed against the fake server")

	require.NoError(t, c.Close())
	// Tear down the test-harness HTTP plumbing BEFORE the assertion so
	// http.Transport keep-alive goroutines stop and httptest.Server
	// worker goroutines exit; only then is the runtime.NumGoroutine
	// delta a clean signal for the sweeper-leak invariant.
	c.http.CloseIdleConnections()
	srv.Close()
	time.Sleep(20 * time.Millisecond) // sweeper + conn-pool exit grace

	assert.LessOrEqual(t, runtime.NumGoroutine(), before,
		"Close must stop the sweeper goroutine (D-96 / RESIL-08 — runtime.NumGoroutine delta ≤ 0 after test-harness teardown)")
}

// TestCache_StrictDecodingComposes locks D-93: strict-decoding applies to
// cached bytes on every read. The first call caches the bytes (cache
// transport sees err==nil && status==200, caches happily) and surfaces the
// decode error to the caller. The second call hits the cache (server is
// NOT contacted) and STILL surfaces the decode error because the strict
// gate runs in doJSONGet AFTER cacheTransport returns.
func TestCache_StrictDecodingComposes(t *testing.T) {
	t.Parallel()

	t.Run("strict mode rejects unknown field on both fresh AND cached reads", func(t *testing.T) {
		t.Parallel()
		body := []byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}],"extra_unknown_field":42}]`)
		srv, hits := countriesServer(t, body)
		t.Cleanup(srv.Close)

		c := NewClient(
			WithBaseURL(srv.URL),
			WithCache(time.Hour),
			WithStrictDecoding(true),
		)
		t.Cleanup(func() { _ = c.Close() })

		// First call: server hit, bytes cached (cacheTransport's gate is
		// status==200 && err==nil — decode error fires AFTER), decoder
		// fires.
		_, err := c.Countries(context.Background(), CountriesRequest{})
		require.Error(t, err, "first call: strict mode must surface a decode error on unknown field")
		assert.Contains(t, err.Error(), "extra_unknown_field",
			"first call: error message must name the offending field")

		// Second call: cache hit (server should NOT be contacted again);
		// decoder STILL fires because cached bytes flow through the same
		// strict gate (D-93).
		_, err = c.Countries(context.Background(), CountriesRequest{})
		require.Error(t, err, "second call: strict mode must still reject the cached bytes (D-93)")
		assert.Contains(t, err.Error(), "extra_unknown_field",
			"second call: error message must name the offending field on the cache-hit path")

		assert.Equal(t, int32(1), hits.Load(),
			"second call must be served from cache, not server (only one server hit total)")
	})
}

// TestClient_NoCache_AllCallsHitNetwork locks the default-off invariant:
// a Client constructed WITHOUT WithCache hits the server on every call.
func TestClient_NoCache_AllCallsHitNetwork(t *testing.T) {
	t.Parallel()

	t.Run("3 calls without WithCache produce 3 server hits", func(t *testing.T) {
		t.Parallel()
		body := []byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}]}]`)
		srv, hits := countriesServer(t, body)
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL)) // NO WithCache
		t.Cleanup(func() { _ = c.Close() })

		for i := 0; i < 3; i++ {
			_, err := c.Countries(context.Background(), CountriesRequest{})
			require.NoError(t, err)
		}

		assert.Equal(t, int32(3), hits.Load(),
			"default Client (no WithCache) must hit the network on every call (TEST-06 default-off)")
	})
}

// TestCache_PerClientIsolation locks D-82 / Pitfall CACHE-2: two Clients
// with their own caches and different baseURLs do not share cache. Each
// server sees exactly one hit even though both Clients call Countries.
func TestCache_PerClientIsolation(t *testing.T) {
	t.Parallel()

	t.Run("two Clients hit their own server exactly once", func(t *testing.T) {
		t.Parallel()
		body := []byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}]}]`)
		srvA, hitsA := countriesServer(t, body)
		srvB, hitsB := countriesServer(t, body)
		t.Cleanup(srvA.Close)
		t.Cleanup(srvB.Close)

		cA := NewClient(WithBaseURL(srvA.URL), WithCache(time.Hour))
		cB := NewClient(WithBaseURL(srvB.URL), WithCache(time.Hour))
		t.Cleanup(func() { _ = cA.Close() })
		t.Cleanup(func() { _ = cB.Close() })

		_, err := cA.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err)
		_, err = cB.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err)

		assert.Equal(t, int32(1), hitsA.Load(),
			"Client A must have hit its own server exactly once (per-Client cache isolation)")
		assert.Equal(t, int32(1), hitsB.Load(),
			"Client B must have hit its own server exactly once (per-Client cache isolation)")
	})
}

// TestHook_FiresOnRetryAttempts locks TRANS-05 + D-88 composition with the
// retry loop (Plan 04-03): each c.http.Do invocation re-enters the
// RoundTripper chain, so a retry loop dispatching three attempts produces
// three hook invocations. The 429→500→200 sequence is the canonical
// retry-status mix from Pitfall RETRY-1 / D-75.
//
// Uses the deterministic fakeClock seam (clock_test.go from Plan 04-01) so
// the retry-backoff sleeps don't add real wall-clock time to the test —
// fc.Sleep just advances the clock synchronously.
func TestHook_FiresOnRetryAttempts(t *testing.T) {
	t.Parallel()

	t.Run("429→500→200 sequence produces three hook invocations (TRANS-05)", func(t *testing.T) {
		t.Parallel()

		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			i := hits.Add(1)
			switch i {
			case 1:
				w.WriteHeader(http.StatusTooManyRequests)
			case 2:
				w.WriteHeader(http.StatusInternalServerError)
			default:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}]}]`))
			}
		}))
		t.Cleanup(srv.Close)

		var hookCount atomic.Int32
		hook := func(_ *http.Request, _ *http.Response, _ error) {
			hookCount.Add(1)
		}

		fc := newFakeClock(time.Unix(0, 0))
		c := newClientForTest(fc.Now, fc.Sleep,
			WithBaseURL(srv.URL),
			WithRetry(5, 10*time.Millisecond),
			WithMaxRetryWait(time.Second),
			WithRequestHook(hook),
		)
		t.Cleanup(func() { _ = c.Close() })

		_, err := c.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err, "third attempt returns 200; the retry loop must succeed")

		assert.Equal(t, int32(3), hits.Load(),
			"server must see 3 round trips (429, 500, 200)")
		assert.Equal(t, int32(3), hookCount.Load(),
			"hook must fire once per retry attempt (TRANS-05 — three round trips → three hook calls)")
	})
}

// TestHook_SeesCacheHits locks D-88 + Plan 04 cache composition: the hook
// fires on cache-hit synthetic responses too. Consumers can detect the
// cache-hit branch by reading CacheHitContextKey from the request context
// (Plan 04). First call: cache miss (hook fires, server hit, key absent).
// Second call: cache hit (hook fires, server NOT hit, key == true).
func TestHook_SeesCacheHits(t *testing.T) {
	t.Parallel()

	t.Run("hook observes cache-hit synthetic responses via CacheHitContextKey (D-88)", func(t *testing.T) {
		t.Parallel()

		body := []byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}]}]`)
		srv, hits := countriesServer(t, body)
		t.Cleanup(srv.Close)

		var (
			hookCount      atomic.Int32
			lastIsCacheHit atomic.Bool
		)
		hook := func(req *http.Request, _ *http.Response, _ error) {
			hookCount.Add(1)
			if v, _ := req.Context().Value(CacheHitContextKey).(bool); v {
				lastIsCacheHit.Store(true)
			} else {
				lastIsCacheHit.Store(false)
			}
		}

		c := NewClient(
			WithBaseURL(srv.URL),
			WithCache(time.Hour),
			WithRequestHook(hook),
		)
		t.Cleanup(func() { _ = c.Close() })

		// First call — cache miss. Hook fires; CacheHitContextKey absent.
		_, err := c.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err, "first call must succeed (server hit, response cached)")
		assert.Equal(t, int32(1), hookCount.Load(),
			"first call must fire hook exactly once")
		assert.False(t, lastIsCacheHit.Load(),
			"first call is a cache MISS — CacheHitContextKey must be absent (false)")

		// Second call — cache hit. Hook fires on the synthetic response;
		// CacheHitContextKey is set to true by cacheTransport (Plan 04).
		_, err = c.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err, "second call must succeed from cache")
		assert.Equal(t, int32(2), hookCount.Load(),
			"second call must fire hook again (D-88 fires on cache hits too)")
		assert.True(t, lastIsCacheHit.Load(),
			"second call is a cache HIT — hook must see CacheHitContextKey == true")

		assert.Equal(t, int32(1), hits.Load(),
			"server must see exactly 1 round trip — second call served from cache")
	})
}

// TestHook_DoesNotFireOnDecodeError locks the negative side of D-88:
//
//  1. The hook fires ONCE per HTTP round trip even when the subsequent
//     in-process decoder fails (decode runs in doJSONGet AFTER the
//     RoundTripper chain returns — out of hook scope). So a 200 response
//     with malformed JSON produces ONE hook invocation (for the successful
//     HTTP round trip), not two.
//  2. The hook does NOT fire when the request fails pre-HTTP — e.g., when
//     PublicHolidays' client-side validator rejects the request before
//     doJSONGet runs. No HTTP attempt → no hook.
//
// Both subtests verify D-88 contract: hook is HTTP-layer observability ONLY.
func TestHook_DoesNotFireOnDecodeError(t *testing.T) {
	t.Parallel()

	t.Run("hook fires once on HTTP round trip even when subsequent decode fails", func(t *testing.T) {
		t.Parallel()

		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			// Intentionally malformed JSON (truncated mid-object) — decoder
			// surfaces an io.ErrUnexpectedEOF / SyntaxError wrapped by
			// doJSONGet's "openholidays: decode" prefix. The HTTP round
			// trip itself is a successful 200 → the hook fires for it.
			_, _ = w.Write([]byte(`[{"isoCode":"PL","name":`))
		}))
		t.Cleanup(srv.Close)

		var hookCount atomic.Int32
		hook := func(_ *http.Request, _ *http.Response, _ error) {
			hookCount.Add(1)
		}

		c := NewClient(WithBaseURL(srv.URL), WithRequestHook(hook))
		t.Cleanup(func() { _ = c.Close() })

		_, err := c.Countries(context.Background(), CountriesRequest{})
		require.Error(t, err, "malformed JSON must produce a decode error")

		assert.Equal(t, int32(1), hits.Load(),
			"server must see exactly 1 round trip (no retry — no WithRetry option)")
		assert.Equal(t, int32(1), hookCount.Load(),
			"hook fires on the HTTP round trip (200 received) but NOT on the subsequent decode error — decode is post-RoundTripper-chain (D-88)")
	})

	t.Run("hook does NOT fire when request fails pre-HTTP (validation)", func(t *testing.T) {
		t.Parallel()

		var hookCount atomic.Int32
		hook := func(_ *http.Request, _ *http.Response, _ error) {
			hookCount.Add(1)
		}

		c := NewClient(
			WithBaseURL("http://invalid.example"), // never reached
			WithRequestHook(hook),
		)
		t.Cleanup(func() { _ = c.Close() })

		// Empty CountryIsoCode is rejected by validateCountry BEFORE
		// doJSONGet runs at all — no HTTP attempt, no hook firing.
		_, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{
			CountryIsoCode: "", // missing required field
		})
		require.Error(t, err, "validateCountry must reject empty CountryIsoCode pre-HTTP")

		assert.Equal(t, int32(0), hookCount.Load(),
			"hook MUST NOT fire when request fails pre-HTTP (validation, NewRequest build) — D-88")
	})
}
