// Package openholidays — client construction and lifecycle.
//
// This file declares the Client struct (immutable after NewClient returns),
// the NewClient constructor that applies functional Options to a fresh
// clientConfig and materializes a usable *http.Client via composeHTTPClient,
// and the Close method that flips a single atomic flag (Phase 2: no-op
// stub; Phase 4 will hook the cache-sweeper cancel here via sync.Once).
//
// No init() and no package-level vars — keeps the CLIENT-10 AST audit in
// internal_test.go green without modification to its allowlist.

package openholidays

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// Client is the immutable HTTP client for the OpenHolidays API. Construct
// one via NewClient and reuse it across goroutines for the lifetime of the
// program; Client carries no per-call mutable state.
//
// The closed flag declared below is the only mutable state on the struct;
// it is flipped exactly once by Close (idempotent), and goroutines that
// call Client methods concurrently with Close observe consistent reads
// without locking (CLIENT-07).
//
// Phase 4 will add: closeOnce sync.Once; cacheSweeper context.CancelFunc.
type Client struct {
	http      *http.Client  // chain-wrapped client built by composeHTTPClient
	baseURL   string        // trailing-slash-trimmed; concatenated with "/EndpointPath"
	userAgent string        // injected by headerTransport when caller request lacks UA
	logger    *slog.Logger  // non-nil; passed to loggingTransport
	timeout   time.Duration // 0 disables the SDK-imposed timeout
	closed    atomic.Bool   // flipped by Close; reads are race-safe
}

// NewClient constructs an *openholidays.Client by applying the supplied
// Options to a fresh internal configuration and returning the resulting
// immutable client. NewClient never returns an error: all Options either
// silently accept any well-formed input (e.g. WithTimeout(0) means "no
// SDK-imposed timeout") or fall back to a documented default (e.g.
// WithLogger(nil) falls back to slog.Default()).
//
// Defaults applied when no Option supplies the field:
//
//   - HTTP client: a zero-valued *http.Client (no caller Timeout)
//   - Base URL:    the upstream production host (D-36 / PROJECT.md)
//   - User-Agent:  the go-openholidays brand string + Version
//   - Logger:      slog.Default()
//   - Timeout:     fifteen seconds (per-request, applied via context.WithTimeout)
//
// The returned Client is safe for concurrent use from any goroutine
// (verified by TestClient_ConcurrentAccess under the race detector in a
// later plan; this plan ships TestClient_Close which mechanically
// asserts the closed-flag invariant under 100 parallel goroutines).
func NewClient(opts ...Option) *Client {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return &Client{
		http:      composeHTTPClient(cfg),
		baseURL:   cfg.baseURL,
		userAgent: cfg.userAgent,
		logger:    cfg.logger,
		timeout:   cfg.timeout,
	}
}

// Close is the idempotent shutdown hook. In v0.1.0 it is a no-op that flips
// an internal closed flag; future versions will stop background goroutines
// (cache sweeper) here. Safe to call from any goroutine; subsequent calls
// also return nil.
//
// Mechanical guarantee (D-40 / CLIENT-08): the underlying closed field is
// an atomic.Bool, so concurrent calls from multiple goroutines under the
// race detector neither race nor produce a non-nil error.
func (c *Client) Close() error {
	c.closed.Store(true)
	return nil
}
