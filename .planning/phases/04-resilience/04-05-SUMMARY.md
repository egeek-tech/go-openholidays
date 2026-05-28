---
phase: 04-resilience
plan: 05
subsystem: observability-hook
tags:
  - trans-05
  - hook
  - roundtripper
  - observability
  - chain-order
dependency-graph:
  requires:
    - 04-01-SUMMARY.md  # fakeClock seam (newFakeClock, fc.Sleep) for deterministic retry timing
    - 04-02-SUMMARY.md  # RequestHookFunc declared in config.go; Client.requestHook field; clientConfig.hook field
    - 04-03-SUMMARY.md  # WithRetry/WithMaxRetryWait + retry loop in doJSONGet (composition substrate for TestHook_FiresOnRetryAttempts)
    - 04-04-SUMMARY.md  # cacheTransport + CacheHitContextKey (composition substrate for TestHook_SeesCacheHits)
  provides:
    - transport_hook.go::hookTransport + RoundTrip
    - options.go::WithRequestHook
    - config.go::buildTransport (hookTransport outermost insertion per D-89)
  affects:
    - All endpoint methods now route through hookTransport when cfg.hook != nil; cache-hit observation works via CacheHitContextKey lifted onto resp.Request by cacheTransport
    - Phase 5 docs append (CHANGELOG, README, PROJECT.md Key Decisions row CL-15/CL-16) — Plan 06 will record the hook surface as a public CL entry
tech-stack:
  added: []
  patterns:
    - "Outermost-decorator pattern (hookTransport above cacheTransport per D-89)"
    - "Synchronous-only callback contract (D-90; consumer owns goroutines; panics propagate)"
    - "Nil-no-op option convention (mirrors WithHTTPClient(nil); buildTransport elides the layer entirely)"
    - "resp.Request-as-canonical-request idiom (hookTransport prefers resp.Request when non-nil so cacheTransport's CacheHitContextKey stamp surfaces to consumers via req.Context() uniformly)"
key-files:
  created:
    - transport_hook.go
    - transport_hook_test.go
  modified:
    - options.go
    - options_test.go
    - config.go
    - client_test.go
decisions:
  - "D-87 wired (WithRequestHook accepts RequestHookFunc; no Hook interface, no per-event flag — one option, one type)."
  - "D-88 wired (hook fires per round trip including cache hits; does NOT fire on decode errors or pre-HTTP failures — three tests lock the contract end-to-end)."
  - "D-89 wired (hookTransport is OUTERMOST in buildTransport; chain order is req → hookTransport → cacheTransport → loggingTransport → headerTransport → underlying)."
  - "D-90 wired (synchronous-on-caller-goroutine; panics propagate; library does NOT defer/recover — TestHookTransport_PanicPropagates locks via testify assert.PanicsWithValue)."
  - "[Rule 1 auto-fix] hookTransport.RoundTrip passes resp.Request (when non-nil) to the hook instead of the inbound req. cacheTransport stamps CacheHitContextKey onto resp.Request only — for the hook to see the key via req.Context() uniformly, the hook callback must receive resp.Request. Stdlib convention also points here: resp.Request is the canonical 'request that produced this response' (preserved across redirects + chain rewrites). On transport error (resp == nil) the inbound req is passed through unchanged."
metrics:
  duration_sec: 317
  tasks_completed: 2
  files_changed: 6
  commits: 4
  completed: 2026-05-28
requirements_complete:
  - TRANS-05
---

# Phase 4 Plan 5: Observability hook layer Summary

**One-liner:** `WithRequestHook(fn)` lands as the OUTERMOST RoundTripper (D-89) so the user-supplied `func(*http.Request, *http.Response, error)` observes every round trip including retries AND cache-hit synthetic responses — closing TRANS-05.

## What shipped

### `transport_hook.go` (new, 84 lines)

- `hookTransport struct { hook RequestHookFunc; next http.RoundTripper }` — the unexported RoundTripper. File-level godoc cites D-87..D-90 + names the OUTERMOST chain placement + cross-references `CacheHitContextKey` for the hook+cache composition idiom.
- `(t *hookTransport) RoundTrip(req)` — synchronous-after-next semantics: delegates to `t.next.RoundTrip(req)`, then invokes `t.hook(hookReq, resp, err)` where `hookReq = resp.Request` when `resp != nil && resp.Request != nil`, else the inbound `req`. The `resp.Request` preference is the Rule 1 auto-fix that makes `req.Context().Value(CacheHitContextKey)` work uniformly inside the hook (see Deviations below).
- Nil hook is a transparent pass-through (defense-in-depth: `buildTransport` already elides the layer when `cfg.hook == nil`).
- Panics propagate (D-90) — no `defer recover()`.

### `transport_hook_test.go` (new, 287 lines)

Reuses `roundTripperFunc` from `transport_header_test.go:19` (D-50 same-package visibility — NOT redeclared).

| Test | Subtests | What it locks |
| --- | --- | --- |
| `TestHookTransport_RoundTrip` | 3 | (1) hook fires once with (req, resp, err); (2) nil-resp on transport error; (3) nil hook is transparent pass-through |
| `TestHookTransport_FiresPerAttempt` | 1 | TRANS-05 layer-level: three `RoundTrip` invocations → three hook calls (429→500→200 status mix) |
| `TestHookTransport_PanicPropagates` | 1 | D-90: `assert.PanicsWithValue("oops", ...)` proves the panic reaches the caller; no `defer recover()` in `RoundTrip` |
| `TestHookTransport_NilSafeOnTransportError` | 1 | D-88 contract: hook sees `resp == nil` and `err != nil` on transport failure |

### `options.go` — `WithRequestHook` added

```go
func WithRequestHook(fn RequestHookFunc) Option {
    return func(cfg *clientConfig) {
        if fn != nil {
            cfg.hook = fn
        }
    }
}
```

Godoc (~60 lines) cites D-87..D-90 + the four contract bullets (per-attempt, cache-hit, not-on-decode, not-on-pre-HTTP), the synchronous + panic-propagation + body invariants, the recommended `CacheHitContextKey` consumer pattern, and the D-89 chain-placement diagram. nil-no-op convention mirrors `WithHTTPClient(nil)`.

### `config.go::buildTransport` — D-89 outermost insertion

Replaced the commented-stub at the end of `buildTransport` with live code:

```go
if cfg.hook != nil {
    rt = &hookTransport{hook: cfg.hook, next: rt}
}
return rt
```

`buildTransport` godoc updated to describe the FULL revised chain: `req → hookTransport → cacheTransport → loggingTransport → headerTransport → underlying`. The "[hookTransport]" bracket marker stays — the layer is conditional on `cfg.hook != nil`.

### `options_test.go` — `TestWithRequestHook` added (3 subtests)

| Subtest | What it locks |
| --- | --- |
| "non-nil fn stored on Client.requestHook" | `NotNil(c.requestHook)` after `WithRequestHook(fn)` |
| "nil fn is no-op (cfg.hook stays nil)" | `Nil(c.requestHook)` after `WithRequestHook(nil)` |
| "default Client has nil requestHook" | `Nil(c.requestHook)` for `NewClient()` |

Function-pointer equality is not directly comparable in Go (no `comparable` for function types), so `NotNil` is the strongest available assertion — sufficient for the contract.

### `client_test.go` — three composition tests appended

| Test | Subtests | What it locks |
| --- | --- | --- |
| `TestHook_FiresOnRetryAttempts` | 1 | TRANS-05 + retry composition: 429→500→200 sequence drives 3 server hits AND 3 hook invocations under deterministic `fakeClock`-driven backoff |
| `TestHook_SeesCacheHits` | 1 | D-88 + cache composition: first call is MISS (`CacheHitContextKey` absent → false), second call is HIT (`CacheHitContextKey == true` in `req.Context()`); server hit exactly once total |
| `TestHook_DoesNotFireOnDecodeError` | 2 | (1) decode error after 200 round trip → hook count is 1 (not 2); (2) pre-HTTP validation failure → hook count is 0 |

## Verified chain order

```text
req
 ├─► hookTransport       (Plan 05 — OUTERMOST per D-89; sees cache hits via synthetic resp.Request)
 │    └─► cacheTransport (Plan 04 — short-circuits on hit; stamps CacheHitContextKey onto resp.Request)
 │         └─► loggingTransport  (Phase 2)
 │              └─► headerTransport (Phase 2)
 │                   └─► underlying RoundTripper (caller-supplied or http.DefaultTransport)
```

Cache hits short-circuit at `cacheTransport`; `loggingTransport` and `headerTransport` do NOT see them. `hookTransport` DOES see them — by D-89 design — and consumers detect the branch via `req.Context().Value(openholidays.CacheHitContextKey)` inside their hook (the value is the untyped boolean `true` on a hit, absent on a miss).

## Verification

```text
go build ./...                                            -> exit 0
go vet ./...                                              -> exit 0
gofmt -l transport_hook.go transport_hook_test.go options.go options_test.go config.go client_test.go
                                                          -> (no output)
go test -race -count=1 -run 'TestHookTransport_RoundTrip|TestHookTransport_FiresPerAttempt|TestHookTransport_PanicPropagates|TestHookTransport_NilSafeOnTransportError|TestWithRequestHook|TestHook_FiresOnRetryAttempts|TestHook_SeesCacheHits|TestHook_DoesNotFireOnDecodeError' ./...
                                                          -> ok (8 TestXxx funcs, all PASS)
go test -race -count=1 ./...                              -> ok 1.96s (full unit suite)
go test -race -count=1 -run TestNoInitOrGlobalState ./... -> ok (allowlist unchanged from Plan 04)
```

Go toolchain: `go1.26.3-X:nodwarf5 linux/amd64`.

## Commits

| Commit | Subject | Files |
| --- | --- | --- |
| `3392914` | test(04-05): add failing hookTransport tests (RED phase) | transport_hook_test.go (+287) |
| `ce8f98e` | feat(04-05): implement hookTransport RoundTripper (GREEN phase) | transport_hook.go (+84) |
| `abd7a43` | test(04-05): add WithRequestHook + composition tests (RED phase) | options_test.go (+31), client_test.go (+187) |
| `9513f76` | feat(04-05): wire WithRequestHook + hookTransport outermost in chain (GREEN) | options.go (+69), config.go (+13/-11), transport_hook.go (+28/-1) |

## Net lines modified

| File | Insertions | Deletions |
| --- | ---: | ---: |
| transport_hook.go | 112 | 1 |
| transport_hook_test.go | 287 | 0 |
| options.go | 69 | 0 |
| options_test.go | 31 | 0 |
| config.go | 13 | 11 |
| client_test.go | 187 | 0 |
| **Total** | **699** | **12** |

## Deviations from Plan

### [Rule 1 — Bug fix] hookTransport passes resp.Request to the hook instead of the inbound req

**Found during:** Task 2 GREEN verification (`TestHook_SeesCacheHits` failed: "second call is a cache HIT — hook must see CacheHitContextKey == true").

**Issue:** The plan's Task 1 `<action>` block provided a verbatim hookTransport.RoundTrip skeleton that called `t.hook(req, resp, err)` with the original inbound `req`. The plan's `must_haves` truth bullet (line 28 of 04-05-PLAN.md) and the matching test `TestHook_SeesCacheHits` both require that the hook see `CacheHitContextKey == true` via `req.Context()` on a cache hit. But `cacheTransport.RoundTrip` (Plan 04, transport_cache.go:124-131) stamps the cache-hit context-key onto a NEW request via `req.WithContext(ctxWithHit)` and attaches it ONLY to `resp.Request` — the inbound `req` pointer that flows back UP to hookTransport never carries the key. With the verbatim plan implementation, `req.Context().Value(CacheHitContextKey)` inside the hook would return nil on a cache hit, breaking the documented D-88 contract.

**Fix:** `transport_hook.go::RoundTrip` now prefers `resp.Request` when `resp != nil && resp.Request != nil`:

```go
hookReq := req
if resp != nil && resp.Request != nil {
    hookReq = resp.Request
}
t.hook(hookReq, resp, err)
```

On transport error (resp == nil) the inbound `req` is passed through unchanged. On every other path the hook receives `resp.Request`, which:

1. On a cache hit: carries `CacheHitContextKey == true` (cacheTransport sets it) → satisfies the D-88 contract.
2. On a cache miss / non-cacheable path: carries the original request context unchanged (cacheTransport does NOT mutate resp.Request, and the stdlib `http.Transport.RoundTrip` sets `resp.Request = req` by convention) → no behavior change for consumers.
3. On retry attempts: each attempt's resp.Request reflects that attempt's request → still one hook invocation per attempt, just with the canonical request reference.

**Why this is Rule 1 (bug), not Rule 4 (architectural):** No new types, no new options, no new chain layer, no new public surface. The fix is two lines inside one private method to bring the implementation into line with the plan's stated behavior contract — the smallest possible diff that satisfies the documented `<must_haves>` truth bullet. The stdlib also points here: `resp.Request` is the canonical "request that produced this response" (preserved across redirects per `net/http` package documentation), so passing it to the hook is the conventional choice and matches what experienced Go consumers will expect.

**Files modified:** `transport_hook.go` only (the change is colocated with the godoc that documents it).

**Commit:** `9513f76`.

No other deviations. No auth gates. No checkpoints triggered. No pre-existing issues fixed (the pre-existing `countries_test.go` gofmt diff from Phase 3 is documented in `.planning/phases/04-resilience/deferred-items.md` and was NOT touched — out of plan scope per the executor SCOPE BOUNDARY rule).

## Threat Surface Scan

No new network endpoints, no new auth paths, no new file access patterns, no schema changes at trust boundaries. The hook is a callback-into-consumer code path — the only new trust boundary is "client → consumer hook function" which the plan's `<threat_model>` already covers (T-04-16 panic propagation, T-04-17 body-reading, T-04-18 mutation, T-04-19 blocking, T-04-SC supply chain). All five threats are `accept` dispositions with documented mitigations either in code (TestHookTransport_PanicPropagates locks T-04-16) or in godoc (RequestHookFunc + WithRequestHook godoc covers T-04-17/18/19).

No new threat flags raised.

## Known Stubs

None. Plan 05 ships TRANS-05 fully wired — the hook fires per round trip, observes cache hits, and is documented end-to-end. There are no "stubs that prevent the plan's goal from being achieved".

The only nil-defaults remaining from Plan 02 (`Client.cache`, `Client.requestHook`) are now BOTH populated when the corresponding opt-in option is supplied. Default clients still have nil for both — that is the documented opt-in shape (D-74 / D-87), not a stub.

## Self-Check: PASSED

- `transport_hook.go` exists at repo root: FOUND
- `transport_hook_test.go` exists at repo root: FOUND
- `options.go` modified (WithRequestHook added): VERIFIED (`grep -n 'func WithRequestHook' options.go` returns one hit)
- `config.go::buildTransport` modified (hookTransport outermost): VERIFIED (`grep -n 'hookTransport{hook:' config.go` returns one hit)
- `options_test.go` modified (TestWithRequestHook added): VERIFIED (`grep -n 'func TestWithRequestHook' options_test.go` returns one hit)
- `client_test.go` modified (3 composition tests added): VERIFIED (all 3 function names grep clean)
- Commit `3392914` (Task 1 RED): FOUND in `git log`
- Commit `ce8f98e` (Task 1 GREEN): FOUND in `git log`
- Commit `abd7a43` (Task 2 RED): FOUND in `git log`
- Commit `9513f76` (Task 2 GREEN): FOUND in `git log`
- `go test -race -count=1 ./...` exits 0: VERIFIED
- `go vet ./...` exits 0: VERIFIED
- `gofmt -l` on touched files: VERIFIED (no output)
- `TestNoInitOrGlobalState` remains green (no new package-level vars added by Plan 05): VERIFIED
- Chain order matches D-89: VERIFIED via `TestHook_SeesCacheHits` (cache hits reach the hook) + `TestHook_FiresOnRetryAttempts` (retries reach the hook) + `TestHook_DoesNotFireOnDecodeError` (post-chain decode is OUT of hook scope)
