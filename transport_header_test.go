package openholidays

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripperFunc is a test-only adapter that lets a plain function satisfy
// the http.RoundTripper interface. It is used as the "next" slot inside a
// transport-under-test so the test can capture the request it sees and
// return a synthesized response. Declared once here and shared with
// transport_logging_test.go via same-package visibility (D-50).
type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip satisfies http.RoundTripper by delegating to f.
func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// TestHeaderTransport_RoundTrip locks the three documented branches of
// headerTransport.RoundTrip:
//
//  1. Defaults are injected when the caller did not supply Accept or
//     User-Agent (TRANS-01).
//  2. Caller-supplied Accept and User-Agent are preserved verbatim (D-30
//     "caller override wins").
//  3. A network-level error from the next RoundTripper is propagated
//     unchanged (no swallow, no decoration).
//
// The Pitfall HTTP-2 invariant (req.Clone-before-mutate) is asserted by the
// first branch via "caller's original req.Header.Get(Accept) remains empty
// after RoundTrip" — proving headerTransport did not mutate the inbound
// *http.Request.
func TestHeaderTransport_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("sets defaults when caller did not supply them", func(t *testing.T) {
		t.Parallel()

		var captured *http.Request
		h := &headerTransport{
			userAgent: "go-openholidays/" + Version,
			next: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				captured = r
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}

		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		require.NotNil(t, captured, "next.RoundTrip was not invoked")

		assert.Equal(t, "application/json", captured.Header.Get("Accept"),
			"headerTransport must default Accept to application/json")
		assert.Equal(t, "go-openholidays/"+Version, captured.Header.Get("User-Agent"),
			"headerTransport must default User-Agent to go-openholidays/<Version>")

		// Pitfall HTTP-2 / D-30: the caller's *http.Request must NOT have been
		// mutated. req.Clone(req.Context()) deep-copies the Header map; if
		// headerTransport mutated req.Header directly, this assertion would
		// observe the leaked "application/json".
		assert.Empty(t, req.Header.Get("Accept"),
			"caller's original req.Header must be untouched (Pitfall HTTP-2 / D-30)")
		assert.Empty(t, req.Header.Get("User-Agent"),
			"caller's original req.Header must be untouched (Pitfall HTTP-2 / D-30)")
	})

	t.Run("preserves caller-supplied Accept and User-Agent", func(t *testing.T) {
		t.Parallel()

		var captured *http.Request
		h := &headerTransport{
			userAgent: "go-openholidays/" + Version,
			next: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				captured = r
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}

		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)
		req.Header.Set("Accept", "application/vnd.custom+json")
		req.Header.Set("User-Agent", "my-app/2.0")

		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		require.NotNil(t, captured, "next.RoundTrip was not invoked")

		assert.Equal(t, "application/vnd.custom+json", captured.Header.Get("Accept"),
			"caller-supplied Accept must survive verbatim (caller override wins)")
		assert.Equal(t, "my-app/2.0", captured.Header.Get("User-Agent"),
			"caller-supplied User-Agent must survive verbatim (caller override wins)")
	})

	t.Run("next RoundTripper error is propagated", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("boom")
		h := &headerTransport{
			userAgent: "go-openholidays/" + Version,
			next: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return nil, wantErr
			}),
		}

		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		assert.Nil(t, resp, "headerTransport must propagate nil response on next error")
		require.Error(t, err)
		assert.Equal(t, "boom", err.Error(),
			"headerTransport must propagate the next RoundTripper's error verbatim")
	})
}
