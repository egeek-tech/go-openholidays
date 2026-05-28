// Package openholidays — client construction and lifecycle.
//
// This file declares the Client struct (immutable after NewClient returns),
// the NewClient constructor that applies functional Options to a fresh
// clientConfig and materializes a usable *http.Client via composeHTTPClient,
// and the Close method that uses sync.Once to coordinate the cache-sweeper
// shutdown (D-85). Phase 4 also wires per-Client time/sleep seams
// (nowFunc=time.Now, sleepFunc=ctxSleep, D-94) and a ChaCha8-seeded
// *math/rand/v2.Rand for jitter (D-78).
//
// No init() and no package-level vars — keeps the CLIENT-10 AST audit in
// internal_test.go green without modification to its allowlist.

package openholidays

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"hash/fnv"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Client is the immutable HTTP client for the OpenHolidays API. Construct
// one via NewClient and reuse it across goroutines for the lifetime of the
// program; Client carries no per-call mutable state.
//
// The closed flag declared below is the only mutable state on the struct;
// it is flipped exactly once by Close (idempotent, guarded by closeOnce),
// and goroutines that call Client methods concurrently with Close observe
// consistent reads without locking (CLIENT-07).
//
// Phase 4 additions:
//
//   - retry, cache, requestHook: nil/zero-value defaults; their option
//     constructors land in Plans 03/04/05.
//   - strict: immutable after NewClient (D-91).
//   - nowFunc, sleepFunc: deterministic-test seam (D-94).
//   - rand: per-Client ChaCha8-seeded jitter source (D-78).
//   - closeOnce: guards cache.Close inside Close (D-85).
type Client struct {
	http        *http.Client                               // chain-wrapped client built by composeHTTPClient
	baseURL     string                                     // trailing-slash-trimmed; concatenated with "/EndpointPath"
	userAgent   string                                     // injected by headerTransport when caller request lacks UA
	logger      *slog.Logger                               // non-nil; passed to loggingTransport
	timeout     time.Duration                              // 0 disables the SDK-imposed timeout
	closed      atomic.Bool                                // flipped by Close; reads are race-safe
	retry       retryConfig                                // D-77; zero-value = disabled
	cache       Cache                                      // D-79; nil = disabled (wired in Plan 04)
	strict      bool                                       // D-91; immutable after NewClient
	requestHook RequestHookFunc                            // D-87; nil = no hook (wired in Plan 05)
	nowFunc     func() time.Time                           // D-94; defaults to time.Now
	sleepFunc   func(context.Context, time.Duration) error // D-94; defaults to ctxSleep
	rand        *rand.Rand                                 // D-78; per-Client ChaCha8-seeded
	closeOnce   sync.Once                                  // D-85; guards cache.Close inside Close()
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
//   - nowFunc:     time.Now (D-94)
//   - sleepFunc:   ctxSleep — a ctx-aware timer-based helper (D-94)
//   - rand:        per-Client *math/rand/v2.Rand seeded by crypto/rand (D-78)
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
		http:        composeHTTPClient(cfg),
		baseURL:     cfg.baseURL,
		userAgent:   cfg.userAgent,
		logger:      cfg.logger,
		timeout:     cfg.timeout,
		retry:       cfg.retry,
		cache:       cfg.cache,
		strict:      cfg.strictDecoding,
		requestHook: cfg.hook,
		nowFunc:     time.Now,
		sleepFunc:   ctxSleep,
		rand:        newClientRand(),
	}
}

// Close is the idempotent shutdown hook. It flips the closed atomic flag
// and best-effort calls cache.Close when a cache backend was wired via
// WithCache or WithCacheBackend (D-85). Safe to call from any goroutine;
// subsequent calls return nil unchanged. Cache.Close errors are
// intentionally swallowed — the cache's contract is best-effort cleanup
// and a Close failure on the cache should not surface to a caller draining
// their Client in defer client.Close() (CLIENT-08).
//
// Mechanical guarantee (D-40 / D-85 / CLIENT-08): the underlying closed
// field is an atomic.Bool and the cache.Close call is guarded by
// sync.Once, so concurrent calls from multiple goroutines under the race
// detector neither race nor produce a non-nil error.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		if c.cache != nil {
			_ = c.cache.Close()
		}
	})
	return nil
}

// ctxSleep is the default Client.sleepFunc — a ctx-aware sleep helper that
// returns immediately on context cancellation. D-94 / Pitfall RETRY-3:
// bare time.Sleep is uninterruptible and would defeat the ≤ 100 ms ctx
// cancellation contract (CLIENT-09). The select on ctx.Done and the timer
// channel is the standard Go pattern for an interruptible sleep.
//
// A non-positive d returns nil immediately without arming the timer.
func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// newClientRand seeds a per-Client *math/rand/v2.Rand via crypto/rand
// (D-78) so two Clients in the same process — and across a fleet — do not
// emit identical jitter sequences (Pitfall RETRY-4: fleet-wide thundering
// herd). NewChaCha8 wants a 32-byte seed; crypto/rand.Read provides it.
//
// crypto/rand.Read is documented to never fail on a healthy system; on the
// rare error path the helper falls back to a multi-source mixed seed
// because NewClient must not return an error (CLIENT-01 contract). The
// fallback fills all 32 bytes by FNV-hashing nanosecond timestamp + pid
// (WR-05); strictly weaker than crypto/rand but covers all 32 bytes of
// ChaCha8 state so two Clients constructed within the same nanosecond on
// the same machine still differ via pid and across hash rounds.
func newClientRand() *rand.Rand {
	var seed [32]byte
	if _, err := crand.Read(seed[:]); err != nil {
		// Defensive fallback per CLIENT-01: NewClient must not error.
		// WR-05: fill all 32 bytes of seed so ChaCha8 has full state
		// diversity. Combine nanosecond timestamp + pid through FNV-128a
		// and rotate the input across two rounds to populate seed[0:16]
		// and seed[16:32] from independent hash states.
		var tb [8]byte
		binary.LittleEndian.PutUint64(tb[:], uint64(time.Now().UnixNano()))
		var pb [8]byte
		binary.LittleEndian.PutUint64(pb[:], uint64(os.Getpid()))
		h1 := fnv.New128a()
		h1.Write(tb[:])
		h1.Write(pb[:])
		copy(seed[:16], h1.Sum(nil))
		h2 := fnv.New128a()
		h2.Write(pb[:])
		h2.Write(tb[:])
		copy(seed[16:], h2.Sum(nil))
	}
	return rand.New(rand.NewChaCha8(seed))
}
