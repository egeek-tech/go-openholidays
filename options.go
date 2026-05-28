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
// applied by defaultConfig is left in place. Callers wanting to point the
// SDK at a mirror should pass the mirror's URL here (D-36 explicitly
// rejects environment-variable overrides; WithBaseURL is the supported
// extension point).
func WithBaseURL(u string) Option {
	return func(cfg *clientConfig) {
		if u == "" {
			return
		}
		cfg.baseURL = strings.TrimRight(u, "/")
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
