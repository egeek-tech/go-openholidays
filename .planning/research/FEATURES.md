# Feature Research

**Domain:** Go HTTP/JSON SDK library wrapping a public REST API (OpenHolidays)
**Researched:** 2026-05-26
**Confidence:** HIGH (verified against `stripe-go`, `google/go-github`, `aws-sdk-go-v2`, `slack-go/slack`, OpenTelemetry contrib, and Go 1.23 release docs)
**Scope:** Milestone M1 — `v0.1.0` of `go-openholidays`. M2–M5 features called out explicitly where deferred.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Missing any of these and a Go backend engineer will reject the library on first read of `pkg.go.dev`. These are the baseline for "this looks like a serious library."

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `NewClient(opts ...Option) *Client` functional options | Canonical Go SDK constructor since Dave Cheney's 2014 post; used by `aws-sdk-go-v2`, `stripe-go`, `slack-go`. Required so users don't break when we add `WithFoo` next release. | S | Options needed in M1: `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithLogger`, `WithTimeout`, `WithRetry`, `WithCache`, `WithRequestHook`, `WithStrictDecoding`. |
| `context.Context` as first arg on every endpoint method | Stripe v82 reaffirmed this is the standard; `go-github`, `aws-sdk-go-v2` all do it. Required for cancellation, deadlines, trace propagation. | S | Already in POC `openholidays/main.go`. |
| Typed request/response structs (`Holiday`, `Subdivision`, `Country`, `Language`) | Returning `map[string]any` would be rejected as un-idiomatic. | S | POC already has shapes — formalize them in `pkg openholidays`. |
| Custom `UnmarshalJSON` for `YYYY-MM-DD` dates | The API returns date strings; `time.Time` default unmarshal expects RFC3339. Without this, every consumer writes parsing glue. | S | Wrap `time.Time` or use `civil.Date` shape (define own type, no dep). |
| Client-side parameter validation before HTTP call | Avoids burning a network round-trip for an obviously bad country code. `aws-sdk-go-v2` does this. | S | 2-letter uppercase ISO, `validFrom <= validTo`, ≤ 3-year window (upstream limit). |
| Typed errors via `errors.Is` / `errors.As` | `errors.As` is the canonical Go 1.13+ pattern. `go-github` returns `*RateLimitError`, `*AcceptedError`; `aws-sdk-go-v2` uses `smithy.APIError`. | S | Sentinels: `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`. Struct: `*APIError{StatusCode, Path, Body}`. |
| Error wrapping with `fmt.Errorf("...: %w", err)` | Standard since Go 1.13. Required for chained `errors.Is`. | S | Used at every transport→domain boundary. |
| `Accept: application/json` + `User-Agent: go-openholidays/<version>` on every request | Operators need to identify SDK traffic in upstream logs; some APIs reject missing UA. | S | Version baked at build time via `runtime/debug.ReadBuildInfo` for module path, fallback constant. |
| Default 15 s timeout on the embedded `http.Client` | A library that hangs forever in production is a P0 bug. AWS SDK default is similar; Stripe defaults to 80 s but exposes it. | S | Overridable via `WithTimeout` AND `WithHTTPClient` (latter wins). |
| `context.Context` cancellation propagates to in-flight HTTP within ~100 ms | `http.NewRequestWithContext` does this for free. Worth stating because users assert it. | S | Just don't break `net/http` semantics. |
| Goroutine-safe `Client` shareable across goroutines | Every serious Go SDK is concurrent-safe. Single client per process is the norm. | S | No shared mutable state beyond `http.Client` (already safe) and optional cache (use `sync.RWMutex` or `sync.Map`). |
| `io.LimitReader` cap on response body (~10 MiB) | A misbehaving / compromised upstream that streams forever would OOM the caller. AWS SDK does similar. | S | Mentioned in `.planning/PROJECT.md` constraints. |
| `slog.Default()`-based structured logging at Debug level only by default | Stdlib `log/slog` is now expected; libraries should NOT log at Info+ unless asked. | S | `WithLogger(*slog.Logger)` to override. Never log response bodies above Debug. |
| Strict-decoding mode (`json.Decoder.DisallowUnknownFields`) opt-in | Lets users catch upstream schema drift. POC observed optional fields (`comment`, `quality`, `subdivisions`) — strict-by-default would break consumers when upstream adds a field. | S | `WithStrictDecoding(true)`; OFF by default. |
| `doc.go` + `example_test.go` per public method | `pkg.go.dev` rendering is the first impression. Go community expects runnable examples. | M | One example per method = ~5 examples for M1. |
| README quickstart ≤ 20 lines that compiles | Adoption signal. `stripe-go` and `go-github` both lead with a tiny working snippet. | S | Mirror the POC's clear shape. |

**Subtotal table stakes complexity:** mostly S, one M (examples). All required for v0.1.0.

---

### Differentiators (Competitive Advantage)

These are where `go-openholidays` beats `holidays-rest/sdk-go`, `rickar/cal/v2/pl`, and naïve hand-rolled clients. Each is justified against the Core Value: regional school-break granularity + idiomatic Go.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **School holidays per województwo / subdivision** with typed `Subdivisions []SubdivisionRef` on `Holiday` | `holidays-rest` doesn't expose this at all; `rickar/cal/v2/pl` is offline & nationwide-only. This is the *literal reason this library exists* per `PROJECT.md`. | S (data model) — heavy lifting is upstream | POC confirmed 4 staggered ferie-zimowe cohorts. |
| **`Holiday.Name(lang string) string` with fallback** | API returns `[]LocalizedText`; without a helper every caller writes the same 5-line scan. `holidays-rest` returns `map[lang]string` and exposes 14 languages — we match the ergonomics. | S | Fallback chain: requested → "EN" → first available → empty. Document the chain. |
| **`Holiday.IsInRegion(code string) bool`** | Filtering school holidays by województwo is the #1 use case; without this every caller writes the same loop. | S | Compares against `Subdivisions[].Code`; nationwide implies all-regions = true. |
| **`Holiday.Days() int` and `Holiday.Range() iter.Seq[time.Time]`** (Go 1.23 range-over-func) | Iterating dates of a ferie period is the second most common downstream operation. `iter.Seq` is the modern Go 1.23+ pattern; competing libraries pre-date it. | S | Trivial wrapper around `time.AddDate`. Verified: `iter.Seq` is in stdlib since Go 1.23 (Aug 2024), now mature. |
| **`iter.Seq[Holiday]` streaming wrapper that auto-batches a >3-year window into ≤3-year API calls** | Upstream caps queries at 3 years. Without this, every multi-year scheduling app writes the same chunking loop. `holidays-rest` has the same constraint but no helper. This is *the* "wow I didn't have to write that" feature. | M | Implement as `client.PublicHolidaysSeq(ctx, country, from, to) iter.Seq2[Holiday, error]`. Yields per-chunk errors so the caller can decide continue-vs-stop. |
| **Opt-in retry with exponential backoff + full jitter, `Retry-After`-aware, GET-only by default** | Increasingly table-stakes-adjacent (AWS SDK is on-by-default; Stripe SDKs do automatic retry on safe verbs). Since OpenHolidays is read-only, ALL requests are safely retryable. Honoring `Retry-After` is the difference between "retry library" and "polite client." | M | Default: 3 attempts, base 250 ms, max 5 s, full jitter. Honor `Retry-After` (seconds OR HTTP date). Bounded by `ctx`. Opt-in via `WithRetry(RetryConfig{...})` — see Anti-features for the on-by-default discussion. |
| **Opt-in in-memory TTL cache for reference endpoints** (`Countries`, `Languages`, `Subdivisions`) | These three return ~stable data; calling them on every request is wasteful. Hot path: < 5 ms when cached (per `PROJECT.md` perf target). Holiday endpoints are NOT cached by default — they have a temporal dimension and ferie dates can be re-published. | M | `sync.RWMutex`-protected `map[cacheKey]cacheEntry`. TTL via `time.AfterFunc` or lazy-expire on read. Opt-in via `WithCache(CacheConfig{TTL: 24*time.Hour})`. **No persistent cache in M1** — see Anti-features. |
| **Observability hook: `WithRequestHook(func(*http.Request, *http.Response, error))`** | Lets consumers wire metrics, tracing, audit logs WITHOUT the SDK depending on OpenTelemetry. Users who want OTel pass an `otelhttp.NewTransport`-wrapped `http.Client` via `WithHTTPClient` (canonical OTel pattern, verified against `otelhttp` docs). The hook covers the "I want to count 4xx by endpoint" use case. | S | Hook fires AFTER each attempt (so retries are visible). `err` may be non-nil even when `resp` is non-nil (e.g. decode error). Document the contract. See trade-off analysis below. |
| **Zero runtime dependencies** | Reduces supply-chain attack surface; `go get` stays fast; passes `govulncheck` on day one. `holidays-rest/sdk-go` pulls in its own deps; `aws-sdk-go-v2` is heavy. For a thin wrapper around a public API, "stdlib only" is a legit selling point. | S (constraint, not a feature to build) | Already declared in `PROJECT.md` constraints. Test-only deps (`go-cmp`) pre-approved. |
| **Fuzz tests for JSON parsers** | Demonstrates production-grade rigor on `pkg.go.dev`. Most competing wrappers don't fuzz. | S | `FuzzUnmarshalHoliday` covering the four optional/varying fields observed in the POC. |
| **Strict-decoding option to surface upstream schema drift** | A library that fails-loud when upstream adds a field is more useful for long-running services than one that silently swallows. Opposite of strict-by-default would break consumers; opt-in is the right shape. | S | Already listed in table stakes for completeness — it's a differentiator vs `holidays-rest`. |

**Trade-off analysis: observability hook vs direct OTel instrumentation**

Three approaches were considered:

1. **Hook with `func(*http.Request, *http.Response, error)`** ← chosen
   - **Pro:** zero runtime deps. Consumer can implement *any* observability backend (Prometheus, OTel, statsd, plain logs). Works with the SDK's "stdlib only" constraint.
   - **Pro:** the OTel-shaped consumer wraps `http.Client` with `otelhttp.NewTransport` (the canonical OTel pattern, verified against `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`) and gets spans + metrics for free — no SDK changes needed.
   - **Con:** consumer must write the OTel glue (~15 lines once).

2. **Emit OTel spans/metrics directly via `go.opentelemetry.io/otel`**
   - **Pro:** zero-glue for OTel users.
   - **Con:** adds a heavy runtime dep tree (`otel`, `otel/trace`, `otel/metric`) — violates the dependency policy. Forces non-OTel users to vendor unused code.
   - **Con:** OTel modules churn frequently; couples our release cadence to theirs (verified — OTel contrib has separate release cycles that produce build-error pain).

3. **No observability surface at all**
   - **Con:** any non-trivial production user wires this anyway; not having a hook means they monkey-patch `http.RoundTripper`. We may as well give them an ergonomic shape.

**Decision:** ship the hook (option 1). Document in `doc.go` that OTel users should pass an `otelhttp`-wrapped `*http.Client` via `WithHTTPClient` — give them a 5-line snippet. This is the same pattern the AWS SDK, Stripe, and `go-github` follow: they don't embed OTel; the user injects the instrumented transport.

---

### Anti-Features (Commonly Requested, Often Problematic)

These are explicitly NOT in M1. Each entry says why it's tempting, why it bites us, and what we do instead.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Retry on-by-default** | Modern SDKs (AWS, Stripe) retry by default; users expect the SDK to "just be reliable." | (a) Surprises users who wrap us in their own retry layer → double-retries → request amplification. (b) OpenHolidays has no observed rate-limit headers (per POC) — naïve retries could DoS the upstream we love. (c) v0.x library: cheaper to flip ON later than to flip OFF. | `WithRetry(DefaultRetryConfig)` is a one-liner. README quickstart shows it. Once we're confident, M2 flips default ON. |
| **Per-endpoint typed errors** (`*PublicHolidayNotFoundError`, `*SchoolHolidayValidationError`, …) | "Type-safe error checking per endpoint!" sounds rigorous. | Combinatorial explosion: 5 endpoints × ~6 status codes = 30 types most users never check. Verified pattern: `go-github` has ~4 typed errors total, partitioned by *category* (rate limit, accepted, two-factor, redirect) — NOT per endpoint. AWS uses one `smithy.APIError` + sentinels. | Single `*APIError{StatusCode, Path, Body}` for all HTTP failures + a small number of CATEGORY types if needed in M2 (e.g. `*NotFoundError` wrapping `*APIError`). Sentinels for validation. |
| **Persistent cache (file / SQLite / BoltDB)** | "What if I want the cache to survive restarts?" | Pulls in a storage dep, raises questions about cache-invalidation, file locking, multi-process safety. Out of scope for a thin REST wrapper. | M1: in-memory only. Document that consumers wanting persistence should layer their own (e.g. wrap `Client` with a Redis-backed cache). M2 may add a `Cache` interface for pluggable backends. |
| **Working-day arithmetic** (`IsWorkingDay`, `NextWorkingDay`, `AddBusinessDays`) | Scheduling apps want this. `rickar/cal/v2` provides it. | Broadens the public contract significantly: must decide weekend semantics per country, half-days, regional differences. Each one is a v1.0 commitment. | M3. M1 ships the data; M3 ships the arithmetic. Users can use `rickb777/date` or `rickar/cal` for arithmetic alongside us until then. |
| **Polish "observances" sub-package** (Dzień Matki, Dzień Dziecka, Dzień Ojca, Andrzejki, koniec roku szkolnego) | The POC docs explicitly note these are *missing* from OpenHolidays and from `holidays-rest`. Local users want them. | Not in upstream data → we'd have to hard-code static dates, becoming a holidays *authority*, not a holidays *client*. Different scope. Mother's Day in PL = 26 V is trivial but Andrzejki / koniec roku szkolnego are heuristic. | M3 `observances` sub-package, clearly labeled "data curated by us, not OpenHolidays." Stays out of v0.1.0. |
| **gRPC / GraphQL / iCal transports** | "What if I want gRPC?" | Upstream is REST/JSON. Translating to other transports = separate library concern. iCal *output* is a serialization, not a transport. | M4 may add iCal output (`Holiday.MarshalICal`) as a thin convenience. gRPC/GraphQL: never — different product. |
| **API-key / OAuth support** | Generic SDKs have it. | OpenHolidays is currently keyless. Adding speculative auth surface = wasted YAGNI work. | If upstream adds auth, we add `WithAPIKey(string)` or `WithAuth(http.RoundTripper)`. Until then: nothing. |
| **Multi-country aggregation** (`AllPublicHolidays(ctx, []countries, from, to)`) | "I have a multinational HR app, just give me everything." | Fan-out concurrency, error aggregation policy, partial-result semantics — all interesting design questions that bloat M1. The trivial implementation is `for c := range countries { client.PublicHolidays(...) }` which the caller can write. | Document the 6-line caller-side pattern in `example_test.go`. M2 considers `BatchPublicHolidays` if real users ask. |
| **Generated types from upstream OpenAPI spec** | "Why hand-write what we can codegen?" | Codegen brings churn we don't need pre-1.0: every upstream spec tweak shifts our generated API. Hand-written shapes let us evolve thoughtfully and shield consumers from cosmetic upstream changes. | M4. Until then: hand-written types from the POC-observed shapes. |
| **Generics-based response decoding** (`Get[T any](ctx, path) (T, error)`) | "Go 1.18 has generics, let's use them!" | For a 5-endpoint SDK with concrete return types, generics add cognitive cost without payoff. Stripe, go-github, AWS SDK v2 don't use generics for endpoint methods (verified). | Concrete methods returning concrete types. Internal `decode[T]` helper if useful, kept unexported. |
| **Builder API / fluent chaining** (`client.PublicHolidays().Country("PL").From(...).To(...).Do(ctx)`) | Java/JS-style fluent feels "modern." | Allocates an intermediate object, adds 3× the API surface, doesn't fit Go's "small interfaces, plain functions" tradition. None of stripe-go / go-github / aws-sdk-go-v2 use this for primary endpoints (AWS does for some inputs, in struct form not fluent). | Plain method with struct params (or positional for the 4-param case): `client.PublicHolidays(ctx, PublicHolidaysParams{Country: "PL", From: ..., To: ...})`. Idiomatic Go. |
| **Localization of error messages** | i18n nice-to-have. | Errors are for developers, not end users. English is the lingua franca of Go developer logs. | Errors stay English. Per `PROJECT.md` constraint. |
| **Shipping `cmd/ohcli` as a CLI binary** | Adoption signal; demos the lib; aligns with `PROJECT.md` Active requirements. | **NUANCED — keep it, but understand the trade-off.** Verified: stripe-go does NOT ship a CLI (stripe-cli is a separate repo). go-github does NOT ship a CLI. aws-sdk-go-v2 does NOT ship a CLI. The norm for serious Go SDKs is "library only, CLI is a separate project." However: this is a v0.x adoption-stage library targeting a hobby/utility niche; `cmd/ohcli` doubles as a usability smoke test, a fixture-generation tool, and a demo for the README. **Keep `cmd/ohcli`, but:** (a) keep it under 300 lines; (b) zero non-stdlib deps (no `cobra`/`urfave/cli` — just `flag`); (c) `goreleaser` to make the binary available; (d) clearly mark it as a demo, not a supported product. If it grows past M1, fork it into `go-openholidays-cli` like Stripe did. | Build it, keep it minimal, watch its weight. See "Feature Dependencies" — `cmd/ohcli` consumes the library, never the reverse. |

---

## Feature Dependencies

```
types (Holiday, Subdivision, Country, Language, LocalizedText, SubdivisionRef)
    │
    ├── transport (http.Client + Accept/UA headers + LimitReader + json.Decode)
    │       │
    │       ├── retry middleware (opt-in, wraps transport, honors Retry-After + ctx)
    │       │       └── request hook (fires per attempt, AFTER retry decision)
    │       │
    │       └── cache layer (opt-in, wraps transport for ref endpoints only)
    │
    ├── endpoint methods (Countries, Languages, Subdivisions, PublicHolidays, SchoolHolidays)
    │       │
    │       ├── client-side validation (country code, date window, validFrom<=validTo)
    │       │       └── sentinel errors (ErrInvalidCountry, ErrDateRangeTooLarge, ...)
    │       │
    │       └── *APIError wrapping (status code, path, body)
    │
    ├── helper methods (Name(lang), IsInRegion(code), Days(), Range() iter.Seq[time.Time])
    │       └── depends on: types only
    │
    ├── auto-batching iterator (PublicHolidaysSeq, SchoolHolidaysSeq)
    │       └── depends on: endpoint methods + Go 1.23 iter.Seq2
    │
    ├── strict-decoding mode
    │       └── depends on: transport (toggle json.Decoder.DisallowUnknownFields)
    │
    └── cmd/ohcli  ──consumes──>  endpoint methods + helper methods
                                  (one-way; lib never depends on CLI)

Cross-cutting:
    slog.Default() logger ──used by──> transport, retry, cache  (Debug level only)
    context.Context       ──flows through──> EVERYTHING

External (M2+):
    persistent cache  ──would-extend──> cache layer (needs a Cache interface in M1 to make this possible later)
    working-day arithmetic  ──would-extend──> helper methods
    observances sub-package ──would-coexist-with──> endpoint methods (separate package, separate data)
    iCal output  ──would-extend──> types (MarshalICal method)
```

### Dependency Notes

- **Types come before everything.** They're the public contract; nothing else can be finalized until they are. Build them first in M1.
- **Transport before endpoints.** Endpoints are 5 thin methods over one shared transport function — the POC `c.get(ctx, path, q, &out)` shape is the right shape. Don't duplicate per-endpoint.
- **Retry wraps transport, not endpoints.** This is critical: retrying at the transport layer keeps endpoint methods clean (no retry loops sprinkled around) AND ensures every future endpoint benefits automatically.
- **Cache also wraps transport.** Same reasoning. The cache is reference-endpoint-only logic that lives at the transport layer with a path-allowlist.
- **The hook fires per attempt, after retry decides.** This means observability sees the full retry tape — including retries that succeeded. Document this explicitly.
- **`cmd/ohcli` is a leaf.** It depends on the library; nothing in the library depends on it. Easy to delete / extract / fork later. Lives in `cmd/ohcli/main.go` with its own minimal `flag` parsing.
- **Auto-batching iterator depends on endpoint methods.** Build endpoint methods first, then the iterator on top. The iterator is the only place Go 1.23 `iter.Seq2` is used for streaming — `Holiday.Range()` uses `iter.Seq[time.Time]` independently.
- **Cache interface, not just cache struct.** Even though M1 ships only in-memory, expose a `type Cache interface { Get(key) ([]byte, bool); Set(key, val, ttl) }` shape so M2/M3 can plug in persistent backends without breaking v0.x consumers. Cost: ~10 extra lines in M1; saves a breaking change later.
- **Conflict to watch:** strict-decoding + caching. If strict mode is on AND a cached entry was decoded under loose mode, behavior could differ. Cache keys must include the decoder config (or simpler: cache the raw bytes, decode on read). Recommend caching raw bytes.

---

## MVP Definition

### Launch With (v0.1.0 / M1)

The minimum bar to publish a library that "looks serious" on `pkg.go.dev` and answers the Core Value question.

- [ ] **Types** — `Holiday`, `Subdivision`, `Country`, `Language`, `LocalizedText`, `SubdivisionRef` with custom `UnmarshalJSON` for dates. *Essential: defines the public contract.*
- [ ] **`NewClient` + functional options** — `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithLogger`, `WithTimeout`, `WithRetry`, `WithCache`, `WithRequestHook`, `WithStrictDecoding`. *Essential: how everyone constructs the client.*
- [ ] **5 endpoint methods** — `Countries`, `Languages`, `Subdivisions`, `PublicHolidays`, `SchoolHolidays`, all `ctx`-first. *Essential: this is the product.*
- [ ] **Client-side validation + sentinel errors** — country code, date window, validFrom<=validTo. *Essential: saves users a network round-trip and a debugging session.*
- [ ] **`*APIError` + `errors.Is`/`errors.As` support** — unified error type for all HTTP failures. *Essential: idiomatic Go error handling.*
- [ ] **Opt-in retry with backoff + jitter + `Retry-After`** — `WithRetry(RetryConfig{...})`. *Essential: any production user expects to opt into this in one line.*
- [ ] **Opt-in in-memory TTL cache for reference endpoints** — `Countries`, `Languages`, `Subdivisions` only. Hidden behind a `Cache` interface for M2 pluggability. *Essential: hot-path perf target is < 5 ms cached, otherwise we miss the perf NFR.*
- [ ] **Helper methods on `Holiday`** — `Name(lang)`, `IsInRegion(code)`, `Days()`, `Range() iter.Seq[time.Time]`. *Essential: differentiates us from passing through raw API shapes.*
- [ ] **Observability hook** — `WithRequestHook(func(*http.Request, *http.Response, error))`. *Essential: production users will plug in metrics; without a hook they'll monkey-patch `http.RoundTripper`.*
- [ ] **Strict-decoding mode** — opt-in, off by default. *Essential: schema-drift defense given POC-observed optional fields.*
- [ ] **`cmd/ohcli` demo CLI** — minimal, stdlib-only, two commands. *Essential per `PROJECT.md`; functions as smoke test and adoption demo. Keep ≤ 300 lines.*
- [ ] **Tests** — unit (httptest, table-driven, golden JSON), integration (build-tag + env-gate, nightly CI), fuzz on JSON parsers, benchmarks for hot paths, `-race` clean. *Essential: quality bar declared in `PROJECT.md`.*
- [ ] **CI** — GH Actions matrix (Go 1.22, 1.23, stable) running `vet`, `build`, `test -race -cover`, `golangci-lint`, `govulncheck`. *Essential: signals to first-time readers that we're serious.*
- [ ] **Release pipeline** — `goreleaser` on `v*` tags producing CLI binaries for linux/darwin/windows × amd64/arm64. *Essential per `PROJECT.md`.*
- [ ] **Docs** — README ≤ 20 lines quickstart, `doc.go`, `example_test.go` per public method, `docs/design.md`, `CHANGELOG.md`, `CONTRIBUTING.md`. *Essential: `pkg.go.dev` Grade A.*
- [ ] **`v0.1.0` tag** — published to `pkg.go.dev`. *Essential: M1 exit criterion.*

### Add After Validation (v0.2 – v0.5 / M2–M3)

Triggered by: real-user feedback after v0.1.0 lands.

- [ ] **Auto-batching iterator** `PublicHolidaysSeq` / `SchoolHolidaysSeq` returning `iter.Seq2[Holiday, error]` for >3-year windows. *Trigger: any user asking "how do I get 5 years in one call?"*
- [ ] **Retry on-by-default** — flip the default once we've watched behavior in the wild. *Trigger: zero reports of retry-amplification problems after 2–3 months.*
- [ ] **Persistent cache via `Cache` interface plug-in** — provide an example Redis adapter under `cache/redis` (its own go.mod with its own deps to keep root clean). *Trigger: user asking for cache-across-restart.*
- [ ] **Working-day arithmetic** — `IsWorkingDay`, `NextWorkingDay`, `AddBusinessDays`. *Trigger: scheduling-app user requesting it. M3.*
- [ ] **Polish observances sub-package** — `pkg observances/pl` with curated static dates (Dzień Matki etc.) clearly labeled as our data, not OpenHolidays. *Trigger: Polish user requesting it. M3.*
- [ ] **Category typed errors** — `*NotFoundError`, `*RateLimitError` wrapping `*APIError`, IF upstream behavior shows the categories are usefully distinct. *Trigger: real catches of `errors.As(&apiErr)` followed by `if apiErr.StatusCode == 404`.*

### Future Consideration (v1.0+ / M4–M5)

Triggered by: maturity, scale, or third-party demand. Not by "wouldn't it be nice."

- [ ] **Generated types from upstream OpenAPI spec** — adds churn we don't need pre-1.0. *Defer until: API is stable AND we have ≥ 1 contributor familiar with codegen workflows. M4.*
- [ ] **iCal output** — `Holiday.MarshalICal() ([]byte, error)`. *Defer until: a user shows up with the use case. M4.*
- [ ] **Multi-country aggregation helpers** — `BatchPublicHolidays(ctx, []countries, ...) iter.Seq2[Holiday, error]`. *Defer until: HR/multinational user appears. M2 at earliest.*
- [ ] **Pluggable retry strategies** — `Retryer` interface (cf. `aws-sdk-go-v2`). *Defer until: someone needs anything beyond exponential-jitter.*

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Types + custom date unmarshal | HIGH | LOW | **P1** |
| `NewClient` + functional options | HIGH | LOW | **P1** |
| 5 endpoint methods | HIGH | LOW (POC done) | **P1** |
| `context.Context` first arg | HIGH | LOW (free with stdlib) | **P1** |
| Client-side validation + sentinels | HIGH | LOW | **P1** |
| `*APIError` + `errors.Is`/`As` | HIGH | LOW | **P1** |
| Error wrapping with `%w` | HIGH | LOW | **P1** |
| `Accept` + `User-Agent` headers | MEDIUM | LOW | **P1** |
| 15 s default timeout | HIGH | LOW | **P1** |
| `io.LimitReader` body cap | MEDIUM | LOW | **P1** |
| Goroutine-safe `Client` | HIGH | LOW (just don't break it) | **P1** |
| `slog`-based logging | MEDIUM | LOW | **P1** |
| Helper methods (`Name`, `IsInRegion`, `Days`, `Range`) | HIGH | LOW | **P1** |
| Opt-in retry w/ jitter + `Retry-After` | HIGH | MEDIUM | **P1** |
| Opt-in in-memory cache (ref endpoints) | MEDIUM | MEDIUM | **P1** |
| `Cache` interface for M2 pluggability | MEDIUM | LOW (10 lines extra) | **P1** |
| Observability hook | HIGH | LOW | **P1** |
| Strict-decoding mode | MEDIUM | LOW | **P1** |
| `cmd/ohcli` demo CLI | MEDIUM | MEDIUM | **P1** (per PROJECT.md) |
| Tests + fuzz + benchmarks | HIGH | MEDIUM | **P1** |
| CI matrix + release pipeline | HIGH | MEDIUM | **P1** |
| Docs (README, doc.go, examples) | HIGH | MEDIUM | **P1** |
| **Auto-batching `iter.Seq[Holiday]` for >3yr** | HIGH | MEDIUM | **P2** |
| **Retry on-by-default** | MEDIUM | LOW (flag flip) | **P2** |
| Persistent cache adapter | LOW | HIGH | **P2** |
| Category typed errors (`NotFoundError`) | LOW | LOW | **P2** |
| Working-day arithmetic | MEDIUM | HIGH | **P3** |
| Polish observances sub-package | MEDIUM | MEDIUM | **P3** |
| Generated types from OpenAPI | LOW | HIGH | **P3** |
| iCal output | LOW | MEDIUM | **P3** |
| Multi-country aggregation | LOW | MEDIUM | **P3** |

**Priority key:**
- **P1** — must ship in v0.1.0 (M1)
- **P2** — add in v0.2 – v0.5 (M2 / early M3) based on user feedback
- **P3** — v1.0+ (M3–M5); reconsider only when validated demand exists

---

## Competitor Feature Analysis

| Feature | `holidays-rest/sdk-go` | `rickar/cal/v2/pl` | `stripe-go` | `google/go-github` | `aws-sdk-go-v2` | **`go-openholidays` (our plan)** |
|---------|-------------------------|---------------------|-------------|--------------------|------------------|-----------------------------------|
| Functional options on `NewClient` | Yes (`WithBaseURL` etc.) | N/A (offline lib) | Yes (via Client config) | Yes | Yes | **Yes — same shape** |
| `context.Context` first arg | Yes | N/A | Yes (v82+) | Yes | Yes | **Yes** |
| Typed errors via `errors.As` | Partial | N/A | Yes (`*stripe.Error`) | Yes (`*RateLimitError` etc.) | Yes (`smithy.APIError`) | **Yes — unified `*APIError` + sentinels** |
| Per-endpoint typed errors | No | N/A | No (one Error type) | Category-level only | Category-level (sentinels) | **Category-level in M2 if needed** |
| Retry on-by-default | No (paid SDK) | N/A | Effectively yes | No | **Yes** (Standard retryer) | **Opt-in M1; on-by-default M2** |
| Honor `Retry-After` | Unknown | N/A | Yes | Yes (`RateLimitError.Rate.Reset`) | Yes | **Yes** |
| In-memory cache | No | N/A (data IS the cache) | No | No | No | **Yes — opt-in for ref endpoints (differentiator)** |
| Auto-batching for windowed queries | No | N/A | N/A | No (uses pagination) | N/A | **Yes (M2) — differentiator** |
| Pagination support | N/A | N/A | Yes (iterators) | Yes (manual) | Yes (paginators) | **N/A — OpenHolidays returns full arrays (3yr cap)** |
| `iter.Seq` / range-over-func | No | No | No (predates 1.23) | No | No | **Yes for `Holiday.Range()` and auto-batch (differentiator)** |
| OTel direct dependency | No | No | No | No | No | **No — hook + user injects `otelhttp.NewTransport`** |
| Request/response hook | Unknown | N/A | Yes (`AppendTo` middleware) | No (use `Transport`) | Yes (middleware stack) | **Yes — single `WithRequestHook`** |
| Strict-decoding mode | No | N/A | No | No | No | **Yes — opt-in (differentiator)** |
| Localized name helpers | Partial (`Name["pl"]`) | No | N/A | N/A | N/A | **Yes — `Holiday.Name(lang)` with fallback chain** |
| Regional / subdivision filter helper | Yes (filter param) | No | N/A | N/A | N/A | **Yes — `Holiday.IsInRegion(code)` + populated `Subdivisions`** |
| Ships a CLI | No | No | **No** (separate `stripe-cli` repo) | No | No | **Yes** — `cmd/ohcli` (small, stdlib-only) |
| Zero runtime deps | No (~3 deps) | Yes (stdlib only) | Yes-ish (small deps) | Few | Many | **Yes — stdlib only** |
| Fuzz tests | Unknown | No | Partial | Partial | Yes | **Yes — JSON parser fuzz** |
| `pkg.go.dev` Grade A target | — | A | A | A | A | **A (M1 exit criterion)** |

**Where we beat the field:**

1. **Only Go SDK** with subdivision-granular school holidays (the killer differentiator vs `rickar/cal`).
2. **Only SDK in this comparison** using Go 1.23 `iter.Seq` natively (vs all competitors predating or ignoring it).
3. **Zero runtime deps + stricter decode + opt-in cache** — matches `rickar/cal`'s lean profile while delivering live API access.
4. **Auto-batching window iterator** — addresses a real upstream constraint (3-year cap) that `holidays-rest` shares but doesn't solve.

**Where we deliberately match (not exceed):**

- Functional options, `ctx` first, typed errors, retry with backoff — these are table-stakes; matching the leaders means we're not penalized.
- Observability via hook + injected transport — exactly the pattern stripe-go and go-github use; safer than adding the OTel dep.

**Where we deliberately do less:**

- No multi-language (we have `Name(lang)` with fallback — that's enough for a v0.x; `holidays-rest` advertising 14 languages is a vanity metric for our scope).
- No pagination iterators (because the API has none; absence is correct).
- No multi-format output in M1 (JSON only; iCal in M4).
- No CLI sub-commands beyond two (vs `stripe-cli` which is a 50-file project).

---

## Sources

- [stripe/stripe-go on GitHub](https://github.com/stripe/stripe-go) — verified functional options pattern + `ctx`-first signature in current Client.
- [stripe-go v74 on pkg.go.dev](https://pkg.go.dev/github.com/stripe/stripe-go/v74) — confirmed CLI is NOT shipped from `stripe-go` (separate `stripe-cli` repo).
- [google/go-github on GitHub](https://github.com/google/go-github) — verified typed errors (`*RateLimitError`, `*AcceptedError`, `*TwoFactorAuthError`) and `errors.As` usage pattern. Category-level, not per-endpoint.
- [aws-sdk-go-v2 retry package](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/aws/retry) — verified default Standard retryer, exponential backoff with full jitter, max 20 s cap. Retry IS on-by-default in AWS.
- [AWS SDK Retries and Timeouts docs](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-retries-timeouts.html) — confirmed exponential-jitter formula and configurability.
- [Stripe API idempotency docs](https://docs.stripe.com/api/idempotent_requests) — confirmed GET/DELETE always-safe-to-retry rationale that informs our opt-in-but-safe default.
- [opentelemetry-go contrib otelhttp](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) — verified `NewTransport` wrapping pattern, which is the canonical OTel integration without requiring the SDK to depend on OTel.
- [Range Over Function Types (Go blog)](https://go.dev/blog/range-functions) — confirmed `iter.Seq` / `iter.Seq2` types in Go 1.23 stdlib.
- [Go 1.23 iterators tutorial (TutorialEdge)](https://tutorialedge.net/golang/go-123-iterators-tutorial/) — adoption examples and `iter` package shape.
- [slack-go/slack on pkg.go.dev](https://pkg.go.dev/github.com/slack-go/slack) — verified options pattern and HTTP client injection (`OptionHTTPClient`) without baked-in caching.
- [Eli Bendersky on Go 1.23 range-over-func](https://eli.thegreenplace.net/2024/ranging-over-functions-in-go-123/) — secondary confirmation of `iter.Seq` semantics.
- `.planning/PROJECT.md` (this repo) — Active requirements, constraints, Key Decisions.
- `./main.go` (POC #1) — `holidays-rest` shape, `rickb777/date` coverage gap.
- `./openholidays/main.go` (POC #2) — live API shape, observed optional fields (`comment`, `quality`, `subdivisions`), 14 PL public holidays + 7 school-holiday periods + 16 województwa.

---
*Feature research for: Go HTTP/JSON SDK library — `go-openholidays` v0.1.0*
*Researched: 2026-05-26*
