// Package openholidays — internal client configuration and HTTP transport composition.
//
// This file declares the unexported clientConfig builder that holds every
// option-supplied value before NewClient finalizes the *Client; defaultConfig,
// which materializes the documented Phase 2 defaults (D-28 / D-36 / D-39 /
// PROJECT.md); composeHTTPClient, the shallow-copy gate that neutralizes
// Pitfall HTTP-1 (caller post-construction mutation of *http.Client per D-37);
// and buildTransport, the RoundTripper chain composer that wires Plan 01's
// headerTransport and loggingTransport into the documented Phase 2 chain
// shape (D-29: req → headerTransport → loggingTransport → underlying).
//
// Phase 4 additions in this file: the Cache interface (D-79), the
// RequestHookFunc type (D-87), and four additive clientConfig fields
// (retry / cache / hook / strictDecoding). The retryConfig struct itself
// lives in retry.go (Plan 03) so all retry types are colocated in a
// single file. Phase 4 fills in retry/cache/hook/strict config fields —
// buildTransport is edited in place by Plans 04 (cache) and 05 (hook);
// this plan only declares the types and config fields, leaving
// buildTransport untouched so the Phase 2 chain order remains in effect
// until cache/hook actually exist. The cacheTTL field that originally
// shipped with this plan was removed in the WR-04 follow-up — it was
// never read by composeHTTPClient or any other production code, and
// MemoryCache.ttl is the real source of truth.
//
// No init() and no package-level vars — keeps the CLIENT-10 AST audit in
// internal_test.go green without modification to its allowlist.

package openholidays

import (
	"log/slog"
	"net/http"
	"time"
)

// clientConfig is the internal builder state filled by Options between
// NewClient's start and Client construction. Unexported — never escapes the
// package. Field-by-field semantics mirror the public WithX godoc.
type clientConfig struct {
	httpClient *http.Client  // shallow-copied in composeHTTPClient (Pitfall HTTP-1 / D-37)
	baseURL    string        // trailing-slash-trimmed by WithBaseURL; concatenated with "/EndpointPath"
	userAgent  string        // non-empty; injected by headerTransport when caller did not set one
	logger     *slog.Logger  // non-nil; falls back to slog.Default() when caller passes nil
	timeout    time.Duration // 0 disables the SDK-imposed timeout
	// Phase 4 (D-77, D-79, D-87, D-91):
	retry          retryConfig     // D-77; zero-value = disabled
	cache          Cache           // D-79; nil = disabled
	hook           RequestHookFunc // D-87; nil = no hook
	strictDecoding bool            // D-91
}

// Cache is the contract for any cache backend wired via WithCache or
// WithCacheBackend (Plan 04). Implementations must be safe for concurrent
// use from multiple goroutines (CLIENT-07).
//
// Get returns the cached value bytes and true on a hit, or nil and false
// on a miss or on entries that have expired according to the cache's
// internal clock. Put stores value under key; replacing an existing entry
// at key is the cache's prerogative. Close is the best-effort shutdown
// hook called from Client.Close — implementations should stop any
// sweeper goroutine and return nil on the typical path.
//
// The interface is declared in Plan 02 (D-79) so Client.Close can call
// c.cache.Close() without a build error; the MemoryCache implementation
// lands in Plan 04.
type Cache interface {
	Get(key string) (value []byte, ok bool)
	Put(key string, value []byte)
	Close() error
}

// RequestHookFunc is the function shape accepted by WithRequestHook
// (Plan 05). It is invoked synchronously on the calling goroutine's stack
// after every HTTP round trip including cache-hit synthetic responses
// (D-88 / D-89). Panics propagate; consumers wrap with defer/recover if
// needed (mirrors stdlib http.Handler convention).
//
// On a transport error resp is nil — implementations MUST nil-check.
// Hooks MUST NOT log resp.Body content above slog.LevelDebug (Pitfall
// LOG-1).
type RequestHookFunc func(*http.Request, *http.Response, error)

// defaultConfig returns a fresh *clientConfig populated with every Phase 2
// default:
//
//   - httpClient: a zero-valued *http.Client (no caller-supplied Timeout;
//     PROJECT.md leaves the Go-level timer disabled — per-request timeouts
//     arrive via context.WithTimeout in the endpoint methods, D-26 / D-27).
//   - baseURL:    the upstream production host per D-36 / PROJECT.md.
//   - userAgent:  the go-openholidays brand string suffixed with the Phase 1
//     Version const (PROJECT.md / version.go).
//   - logger:     slog.Default() (D-39; library never mutates the process default).
//   - timeout:    fifteen seconds (CLIENT-06 / D-28 / PROJECT.md).
//
// Every default literal appears in the struct literal below and nowhere else
// in this file: RESEARCH OQ-4 + D-36 lock the upstream URL, so an extracted
// const buys nothing but the indirection cost.
func defaultConfig() *clientConfig {
	return &clientConfig{
		httpClient: &http.Client{},
		baseURL:    "https://openholidaysapi.org",
		userAgent:  "go-openholidays/" + Version,
		logger:     slog.Default(),
		timeout:    15 * time.Second,
	}
}

// composeHTTPClient shallow-copies cfg.httpClient so that caller mutations of
// the original *http.Client after NewClient returns do not affect the SDK
// (Pitfall HTTP-1 / D-37). The Transport on the copy is replaced with the
// chain returned by buildTransport (D-29).
//
// The shallow copy preserves every non-Transport field on the caller's
// *http.Client (CheckRedirect, Jar, Timeout) so callers who supplied a
// pre-configured client keep those settings — only Transport is overwritten
// so the SDK's middleware chain is invoked on every request.
func composeHTTPClient(cfg *clientConfig) *http.Client {
	cp := *cfg.httpClient
	cp.Transport = buildTransport(cfg)
	return &cp
}

// buildTransport composes the RoundTripper chain. The full Phase 4 chain
// per D-89 (revised — retry moved to the endpoint layer per RESIL-05) is:
//
//	req → [hookTransport] → [cacheTransport] → loggingTransport →
//	      headerTransport → underlying
//
// Where underlying is cfg.httpClient.Transport if the caller supplied a
// custom Transport on their *http.Client, else the stdlib default. Layers
// in square brackets are conditional on the corresponding option being
// passed (WithCache/WithCacheBackend for cacheTransport; WithRequestHook
// for hookTransport).
//
// hookTransport is OUTERMOST (D-89) so it observes every round trip —
// including the synthetic *http.Response returned by cacheTransport on
// cache hits. Consumers detect cache hits inside their hook via
// req.Context().Value(openholidays.CacheHitContextKey).
//
// Composition order in code is INVERSE the documented chain order
// because Go's RoundTripper decorator pattern wraps inside-out — the
// outermost layer (which sees the request FIRST) is constructed LAST.
// The "innermost first" comments below mark the build order so a future
// reader can verify chain semantics against D-29 / D-89 without
// re-deriving them.
//
// Pre-1.0, this constructor is edited in place rather than abstracted
// into a generic middleware list (D-29 explicit) — the chain is small
// enough that one constructor edit is cheaper than a framework, and the
// chain order is load-bearing semantics (outermost wraps inner; the
// request flows outermost-to-innermost, the response innermost-to-
// outermost).
func buildTransport(cfg *clientConfig) http.RoundTripper {
	underlying := cfg.httpClient.Transport
	if underlying == nil {
		underlying = http.DefaultTransport
	}
	var rt http.RoundTripper = underlying
	// Innermost first (request flows outermost → innermost):
	rt = &headerTransport{userAgent: cfg.userAgent, next: rt}
	rt = &loggingTransport{logger: cfg.logger, next: rt}
	// Phase 4 cache layer (D-89: above logging so cache-hit logs still
	// emit — the loggingTransport above does NOT see a cache hit because
	// cacheTransport short-circuits; the hookTransport above DOES see it).
	if cfg.cache != nil {
		rt = &cacheTransport{
			cache:         cfg.cache,
			cacheablePath: isCacheablePath,
			next:          rt,
		}
	}
	// Phase 4 hook layer — OUTERMOST per D-89 so it observes cache-hit
	// synthetic responses returned by cacheTransport above. Elided
	// entirely when cfg.hook is nil so non-observability callers pay zero
	// overhead (no extra RoundTrip frame, no nil-check per request).
	if cfg.hook != nil {
		rt = &hookTransport{hook: cfg.hook, next: rt}
	}
	return rt
}
