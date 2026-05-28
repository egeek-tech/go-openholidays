// Package openholidays — retry layer tests (Gold Rule 3 + D-77 + TEST-05).
//
// This file is the comprehensive test suite for the Phase 4 retry layer:
//
//   - TestShouldRetry / TestParseRetryAfter / TestComputeBackoff /
//     TestComputeBackoff_HonorsRetryAfter — pure-function unit tests
//     locking the helpers declared in retry.go (Task 1).
//   - TestRetry_E2E_429Then500Then200 — the marquee TEST-05 case proving
//     the loop fires exactly three round trips on 429 → 500 → 200 under
//     a deterministic clock.
//   - TestRetry_HonorsRetryAfterSeconds / TestRetry_HonorsRetryAfterDate
//     — D-76 integer-seconds and HTTP-date Retry-After paths.
//   - TestRetry_CtxCancel — RESIL-04: ctx cancellation interrupts the
//     retry loop within ≤ 100 ms.
//   - TestRetry_NeverRetriesCtxErrors — D-75: shouldRetry never returns
//     true for raw context.Canceled / context.DeadlineExceeded.
//   - TestRetry_DeterministicClock — TEST-05 explicit: bounded fake-
//     clock advance via fakeClock.
//   - TestRetry_NotARoundTripper — RESIL-05 structural audit: no
//     transport_retry.go file, no retryTransport struct.
//
// All tests follow the Gold Rule 3 etiquette: t.Parallel() at outer
// level, every leaf subtest t.Parallel(), table-driven where ≥ 2 cases
// share setup, require for preconditions, assert for verifications.
// roundTripperFunc is reused from transport_header_test.go:19
// (same-package visibility).

package openholidays

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"math/rand/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeNetErr is a test-only net.Error implementation used to drive
// shouldRetry's net.Error.Timeout() branch (D-75). The Timeout() flag
// is the only field that matters for the retry matrix; Temporary() is
// deprecated but still part of the net.Error interface so it returns
// false (a non-temporary timeout is still retryable per D-75).
type fakeNetErr struct{ timeout bool }

func (e *fakeNetErr) Error() string   { return "fake net error" }
func (e *fakeNetErr) Timeout() bool   { return e.timeout }
func (e *fakeNetErr) Temporary() bool { return false }

// newTestRand builds a deterministic rand.Rand for the helper tests via
// rand.NewChaCha8 with a fixed 32-byte seed. Test isolation matters
// more than entropy here — the same seed across runs produces the same
// jitter sequence, so range assertions (lower ≤ got < upper) are
// stable.
func newTestRand() *rand.Rand {
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	return rand.New(rand.NewChaCha8(seed))
}

// TestShouldRetry locks the D-75 retryable-conditions matrix verbatim.
// Eight+ cases: HTTP statuses (true and false branches), net.Error
// timeout (true), syscall.ECONNRESET (true), raw ctx errors (false —
// shouldRetry must never claim to retry these), defensive nil-pair
// (false).
func TestShouldRetry(t *testing.T) {
	t.Parallel()

	type tc struct {
		name string
		resp *http.Response
		err  error
		want bool
	}
	cases := []tc{
		// Retryable HTTP statuses (D-75).
		{name: "408 Request Timeout retryable", resp: &http.Response{StatusCode: 408}, err: nil, want: true},
		{name: "429 Too Many Requests retryable", resp: &http.Response{StatusCode: 429}, err: nil, want: true},
		{name: "500 Internal Server Error retryable", resp: &http.Response{StatusCode: 500}, err: nil, want: true},
		{name: "502 Bad Gateway retryable", resp: &http.Response{StatusCode: 502}, err: nil, want: true},
		{name: "503 Service Unavailable retryable", resp: &http.Response{StatusCode: 503}, err: nil, want: true},
		{name: "504 Gateway Timeout retryable", resp: &http.Response{StatusCode: 504}, err: nil, want: true},
		// Non-retryable HTTP statuses.
		{name: "200 OK not retryable", resp: &http.Response{StatusCode: 200}, err: nil, want: false},
		{name: "201 Created not retryable", resp: &http.Response{StatusCode: 201}, err: nil, want: false},
		{name: "400 Bad Request not retryable", resp: &http.Response{StatusCode: 400}, err: nil, want: false},
		{name: "401 Unauthorized not retryable", resp: &http.Response{StatusCode: 401}, err: nil, want: false},
		{name: "403 Forbidden not retryable", resp: &http.Response{StatusCode: 403}, err: nil, want: false},
		{name: "404 Not Found not retryable", resp: &http.Response{StatusCode: 404}, err: nil, want: false},
		// Transport errors.
		{name: "net.Error Timeout()==true retryable", resp: nil, err: &fakeNetErr{timeout: true}, want: true},
		{name: "net.Error Timeout()==false not retryable", resp: nil, err: &fakeNetErr{timeout: false}, want: false},
		{name: "syscall.ECONNRESET wrapped in net.OpError retryable", resp: nil, err: &net.OpError{Op: "read", Err: syscall.ECONNRESET}, want: true},
		{name: "bare syscall.ECONNRESET retryable", resp: nil, err: syscall.ECONNRESET, want: true},
		// Ctx errors — D-75: NEVER retried.
		{name: "context.Canceled not retryable", resp: nil, err: context.Canceled, want: false},
		{name: "context.DeadlineExceeded not retryable", resp: nil, err: context.DeadlineExceeded, want: false},
		// Defensive nil/nil pair.
		{name: "nil response + nil err defensive false", resp: nil, err: nil, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := shouldRetry(c.resp, c.err)
			assert.Equal(t, c.want, got,
				"shouldRetry(%v, %v) want %v got %v", c.resp, c.err, c.want, got)
		})
	}
}

// TestParseRetryAfter locks D-76 + Pitfall 9 past-date guard. Six
// cases cover integer seconds, RFC 1123 future date, RFC 1123 past
// date (rejected per Pitfall 9), ANSI C asctime format, empty header,
// and garbage input.
func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 7, 27, 0, 0, time.UTC)

	type tc struct {
		name   string
		h      string
		now    time.Time
		wantD  time.Duration
		wantOK bool
	}
	cases := []tc{
		{
			name:   "integer seconds",
			h:      "2",
			now:    now,
			wantD:  2 * time.Second,
			wantOK: true,
		},
		{
			name:   "RFC 1123 future date returns date-minus-now",
			h:      "Wed, 21 Oct 2026 07:28:00 GMT",
			now:    now,
			wantD:  60 * time.Second,
			wantOK: true,
		},
		{
			name:   "RFC 1123 past date rejected (Pitfall 9 / threat T-04-06)",
			h:      "Wed, 21 Oct 2020 07:28:00 GMT",
			now:    now,
			wantD:  0,
			wantOK: false,
		},
		{
			name:   "ANSI C asctime accepted via http.ParseTime",
			h:      "Wed Oct 21 07:28:00 2026",
			now:    now,
			wantD:  60 * time.Second,
			wantOK: true,
		},
		{
			name:   "empty header",
			h:      "",
			now:    now,
			wantD:  0,
			wantOK: false,
		},
		{
			name:   "garbage input",
			h:      "garbage",
			now:    now,
			wantD:  0,
			wantOK: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			gotD, gotOK := parseRetryAfter(c.h, c.now)
			assert.Equal(t, c.wantOK, gotOK,
				"parseRetryAfter(%q, now) ok-flag want %v got %v", c.h, c.wantOK, gotOK)
			assert.Equal(t, c.wantD, gotD,
				"parseRetryAfter(%q, now) duration want %v got %v", c.h, c.wantD, gotD)
		})
	}
}

// TestComputeBackoff locks the full-jitter formula at three attempt
// counts: attempt=0 (capped at baseDelay), attempt=3 (capped at
// baseDelay << 3 < maxWait), and a Retry-After-only test in
// TestComputeBackoff_HonorsRetryAfter. The deterministic rand.Rand
// seed makes the upper-bound assertion stable.
func TestComputeBackoff(t *testing.T) {
	t.Parallel()

	cfg := retryConfig{
		baseDelay: 100 * time.Millisecond,
		maxWait:   time.Second,
	}

	t.Run("attempt 0 produces jitter in [0, baseDelay)", func(t *testing.T) {
		t.Parallel()
		rnd := newTestRand()
		got := computeBackoff(0, 0, cfg, rnd)
		// baseDelay << 0 = 100ms; capped = min(100ms, 1s) = 100ms;
		// jitter ∈ [0, 100ms).
		assert.GreaterOrEqual(t, got, time.Duration(0),
			"jitter must be non-negative")
		assert.Less(t, got, 100*time.Millisecond,
			"attempt 0: jitter must be < baseDelay (full-jitter formula)")
	})

	t.Run("attempt 3 produces jitter in [0, 800ms)", func(t *testing.T) {
		t.Parallel()
		rnd := newTestRand()
		got := computeBackoff(3, 0, cfg, rnd)
		// baseDelay << 3 = 800ms; capped = min(800ms, 1s) = 800ms;
		// jitter ∈ [0, 800ms).
		assert.GreaterOrEqual(t, got, time.Duration(0),
			"jitter must be non-negative")
		assert.Less(t, got, 800*time.Millisecond,
			"attempt 3: jitter must be < baseDelay<<3 (exponential ceiling)")
	})

	t.Run("attempt large enough to exceed maxWait is capped at maxWait", func(t *testing.T) {
		t.Parallel()
		rnd := newTestRand()
		// baseDelay << 10 = 102.4s; cap at maxWait=1s; jitter ∈ [0, 1s).
		got := computeBackoff(10, 0, cfg, rnd)
		assert.GreaterOrEqual(t, got, time.Duration(0),
			"jitter must be non-negative")
		assert.Less(t, got, time.Second,
			"attempt 10: jitter must be capped at maxWait (1s) — exponential ceiling clamped")
	})
}

// TestComputeBackoff_HonorsRetryAfter locks the Retry-After promotion
// branch in computeBackoff: when retryAfter > jitter, retryAfter
// wins; when retryAfter > maxWait, the result is capped at maxWait
// (threat T-04-05 — hostile Retry-After: 999999999 can't hold the
// request).
func TestComputeBackoff_HonorsRetryAfter(t *testing.T) {
	t.Parallel()

	t.Run("retryAfter > jitter — retryAfter wins", func(t *testing.T) {
		t.Parallel()
		cfg := retryConfig{
			baseDelay: 100 * time.Millisecond,
			maxWait:   10 * time.Second,
		}
		rnd := newTestRand()
		// retryAfter=5s; jitter ≤ 100ms; retryAfter wins; ≤ maxWait=10s.
		got := computeBackoff(0, 5*time.Second, cfg, rnd)
		assert.Equal(t, 5*time.Second, got,
			"retryAfter > jitter must promote retryAfter (capped at maxWait); got %v", got)
	})

	t.Run("retryAfter > maxWait — capped at maxWait (threat T-04-05)", func(t *testing.T) {
		t.Parallel()
		cfg := retryConfig{
			baseDelay: 100 * time.Millisecond,
			maxWait:   2 * time.Second,
		}
		rnd := newTestRand()
		// retryAfter=5s exceeds maxWait=2s; cap fires.
		got := computeBackoff(0, 5*time.Second, cfg, rnd)
		assert.Equal(t, 2*time.Second, got,
			"retryAfter must be capped at maxWait — threat T-04-05 (hostile Retry-After); got %v", got)
	})
}

// TestRetry_E2E_429Then500Then200 is the marquee TEST-05 case: a
// fake transport sequences 429 → 500 → 200; the endpoint method
// returns the decoded payload without error and the handler observes
// exactly three round trips. Verifies the loop body + retry
// composition end-to-end under a deterministic fakeClock.
func TestRetry_E2E_429Then500Then200(t *testing.T) {
	t.Parallel()

	t.Run("429 then 500 then 200 — exactly three round trips, payload decoded", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n := hits.Add(1)
			switch n {
			case 1:
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("rate limited"))
			case 2:
				w.WriteHeader(http.StatusInternalServerError)
			default:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}],"officialLanguages":["pl"]}]`))
			}
		}))
		t.Cleanup(srv.Close)

		fc := newFakeClock(time.Unix(0, 0))
		c := newClientForTest(fc.Now, fc.Sleep,
			WithBaseURL(srv.URL),
			WithRetry(5, 10*time.Millisecond),
			WithMaxRetryWait(time.Second),
		)
		cs, err := c.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err, "after 429→500→200 sequence, final 200 must decode without error")
		assert.Len(t, cs, 1, "decoded payload must produce exactly one Country")
		assert.Equal(t, int32(3), hits.Load(),
			"expected exactly 3 round trips (429 → 500 → 200); got %d", hits.Load())
	})
}

// TestRetry_HonorsRetryAfterSeconds locks D-76 integer-seconds
// branch: when an upstream sends Retry-After: 2, the per-attempt
// sleep computed by computeBackoff is at least 2 seconds of fake-
// clock advance.
func TestRetry_HonorsRetryAfterSeconds(t *testing.T) {
	t.Parallel()

	t.Run("Retry-After: 2 produces ≥ 2s of fake-clock advance", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n := hits.Add(1)
			if n == 1 {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}],"officialLanguages":["pl"]}]`))
		}))
		t.Cleanup(srv.Close)

		fc := newFakeClock(time.Unix(0, 0))
		fcStart := fc.Now()
		c := newClientForTest(fc.Now, fc.Sleep,
			WithBaseURL(srv.URL),
			WithRetry(2, 10*time.Millisecond),
			WithMaxRetryWait(10*time.Second),
		)
		_, err := c.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err, "after Retry-After: 2 + 200, call must succeed")
		assert.GreaterOrEqual(t, fc.Now().Sub(fcStart), 2*time.Second,
			"Retry-After: 2 must produce at least 2s of fake-clock advance (D-76); got %v", fc.Now().Sub(fcStart))
	})
}

// TestRetry_HonorsRetryAfterDate locks D-76 HTTP-date branch:
// Retry-After can also be an RFC 7231 date (RFC 1123 + RFC 850 +
// ANSI C asctime); parseRetryAfter computes the delta against
// c.nowFunc() (fakeClock) and produces the expected fake-clock
// advance. We seed fakeClock at a fixed instant 30s before the
// fixed date in the header.
func TestRetry_HonorsRetryAfterDate(t *testing.T) {
	t.Parallel()

	t.Run("HTTP-date Retry-After produces ≥ 30s of fake-clock advance", func(t *testing.T) {
		t.Parallel()
		fixedFuture := time.Date(2026, 10, 21, 7, 28, 0, 0, time.UTC)
		fcStart := time.Date(2026, 10, 21, 7, 27, 30, 0, time.UTC) // 30s before fixedFuture

		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n := hits.Add(1)
			if n == 1 {
				w.Header().Set("Retry-After", fixedFuture.Format(http.TimeFormat))
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}],"officialLanguages":["pl"]}]`))
		}))
		t.Cleanup(srv.Close)

		fc := newFakeClock(fcStart)
		c := newClientForTest(fc.Now, fc.Sleep,
			WithBaseURL(srv.URL),
			WithRetry(2, 10*time.Millisecond),
			WithMaxRetryWait(10*time.Minute),
		)
		_, err := c.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err, "after HTTP-date Retry-After + 200, call must succeed")
		assert.GreaterOrEqual(t, fc.Now().Sub(fcStart), 30*time.Second,
			"HTTP-date Retry-After must produce ≥ 30s of fake-clock advance (D-76 / RFC 7231 §7.1.1.1); got %v",
			fc.Now().Sub(fcStart))
	})
}

// TestRetry_CtxCancel verifies RESIL-04 + CLIENT-09 ≤ 100 ms
// cancellation contract: a ctx canceled BEFORE the retry loop runs
// causes the very first c.http.Do(req) to return ctx.Err(), and the
// post-loop wrap surfaces an error matching errors.Is(err,
// context.Canceled).
//
// Two pathways are tested:
//
//  1. Ctx canceled BEFORE the call: loop-top ctx.Err() check fires.
//  2. Ctx with a short deadline canceled DURING the sleep between
//     attempts: fc.Sleep returns ctx.Err() and the loop exits early.
func TestRetry_CtxCancel(t *testing.T) {
	t.Parallel()

	t.Run("ctx canceled before call returns ctx.Err() within ≤ 100 ms (loop-top check)", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(srv.Close)

		fc := newFakeClock(time.Unix(0, 0))
		c := newClientForTest(fc.Now, fc.Sleep,
			WithBaseURL(srv.URL),
			WithRetry(5, 100*time.Millisecond),
			WithMaxRetryWait(time.Second),
		)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel BEFORE the call so the loop-top ctx.Err() fires on attempt 0

		start := time.Now()
		_, err := c.Countries(ctx, CountriesRequest{})
		elapsed := time.Since(start)

		require.Error(t, err, "canceled ctx must produce an error")
		assert.True(t, errors.Is(err, context.Canceled),
			"expected errors.Is(err, context.Canceled); got %v", err)
		assert.Less(t, elapsed, 200*time.Millisecond,
			"ctx cancel must interrupt within ≤ 100 ms target (200 ms ceiling for CI slack); took %v", elapsed)
	})

	t.Run("ctx canceled during sleep between attempts returns ctx.Err() (fc.Sleep path)", func(t *testing.T) {
		t.Parallel()
		// Server returns 503 unconditionally so the loop reaches the
		// sleep step. fakeClock.Sleep checks ctx.Err() at the top and
		// returns it immediately when canceled — matching D-94 / D-95.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(srv.Close)

		fc := newFakeClock(time.Unix(0, 0))
		// Wrap fc.Sleep so we can cancel ctx the moment Sleep is
		// entered for the second attempt's pre-sleep — i.e. AFTER the
		// first c.http.Do returned 503 and shouldRetry==true. This
		// proves the ctx-aware sleep returns ctx.Err() per Pitfall
		// RETRY-3.
		ctx, cancel := context.WithCancel(context.Background())
		var sleepCount atomic.Int32
		wrappedSleep := func(ctxArg context.Context, d time.Duration) error {
			if sleepCount.Add(1) == 1 {
				cancel()
			}
			return fc.Sleep(ctxArg, d)
		}
		c := newClientForTest(fc.Now, wrappedSleep,
			WithBaseURL(srv.URL),
			WithRetry(5, 10*time.Millisecond),
			WithMaxRetryWait(time.Second),
		)

		start := time.Now()
		_, err := c.Countries(ctx, CountriesRequest{})
		elapsed := time.Since(start)

		require.Error(t, err, "canceled ctx during sleep must produce an error")
		assert.True(t, errors.Is(err, context.Canceled),
			"expected errors.Is(err, context.Canceled); got %v", err)
		assert.Less(t, elapsed, 200*time.Millisecond,
			"ctx cancel during sleep must interrupt within ≤ 100 ms target (RESIL-04); took %v", elapsed)
	})
}

// TestRetry_NeverRetriesCtxErrors is the direct verification of D-75
// "ctx errors NEVER retried": shouldRetry must return false for both
// context.Canceled and context.DeadlineExceeded passed as raw errors.
// This is a pure-function test — no HTTP path — that locks the
// shouldRetry contract on its own.
//
// The integration test for "ctx-aware retry loop returns ctx.Err()
// without invoking shouldRetry" is TestRetry_CtxCancel above.
func TestRetry_NeverRetriesCtxErrors(t *testing.T) {
	t.Parallel()

	t.Run("shouldRetry(nil, context.Canceled) is false", func(t *testing.T) {
		t.Parallel()
		assert.False(t, shouldRetry(nil, context.Canceled),
			"D-75: shouldRetry must NEVER claim to retry context.Canceled")
	})

	t.Run("shouldRetry(nil, context.DeadlineExceeded) is false", func(t *testing.T) {
		t.Parallel()
		assert.False(t, shouldRetry(nil, context.DeadlineExceeded),
			"D-75: shouldRetry must NEVER claim to retry context.DeadlineExceeded")
	})
}

// TestRetry_DeterministicClock is the TEST-05 explicit "fake clock"
// case: a server returns 503 three times then 200; the retry loop
// fires three sleeps (between attempts 1→2, 2→3, 3→4) before the
// final successful 4th attempt. With baseDelay=100ms and full-jitter,
// the worst-case cumulative fake-clock advance is 100+200+400 = 700ms.
// Asserting elapsed ≤ 700ms locks the bound; asserting elapsed ≥ 0
// is a sanity check.
func TestRetry_DeterministicClock(t *testing.T) {
	t.Parallel()

	t.Run("3×503 then 200 with baseDelay=100ms produces ≤ 700ms fake-clock advance", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n := hits.Add(1)
			if n < 4 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"isoCode":"PL","name":[{"language":"en","text":"Poland"}],"officialLanguages":["pl"]}]`))
		}))
		t.Cleanup(srv.Close)

		fc := newFakeClock(time.Unix(0, 0))
		start := fc.Now()
		c := newClientForTest(fc.Now, fc.Sleep,
			WithBaseURL(srv.URL),
			WithRetry(4, 100*time.Millisecond),
			WithMaxRetryWait(10*time.Second),
		)
		_, err := c.Countries(context.Background(), CountriesRequest{})
		require.NoError(t, err, "4-attempt loop with 3×503+200 must succeed")
		assert.Equal(t, int32(4), hits.Load(),
			"expected exactly 4 round trips (3×503 + 1×200)")

		elapsed := fc.Now().Sub(start)
		// Worst-case: 100ms + 200ms + 400ms = 700ms (full-jitter
		// upper bound at each step).
		assert.LessOrEqual(t, elapsed, 700*time.Millisecond,
			"max fake-clock advance is 700ms with baseDelay=100ms across 3 retry sleeps (full-jitter ceiling); got %v",
			elapsed)
		assert.GreaterOrEqual(t, elapsed, time.Duration(0),
			"fake clock must have advanced at least 0; got %v", elapsed)
	})
}

// TestRetry_NotARoundTripper is the RESIL-05 structural audit: the
// repo must NOT contain a transport_retry.go file and no Go file at
// the repo root may declare a `type retryTransport` struct (only this
// test itself may mention the name). This locks the D-77 placement
// decision against accidental regression: a future "let's just make
// retry a RoundTripper" PR breaks here.
func TestRetry_NotARoundTripper(t *testing.T) {
	t.Parallel()

	repoRoot, err := findRepoRoot()
	require.NoError(t, err, "could not locate repo root for structural audit")

	t.Run("no transport_retry.go file exists at repo root", func(t *testing.T) {
		t.Parallel()
		require.NoFileExists(t, filepath.Join(repoRoot, "transport_retry.go"),
			"RESIL-05: retry is in the endpoint layer (D-77), NOT a RoundTripper. transport_retry.go must NOT exist.")
	})

	t.Run("no production .go file declares 'type retryTransport'", func(t *testing.T) {
		t.Parallel()
		entries, err := os.ReadDir(repoRoot)
		require.NoError(t, err, "could not read repo root for structural audit")

		var offenders []string
		for _, ent := range entries {
			name := ent.Name()
			if ent.IsDir() || !strings.HasSuffix(name, ".go") {
				continue
			}
			// Skip the audit file itself — the test reads the
			// retryTransport literal into a string above, and we don't
			// want a self-match.
			if name == "retry_test.go" {
				continue
			}
			// Skip *_test.go files — RESIL-05 audits production code.
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(repoRoot, name)
			f, err := os.Open(path)
			require.NoErrorf(t, err, "could not open %s for audit", path)
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				// Match "type retryTransport struct" or any "type
				// retryTransport ..." declaration. A leading whitespace
				// is allowed (it would be a syntax error in Go but
				// guard anyway).
				trimmed := strings.TrimLeft(line, " \t")
				if strings.HasPrefix(trimmed, "type retryTransport") {
					offenders = append(offenders, name+": "+line)
				}
			}
			_ = f.Close()
			require.NoErrorf(t, scanner.Err(), "scanner error reading %s", path)
		}
		assert.Empty(t, offenders,
			"RESIL-05: no production .go file may declare 'type retryTransport' — retry lives in doJSONGet, NOT a RoundTripper. Offenders: %v",
			offenders)
	})
}
