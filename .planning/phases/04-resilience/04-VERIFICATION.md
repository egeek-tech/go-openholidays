---
phase: 04-resilience
verified: 2026-05-28T12:00:00Z
status: passed
score: 13/13
overrides_applied: 0
plans_verified: 6
requirements_verified: 13
---

# Phase 4: Resilience Verification Report

**Phase Goal:** Retry, cache, observability hook, and strict-decoding land as transparent middleware. Endpoint method signatures from Phase 3 remain UNCHANGED — the RoundTripper chain absorbs all new behavior. `Client.Close()` becomes load-bearing (stops cache sweeper).
**Verified:** 2026-05-28T12:00:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | Retry with exponential backoff + full jitter + Retry-After, ctx-aware, endpoint-layer (not RoundTripper) | VERIFIED | `retry.go` has `shouldRetry`/`parseRetryAfter`/`computeBackoff`; `request.go` has retry loop calling `c.sleepFunc`; `TestRetry_E2E_429Then500Then200` PASS; `TestRetry_NotARoundTripper` PASS; no `transport_retry.go` |
| 2 | Cache for Countries/Languages/Subdivisions only; raw bytes; `< 5 ms` on hit; strict-decoding composes | VERIFIED | `cache.go` `MemoryCache`; `transport_cache.go` `cacheTransport`; `isCacheablePath` exact-match; `TestCache_StrictDecodingComposes` PASS; `TestCacheTransport_HolidayPathsBypass` PASS |
| 3 | `Client.Close()` stops sweeper; idempotent; race-safe | VERIFIED | `closeOnce sync.Once` in `client.go`; `MemoryCache.closeOnce`; `TestClient_CloseStopsSweeper` PASS; `TestMemoryCache_CloseIdempotent` PASS (100 goroutines) |
| 4 | `WithRequestHook` fires after every round trip including cache hits and retries; no OTel dependency | VERIFIED | `transport_hook.go` outermost; `TestHook_FiresOnRetryAttempts` PASS (3 invocations); `TestHook_SeesCacheHits` PASS; no external imports |
| 5 | `WithStrictDecoding(true)` rejects unknown fields; OFF by default; Phase 3 signatures unchanged | VERIFIED | `request.go` L154-156 `if c.strict { decoder.DisallowUnknownFields() }`; `TestWithStrictDecoding_RejectsUnknown` PASS; `TestWithStrictDecoding_DefaultLenient` PASS; endpoint files unmodified since Phase 3 |

**Score:** 5/5 observable truths VERIFIED

---

### Phase 4 Success Criteria Verification (ROADMAP.md)

| # | Success Criterion | Status | Evidence |
|---|------------------|--------|---------|
| SC-1 | `NewClient(WithRetry(5, 250ms))` against 429→500→200: exactly 3 round trips, exponential backoff + full jitter, honors `Retry-After: 2` and RFC 1123 date, ctx cancels mid-sleep | VERIFIED | `TestRetry_E2E_429Then500Then200` PASS; `TestRetry_HonorsRetryAfterSeconds` PASS; `TestRetry_HonorsRetryAfterDate` PASS; `TestRetry_CtxCancel` PASS; retry in endpoint layer per RESIL-05 |
| SC-2 | `WithCache(24h)` caches Countries/Languages/Subdivisions; second call `< 5 ms`; PublicHolidays/SchoolHolidays never cached; strict-decoding on cached bytes | VERIFIED | `TestCacheTransport_HitMissBehavior` PASS; `TestCacheTransport_HolidayPathsBypass` PASS; `TestCache_StrictDecodingComposes` PASS (2nd call = cache hit, strict fires) |
| SC-3 | `Client.Close()` stops sweeper; goroutine delta ≤ 0 after Close+10ms; safe to call twice from concurrent goroutines | VERIFIED | `TestClient_CloseStopsSweeper` PASS; `TestMemoryCache_CloseIdempotent` PASS; `TestClient_Close/concurrent_close_is_race-safe_(100_goroutines)` PASS |
| SC-4 | `WithRequestHook` fires per round trip including cache hits; no OTel dependency | VERIFIED | `TestHook_FiresOnRetryAttempts` PASS (3 hits); `TestHook_SeesCacheHits` PASS; no external imports in `transport_hook.go` |
| SC-5 | `WithStrictDecoding(true)` rejects unknown fields; OFF by default; Phase 3 signatures untouched | VERIFIED | `TestWithStrictDecoding_RejectsUnknown`/`DefaultLenient` PASS; no diff on Phase 3 endpoint files since Phase 3 commits |

---

### Required Artifacts (must_haves from all 6 plans)

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `clock_test.go` | `fakeClock` + `Now`/`Advance`/`Sleep` + `TestFakeClock_RaceFree` | VERIFIED | File exists; `package openholidays`; all 3 methods present; `TestFakeClock_RaceFree` PASS |
| `retry.go` | `retryConfig`, `shouldRetry`, `parseRetryAfter`, `computeBackoff`, consts | VERIFIED | All 4 symbols present; `defaultBaseDelay=250ms`, `defaultMaxRetryWait=60s`; 260 LOC |
| `cache.go` | `MemoryCache`, `NewMemoryCache`, `newMemoryCacheWithClock`, sweeper | VERIFIED | All symbols present; lazy sweeper via `startOnce`; 229 LOC |
| `transport_cache.go` | `cacheTransport`, `isCacheablePath`, `CacheHitContextKey` | VERIFIED | All symbols present; `var CacheHitContextKey = cacheHitKeyType{}` |
| `transport_hook.go` | `hookTransport` struct + `RoundTrip` method | VERIFIED | File exists; `type hookTransport struct`; outermost in chain |
| `client.go` | 14-field Client struct including `closeOnce sync.Once` | VERIFIED | All 14 fields present; `Close()` uses `closeOnce.Do` |
| `config.go` | `Cache` interface, `RequestHookFunc`, `retryConfig` (filled), 5 new `clientConfig` fields | VERIFIED | All types declared; `buildTransport` inserts cache then hook conditionally |
| `options.go` | `WithStrictDecoding`, `WithRetry`, `WithMaxRetryWait`, `WithCache`, `WithCacheBackend`, `WithRequestHook` | VERIFIED | All 6 options present and callable |
| `request.go` | Retry loop `for attempt := 0; attempt < maxAttempts; attempt++`; `if c.strict` gate | VERIFIED | Retry loop at L88-128; strict gate at L154-156 |
| `internal_test.go` | `allowedVars` includes `CacheHitContextKey` | VERIFIED | Line 81: `"CacheHitContextKey": {}` present |
| No `transport_retry.go` | RESIL-05 enforcement | VERIFIED | `find` returns nothing; `TestRetry_NotARoundTripper` PASS |
| `PROJECT.md` | CL-15 and CL-16 rows in Key Decisions table | VERIFIED | `grep -cE '^\| CL-'` returns 9 total; CL-15 (cache) and CL-16 (strict) present |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `request.go::doJSONGet` retry loop | `retry.go::shouldRetry + computeBackoff + parseRetryAfter` | function calls inside `for attempt` loop | VERIFIED | `shouldRetry(resp, httpErr)` at L101; `computeBackoff(attempt, retryAfter, c.retry, c.rand)` at L124 |
| `request.go::doJSONGet` ctx-sleep | `client.go::Client.sleepFunc` | `c.sleepFunc(ctx, delay)` | VERIFIED | L125: `c.sleepFunc(ctx, delay)` |
| `config.go::buildTransport` | `transport_cache.go::cacheTransport` | `&cacheTransport{...}` when `cfg.cache != nil` | VERIFIED | L163-169 in `config.go` |
| `config.go::buildTransport` | `transport_hook.go::hookTransport` | `&hookTransport{...}` outermost when `cfg.hook != nil` | VERIFIED | L174-176 in `config.go`; OUTERMOST confirmed |
| `client.go::Close` | `cache.go::MemoryCache.Close` | `c.cache.Close()` inside `closeOnce.Do` | VERIFIED | L120-122 in `client.go` |
| `transport_cache.go::CacheHitContextKey` | `transport_hook.go::hookTransport` (hook sees it) | synthetic resp.Request.Context() carries key; hookTransport passes `resp.Request` to hook | VERIFIED | `TestHook_SeesCacheHits` PASS |

---

### Data-Flow Trace (Level 4)

Not applicable — this is a Go SDK library (no rendering/dynamic UI components). Data flows are verified via behavioral tests (retry loop test, cache hit/miss test, hook invocation tests).

---

### Behavioral Spot-Checks

| Behavior | Result | Status |
|----------|--------|--------|
| `go test -race -count=1 ./...` | `ok github.com/egeek-tech/go-openholidays 1.935s` | PASS |
| `go vet ./...` | No output | PASS |
| `TestRetry_E2E_429Then500Then200` | 3 round trips, payload decoded | PASS |
| `TestRetry_CtxCancel` | ctx.Err() returned | PASS |
| `TestClient_CloseStopsSweeper` | goroutine delta ≤ 0 after Close+10ms | PASS |
| `TestCache_StrictDecodingComposes` | decode error on 2nd (cached) call with strict mode | PASS |
| `TestHook_SeesCacheHits` | hook fires with CacheHitContextKey on 2nd call | PASS |
| `TestHook_FiresOnRetryAttempts` | 3 hook invocations for 429→500→200 | PASS |
| `TestNoInitOrGlobalState` | PASS | PASS |
| `TestRetry_NotARoundTripper` | no transport_retry.go; no retryTransport | PASS |

---

### Requirements Coverage

| Requirement | Plans | Description | Status | Evidence |
|-------------|-------|-------------|--------|---------|
| RESIL-01 | 03 | Exponential backoff + full jitter (`math/rand/v2`) | VERIFIED | `computeBackoff` uses `rnd.Int64N`; `TestComputeBackoff` PASS |
| RESIL-02 | 03 | Retry on 429, 5xx, transient net errors; NOT 4xx (except 429) | VERIFIED | `shouldRetry` switch on {408,429,500,502,503,504}; `TestShouldRetry` PASS |
| RESIL-03 | 03 | Retry honors `Retry-After` (integer + HTTP-date) | VERIFIED | `parseRetryAfter`; `TestRetry_HonorsRetryAfterSeconds/Date` PASS |
| RESIL-04 | 03 | Retry loop ctx-aware; `ctx.Done()` interrupts sleep | VERIFIED | loop-top `ctx.Err()` check; `c.sleepFunc` ctx-aware; `TestRetry_CtxCancel` PASS |
| RESIL-05 | 03 | Retry in endpoint layer, NOT a RoundTripper | VERIFIED | `transport_retry.go` absent; `TestRetry_NotARoundTripper` PASS |
| RESIL-06 | 04 | `Cache` interface public (`Get`/`Put`/`Close`) | VERIFIED | `Cache` interface in `config.go`; `MemoryCache` exported; `var _ Cache = (*MemoryCache)(nil)` in `cache_test.go` |
| RESIL-07 | 04 | `WithCache(ttl)` caches only Countries/Languages/Subdivisions | VERIFIED | `isCacheablePath` exact-match allowlist; `TestCacheTransport_HolidayPathsBypass` PASS |
| RESIL-08 | 04 | Sweeper lazy start on first Put; stopped by Close | VERIFIED | `startOnce.Do(m.startSweeper)`; `TestMemoryCache_SweeperLazyStart` PASS; `TestClient_CloseStopsSweeper` PASS |
| RESIL-09 | 04 | Cache stores raw bytes; decode happens on read | VERIFIED | `cacheTransport` caches raw bytes; strict gate in `doJSONGet` runs on every call including cache hits; `TestCache_StrictDecodingComposes` PASS |
| TRANS-05 | 05 | `WithRequestHook` for OTel/metrics; no external dep | VERIFIED | `hookTransport` outermost; `TestHook_FiresOnRetryAttempts` (3 hits); no imports beyond stdlib |
| OBS-03 | 02 | `WithStrictDecoding(bool)` opt-in strict JSON decode | VERIFIED | `decoder.DisallowUnknownFields()` gate; `TestWithStrictDecoding_RejectsUnknown/DefaultLenient` PASS |
| TEST-05 | 01, 03 | Retry tests with fake clock/transport | VERIFIED | `fakeClock` in `clock_test.go`; 10 retry tests in `retry_test.go` all PASS |
| TEST-06 | 01, 04 | Cache TTL eviction tests with fake clock | VERIFIED | `newMemoryCacheWithClock` test seam; `TestMemoryCache_TTLEviction` PASS under `fakeClock` |
| CLIENT-08 | 02, 04 | `Client.Close()` stops goroutines; idempotent | VERIFIED | `closeOnce.Do`; `TestClient_Close/concurrent_close_is_race-safe` PASS |

**All 13 requirement IDs (+ CLIENT-08) accounted for and VERIFIED.**

Note on RESIL-06 interface deviation: REQUIREMENTS.md originally specified `Set(key, val, ttl)` but the planning documents (04-CONTEXT.md D-79) deliberately revised the interface to `Put(key, val)` with TTL owned by the cache at construction, not per-entry. This is a documented intentional design decision recorded in D-79 and reflected in CL-15 in PROJECT.md. The implementation matches the planning spec exactly.

---

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| `countries_test.go` | Missing trailing newline (gofmt reports it) | Info | Pre-dates Phase 4 — last modified in Phase 3 commit `9730014`; not a Phase 4 regression |

No Phase 4 files have unresolved `TBD`, `FIXME`, `XXX`, placeholder returns, or empty implementations.

---

### Human Verification Required

None. All success criteria are verifiable programmatically and confirmed by the test suite.

---

## Summary

Phase 4 goal achieved in full. All 5 observable truths VERIFIED. All 5 ROADMAP.md success criteria VERIFIED. All 13 Phase 4 requirement IDs (+ CLIENT-08) satisfied with passing tests. Full test suite exits 0 under `-race`. No transport_retry.go file; retry lives in the endpoint layer as required (RESIL-05). Phase 3 endpoint signatures are completely unchanged. `Client.Close()` is load-bearing (stops the cache sweeper), idempotent, and race-safe under 100 concurrent goroutines. The RoundTripper chain order matches D-89: `req → hookTransport → cacheTransport → loggingTransport → headerTransport → underlying`.

---

_Verified: 2026-05-28T12:00:00Z_
_Verifier: Claude (gsd-verifier)_
