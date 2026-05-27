package openholidays

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackedReader is a test-only io.Reader that counts how many times Read is
// invoked. It is used by the OBS-01 invariant assertion: loggingTransport
// must NOT read the response body inside RoundTrip. The Read counter must
// remain at zero after RoundTrip returns. Read returns io.EOF immediately
// without producing any bytes — the test does not care about the content,
// only about whether anything tried to consume it.
type trackedReader struct {
	reads atomic.Int64
}

// Read increments the read counter and returns io.EOF. The counter uses
// atomic.Int64 because RoundTrip and the assertion run on the same
// goroutine in this test, but t.Parallel sibling subtests could otherwise
// concurrently mutate a shared trackedReader (each subtest constructs its
// own here, but the atomic is defense-in-depth).
func (r *trackedReader) Read(_ []byte) (int, error) {
	r.reads.Add(1)
	return 0, io.EOF
}

// TestLoggingTransport_RoundTrip locks the four documented branches of
// loggingTransport.RoundTrip:
//
//  1. A single slog.LevelDebug record with all six OBS-02 fields is
//     emitted (D-31).
//  2. resp.ContentLength == -1 (HTTP/2 chunked, the live OpenHolidays
//     wire shape) is forwarded as bytes_in == -1, NOT coerced to 0.
//  3. On network-level failure (next returns (nil, err)) the record still
//     emits with status == -1 and bytes_in == -1 — the nil-safe paths in
//     statusOf / bytesIn.
//  4. The response body is NEVER read inside RoundTrip — the OBS-01 / Pitfall
//     OBS-1 invariant, asserted mechanically via a trackedReader whose
//     Read counter must remain at zero.
//
// Each subtest constructs its own bytes.Buffer + slog.Logger so subtests can
// safely t.Parallel without contending on a shared sink.
func TestLoggingTransport_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("emits Debug record with all OBS-02 fields", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		l := &loggingTransport{
			logger: logger,
			next: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					ContentLength: 6055,
					Body:          io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}

		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		_, err = l.RoundTrip(req)
		require.NoError(t, err)

		var rec map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &rec),
			"slog JSON output must be a single decodable object")

		assert.Equal(t, "DEBUG", rec["level"], "record must be emitted at Debug (OBS-01)")
		assert.Equal(t, "openholidays http", rec["msg"], "record message must be the fixed token")
		assert.Equal(t, "GET", rec["method"])
		assert.Equal(t, "/Countries", rec["path"])
		assert.EqualValues(t, 200, rec["status"])
		assert.EqualValues(t, 1, rec["attempt"], "Phase 2 hardcodes attempt=1 (D-31)")
		assert.EqualValues(t, 6055, rec["bytes_in"], "bytes_in must mirror resp.ContentLength when known")

		dur, ok := rec["duration_ms"].(float64)
		require.True(t, ok, "duration_ms missing or wrong type: %v", rec["duration_ms"])
		assert.GreaterOrEqual(t, dur, 0.0, "duration_ms must be non-negative")
	})

	t.Run("forwards ContentLength=-1 as bytes_in=-1 (HTTP/2 chunked)", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		l := &loggingTransport{
			logger: logger,
			next: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					ContentLength: -1,
					Body:          io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}

		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		_, err = l.RoundTrip(req)
		require.NoError(t, err)

		var rec map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))

		// Critical: bytes_in must be -1, NOT 0. Coercing -1 to 0 would lose
		// the diagnostic signal that the response was chunked.
		assert.EqualValues(t, -1, rec["bytes_in"],
			"loggingTransport must forward ContentLength=-1 unchanged for HTTP/2 chunked responses (D-31)")
		assert.EqualValues(t, 200, rec["status"])
	})

	t.Run("logs network error with status=-1 and bytes_in=-1", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		l := &loggingTransport{
			logger: logger,
			next: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			}),
		}

		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		resp, err := l.RoundTrip(req)
		require.Error(t, err, "network error must propagate to the caller")
		assert.Nil(t, resp, "loggingTransport must propagate nil response on next error")

		var rec map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &rec),
			"loggingTransport must still emit a record even when next returns an error")

		// nil-safe paths in statusOf / bytesIn produce -1 sentinels.
		assert.EqualValues(t, -1, rec["status"], "statusOf must return -1 when resp is nil")
		assert.EqualValues(t, -1, rec["bytes_in"], "bytesIn must return -1 when resp is nil")
		assert.Equal(t, "GET", rec["method"])
		assert.Equal(t, "/Countries", rec["path"])
	})

	t.Run("does not read resp.Body (OBS-01)", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		tr := &trackedReader{}
		l := &loggingTransport{
			logger: logger,
			next: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					ContentLength: 100,
					Body:          io.NopCloser(tr),
				}, nil
			}),
		}

		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		_, err = l.RoundTrip(req)
		require.NoError(t, err)

		// OBS-01 / Pitfall OBS-1: the response body must never be read inside
		// loggingTransport. If a future change introduced Read/ReadAll/Copy
		// on the body, this counter would be >= 1.
		assert.Zero(t, tr.reads.Load(),
			"loggingTransport must not read resp.Body (OBS-01 / Pitfall OBS-1)")
	})
}
