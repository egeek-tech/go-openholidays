// Package openholidays — functional Option constructors and the Option type.
//
// This file declares Option (the functional-option signature) and the five
// public WithX constructors that callers compose at NewClient time. Options
// mutate only the internal *clientConfig (declared in config.go); they never
// touch the Client after construction (D-35).
//
// No init() and no package-level vars — keeps the CLIENT-10 AST audit in
// internal_test.go green without modification to its allowlist.

package openholidays

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Option configures a Client at construction time. Options compose via
// NewClient: each Option mutates a private *clientConfig builder, and the
// final *Client is constructed from that builder. After NewClient returns,
// the Client is immutable; further Option calls on a constructed Client
// have no effect by design (no setter exists).
type Option func(*clientConfig)

// WithHTTPClient supplies a pre-configured *http.Client. The SDK
// shallow-copies the supplied client inside composeHTTPClient and replaces
// the copy's Transport with the SDK's RoundTripper chain (D-37 / Pitfall
// HTTP-1); caller mutations of the supplied *http.Client after NewClient
// returns therefore do not affect the SDK.
//
// A nil argument is a no-op — the SDK retains its zero-valued default
// *http.Client. To suppress all SDK middleware, supply an *http.Client
// whose Transport is set to a caller-owned http.RoundTripper and accept
// that buildTransport will wrap it with the documented chain.
//
// NOTE: setting Timeout on the supplied *http.Client may cause spurious
// "context canceled" errors on body close (see golang/go#49521); prefer
// WithTimeout(d) to bound per-request duration via context (D-26).
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *clientConfig) {
		if c != nil {
			cfg.httpClient = c
		}
	}
}

// WithBaseURL overrides the default base URL. A trailing slash, if present,
// is trimmed so endpoint paths (always beginning with "/") concatenate
// cleanly. Multiple trailing slashes are trimmed too.
//
// WithBaseURL("") is treated as "use the default" — the default base URL
// applied by defaultConfig is left in place. Inputs that collapse to an
// empty string after trailing-slash trimming (e.g. "/", "//", "///") are
// also treated as "use the default" so callers reading base URLs from
// environment variables that default to "/" do not silently land in an
// unusable state where downstream HTTP calls fail with opaque
// "unsupported protocol scheme" errors far from the misconfiguration
// (WR-01 follow-up).
//
// Callers wanting to point the SDK at a mirror should pass the mirror's
// URL here (D-36 explicitly rejects environment-variable overrides;
// WithBaseURL is the supported extension point).
func WithBaseURL(u string) Option {
	return func(cfg *clientConfig) {
		if u == "" {
			return
		}
		trimmed := strings.TrimRight(u, "/")
		if trimmed == "" {
			// All-slash input collapses to empty; keep the default
			// rather than silently assigning "" to cfg.baseURL.
			return
		}
		cfg.baseURL = trimmed
	}
}

// WithUserAgent overrides the default User-Agent header
// ("go-openholidays/<Version>") sent on every HTTP request.
//
// An empty string is treated as "use the default" — the library never
// sends an empty User-Agent (D-38) because some CDNs reject empty-UA
// requests as bot traffic (Pitfall HTTP-5). To suppress the User-Agent
// entirely, the caller must supply a custom http.RoundTripper via
// WithHTTPClient.
func WithUserAgent(s string) Option {
	return func(cfg *clientConfig) {
		if s != "" {
			cfg.userAgent = s
		}
	}
}

// WithLogger injects a structured logger. The SDK emits one slog.LevelDebug
// record per HTTP round trip via loggingTransport (transport.go) with the
// six OBS-02 fields (method, path, status, duration_ms, attempt, bytes_in).
//
// A nil argument falls back to slog.Default() (D-39). The library NEVER
// mutates the process-wide default logger — this preserves the consuming
// application's global logger configuration.
func WithLogger(l *slog.Logger) Option {
	return func(cfg *clientConfig) {
		if l == nil {
			cfg.logger = slog.Default()
			return
		}
		cfg.logger = l
	}
}

// WithTimeout sets the per-request timeout applied via context.WithTimeout
// inside every endpoint method (D-26 / D-27). The default is fifteen
// seconds (CLIENT-06 / D-28).
//
// A zero duration disables the SDK-imposed timeout; the caller's ctx
// becomes the only deadline. The value is stored verbatim (negative
// durations are accepted as-is per D-28 "verbatim" — the endpoint
// methods interpret a non-positive value as "no SDK timeout").
//
// WithTimeout does NOT mutate cfg.httpClient.Timeout (D-26): setting the
// stdlib Client.Timeout is known to cause spurious "context canceled"
// errors on response-body close (golang/go#49521), so the SDK uses ctx
// timeouts exclusively.
func WithTimeout(d time.Duration) Option {
	return func(cfg *clientConfig) {
		cfg.timeout = d
	}
}

// WithStrictDecoding enables strict JSON decoding via
// json.Decoder.DisallowUnknownFields (D-91 / D-92 / CL-15). When strict is
// true, every JSON response decoded by the SDK rejects payloads
// containing fields absent from the destination Go struct — useful for
// surfacing upstream schema drift loudly during integration tests or in
// canary deployments.
//
// Strict-decoding is OFF by default (Pitfall JSON-1): the upstream
// OpenHolidays API adds fields routinely, and silent rejection would
// break consumers on every benign schema bump. Opt in only when the
// consumer wants the loud-fail behavior.
//
// The flag is immutable after NewClient. No per-call override and no
// runtime toggle exist by design — toggling at runtime would let cached
// bytes decoded under one mode surface as a strict-failure after the
// toggle (D-93). Consumers wanting "cache lenient + fresh strict" must
// instantiate two Clients.
//
// false is stored verbatim (no defensive special-case) — matches the
// WithTimeout verbatim convention.
func WithStrictDecoding(strict bool) Option {
	return func(cfg *clientConfig) {
		cfg.strictDecoding = strict
	}
}

// WithRetry enables retry with exponential backoff + full jitter for
// every endpoint method on the constructed Client (D-73 / D-74 / D-75 /
// D-76 / D-77 / RESIL-01..05). Retry is OFF by default — calling
// WithRetry is the only way to enable it.
//
// Arguments:
//
//   - maxAttempts: maximum number of c.http.Do invocations inside the
//     retry loop. <=0 is interpreted as DISABLED (the loop runs exactly
//     once and surfaces the first response/error verbatim — defensive
//     symmetry with WithTimeout(0) per D-74).
//   - baseDelay: base unit for exponential backoff. <=0 falls back to
//     defaultBaseDelay (250ms) per D-74.
//
// When WithMaxRetryWait is NOT also called, WithRetry sets the per-
// attempt sleep ceiling to defaultMaxRetryWait (60s) per D-74 so a
// caller that opts in to retry with a single line never accidentally
// disables the cap. If both WithRetry and WithMaxRetryWait are called,
// last-wins applies per the functional-options convention.
//
// Retryable conditions (D-75):
//
//   - HTTP statuses 408, 429, 500, 502, 503, 504 (Pitfall RETRY-1).
//   - net.Error with Timeout() == true (transport timeout).
//   - errors wrapping syscall.ECONNRESET (connection reset).
//   - context.Canceled and context.DeadlineExceeded are NEVER retried
//     — they propagate as ctx.Err() immediately.
//
// Retry-After handling (D-76): when an upstream response carries a
// Retry-After header (integer seconds or RFC 7231 HTTP-date), the
// per-attempt sleep is max(retryAfter, jitterDelay) capped at the
// per-attempt ceiling. Past-dated HTTP-dates are rejected (Pitfall 9 /
// threat T-04-06) so backoff never collapses to zero. When the header
// is absent or unparseable, the sleep is full-jitter exponential:
// uniform random in [0, baseDelay << attempt) capped at the per-
// attempt ceiling.
//
// Placement (D-77 + RESIL-05): the retry loop lives inside
// doJSONGet (the endpoint layer), NOT a RoundTripper. Consumers who
// supply their own retrying *http.Client via WithHTTPClient therefore
// do NOT see double-firing of attempts. Retry is an opt-in SDK feature;
// callers wanting it disable in their custom transport (or just don't
// call WithRetry) will see exactly one round trip per endpoint method.
//
// The per-attempt ceiling bounds each individual sleep (default 60s),
// NOT the cumulative retry budget — five attempts with 60s cap can
// still take ~5 min total. Consumers wanting a cumulative cap supply
// ctx.WithTimeout(ctx, totalBudget); the SDK's retry loop is ctx-aware
// (Pitfall RETRY-3) and returns ctx.Err() within ≤ 100 ms of caller
// cancellation (CLIENT-09 / RESIL-04).
func WithRetry(maxAttempts int, baseDelay time.Duration) Option {
	return func(cfg *clientConfig) {
		cfg.retry.maxAttempts = maxAttempts
		if baseDelay > 0 {
			cfg.retry.baseDelay = baseDelay
		} else {
			cfg.retry.baseDelay = defaultBaseDelay
		}
		// Ensure maxWait has a sane default if the caller did not also
		// call WithMaxRetryWait (D-74). Without this, a single-line
		// WithRetry(3, 100*time.Millisecond) leaves cfg.retry.maxWait
		// at its zero value, which would make computeBackoff produce a
		// 1ms-bounded jitter regardless of attempt (capped path takes
		// over). 60s cap is the documented default; last-wins is
		// preserved because WithMaxRetryWait can still overwrite it if
		// the caller composes Options in either order.
		if cfg.retry.maxWait <= 0 {
			cfg.retry.maxWait = defaultMaxRetryWait
		}
	}
}

// WithMaxRetryWait sets the per-attempt sleep ceiling applied by the
// retry loop (D-74 default 60s). A non-positive duration falls back to
// defaultMaxRetryWait (60s) per D-74 — calling
// WithMaxRetryWait(0) does NOT disable the cap.
//
// The ceiling applies to each individual sleep, NOT the cumulative
// retry budget (CONTEXT.md `<specifics>` 5). Five attempts with a 60s
// cap can still take ~5 minutes total. Consumers wanting a cumulative
// cap supply ctx.WithTimeout(ctx, totalBudget) themselves — the SDK
// does not enforce a cumulative budget because it would conflict with
// the per-attempt semantics (deferred per CONTEXT.md `<deferred>`).
//
// The cap also bounds Retry-After promotion: a hostile upstream
// returning Retry-After: 999999999 cannot hold the request for the
// lifetime of the process (threat T-04-05) because computeBackoff
// applies min(retryAfter, maxWait) per D-76.
//
// Note that calling WithMaxRetryWait alone (without WithRetry) has no
// observable effect — retry is opt-in, and the cap is only consulted
// when the retry loop runs (maxAttempts > 0). The intended idiom is to
// pass both options together when finer control over the cap is
// needed:
//
//	c := NewClient(
//	    WithRetry(5, 100*time.Millisecond),
//	    WithMaxRetryWait(10*time.Second),
//	)
func WithMaxRetryWait(d time.Duration) Option {
	return func(cfg *clientConfig) {
		if d > 0 {
			cfg.retry.maxWait = d
		} else {
			cfg.retry.maxWait = defaultMaxRetryWait
		}
	}
}

// WithCache enables the default in-memory TTL cache with the supplied TTL
// (D-79 / D-80 / D-83 / RESIL-06..09). When ttl > 0, the option constructs
// a *MemoryCache via newMemoryCacheWithClock(ttl, time.Now) and stores it
// on the internal Cache field; the Client's RoundTripper chain inserts a
// cacheTransport layer that consults this cache for the three reference
// endpoints (/Countries, /Languages, /Subdivisions). Holiday endpoints
// (/PublicHolidays, /SchoolHolidays) bypass the cache by default —
// RESIL-07 / temporal-data trap.
//
// Cache-hit semantics:
//
//   - Hit: cacheTransport returns a synthetic 200 OK response with the
//     cached bytes; the downstream decoder runs against those bytes,
//     including strict-mode (D-93). The req.Context() of the synthetic
//     response carries CacheHitContextKey == true so consumers can
//     detect cache hits in WithRequestHook.
//   - Miss: cacheTransport forwards to the next RoundTripper; on
//     success (err == nil && status == 200) the response bytes are
//     cached for the configured TTL (Pitfall CACHE-1).
//
// ttl <= 0 is treated as DISABLED (defensive symmetry with WithTimeout(0)
// per D-80). The default Client has NO cache — opt in via WithCache or
// WithCacheBackend.
//
// Clock seam (D-86): WithCache uses time.Now literally because options
// run BEFORE Client construction; the Client's internal nowFunc cannot
// be picked up retroactively by the cache. Tests that need a fake clock
// route through WithCacheBackend(newMemoryCacheWithClock(ttl, fc.Now))
// instead.
//
// Lifecycle: Client.Close calls cache.Close (D-85), which stops the
// sweeper goroutine. Consumers MUST defer client.Close() to avoid
// leaking the sweeper (Pitfall CONC-2). For a constructed-but-never-used
// cache, Close is still safe and returns nil (no sweeper was ever
// started — D-84 lazy).
func WithCache(ttl time.Duration) Option {
	return func(cfg *clientConfig) {
		if ttl <= 0 {
			return // D-80: ttl <= 0 disables; leave cfg.cache nil.
		}
		cfg.cache = newMemoryCacheWithClock(ttl, time.Now)
		cfg.cacheTTL = ttl
	}
}

// WithCacheBackend supplies a custom Cache implementation; supersedes any
// prior WithCache(ttl) per the D-80 last-wins functional-options
// convention. A nil argument is a no-op (mirrors WithHTTPClient(nil)).
//
// When the caller supplies a backend, the Client does NOT own the
// backend's goroutines or resources — Client.Close still calls
// c.Close() per the interface contract, but the backend is responsible
// for its own lifecycle. Pitfall CACHE-4 (read-during-evict race) only
// applies to the default MemoryCache; custom backends MUST implement
// their own thread-safety because cacheTransport does NOT serialize
// access to Get/Put/Close (the RoundTripper is a pure pass-through).
//
// Use case: integration tests can swap in a deterministic clock-driven
// MemoryCache via newMemoryCacheWithClock(ttl, fc.Now) (TEST-06); future
// consumers can implement Redis-backed or LRU-backed caches without a
// library change.
func WithCacheBackend(c Cache) Option {
	return func(cfg *clientConfig) {
		if c != nil {
			cfg.cache = c // D-80: last-wins; overrides prior WithCache(ttl).
		}
	}
}

// WithRequestHook supplies an observability hook function invoked after
// every HTTP round trip the Client performs (D-87 / D-88 / D-89 / D-90 /
// TRANS-05). The hook receives the (*http.Request, *http.Response, error)
// triple produced by the RoundTripper chain. Use it to wire metrics
// counters, distributed-tracing spans, or per-request audit logs into the
// SDK without modifying it — the hook is the single observability seam
// the library exposes.
//
// Hook contract (D-87..D-90):
//
//   - Fires AFTER every real HTTP round trip — including each retry attempt
//     (retry lives in doJSONGet per RESIL-05, so each c.http.Do dispatch
//     re-enters the chain; a 429→500→200 sequence triggers three hook
//     invocations).
//   - Fires on cache-hit synthetic responses too (D-88). cacheTransport
//     stores cached bytes for /Countries, /Languages, /Subdivisions; on a
//     hit it builds a synthetic *http.Response and the hook above sees it.
//     Distinguish hits from real round trips via
//     req.Context().Value(openholidays.CacheHitContextKey) — the value is
//     the untyped boolean true on a hit, and absent (nil) on a miss.
//   - Does NOT fire on decode errors. JSON decoding runs in doJSONGet
//     AFTER the RoundTripper chain returns; a successful HTTP round trip
//     whose body fails decode produces exactly one hook invocation (for
//     the HTTP layer success), not two.
//   - Does NOT fire on pre-HTTP failures (validateCountry rejecting an
//     empty CountryIsoCode, validateDateRange rejecting a backwards window,
//     etc.). No HTTP attempt → no hook.
//
// Synchronous-only contract (D-90 / Pitfall CONC-2): the hook runs on the
// calling goroutine's stack. If you need asynchronous behavior, spawn a
// goroutine inside your hook — but YOU own the goroutine and any leak.
// Async-hook support (background queue with bounded buffer) is explicitly
// deferred (CONTEXT.md `<deferred>`).
//
// Panic propagation (D-90 / mirrors stdlib http.Handler): a panicking hook
// propagates the panic to the caller. The library does NOT use
// defer/recover — silent recovery would hide bugs in consumer hooks.
// Consumers wanting recovery wrap their hook body with their own
// defer/recover; the library will never do it for them.
//
// Body invariant (Pitfall LOG-1 / OBS-01): the hook MUST NOT read resp.Body
// — doing so depletes bytes before the downstream decoder runs in
// doJSONGet. The hook also MUST NOT log resp.Body content above
// slog.LevelDebug because that would leak payload data into operator logs
// (PROJECT.md). Log method, URL, status, duration, attempt counters,
// trace-context — never the body content.
//
// Nil-safe contract (D-88): on a transport-level failure (DNS error, TCP
// reset, ctx cancel during request body write) the hook receives
// (req, nil, err). Implementations MUST nil-check resp before accessing
// any field on it.
//
// Chain placement (D-89): hookTransport is the OUTERMOST RoundTripper in
// the chain, so it observes EVERY round trip the chain performs:
//
//	req → hookTransport → cacheTransport → loggingTransport →
//	      headerTransport → underlying
//
// A nil fn is a no-op (mirrors WithHTTPClient(nil)) — the default Client
// has no hook, and buildTransport elides the hookTransport layer entirely
// when cfg.hook is nil so there is zero overhead for callers not using
// observability.
func WithRequestHook(fn RequestHookFunc) Option {
	return func(cfg *clientConfig) {
		if fn != nil {
			cfg.hook = fn
		}
	}
}
