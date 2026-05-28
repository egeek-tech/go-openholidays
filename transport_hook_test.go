// Package openholidays — transport_hook_test.go: tests for the hookTransport
// http.RoundTripper layer (Phase 4 Plan 05 / TRANS-05).
//
// Gold Rule 3: testify (assert + require) is the primary assertion library;
// one TestXxx function per observable layer-level concern; every leaf is
// inside a t.Run with t.Parallel() (loop-var capture for table-driven cases
// is automatic since Go 1.22).
//
// Coverage matrix (per CONTEXT.md D-87..D-90 + plan must_haves):
//
//   - TestHookTransport_RoundTrip — the three documented branches of the
//     wrapper itself (invocation, transport error, nil hook).
//   - TestHookTransport_FiresPerAttempt — the layer-level contract that
//     three round trips through the wrapper produce three hook invocations
//     (TRANS-05 — composes with retry in client_test.go's integration test).
//   - TestHookTransport_PanicPropagates — D-90 explicit: a panicking hook
//     propagates the panic to the caller (mirrors stdlib http.Handler
//     convention); the library does NOT use defer/recover.
//   - TestHookTransport_NilSafeOnTransportError — D-88 explicit: hook is
//     invoked with resp == nil on transport-level failure; implementations
//     MUST nil-check resp (documented contract on RequestHookFunc godoc in
//     config.go).
//
// roundTripperFunc is reused via same-package visibility from
// transport_header_test.go (D-50) — DO NOT redeclare here.

package openholidays

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHookTransport_RoundTrip locks the three documented branches of
// hookTransport.RoundTrip:
//
//  1. The hook is invoked exactly once after the next RoundTripper returns
//     with the (req, resp, err) triple — D-88 / Pattern 3.
//  2. The hook receives nil resp on transport error — D-88 nil-safe contract.
//  3. A nil hook is a transparent pass-through (no panic, no extra call,
//     resp matches what next returned).
func TestHookTransport_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("hook is invoked exactly once per round trip with the (req, resp, err) triple", func(t *testing.T) {
		t.Parallel()

		var (
			capturedReq  *http.Request
			capturedResp *http.Response
			capturedErr  error
			calls        atomic.Int32
		)
		hook := func(req *http.Request, resp *http.Response, err error) {
			capturedReq = req
			capturedResp = resp
			capturedErr = err
			calls.Add(1)
		}
		next := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		})
		tr := &hookTransport{hook: hook, next: next}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		resp, err := tr.RoundTrip(req)
		require.NoError(t, err, "next returned success — hookTransport must propagate")
		require.NotNil(t, resp, "next returned a response — hookTransport must propagate")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, int32(1), calls.Load(),
			"hook must be invoked exactly once per round trip (D-88)")
		assert.NotNil(t, capturedReq, "hook must receive the request pointer")
		assert.NotNil(t, capturedResp, "hook must receive the response pointer on success")
		assert.NoError(t, capturedErr, "hook must receive nil error on success")
	})

	t.Run("hook receives nil resp on transport error (D-88 nil-safe contract)", func(t *testing.T) {
		t.Parallel()

		var (
			capturedResp *http.Response
			capturedErr  error
			calls        atomic.Int32
		)
		hook := func(_ *http.Request, resp *http.Response, err error) {
			capturedResp = resp
			capturedErr = err
			calls.Add(1)
		}
		boom := errors.New("boom")
		next := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, boom
		})
		tr := &hookTransport{hook: hook, next: next}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		resp, err := tr.RoundTrip(req)
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
		}
		assert.Nil(t, resp, "hookTransport must propagate nil resp on next error")
		require.Error(t, err, "hookTransport must propagate next's error")
		assert.Equal(t, "boom", err.Error(), "error must propagate verbatim (no decoration)")

		assert.Equal(t, int32(1), calls.Load(),
			"hook must still be invoked on transport error (D-88 fires on every round trip)")
		assert.Nil(t, capturedResp,
			"hook must receive nil resp on transport error (D-88; implementations must nil-check)")
		require.Error(t, capturedErr, "hook must receive non-nil err on transport error")
		assert.Equal(t, "boom", capturedErr.Error(),
			"hook must receive the next RoundTripper's error verbatim")
	})

	t.Run("nil hook is transparent (no panic, no extra call, resp matches next)", func(t *testing.T) {
		t.Parallel()

		wantBody := io.NopCloser(strings.NewReader("hello"))
		next := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       wantBody,
			}, nil
		})
		tr := &hookTransport{hook: nil, next: next}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		require.NotPanics(t, func() {
			resp, rtErr := tr.RoundTrip(req)
			require.NoError(t, rtErr, "nil hook must not affect next's success return")
			require.NotNil(t, resp, "nil hook must pass through next's resp")
			defer func() { _ = resp.Body.Close() }()
			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"nil hook must NOT modify or replace next's response")
		})
	})
}

// TestHookTransport_FiresPerAttempt locks TRANS-05 at the layer level:
// invoking RoundTrip N times produces N hook invocations. The integration
// view (retry loop calls RoundTrip three times for a 429→500→200 sequence)
// is verified separately in client_test.go::TestHook_FiresOnRetryAttempts.
//
// This test exists to lock the layer-level contract independent of the
// endpoint pipeline — if a future change ever inserted accidental batching
// or caching at the hook layer, the integration test alone might miss it.
func TestHookTransport_FiresPerAttempt(t *testing.T) {
	t.Parallel()

	t.Run("three RoundTrip invocations produce three hook calls (429→500→200 layer-level)", func(t *testing.T) {
		t.Parallel()

		var (
			serverHits atomic.Int32
			hookCalls  atomic.Int32
		)
		next := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			i := serverHits.Add(1)
			switch i {
			case 1:
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			case 2:
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}
		})
		hook := func(_ *http.Request, _ *http.Response, _ error) {
			hookCalls.Add(1)
		}
		tr := &hookTransport{hook: hook, next: next}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		// Manually invoke RoundTrip three times — simulates the retry loop
		// in doJSONGet which dispatches a fresh c.http.Do per attempt. The
		// hook must fire on each.
		for range 3 {
			resp, _ := tr.RoundTrip(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
		}

		assert.Equal(t, int32(3), serverHits.Load(),
			"next RoundTripper must be called once per RoundTrip invocation (sanity)")
		assert.Equal(t, int32(3), hookCalls.Load(),
			"hook must fire once per RoundTrip (TRANS-05 — three attempts → three invocations)")
	})
}

// TestHookTransport_PanicPropagates locks D-90 explicit: a panicking hook
// propagates the panic to the caller. The library does NOT use defer/recover
// (mirrors stdlib http.Handler convention — silent recovery would hide bugs
// and is the documented "consumer responsibility" path).
//
// testify's assert.PanicsWithValue catches the panic without letting it
// escape the test goroutine; the assertion proves that the panic value
// reaches the caller (RoundTrip's frame did not swallow it).
func TestHookTransport_PanicPropagates(t *testing.T) {
	t.Parallel()

	t.Run("a panicking hook propagates the panic to the caller (D-90)", func(t *testing.T) {
		t.Parallel()

		panickyHook := func(_ *http.Request, _ *http.Response, _ error) {
			panic("oops")
		}
		next := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		})
		tr := &hookTransport{hook: panickyHook, next: next}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		assert.PanicsWithValue(t, "oops", func() {
			// nolint:bodyclose // the hook panics before RoundTrip returns,
			// so the response body never reaches the caller and cannot be
			// closed here. The panic itself is the asserted behavior.
			_, _ = tr.RoundTrip(req)
		}, "panic in user hook MUST propagate — library does not defer/recover (D-90)")
	})
}

// TestHookTransport_NilSafeOnTransportError documents the nil-resp contract
// at the hook side: on a transport error the hook receives a nil *http.Response
// and a non-nil error. Implementations MUST nil-check resp before
// dereferencing. The RequestHookFunc godoc in config.go states this contract;
// this test locks it mechanically.
func TestHookTransport_NilSafeOnTransportError(t *testing.T) {
	t.Parallel()

	t.Run("hook nil-checks resp on transport error", func(t *testing.T) {
		t.Parallel()

		netFailure := errors.New("net failure")
		next := roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, netFailure
		})

		var hookSawNilResp bool
		var hookSawErr bool
		hook := func(_ *http.Request, resp *http.Response, err error) {
			// MUST nil-check resp before any field access. Test asserts
			// this is the actual shape received.
			if resp == nil {
				hookSawNilResp = true
			}
			if err != nil {
				hookSawErr = true
			}
		}
		tr := &hookTransport{hook: hook, next: next}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		resp, err := tr.RoundTrip(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		require.Error(t, err, "RoundTrip must surface the transport error")
		assert.Equal(t, "net failure", err.Error(),
			"transport error must propagate verbatim")

		assert.True(t, hookSawNilResp,
			"hook must see resp == nil on transport error (D-88 contract)")
		assert.True(t, hookSawErr,
			"hook must see err != nil on transport error (D-88 contract)")
	})
}
