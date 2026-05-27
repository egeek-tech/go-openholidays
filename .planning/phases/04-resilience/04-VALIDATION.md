---
phase: 4
slug: resilience
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-27
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Source: `04-RESEARCH.md` § "Validation Architecture" (lines 908-981).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `testing` (stdlib) + `github.com/stretchr/testify` v1.11.1 (already in `go.mod`) |
| **Config file** | none — Go test discovery via `_test.go` naming |
| **Quick run command** | `go test ./... -run 'TestRetry\|TestCache\|TestHook\|TestStrict\|TestClient_Close\|TestNewMemoryCache\|TestCacheTransport\|TestHookTransport' -race -count=1` |
| **Full suite command** | `go test ./... -race -cover -count=1` |
| **Estimated runtime** | quick ~30 s · full ~3 min on modern dev machine |
| **Coverage target** | ≥ 85% (TEST-10; Phase 5 enforces in CI; Phase 4 must not regress) |

---

## Sampling Rate

- **After every task commit:** `go test ./... -run 'Test{ConcernUnderEdit}' -race -count=1` (e.g., `-run TestRetry` after retry-touching commits). Sub-30-second.
- **After every plan wave:** `go test ./... -race -count=1` (full unit suite, excluding fuzz/integration). Sub-3-minute.
- **Before `/gsd:verify-work`:** Full suite green (`go test ./... -race -cover -count=1`).
- **Max feedback latency:** ~30 s per-task, ~180 s per-wave.

---

## Per-Task Verification Map

> Concrete task IDs are assigned by the planner. The map below lists the required test behaviors keyed by Phase 4 requirement IDs; the planner MUST cite the corresponding `<automated>` command in each task's `acceptance_criteria`.

| Req ID | Behavior | Test Type | Automated Command | File Exists |
|--------|----------|-----------|-------------------|-------------|
| RESIL-01 | `WithRetry(5, 250ms)` enables retry with exp backoff + full jitter | unit | `go test -run TestWithRetry -race -count=1` | ❌ W0: `options_test.go` ext OR `retry_test.go::TestWithRetry_AppliesConfig` |
| RESIL-01 | `math/rand/v2.Int64N` produces in-range jitter delays | unit | `go test -run TestComputeBackoff -race -count=1` | ❌ W0: `retry_test.go::TestComputeBackoff_FullJitter` |
| RESIL-02 | Retry on `{408,429,500,502,503,504}` only; not on 400/401/403/404 | unit (table) | `go test -run TestShouldRetry -race -count=1` | ❌ W0: `retry_test.go::TestShouldRetry` |
| RESIL-02 | Retry on `net.Error{Timeout:true}` and `syscall.ECONNRESET`-wrapped errors | unit | `go test -run TestShouldRetry_TransportErrors -race -count=1` | ❌ W0: same file |
| RESIL-02 | NEVER retry on `context.Canceled` / `context.DeadlineExceeded` | unit | `go test -run TestRetry_NeverRetriesCtxErrors -race -count=1` | ❌ W0: `retry_test.go` |
| RESIL-03 | `parseRetryAfter` accepts integer seconds and HTTP-date (3 forms) | unit (table) | `go test -run TestParseRetryAfter -race -count=1` | ❌ W0: `retry_test.go::TestParseRetryAfter` |
| RESIL-03 | `computeBackoff` returns `max(jitter, retryAfter)` capped at `maxRetryWait` | unit | `go test -run TestComputeBackoff_HonorsRetryAfter -race -count=1` | ❌ W0: same file |
| RESIL-04 | Retry loop interrupted by `ctx.Done()` mid-sleep within < 100 ms | unit | `go test -run TestRetry_CtxCancel -race -count=1` | ❌ W0: `retry_test.go` — uses `fakeClock` + real ctx cancel |
| RESIL-05 | Retry implemented in `doJSONGet`, NOT as a RoundTripper | unit (AST/structural) | `go test -run TestRetry_NotARoundTripper -race -count=1` | ❌ W0: assert no `retryTransport` type exists (`internal_test.go` ext) |
| RESIL-06 | `Cache` interface exported with `Get/Put/Close` shape | unit | `go test -run TestCacheInterface_Conformance -race -count=1` | ❌ W0: `cache_test.go` — `var _ Cache = (*MemoryCache)(nil)` |
| RESIL-07 | `WithCache(ttl)` caches `/Countries`, `/Languages`, `/Subdivisions` only | unit | `go test -run TestCacheTransport_PathAllowlist -race -count=1` | ❌ W0: `transport_cache_test.go` |
| RESIL-07 | `WithCache` does NOT cache `/PublicHolidays` / `/SchoolHolidays` | unit | `go test -run TestCacheTransport_HolidayPathsBypass -race -count=1` | ❌ W0: same file |
| RESIL-07 | `WithCache(ttl <= 0)` disables cache | unit | `go test -run TestWithCache_NonPositiveTTLDisables -race -count=1` | ❌ W0: `options_test.go` ext OR `cache_test.go` |
| RESIL-08 | Sweeper starts lazily on first `Put` | unit | `go test -run TestMemoryCache_SweeperLazyStart -race -count=1` | ❌ W0: `cache_test.go` — uses `runtime.NumGoroutine()` delta |
| RESIL-08 | `Client.Close()` stops sweeper; `runtime.NumGoroutine()` delta ≤ 0 | unit | `go test -run TestClient_CloseStopsSweeper -race -count=1` | ❌ W0: `client_test.go` ext |
| RESIL-08 | `MemoryCache.Close()` idempotent (safe to call twice from concurrent goroutines) | unit | `go test -run TestMemoryCache_CloseIdempotent -race -count=1` | ❌ W0: `cache_test.go` |
| RESIL-09 | Cache stores raw bytes keyed by `(method, path, query)`; decode on read | unit | `go test -run TestCacheTransport_RawBytesKey -race -count=1` | ❌ W0: `transport_cache_test.go` |
| RESIL-09 | Strict-decoding applies to cached bytes on re-read | unit (composition) | `go test -run TestCache_StrictDecodingComposes -race -count=1` | ❌ W0: composition test in `client_test.go` ext |
| TRANS-05 | `WithRequestHook` fires after every round trip including retries | unit | `go test -run TestHookTransport_FiresPerAttempt -race -count=1` | ❌ W0: `transport_hook_test.go` |
| TRANS-05 | `WithRequestHook` fires on cache-hit synthetic response | unit (composition) | `go test -run TestHook_SeesCacheHits -race -count=1` | ❌ W0: `client_test.go` ext |
| TRANS-05 | Hook does NOT fire on decode errors or pre-HTTP failures | unit | `go test -run TestHook_DoesNotFireOnDecodeError -race -count=1` | ❌ W0: `client_test.go` ext |
| TRANS-05 | Hook is synchronous; panic propagates to caller | unit | `go test -run TestHookTransport_PanicPropagates -race -count=1` | ❌ W0: `transport_hook_test.go` |
| OBS-03 | `WithStrictDecoding(true)` rejects unknown fields | unit | `go test -run TestWithStrictDecoding_RejectsUnknown -race -count=1` | ❌ W0: `client_test.go` ext OR `request_test.go` ext |
| OBS-03 | `WithStrictDecoding(false)` (default) accepts unknown fields | unit | `go test -run TestWithStrictDecoding_DefaultLenient -race -count=1` | ❌ W0: same file |
| TEST-05 | Fake transport returning 429→500→200 produces correct retry behavior | unit (table) | `go test -run TestRetry_E2E -race -count=1` | ❌ W0: `retry_test.go::TestRetry_E2E_429Then500Then200` |
| TEST-05 | Retry uses `fakeClock` (no real `time.Sleep`); total fake-clock advance matches expected | unit | `go test -run TestRetry_DeterministicClock -race -count=1` | ❌ W0: `retry_test.go` |
| TEST-05 | `Retry-After` integer-seconds form respected | unit | `go test -run TestRetry_HonorsRetryAfterSeconds -race -count=1` | ❌ W0: same file |
| TEST-05 | `Retry-After` HTTP-date form respected | unit | `go test -run TestRetry_HonorsRetryAfterDate -race -count=1` | ❌ W0: same file |
| TEST-06 | Cache TTL eviction works under `fakeClock` (no real `time.Sleep`) | unit | `go test -run TestMemoryCache_TTLEviction -race -count=1` | ❌ W0: `cache_test.go` |
| TEST-06 | Cache hit returns identical bytes; cache miss does HTTP round trip | unit | `go test -run TestCacheTransport_HitMissBehavior -race -count=1` | ❌ W0: `transport_cache_test.go` |
| TEST-06 | Default-off behavior: no cache means every call hits the network | unit | `go test -run TestClient_NoCache_AllCallsHitNetwork -race -count=1` | ❌ W0: `client_test.go` ext |
| CLIENT-08 (existing invariant) | `Client.Close()` is idempotent and concurrent-safe | unit | `go test -run TestClient_Close -race -count=1` | ✅ existing `TestClient_Close` — Phase 4 extends with sweeper-stop assertion |
| CLIENT-10 (existing invariant) | No new `init()` functions; no new package-level vars; `allowedVars` unchanged | unit (AST) | `go test -run TestNoInitOrGlobalState -race -count=1` | ✅ existing — must stay green without modification |
| Internal | `newClientForTest(opts..., now, sleep)` produces a `*Client` with the seam wired | unit | `go test -run TestNewClientForTest -race -count=1` | ❌ W0: `client_test.go` ext |
| Internal | `fakeClock.Advance` is thread-safe under `-race` | unit | `go test -run TestFakeClock_RaceFree -race -count=1` | ❌ W0: `clock_test.go::TestFakeClock_RaceFree` |

*Status legend: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky. Planner fills task-ID column when plans are finalized.*

---

## Wave 0 Requirements

The following test files / fixtures / harnesses MUST exist before any Phase 4 task that adds production code can pass verification:

- [ ] `clock_test.go` — `fakeClock` struct with `Now()`, `Advance(d)`, and `Sleep(ctx, d) error` methods (D-95 pattern). Lands first.
- [ ] `retry_test.go` — tests for `shouldRetry`, `parseRetryAfter`, `computeBackoff`, full E2E retry behavior under `httptest.Server` with `fakeClock`. Requires `clock_test.go`.
- [ ] `cache_test.go` — tests for `MemoryCache` Get/Put/Close lifecycle, sweeper start/stop, TTL eviction, idempotent Close, clock-injected behavior. Requires `clock_test.go`.
- [ ] `transport_cache_test.go` — tests for `cacheTransport` path allow-list, success-only caching, synthetic response shape, body drain-and-close, `CacheHitContextKey` set on hit. Requires `cache_test.go`.
- [ ] `transport_hook_test.go` — tests for `hookTransport` firing per round trip, synchronous contract, panic propagation. Standalone (no fakeClock dependency).
- [ ] `client_test.go` extensions — composition tests: `TestClient_CloseStopsSweeper`, `TestHook_SeesCacheHits`, `TestCache_StrictDecodingComposes`, `TestClient_NoCache_AllCallsHitNetwork`, `TestNewClientForTest`. Lives in existing `client_test.go` (no new file).

**Framework install:** not needed — `testify` v1.11.1 already in `go.mod` (Phase 1).

*Existing infrastructure adequate for: `httptest.NewServer` pattern (Phase 2); AST audit pattern from `internal_test.go` (Phase 4 does not modify allowlist); testify `require`/`assert` patterns from Phase 1-3.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Real-world `Retry-After` from live upstream (`openholidaysapi.org` does not currently emit one) | RESIL-03 | Requires server cooperation that does not happen in practice | Out of scope; document in PROJECT.md "Open Question" if upstream behavior changes |

*All other Phase 4 behaviors have automated verification per the map above.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify command OR a Wave 0 dependency
- [ ] Sampling continuity: no 3 consecutive tasks without an automated verify
- [ ] Wave 0 covers all MISSING references in "Per-Task Verification Map"
- [ ] No watch-mode flags (`-watch`, `--watch`, `go test -count=∞`) anywhere
- [ ] Feedback latency < 30 s per-task / < 180 s per-wave
- [ ] `nyquist_compliant: true` set in frontmatter after planner finalizes task IDs

**Approval:** pending
