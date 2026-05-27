// Package openholidays — tests for the five public WithX option constructors.
//
// One TestXxx per exported production function (Gold Rule 3). Each test
// is table-driven where ≥ 2 cases share setup, otherwise explicit subtests.
// require for preconditions, assert for verifications; every leaf t.Run
// calls t.Parallel() per Phase 1 etiquette.

package openholidays

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithHTTPClient covers CLIENT-02: nil = no-op, non-nil shallow-copies
// so caller mutations don't leak, and CheckRedirect is preserved across
// the shallow copy (RESEARCH OQ-2).
func TestWithHTTPClient(t *testing.T) {
	t.Parallel()

	t.Run("nil is no-op (default httpClient preserved)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithHTTPClient(nil))
		require.NotNil(t, c)
		assert.NotNil(t, c.http,
			"default httpClient must be present when WithHTTPClient(nil) supplied")
	})

	t.Run("non-nil shallow-copies (Pitfall HTTP-1 / D-37)", func(t *testing.T) {
		t.Parallel()
		custom := &http.Client{Timeout: 5 * time.Second}
		c := NewClient(WithHTTPClient(custom))
		require.NotNil(t, c)
		originalT := custom.Timeout
		// Mutate the caller's *http.Client after NewClient returned. The
		// SDK's internal *http.Client must NOT observe the mutation per
		// Pitfall HTTP-1.
		custom.Timeout = 100 * time.Millisecond
		assert.Equal(t, originalT, c.http.Timeout,
			"SDK's internal *http.Client must NOT observe post-NewClient mutation (Pitfall HTTP-1 / D-37)")
	})

	t.Run("preserves CheckRedirect across shallow copy", func(t *testing.T) {
		t.Parallel()
		// RESEARCH OQ-2: composeHTTPClient's shallow copy must preserve
		// every non-Transport field on the caller's *http.Client.
		custom := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		c := NewClient(WithHTTPClient(custom))
		require.NotNil(t, c)
		assert.NotNil(t, c.http.CheckRedirect,
			"CheckRedirect must be preserved by composeHTTPClient's shallow copy")
	})
}

// TestWithBaseURL covers CLIENT-03 and the trailing-slash trim from
// RESEARCH OQ-4 (single + multiple trailing slashes), plus the empty =
// no-op fallback.
func TestWithBaseURL(t *testing.T) {
	t.Parallel()

	type tc struct {
		name string
		in   string
		want string
	}
	cases := []tc{
		{name: "no trailing slash passes through", in: "https://example.test", want: "https://example.test"},
		{name: "single trailing slash trimmed", in: "https://example.test/", want: "https://example.test"},
		{name: "multiple trailing slashes trimmed", in: "https://example.test///", want: "https://example.test"},
		{name: "empty string is no-op (default kept)", in: "", want: "https://openholidaysapi.org"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cli := NewClient(WithBaseURL(c.in))
			require.NotNil(t, cli)
			assert.Equal(t, c.want, cli.baseURL)
		})
	}
}

// TestWithUserAgent covers CLIENT-04 and D-38 (empty = no-op so the SDK
// never sends an empty User-Agent — Pitfall HTTP-5 mitigation).
func TestWithUserAgent(t *testing.T) {
	t.Parallel()

	t.Run("non-empty replaces default", func(t *testing.T) {
		t.Parallel()
		cli := NewClient(WithUserAgent("my-app/2.0"))
		require.NotNil(t, cli)
		assert.Equal(t, "my-app/2.0", cli.userAgent,
			"non-empty argument must replace the default UA")
	})

	t.Run("empty string is no-op (default kept)", func(t *testing.T) {
		t.Parallel()
		cli := NewClient(WithUserAgent(""))
		require.NotNil(t, cli)
		assert.Equal(t, "go-openholidays/"+Version, cli.userAgent,
			"empty argument must keep the default UA (D-38 / Pitfall HTTP-5)")
	})
}

// TestWithLogger covers CLIENT-05 and D-39 (nil falls back to
// slog.Default()). Two explicit subtests because the nil-vs-non-nil
// comparison is fiddly in a struct literal.
func TestWithLogger(t *testing.T) {
	t.Parallel()

	t.Run("non-nil is assigned verbatim", func(t *testing.T) {
		t.Parallel()
		customLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		cli := NewClient(WithLogger(customLogger))
		require.NotNil(t, cli)
		assert.Same(t, customLogger, cli.logger,
			"custom logger must be assigned verbatim, not wrapped")
	})

	t.Run("nil falls back to slog.Default() (D-39)", func(t *testing.T) {
		t.Parallel()
		cli := NewClient(WithLogger(nil))
		require.NotNil(t, cli)
		// slog.Default() returns a non-stable pointer, so we cannot assert
		// pointer equality. The contract is "the logger is non-nil and the
		// library did not mutate the process-wide default" — non-nil is
		// mechanically checked here; the no-mutation invariant is checked
		// at the package level by acceptance grep (count must be 0 across
		// production source).
		assert.NotNil(t, cli.logger,
			"nil logger argument must be replaced by slog.Default(), never left nil")
	})
}

// TestWithTimeout covers CLIENT-06 and D-28 (zero = no SDK timeout, stored
// verbatim; negative durations are also stored verbatim per D-28).
func TestWithTimeout(t *testing.T) {
	t.Parallel()

	t.Run("positive duration assigned", func(t *testing.T) {
		t.Parallel()
		cli := NewClient(WithTimeout(30 * time.Second))
		require.NotNil(t, cli)
		assert.Equal(t, 30*time.Second, cli.timeout,
			"positive duration must be assigned verbatim")
	})

	t.Run("zero disables SDK timeout (verbatim)", func(t *testing.T) {
		t.Parallel()
		cli := NewClient(WithTimeout(0))
		require.NotNil(t, cli)
		assert.Equal(t, time.Duration(0), cli.timeout,
			"zero duration must be stored verbatim per D-28 (no fallback to default)")
	})

	t.Run("negative stored verbatim", func(t *testing.T) {
		t.Parallel()
		cli := NewClient(WithTimeout(-5 * time.Second))
		require.NotNil(t, cli)
		assert.Equal(t, -5*time.Second, cli.timeout,
			"negative duration must be stored verbatim per D-28 (endpoint methods treat non-positive as no-timeout)")
	})
}
