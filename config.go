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
}

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

// buildTransport composes the RoundTripper chain for Phase 2:
//
//	req → headerTransport → loggingTransport → underlying
//
// Where underlying is cfg.httpClient.Transport if the caller supplied a
// custom Transport on their *http.Client, else the stdlib default.
//
// Phase 3 will add retryTransport outermost; Phase 4 will add cacheTransport
// and hookTransport. Pre-1.0, this constructor is edited in place rather
// than abstracted into a generic middleware list (D-29 explicit) — the
// chain is small enough that one constructor edit is cheaper than a
// framework, and the chain order is load-bearing semantics (outermost wraps
// inner; the request flows left-to-right, the response right-to-left).
func buildTransport(cfg *clientConfig) http.RoundTripper {
	underlying := cfg.httpClient.Transport
	if underlying == nil {
		underlying = http.DefaultTransport
	}
	var rt http.RoundTripper = underlying
	rt = &loggingTransport{logger: cfg.logger, next: rt}
	rt = &headerTransport{userAgent: cfg.userAgent, next: rt}
	return rt
}
