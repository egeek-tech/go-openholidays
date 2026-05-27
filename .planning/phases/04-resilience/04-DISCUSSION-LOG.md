# Phase 4: Resilience - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-27
**Phase:** 04-resilience
**Areas discussed:** Retry public surface & defaults, Cache interface/pluggability/sweeper lifecycle, Hook contract, Strict-decoding plumbing + fake-clock injection

---

## Retry public surface & defaults

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal: just `WithRetry(maxAttempts, baseDelay)` | Internal constants for max-wait cap (60 s), retryable statuses, transient net errors. Zero/neg maxAttempts → disabled. No caller-side max-wait tuning in v0.1.0. | |
| Minimal + `WithMaxRetryWait(d)` escape hatch | Adds one knob (per-attempt sleep cap) so a slow Retry-After can't hold a request indefinitely. Same internal status set. | ✓ |
| Config-struct shim under positional signature | Internal `retryConfig` struct carries maxWait + status set; expose later via new options without breaking the positional one. Lays groundwork for v0.2 knobs. | |

**User's choice:** Minimal + `WithMaxRetryWait(d)` escape hatch.
**Notes:** Lands as two exported options (`WithRetry`, `WithMaxRetryWait`). Defaults locked per Claude's discretion: maxRetryWait = 60 s, retryable statuses = {408, 429, 500, 502, 503, 504} + transient net errors (`net.Error.Timeout()`, `syscall.ECONNRESET`), zero/neg attempts disables retry, zero/neg baseDelay → 250 ms. Random source = per-Client `*rand.Rand` (`math/rand/v2` ChaCha8) crypto-seeded at NewClient.

---

## Cache: interface, pluggability, sweeper/Close lifecycle

| Option (pluggability) | Description | Selected |
|--------|-------------|----------|
| `WithCache(ttl)` only; `Cache` interface ships unused | ROADMAP signature stays; future pluggability anchored by the public interface but no v0.1.0 way to pass a custom one. | |
| `WithCache(ttl)` + `WithCacheBackend(c Cache)` overload | Two options. `NewMemoryCache(ttl)` exported. Backends pluggable in v0.1.0. | ✓ |
| Replace with `WithCache(c Cache)` only | Drops ROADMAP literal signature; caller always passes a Cache. More explicit, more verbose. | |

**User's choice:** `WithCache(ttl)` + `WithCacheBackend(c Cache)` overload.
**Notes:** Both options ship in v0.1.0. `NewMemoryCache(ttl) *MemoryCache` exported. Recorded as CL-14 in PROJECT.md.

| Option (interface method set) | Description | Selected |
|--------|-------------|----------|
| `Get/Set/Close`, `[]byte`, no errors | TTL passed per-Set so wrappers can vary it. Get's bool distinguishes absent from empty. | |
| `Get/Set/Close + Delete`, errors on every op | Larger surface; more for v1 to commit to. | |
| `Get/Put/Close` with cache-owned TTL | TTL set at construction; not per-Put. Simpler interface; custom backends can't vary TTL per entry. | ✓ |

**User's choice:** `Get/Put/Close` with cache-owned TTL.
**Notes:** Cache interface = `Get(key string) ([]byte, bool)` / `Put(key string, value []byte)` / `Close() error`. TTL fixed at construction.

| Option (sweeper/Close) | Description | Selected |
|--------|-------------|----------|
| Lazy on first Put; interval = max(TTL/4, 30 s); Close stops sweeper + calls Cache.Close | Sweeper spawns on first Put. Short-TTL test caches sweep often; long-TTL prod caches don't churn. | ✓ |
| Eager start at NewClient; fixed 60 s interval | Goroutine always exists when cache is configured. Simpler control flow. | |
| Lazy on first Put + interval = TTL/2, clamped [10 s, 5 min] | More aggressive sweep cadence for small TTLs. | |

**User's choice:** Lazy on first Put; interval = max(TTL/4, 30 s); Close stops sweeper + calls Cache.Close.
**Notes:** `Client.closeOnce sync.Once` added. `Close()` flips existing atomic.Bool, then runs once-guarded `cache.Close()`. Error from `cache.Close()` swallowed (best-effort cleanup).

---

## Hook contract (cache hits, retries, sync vs async, decode errors)

| Option | Description | Selected |
|--------|-------------|----------|
| HTTP layer only: per round trip + per cache hit; never on decode/pre-HTTP ctx errors | Clean HTTP-layer observability. Users wanting decode visibility wrap their own transport. | ✓ |
| Full request lifecycle: fires per attempt AND on every error path | Surfaces all observable error states uniformly. Larger contract. | |
| HTTP layer + separate `ResultHook` for decode/validation errors | Two hooks; doubles public surface. Rejected unless decode visibility critical. | |

**User's choice:** HTTP layer only — fires per round trip + per cache hit, NOT on decode errors or pre-HTTP ctx errors.
**Notes:** Hook chain placement = outermost RoundTripper (above cache) so cache hits are naturally visible — this revises STATE.md's older order which had hook below cache (predates RESIL-05 retry-relocation). Synchronous-only documented (Pitfall CONC-2). Exported `CacheHitContextKey` lets consumers detect cache hits inside their hook.

---

## Strict-decoding plumbing + fake-clock injection

| Option | Description | Selected |
|--------|-------------|----------|
| Internal `nowFunc` + `sleepFunc` on Client; not exported | Two unexported fields; defaults `time.Now` and ctx-aware sleep. Test access via unexported `newClientForTest`. Zero public surface. | ✓ |
| Public `WithClock(Clock)` option | `type Clock interface { Now(); Sleep(ctx, d) error }`. Consumers can plug custom clocks for production too. Adds 1 public option + 1 public interface. | |
| Test-only build-tagged shim with package-level var | Plain `var` indirection; ugly + racy + conflicts with CLIENT-10 no-global-mutable-state. | |

**User's choice:** Internal `nowFunc` + `sleepFunc` on Client; not exported.
**Notes:** Strict-decoding flag is also immutable on Client (no per-call override). Cache hits re-run the strict decoder on every read — intended behavior. CL-15 recorded for the immutability rationale.

---

## Claude's Discretion

- File layout: `retry.go`, `cache.go`, `transport_hook.go`, `transport_cache.go` ship at repo root with sibling `_test.go` files. `clock_test.go` for the shared fake-clock helper.
- No new exported sentinels in Phase 4. Retry-exhausted errors wrap `lastErr` via `fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", ...)` so callers branch on the underlying sentinel (`*APIError`, `ErrEmptyResponse`, etc.).
- `parseRetryAfter` uses `http.ParseTime` (accepts RFC 1123 + RFC 850 + ANSI C asctime per RFC 7231 §7.1.1.1) and `strconv.Atoi` for the integer-seconds form.
- Cache key encoding: `req.Method + " " + req.URL.Path + "?" + req.URL.Query().Encode()`. Excludes Host (per-Client isolation per Pitfall CACHE-2).
- Cache only success responses (status 200, err == nil).
- `cacheTransport` sets `resp.ContentLength = int64(len(cachedBytes))` on synthetic cache-hit responses so OBS-02 `bytes_in` is correct on cache hits.
- Goroutine-leak audit uses `runtime.NumGoroutine()` delta (consistent with Phase 2 D-49) rather than adding `go.uber.org/goleak` as a test-only dep.
- PROJECT.md Key Decisions appends CL-14 (Cache surface) and CL-15 (StrictDecoding immutability).

## Deferred Ideas

- Negative caching (`WithNegativeCacheTTL`) — v0.2 if a real consumer asks.
- Persistent cache backends (Redis, BoltDB) — out of v0.1.0; `Cache` interface anchors future extension via `WithCacheBackend`.
- Retry-After cumulative-budget cap (`WithMaxRetryBudget`) — consumers use `ctx.WithTimeout` instead.
- Per-status custom retry policies (`WithRetryStatus`) — v0.2 if asked.
- Async hook variant — explicit no; consumers own their own goroutines.
- Per-call strict-decoding override — out; instantiate two Clients instead.
- `WithCacheable(paths ...string)` to add custom cacheable endpoints — out; protects against temporal-data trap.
- Distributed-tracing propagation in the SDK itself — out (zero-dep); consumers thread it via `WithRequestHook`.
- Cache `Vary`-header awareness (RFC 7234) — out for v0.1.0.
- Connection-pool tuning — consumers supply their own `*http.Client`.
- `go.uber.org/goleak` adoption — out; revisit if real flakes surface.
- Retry attempt counter wired into `loggingTransport.attempt` — Phase 5 / v0.2.
