// Package openholidays — hookTransport: the observability-hook
// http.RoundTripper that fires the user-supplied RequestHookFunc after every
// HTTP round trip.
//
// Chain placement (D-89): hookTransport is the OUTERMOST RoundTripper in the
// Phase 4 chain. The revised chain order is:
//
//	req → hookTransport → cacheTransport → loggingTransport →
//	      headerTransport → underlying
//
// Because it sits above cacheTransport, the hook observes EVERY round trip
// the chain performs — including the synthetic *http.Response returned by
// cacheTransport on a cache hit (D-88). Consumers detect cache-hit vs
// fresh-fetch by reading CacheHitContextKey from req.Context() — see the
// CacheHitContextKey godoc in transport_cache.go and the WithRequestHook
// godoc in options.go.
//
// Hook contract (D-87..D-90; full text on RequestHookFunc in config.go):
//
//   - Synchronous: invoked on the calling goroutine's stack. Consumers wanting
//     async behavior own the goroutine and the leak (Pitfall CONC-2).
//   - Per-attempt: each c.http.Do invocation re-enters the chain, so a retry
//     loop dispatching three attempts produces three hook invocations
//     (TRANS-05).
//   - Cache-hit safe: the synthetic cache-hit response flows through
//     hookTransport because hookTransport is OUTERMOST. The hook receives the
//     synthetic (resp, nil) triple.
//   - Panic propagation: a panicking hook propagates the panic to the caller.
//     The library does NOT use defer/recover (D-90; mirrors stdlib
//     http.Handler convention — silent recovery would hide bugs).
//   - Nil-safe: on transport error, the hook receives (req, nil, err).
//     Implementations MUST nil-check resp before any field access.
//   - Body invariant: the hook MUST NOT read resp.Body — doing so would
//     deplete bytes before the downstream decoder runs in doJSONGet.
//
// RequestHookFunc was declared in config.go by Phase 4 Plan 02 (D-87). This
// file consumes that type — DO NOT redeclare here.
//
// No init() and no package-level vars — keeps the CLIENT-10 AST audit in
// internal_test.go green without modification to its allowlist.

package openholidays

import "net/http"

// hookTransport is the observability-hook RoundTripper. It calls
// t.hook(req, resp, err) synchronously after t.next.RoundTrip returns, then
// returns (resp, err) verbatim to the caller. The hook field is the only
// non-pointer mutable surface; once constructed by buildTransport, the
// struct is read-only and safe for concurrent use across goroutines.
//
// When t.hook is nil, RoundTrip is a transparent pass-through (delegates to
// next; no overhead beyond the nil check). buildTransport in config.go
// elides hookTransport from the chain when cfg.hook is nil (D-89), so this
// internal nil-check is defense-in-depth.
//
// See the package-level doc above for the full D-87..D-90 contract.
type hookTransport struct {
	hook RequestHookFunc
	next http.RoundTripper
}

// RoundTrip delegates to t.next, then invokes t.hook(req, resp, err)
// synchronously on the calling goroutine, then returns the next
// RoundTripper's (resp, err) verbatim.
//
// Invariants (D-87..D-90):
//
//   - The hook is invoked exactly once per RoundTrip call, even when err is
//     non-nil or status >= 400. The hook is HTTP-layer observability, not
//     error filtering.
//   - The hook receives resp == nil on transport-level failure. Hooks MUST
//     nil-check resp before any field access (documented on RequestHookFunc
//     in config.go).
//   - A panic in the hook propagates to the caller — no defer/recover.
//   - RoundTrip does NOT read resp.Body. Hooks MUST NOT read it either; the
//     downstream decoder in doJSONGet expects a fresh stream.
func (t *hookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if t.hook != nil {
		t.hook(req, resp, err) // D-90: synchronous; panics propagate; consumer owns goroutines.
	}
	return resp, err
}
