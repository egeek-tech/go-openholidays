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
		// WR-01 regression: inputs that collapse to "" after
		// strings.TrimRight(u, "/") must be treated as no-op too,
		// otherwise cfg.baseURL silently becomes "" and downstream
		// HTTP calls fail with opaque "unsupported protocol scheme"
		// errors far from the misconfiguration.
		{name: "single slash trims to empty is no-op (default kept)", in: "/", want: "https://openholidaysapi.org"},
		{name: "all-slashes trims to empty is no-op (default kept)", in: "////", want: "https://openholidaysapi.org"},
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

// TestWithStrictDecoding covers D-91 / Pitfall JSON-1: strict-decoding is
// OFF by default; WithStrictDecoding(true) flips the immutable c.strict
// flag; WithStrictDecoding(false) stores false verbatim. The flag is
// immutable after NewClient by design (no runtime toggle exists — see
// D-91 + CL-15).
func TestWithStrictDecoding(t *testing.T) {
	t.Parallel()

	t.Run("stores true verbatim", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithStrictDecoding(true))
		require.NotNil(t, c)
		assert.True(t, c.strict,
			"WithStrictDecoding(true) must set Client.strict to true (D-91)")
	})

	t.Run("stores false verbatim (default behavior)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithStrictDecoding(false))
		require.NotNil(t, c)
		assert.False(t, c.strict,
			"WithStrictDecoding(false) must store false verbatim (no defensive special-case per D-91)")
	})

	t.Run("default Client has strict == false (off by default per D-91 / JSON-1)", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		assert.False(t, c.strict,
			"strict-decoding must be OFF by default — upstream schema drifts and silent rejection would break consumers (Pitfall JSON-1)")
	})
}

// TestWithRetry covers D-73 / D-74 / RESIL-01..05: the public WithRetry
// option stores maxAttempts verbatim (including the <=0 disabled
// sentinel) and applies the defaultBaseDelay fallback when the supplied
// baseDelay is non-positive. The default-disabled invariant (no
// WithRetry call → maxAttempts == 0) is locked too.
func TestWithRetry(t *testing.T) {
	t.Parallel()

	t.Run("positive args stored verbatim", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithRetry(5, 100*time.Millisecond))
		require.NotNil(t, c)
		assert.Equal(t, 5, c.retry.maxAttempts,
			"positive maxAttempts must be stored verbatim per D-74")
		assert.Equal(t, 100*time.Millisecond, c.retry.baseDelay,
			"positive baseDelay must be stored verbatim per D-74")
		assert.Equal(t, defaultMaxRetryWait, c.retry.maxWait,
			"WithRetry must seed maxWait to defaultMaxRetryWait when WithMaxRetryWait is not also called (D-74)")
	})

	t.Run("maxAttempts <= 0 stored as DISABLED (defensive symmetry with WithTimeout(0))", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithRetry(0, 100*time.Millisecond))
		require.NotNil(t, c)
		assert.Equal(t, 0, c.retry.maxAttempts,
			"maxAttempts=0 must be stored verbatim — interpreted as DISABLED by the retry loop per D-74")
	})

	t.Run("baseDelay <= 0 falls back to defaultBaseDelay", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithRetry(3, 0))
		require.NotNil(t, c)
		assert.Equal(t, 3, c.retry.maxAttempts,
			"maxAttempts must still be stored when baseDelay falls back to default")
		assert.Equal(t, defaultBaseDelay, c.retry.baseDelay,
			"baseDelay <= 0 must fall back to defaultBaseDelay (250ms) per D-74")
	})

	t.Run("negative baseDelay falls back to defaultBaseDelay", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithRetry(3, -1*time.Second))
		require.NotNil(t, c)
		assert.Equal(t, defaultBaseDelay, c.retry.baseDelay,
			"negative baseDelay must fall back to defaultBaseDelay per D-74 (treat non-positive uniformly)")
	})

	t.Run("default Client (no WithRetry) has retry disabled", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		assert.Equal(t, 0, c.retry.maxAttempts,
			"default Client must have retry disabled — opt-in per D-74 / STATE.md")
	})

	t.Run("WithRetry followed by WithMaxRetryWait — last-wins on maxWait", func(t *testing.T) {
		t.Parallel()
		c := NewClient(
			WithRetry(3, 100*time.Millisecond),
			WithMaxRetryWait(5*time.Second),
		)
		require.NotNil(t, c)
		assert.Equal(t, 5*time.Second, c.retry.maxWait,
			"WithMaxRetryWait after WithRetry must overwrite the default maxWait (functional-options last-wins)")
	})

	t.Run("WithMaxRetryWait followed by WithRetry — WithRetry preserves caller-supplied maxWait", func(t *testing.T) {
		t.Parallel()
		c := NewClient(
			WithMaxRetryWait(5*time.Second),
			WithRetry(3, 100*time.Millisecond),
		)
		require.NotNil(t, c)
		// WithRetry only seeds maxWait when cfg.retry.maxWait <= 0; a
		// prior WithMaxRetryWait(5s) leaves it at 5s, so WithRetry
		// must NOT clobber it. This is the D-74 "preserve caller
		// intent" rule.
		assert.Equal(t, 5*time.Second, c.retry.maxWait,
			"WithRetry must preserve a caller-supplied positive maxWait from a prior WithMaxRetryWait call")
	})
}

// TestWithMaxRetryWait covers D-74: positive duration stored verbatim;
// non-positive duration falls back to defaultMaxRetryWait. The cap is
// per-attempt, NOT cumulative (godoc-documented; not mechanically
// checked here).
func TestWithMaxRetryWait(t *testing.T) {
	t.Parallel()

	t.Run("positive d stored verbatim", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithMaxRetryWait(10 * time.Second))
		require.NotNil(t, c)
		assert.Equal(t, 10*time.Second, c.retry.maxWait,
			"positive d must be stored verbatim per D-74")
	})

	t.Run("zero d falls back to defaultMaxRetryWait", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithMaxRetryWait(0))
		require.NotNil(t, c)
		assert.Equal(t, defaultMaxRetryWait, c.retry.maxWait,
			"zero d must fall back to defaultMaxRetryWait (60s) per D-74 — calling WithMaxRetryWait(0) does NOT disable the cap")
	})

	t.Run("negative d falls back to defaultMaxRetryWait", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithMaxRetryWait(-1 * time.Second))
		require.NotNil(t, c)
		assert.Equal(t, defaultMaxRetryWait, c.retry.maxWait,
			"negative d must fall back to defaultMaxRetryWait per D-74 (treat non-positive uniformly)")
	})
}

// TestWithCache covers D-79 / D-80: positive ttl populates cfg.cache (a
// real *MemoryCache); ttl <= 0 disables (cfg.cache stays nil).
func TestWithCache(t *testing.T) {
	t.Parallel()

	t.Run("positive ttl populates cache (MemoryCache.ttl reflects argument)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithCache(2 * time.Hour))
		require.NotNil(t, c)
		require.NotNil(t, c.cache, "positive ttl must populate the cache field")
		t.Cleanup(func() { _ = c.Close() })

		// Underlying type is the default in-memory implementation.
		// MemoryCache.ttl is the real source of truth (WR-04 follow-up
		// removed the redundant clientConfig.cacheTTL mirror field).
		mc, ok := c.cache.(*MemoryCache)
		require.True(t, ok, "WithCache(ttl) must wire a *MemoryCache by default")
		assert.Equal(t, 2*time.Hour, mc.ttl, "MemoryCache.ttl must reflect the WithCache argument")
	})

	t.Run("ttl == 0 disables (cfg.cache stays nil)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithCache(0))
		require.NotNil(t, c)
		assert.Nil(t, c.cache,
			"WithCache(0) must NOT populate the cache field (D-80 ttl <= 0 disables)")
	})

	t.Run("negative ttl disables (cfg.cache stays nil)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithCache(-time.Hour))
		require.NotNil(t, c)
		assert.Nil(t, c.cache,
			"WithCache(<0) must NOT populate the cache field (D-80 defensive symmetry)")
	})
}

// TestWithCache_NonPositiveTTLDisables is the explicit RESIL-07/D-80 lock
// — duplication intentional (named test documenting the requirement).
func TestWithCache_NonPositiveTTLDisables(t *testing.T) {
	t.Parallel()

	t.Run("zero ttl is treated as disabled", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithCache(0))
		require.NotNil(t, c)
		assert.Nil(t, c.cache, "WithCache(0) must NOT enable caching")
	})

	t.Run("negative ttl is treated as disabled", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithCache(-5 * time.Minute))
		require.NotNil(t, c)
		assert.Nil(t, c.cache, "WithCache(<0) must NOT enable caching")
	})
}

// TestWithCacheBackend covers D-80 last-wins + nil-no-op convention.
func TestWithCacheBackend(t *testing.T) {
	t.Parallel()

	t.Run("non-nil backend stored verbatim", func(t *testing.T) {
		t.Parallel()
		custom := NewMemoryCache(time.Hour)
		c := NewClient(WithCacheBackend(custom))
		require.NotNil(t, c)
		t.Cleanup(func() { _ = c.Close() })

		assert.Same(t, custom, c.cache,
			"WithCacheBackend must store the supplied backend verbatim (identity-equal)")
	})

	t.Run("nil backend is a no-op (cfg.cache stays nil)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithCacheBackend(nil))
		require.NotNil(t, c)
		assert.Nil(t, c.cache,
			"WithCacheBackend(nil) must NOT overwrite cfg.cache (nil-no-op convention per WithHTTPClient analog)")
	})

	t.Run("WithCacheBackend supersedes prior WithCache(ttl) (D-80 last-wins)", func(t *testing.T) {
		t.Parallel()
		custom := NewMemoryCache(time.Hour)
		c := NewClient(WithCache(2*time.Hour), WithCacheBackend(custom))
		require.NotNil(t, c)
		t.Cleanup(func() {
			_ = c.Close()
			_ = custom.Close() // the WithCache-built MemoryCache was overridden; close defensively
		})

		assert.Same(t, custom, c.cache,
			"WithCacheBackend after WithCache must overwrite cfg.cache (D-80 last-wins)")
	})
}

// TestWithRequestHook covers D-87: non-nil fn causes buildTransport to
// install a hookTransport layer; nil fn is a no-op (no hookTransport
// layer is installed; nil-no-op convention per WithHTTPClient analog);
// the default Client (no WithRequestHook call) likewise has no
// hookTransport layer in its chain. The hookTransport struct is
// unexported but visible to same-package tests via type-assertion on
// c.http.Transport — that's the load-bearing observable for "the hook
// option had its documented effect". Hook firing behavior end-to-end is
// covered by TestHook_FiresOnRetryAttempts, TestHook_SeesCacheHits, and
// TestHook_DoesNotFireOnDecodeError. (WR-04 follow-up removed the
// previously-unread Client.requestHook field; this test now asserts on
// the chain rather than on a write-only struct field.)
func TestWithRequestHook(t *testing.T) {
	t.Parallel()

	t.Run("non-nil fn installs hookTransport at the outermost layer", func(t *testing.T) {
		t.Parallel()
		fn := func(_ *http.Request, _ *http.Response, _ error) {}
		c := NewClient(WithRequestHook(fn))
		require.NotNil(t, c)
		require.NotNil(t, c.http, "client.http must be non-nil after NewClient")
		_, isHookLayer := c.http.Transport.(*hookTransport)
		assert.True(t, isHookLayer,
			"WithRequestHook(non-nil) must install hookTransport as the outermost RoundTripper (D-87 / D-89)")
	})

	t.Run("nil fn is no-op (no hookTransport layer installed)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithRequestHook(nil))
		require.NotNil(t, c)
		require.NotNil(t, c.http, "client.http must be non-nil after NewClient")
		_, isHookLayer := c.http.Transport.(*hookTransport)
		assert.False(t, isHookLayer,
			"WithRequestHook(nil) must NOT install hookTransport (nil-no-op convention per WithHTTPClient analog)")
	})

	t.Run("default Client has no hookTransport layer", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		require.NotNil(t, c.http, "client.http must be non-nil after NewClient")
		_, isHookLayer := c.http.Transport.(*hookTransport)
		assert.False(t, isHookLayer,
			"default Client must have no hookTransport layer — hook is opt-in per D-87")
	})
}
