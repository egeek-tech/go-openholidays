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
	"sync"
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

	t.Run("ctx cancel interrupts in-flight HTTP within 200ms", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)
		start := time.Now()
		_, err := c.Countries(ctx, CountriesRequest{})
		elapsed := time.Since(start)

		require.Error(t, err)
		// 100 ms target + 100 ms CI slack = 200 ms ceiling (D-48).
		assert.Less(t, elapsed, 200*time.Millisecond,
			"ctx cancel must interrupt in-flight HTTP within 200 ms; took %v", elapsed)
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
