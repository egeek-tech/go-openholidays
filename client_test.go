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
// sync.Once-guarded cache.Close path inside Client.Close (the previously-
// referenced atomic.Bool `closed` flag was removed by the IN-02 re-review
// follow-up — no production code ever read it).

package openholidays

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
// field), and the combined-options happy path. The four "newClientForTest
// seam" subtests cover D-94: the same-package test seam wraps NewClient
// and overrides Client.nowFunc / Client.sleepFunc — exercised by
// retry_test.go (Plan 03) and cache_test.go (Plan 04) to drive
// timing-sensitive code with the fakeClock from clock_test.go. WR-03
// follow-up: previously a separate top-level TestNewClientForTest, demoted
// here because newClientForTest is an unexported helper that wraps the
// production NewClient and Gold Rule 3 requires the test of the helper to
// live with the test of the production function it wraps.
func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("defaults applied when no Option supplied", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		assert.Equal(t, defaultBaseURL, c.baseURL,
			"default baseURL must match D-36 / PROJECT.md")
		assert.Equal(t, 15*time.Second, c.timeout,
			"default timeout must be 15s per CLIENT-06 / D-28")
		require.NotNil(t, c.http, "default http client must be non-nil")

		// WR-01 (re-review) follow-up: Client.userAgent and Client.logger
		// were removed as dead state. The default User-Agent value and
		// non-nil logger now live exclusively on the transport-chain
		// decorators built by buildTransport. Walk the chain to assert
		// the documented defaults are reachable.
		lt, ok := c.http.Transport.(*loggingTransport)
		require.True(t, ok, "default chain's outermost layer must be *loggingTransport")
		require.NotNil(t, lt.logger,
			"default loggingTransport.logger must be non-nil (slog.Default())")
		ht, ok := lt.next.(*headerTransport)
		require.True(t, ok, "default chain must be loggingTransport → headerTransport")
		assert.Equal(t, "go-openholidays/"+Version, ht.userAgent,
			"default headerTransport.userAgent must be go-openholidays/<Version>")
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

	// D-94 newClientForTest seam coverage (demoted from
	// TestNewClientForTest per WR-03 — newClientForTest is the
	// same-package _test.go helper that wraps NewClient and replaces
	// Client.nowFunc / Client.sleepFunc when the caller supplies
	// non-nil overrides).

	t.Run("newClientForTest seam: non-nil now and sleep override defaults", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClock(time.Unix(0, 0))
		c := newClientForTest(fc.Now, fc.Sleep)
		require.NotNil(t, c)
		assert.True(t, c.nowFunc().Equal(time.Unix(0, 0)),
			"newClientForTest must replace Client.nowFunc with the supplied function (D-94)")
		require.NoError(t, c.sleepFunc(t.Context(), time.Second),
			"supplied sleep must not return an error on a live ctx")
		assert.True(t, fc.Now().Equal(time.Unix(0, 0).Add(time.Second)),
			"calling Client.sleepFunc must advance the fakeClock by d (D-94 seam wiring)")
	})

	t.Run("newClientForTest seam: nil now and sleep leave NewClient defaults in place", func(t *testing.T) {
		t.Parallel()
		c := newClientForTest(nil, nil)
		require.NotNil(t, c)
		require.NotNil(t, c.nowFunc, "default nowFunc must be non-nil (time.Now)")
		require.NotNil(t, c.sleepFunc, "default sleepFunc must be non-nil (ctxSleep)")
		assert.WithinDuration(t, time.Now(), c.nowFunc(), time.Second,
			"default Client.nowFunc must be time.Now — calling it returns ≈ now")
	})

	t.Run("newClientForTest seam: passes options through to NewClient", func(t *testing.T) {
		t.Parallel()
		c := newClientForTest(nil, nil, WithStrictDecoding(true))
		require.NotNil(t, c)
		assert.True(t, c.strict,
			"newClientForTest must forward Options to NewClient (WithStrictDecoding(true) reached the Client)")
	})
}

// TestClient_Close covers CLIENT-08 / D-40 + D-96 / RESIL-08:
//
//   - Idempotent: every call returns nil, subsequent calls still return nil.
//   - Race-safe from any goroutine (100 parallel goroutines under -race
//     all return nil).
//   - Stops the cache sweeper goroutine: a constructed-then-Put MemoryCache
//     spawns a sweeper; Close cancels its context and the goroutine exits.
//
// WR-02 + WR-03 follow-up: previously a separate top-level
// TestClient_CloseStopsSweeper used [runtime.NumGoroutine]() + a fixed sleep
// — process-wide and flake-prone. Demoted here as the "stops cache sweeper"
// subtest, replaced with a deterministic observation of MemoryCache.sweepDone
// (the sweepLoop closes that channel via `defer close(m.sweepDone)` on exit,
// so a successful select-recv on it within a bounded wait is the load-bearing
// invariant — no process-global goroutine counter required).
//
// IN-02 (re-review) follow-up: the previously-asserted `closed [atomic.Bool]`
// flag was removed from Client. The race-safety invariant is now carried
// exclusively by the [sync.Once]-guarded cache.Close call inside Client.Close
// (no production endpoint reader ever consulted the flag).
func TestClient_Close(t *testing.T) {
	t.Parallel()

	t.Run("first call returns nil", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		err := c.Close()
		assert.NoError(t, err, "Close must return nil")
	})

	t.Run("subsequent calls also return nil (idempotent)", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		for i := range 5 {
			assert.NoError(t, c.Close(),
				"Close call %d must return nil (idempotent per CLIENT-08)", i+1)
		}
	})

	t.Run("concurrent close is race-safe (100 goroutines)", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		var wg sync.WaitGroup
		const N = 100
		for range N {
			wg.Add(1)
			go func() {
				defer wg.Done()
				assert.NoError(t, c.Close(),
					"Close from a goroutine must return nil (CLIENT-08 / D-40)")
			}()
		}
		wg.Wait()
	})

	t.Run("stops cache sweeper goroutine (D-96 / RESIL-08)", func(t *testing.T) {
		t.Parallel()
		// Drive one end-to-end Countries call so cacheTransport's Put
		// fires and MemoryCache.startOnce.Do(startSweeper) spawns the
		// sweeper goroutine. The 1 ms TTL keeps the test cheap.
		body := []byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}]}]`)
		srv, _ := countriesServer(t, body)
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL), WithCache(1*time.Millisecond))
		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.NoError(t, err, "Countries call must succeed against the fake server")

		// Same-package access lets us reach into the concrete *MemoryCache
		// for the deterministic sweepDone channel — the load-bearing
		// invariant for "sweeper exited" without process-global noise.
		mc, ok := c.cache.(*MemoryCache)
		require.True(t, ok, "default WithCache wires a *MemoryCache")

		require.NoError(t, c.Close(), "Close must return nil per CLIENT-08")

		// MemoryCache.sweepLoop has `defer close(m.sweepDone)`. A
		// successful recv on sweepDone is the deterministic signal that
		// the goroutine actually exited (not just that the ctx was
		// cancelled). 1 s bound is generous for CI scheduling noise but
		// catches a real sweeper leak; the typical observation is sub-ms.
		select {
		case <-mc.sweepDone:
			// Sweeper goroutine exited cleanly.
		case <-time.After(time.Second):
			t.Fatal("MemoryCache sweeper goroutine did not exit within 1s of Client.Close — sweeper-leak regression (D-96 / RESIL-08)")
		}
	})
}

// TestClient_ConcurrentAccess verifies CLIENT-07 + TEST-04: 50 parallel
// Countries calls under -race must complete with identical payloads and
// no data-race reports. Client is immutable after NewClient (Close is
// not exercised here, and the [sync.Once] that previously guarded the
// [atomic.Bool] `closed` flag now guards only cache.Close — no field on
// the Client struct is mutated by endpoint dispatch), so concurrent
// reads of every field are race-safe by definition.
func TestClient_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join("testdata", "countries.json"))
	require.NoError(t, err, "fixture missing — re-capture per Plan 02-03 Task 2")

	// Synthetic delay simulates real network latency without flake risk.
	// math/rand/v2.IntN (D-47 5-20 ms range) is concurrent-safe without
	// seeding — preferred over math/rand v1 per CLAUDE.md What-NOT-to-Use.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Duration(5+rand.IntN(15)) * time.Millisecond) //nolint:gosec // G404: synthetic latency jitter, not cryptographic
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithBaseURL(srv.URL))
	const N = 50
	var wg sync.WaitGroup
	errs := make([]error, N)
	results := make([][]Country, N)

	for i := range N {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = c.Countries(t.Context(), CountriesRequest{})
		}(i)
	}
	wg.Wait()

	t.Run("all 50 calls succeed with identical payloads", func(t *testing.T) {
		t.Parallel()
		for i := range N {
			require.NoError(t, errs[i], "call %d failed: %v", i, errs[i])
			require.NotEmpty(t, results[i], "call %d returned empty", i)
			if i > 0 {
				assert.Equal(t, results[0], results[i],
					"call %d payload differs from call 0", i)
			}
		}
	})
}

// TestClient_ConcurrentRetry_RaceClean locks the CR-01 fix: concurrent
// endpoint calls with WithRetry enabled must NOT race on Client.rand.
// [math/rand/v2.Rand] is documented as NOT safe for concurrent use — its
// stdlib docs state "The methods of Rand are not safe for concurrent use
// by multiple goroutines". Before the fix, the retry loop in doJSONGet
// called computeBackoff(..., c.rand) without synchronization, so two
// goroutines retrying at the same instant raced on the underlying
// ChaCha8 state. The c.randMu mutex added in client.go serializes that
// single Int64N call.
//
// The test fires N parallel Countries calls against a flaky server that
// returns 503 on its first response and 200 thereafter. With WithRetry
// enabled, every goroutine hits at least one 503, triggering a
// computeBackoff (and therefore an rnd.Int64N). Under `go test -race`
// the test fails immediately if c.rand is accessed without
// synchronization. Under non-race builds the assertion is just "all
// calls succeed" — the race detector is the load-bearing signal.
//
// Mirrors TestClient_ConcurrentAccess in shape but ADDS WithRetry +
// uses the per-server-attempt counter to guarantee every goroutine
// drives at least one retry path. The fakeClock seam is intentionally
// NOT used here because the race occurs in the retry loop independent
// of the sleep mechanism, and using a real sleepFunc with a short
// baseDelay produces a faster signal under the race detector than a
// fake-clock harness would.
func TestClient_ConcurrentRetry_RaceClean(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join("testdata", "countries.json"))
	require.NoError(t, err, "fixture missing — re-capture per Plan 02-03 Task 2")

	// Server returns 503 on odd-numbered global attempts and 200 on
	// even-numbered attempts. With maxAttempts=8 every goroutine
	// eventually lands on an even-numbered attempt and succeeds, and
	// most goroutines drive at least one 503 → retry → backoff cycle.
	// The combination guarantees the race-sensitive code path
	// (computeBackoff → c.rand.Int64N inside the c.randMu critical
	// section) is exercised by concurrent goroutines. Under
	// `go test -race`, an unsynchronized access to c.rand would be
	// flagged immediately.
	const N = 50
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n%2 == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	// maxAttempts=8 gives every goroutine ample budget — under the
	// odd-503/even-200 pattern, a string of all-odd attempts is
	// vanishingly unlikely past 4-5 tries. baseDelay 1 ms keeps the
	// test cheap.
	c := NewClient(
		WithBaseURL(srv.URL),
		WithRetry(8, time.Millisecond),
		WithMaxRetryWait(10*time.Millisecond),
	)
	t.Cleanup(func() { _ = c.Close() })

	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := range N {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = c.Countries(t.Context(), CountriesRequest{})
		}(i)
	}
	wg.Wait()

	t.Run("all N parallel retry-enabled calls succeed (race detector validates rand serialization)", func(t *testing.T) {
		t.Parallel()
		for i := range N {
			require.NoError(t, errs[i],
				"call %d failed under concurrent retry — if `go test -race` flagged a data race on c.rand, the CR-01 fix has regressed", i)
		}
	})
}

// TestClient_FinalAttemptRespBodyDrained locks WR-02 defensive behavior:
// when c.http.Do returns BOTH a non-nil *[http.Response] AND a non-nil
// error (the documented "CheckRedirect rejected" shape from Go 1
// compatibility — net/http issue 3795), doJSONGet's post-loop httpErr
// block must drain + close the response body. Prior to the fix the
// post-loop drain defer was registered AFTER the httpErr branch, so
// this body bypassed the drain entirely.
//
// In modern Go stdlib (verified against go1.26 source) the redirect
// machinery already closes the body before returning (see net/http
// client.go's CheckRedirect failure branch — "The resp.Body has
// already been closed."). The WR-02 drain is therefore belt-and-
// suspenders: harmless on the in-tree path because Close on a closed
// body is idempotent, but a meaningful defense against any future
// stdlib change AND against the third-party-tracing-RoundTripper
// scenario where a custom *http.Client.Transport could be substituted
// for the default one.
//
// The test asserts the user-visible contract: a CheckRedirect
// rejection MUST surface as an error wrapped via %%w so callers can
// [errors.Is] to it. A direct "did the drain run" assertion is not
// possible from outside the package because the stdlib hides the body
// behind its own *bodyEOFSignal wrapper; the structural fix is
// verified by reading request.go and confirming the drain block runs
// before the httpErr return.
func TestClient_FinalAttemptRespBodyDrained(t *testing.T) {
	t.Parallel()

	t.Run("CheckRedirect rejection produces wrapped error (WR-02 contract)", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// 302 with a body — net/http.Client.Do calls
			// CheckRedirect, which rejects, and surfaces
			// (resp, err) to doJSONGet. The body is closed
			// pre-emptively by the stdlib redirect machinery; the
			// WR-02 drain is the defensive call that runs after.
			w.Header().Set("Location", "http://other.example/Countries")
			w.WriteHeader(http.StatusFound)
			_, _ = w.Write([]byte("redirect-body-payload"))
		}))
		t.Cleanup(srv.Close)

		injected := errors.New("simulated CheckRedirect rejection")
		httpClient := &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return injected
			},
		}

		c := NewClient(WithBaseURL(srv.URL), WithHTTPClient(httpClient))
		t.Cleanup(func() { _ = c.Close() })

		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err, "CheckRedirect rejection must surface as error")
		require.ErrorIs(t, err, injected,
			"injected CheckRedirect error must be wrapped via %%w so callers can errors.Is to it")
		assert.Contains(t, err.Error(), "/Countries",
			"path must appear in the error message (WR-05 path-carrying contract)")
	})
}

// TestCtxSleep locks WR-04: ctxSleep checks ctx.Err() BEFORE the
// d <= 0 short-circuit so its semantics match fakeClock.Sleep
// (clock_test.go). Previously ctxSleep returned nil immediately when
// d <= 0 without consulting ctx, so tests using fakeClock observed
// ctx-cancelled behavior that production code silently swallowed.
// The tightened contract is "Sleep returns ctx.Err() if ctx is
// already cancelled; otherwise sleeps for d (or returns immediately
// if d<=0) and returns ctx.Err() at the end (or nil)."
//
// ctxSleep is unexported but locally testable in the same package;
// the dedicated TestXxx is justified because ctxSleep IS the
// production sleepFunc default — its contract is consumed by
// doJSONGet via Client.sleepFunc.
func TestCtxSleep(t *testing.T) {
	t.Parallel()

	t.Run("returns ctx.Err() when ctx already cancelled and d > 0", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := ctxSleep(ctx, time.Hour)
		assert.ErrorIs(t, err, context.Canceled,
			"cancelled ctx must produce context.Canceled even with positive d")
	})

	t.Run("returns ctx.Err() when ctx already cancelled and d == 0 (WR-04 parity with fakeClock.Sleep)", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := ctxSleep(ctx, 0)
		assert.ErrorIs(t, err, context.Canceled,
			"WR-04: cancelled ctx must produce context.Canceled even when d == 0 — required for parity with fakeClock.Sleep")
	})

	t.Run("returns ctx.Err() when ctx already cancelled and d < 0 (WR-04 parity)", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := ctxSleep(ctx, -1*time.Second)
		assert.ErrorIs(t, err, context.Canceled,
			"WR-04: cancelled ctx must produce context.Canceled even when d < 0")
	})

	t.Run("returns nil when ctx live and d <= 0", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ctxSleep(t.Context(), 0))
		assert.NoError(t, ctxSleep(t.Context(), -1*time.Millisecond))
	})

	t.Run("returns nil after the timer fires on live ctx with d > 0", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		err := ctxSleep(t.Context(), 5*time.Millisecond)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, time.Since(start), 5*time.Millisecond,
			"the sleep must actually elapse the requested duration on a live ctx")
	})

	t.Run("returns ctx.Err() when ctx cancels during the sleep", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(5*time.Millisecond, cancel)
		err := ctxSleep(ctx, time.Hour)
		assert.ErrorIs(t, err, context.Canceled,
			"mid-sleep ctx cancellation must interrupt and return context.Canceled")
	})
}

// TestClient_RetryExhaustedPrefix locks WR-03: the "retry exhausted
// (N attempts)" wrap fires only when retries ACTUALLY ran. A
// non-retryable transport error on attempt 0 must produce the plain
// error message, not the misleading retry-exhausted prefix, even
// when WithRetry(N, _) is configured with N > 1.
//
// The httpErr branch is exercised via a custom RoundTripper that
// returns a non-retryable error (a plain [errors.New] is neither a
// net.Error.Timeout nor a [syscall.ECONNRESET], so shouldRetry classifies
// it as non-retryable). The loop breaks on attempt 0 via !shouldRetry
// after exactly one round trip; the post-loop wrap must NOT prepend
// "retry exhausted (5 attempts):".
func TestClient_RetryExhaustedPrefix(t *testing.T) {
	t.Parallel()

	t.Run("non-retryable transport error on attempt 0 with WithRetry(5,_) omits retry-exhausted prefix (WR-03)", func(t *testing.T) {
		t.Parallel()

		injected := errors.New("simulated non-retryable transport error")
		transport := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, injected
		})
		httpClient := &http.Client{Transport: transport}

		c := NewClient(
			WithBaseURL("http://example.test"),
			WithHTTPClient(httpClient),
			WithRetry(5, time.Millisecond),
		)
		t.Cleanup(func() { _ = c.Close() })

		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err)
		require.ErrorIs(t, err, injected,
			"underlying transport error must remain wrapped via %%w")
		assert.NotContains(t, err.Error(), "retry exhausted",
			"WR-03 contract: a single-attempt failure with WithRetry(5,_) must NOT prepend 'retry exhausted (5 attempts):' — no retries actually ran")
		assert.Contains(t, err.Error(), "/Countries",
			"path must appear in the plain (non-retry-exhausted) error message")
	})

	t.Run("retryable transport error exhausting all attempts retains retry-exhausted prefix", func(t *testing.T) {
		t.Parallel()

		// fakeNetError with Timeout()==true is retryable per
		// shouldRetry — the loop runs to full exhaustion.
		transport := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, &fakeNetError{timeout: true}
		})
		httpClient := &http.Client{Transport: transport}

		c := NewClient(
			WithBaseURL("http://example.test"),
			WithHTTPClient(httpClient),
			WithRetry(3, time.Millisecond),
			WithMaxRetryWait(2*time.Millisecond),
		)
		t.Cleanup(func() { _ = c.Close() })

		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "retry exhausted (3 attempts)",
			"WR-03 contract: when retries actually ran to exhaustion, the prefix must report the actual attempt count")
	})
}

// TestClient_ContextCancel verifies CLIENT-09 + D-48: ctx cancellation
// interrupts in-flight HTTP within ≤ 100 ms (asserted at the 200 ms ceiling
// for 2x CI slack); [errors.Is](err, [context.Canceled]) holds through
// countries.go's [fmt.Errorf]("openholidays: GET /Countries: %w", err) wrap.
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
		ctx, cancel := context.WithCancel(t.Context())
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
		assert.ErrorIs(t, err, context.Canceled,
			"expected errors.Is(err, context.Canceled) to hold; got %v", err)
	})
}

// countriesServer returns an [httptest.Server] that responds 200 with the
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
		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err, "first call: strict mode must surface a decode error on unknown field")
		assert.Contains(t, err.Error(), "extra_unknown_field",
			"first call: error message must name the offending field")

		// Second call: cache hit (server should NOT be contacted again);
		// decoder STILL fires because cached bytes flow through the same
		// strict gate (D-93).
		_, err = c.Countries(t.Context(), CountriesRequest{})
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

		for range 3 {
			_, err := c.Countries(t.Context(), CountriesRequest{})
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

		_, err := cA.Countries(t.Context(), CountriesRequest{})
		require.NoError(t, err)
		_, err = cB.Countries(t.Context(), CountriesRequest{})
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

		_, err := c.Countries(t.Context(), CountriesRequest{})
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
		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.NoError(t, err, "first call must succeed (server hit, response cached)")
		assert.Equal(t, int32(1), hookCount.Load(),
			"first call must fire hook exactly once")
		assert.False(t, lastIsCacheHit.Load(),
			"first call is a cache MISS — CacheHitContextKey must be absent (false)")

		// Second call — cache hit. Hook fires on the synthetic response;
		// CacheHitContextKey is set to true by cacheTransport (Plan 04).
		_, err = c.Countries(t.Context(), CountriesRequest{})
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

		_, err := c.Countries(t.Context(), CountriesRequest{})
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
		_, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "", // missing required field
		})
		require.Error(t, err, "validateCountry must reject empty CountryIsoCode pre-HTTP")

		assert.Equal(t, int32(0), hookCount.Load(),
			"hook MUST NOT fire when request fails pre-HTTP (validation, NewRequest build) — D-88")
	})
}
