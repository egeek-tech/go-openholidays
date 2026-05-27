# Phase 2: Transport - Research

**Researched:** 2026-05-27
**Domain:** Go stdlib HTTP client transport — `*http.Client`, `RoundTripper` chain, `context` propagation, `io.LimitReader`-bounded JSON decode, `slog` Debug-level structured logging, the first end-to-end endpoint (`Countries`)
**Confidence:** HIGH (CONTEXT.md locks 27 implementation decisions D-24..D-50; nearly every pattern below is corroborated by Go stdlib docs, ARCHITECTURE.md Patterns 1-2-6, PITFALLS.md HTTP-1..4 + OBS-1, and live verification against `openholidaysapi.org/Countries` on 2026-05-27)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**A1 — Oversized-body error (CL-07)**

- **D-24:** A new exported sentinel `ErrResponseTooLarge` is added to `errors.go`. Phase 2's transport returns this wrapped via `%w` (with the actual byte count in the message) when `io.LimitReader` truncates a response above 10 MiB. Callers branch via `errors.Is(err, openholidays.ErrResponseTooLarge)`. This expands the public sentinel surface from 5 to 6 — must be recorded as **CL-07** in PROJECT.md `Key Decisions` before Phase 2 closes.
- **D-25:** The 10 MiB ceiling lives as an unexported `const maxResponseBytes = 10 << 20` in `transport.go`. Not configurable in v0.1.0.

**A2 — Timeout enforcement**

- **D-26:** `WithTimeout(d time.Duration) Option` sets `cfg.timeout` only; `cfg.httpClient.Timeout` is left at the caller's value (default 0 = no Go-level timer).
- **D-27:** Every endpoint method wraps the incoming `ctx` with `ctx, cancel := context.WithTimeout(ctx, c.timeout); defer cancel()` immediately after the nil-ctx guard. Caller-supplied tighter deadlines compose naturally (the shorter of caller's ctx and `c.timeout` wins).
- **D-28:** Default `cfg.timeout = 15 * time.Second`. `WithTimeout(0)` is interpreted as "no SDK-imposed timeout".

**A3 — Transport scaffold scope (minimal)**

- **D-29:** `buildTransport(cfg *clientConfig) http.RoundTripper` returns exactly:
  ```
  req → headerTransport{next: loggingTransport{next: underlying}} → underlying
  ```
  Where `underlying = cfg.httpClient.Transport` if non-nil else `http.DefaultTransport`. No retry, cache, hook in Phase 2.
- **D-30:** `headerTransport.RoundTrip` clones `req` via `req.Clone(req.Context())` before mutating headers. Sets `Accept: application/json` and `User-Agent: go-openholidays/<Version>` only when caller hasn't already supplied them (caller override wins).
- **D-31:** `loggingTransport.RoundTrip` wraps the round-trip in `start := time.Now()` and emits a single `slog.LogAttrs` Debug record with `method, path, status, duration_ms, attempt, bytes_in`. `attempt` is hardcoded to `1`. `bytes_in` is `resp.ContentLength` when known (≥ 0), else `-1`. Response body is **never** read inside `loggingTransport`.

**A4 — Phase 1 W-01 fix folded into Phase 2**

- **D-32:** A dedicated plan in Phase 2 (sequenced after the transport plans) fixes `validate.go`'s Unicode case-fold bypass:
  - Reorder: ASCII-shape check runs **before** `strings.ToUpper`/`strings.ToLower` canonicalization.
  - Shape predicate uses byte-level checks (no `unicode` package).
  - Extend `validate_test.go` with regression cases: `"KK"` (U+212A x2), `"İ"`+1 char, `"ı"`+1 char, `"ſ"`+1 char.
- **D-33:** No CL row for the W-01 fix — defect fix against the locked VALID-01/04 contract.
- **D-34:** Does NOT touch W-02, W-03, W-04 follow-ups.

**Client surface, defaults, and lifecycle**

- **D-35:** `type Option func(*clientConfig)`. `NewClient` applies them to a fresh `clientConfig`, then constructs an immutable `Client`.
- **D-36:** `defaultBaseURL = "https://openholidaysapi.org"`. No environment-variable override.
- **D-37:** `WithHTTPClient(c *http.Client) Option` performs a shallow copy of `*c` inside `composeHTTPClient(cfg)`.
- **D-38:** `WithUserAgent(s string)` overrides the default. Empty string treated as "use default" (no-op).
- **D-39:** `WithLogger(l *slog.Logger)` defaults to `slog.Default()` when not supplied or nil. Library never calls `slog.SetDefault`.
- **D-40:** `Client.Close() error` flips a single `closed atomic.Bool` and returns `nil` always. Phase 4's cache will hook a `sync.Once` + sweeper-goroutine cancel onto this.

**Countries endpoint**

- **D-41:** `(c *Client) Countries(ctx context.Context) ([]Country, error)`. Internal HTTP request `GET <baseURL>/Countries` with no query parameters.
- **D-42:** Error handling order: nil-ctx guard → `context.WithTimeout` wrap → `http.NewRequestWithContext` → `c.http.Do` → defer-drain-and-close → status-code check → JSON decode.
- **D-43:** On `resp.StatusCode >= 400` construct `*APIError{StatusCode, Path: "/Countries", Body, Message}` where `Body` is the truncated upstream body (capped at 4 KiB per Phase 1 D-17) and `Message` is best-effort parsed from upstream `{"error": ...}` / `{"detail": ...}` / `{"title": ...}` shapes.
- **D-44:** On `2xx` use `json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&countries)`. Lenient default per Phase 1 JSON-1 decision.
- **D-45:** Drain-and-close: `defer func() { io.Copy(io.Discard, resp.Body); resp.Body.Close() }()` placed immediately after the `c.http.Do(req)` call returns no error.

**Test architecture**

- **D-46:** All Phase 2 HTTP tests use `httptest.NewServer`. A 2-country `testdata/countries.json` fixture (PL + DE) backs `TestClient_Countries` happy-path.
- **D-47:** `TestClient_ConcurrentAccess` (TEST-04) runs 50 parallel `Countries` calls against a `httptest.Server` that delays 5-20ms. Race detector active.
- **D-48:** `TestClient_ContextCancel` (CLIENT-09) creates a server that hangs 10s, then calls `cancel()` 50ms in, asserts `errors.Is(err, context.Canceled)` and total elapsed < 200 ms.
- **D-49:** `TestClient_OversizedResponse` (TRANS-02 + D-24) starts a server that streams 11 MiB of JSON, asserts `errors.Is(err, ErrResponseTooLarge)`. Verifies body drain via `runtime.NumGoroutine` delta (not goleak — avoid test dep).
- **D-50:** Per-RoundTripper unit tests: `transport_header_test.go` and `transport_logging_test.go` use a tiny `roundTripperFunc` adapter for the `next` slot.

### Claude's Discretion

- File layout: `client.go` (Client struct + NewClient + Close), `options.go` (Option + WithX), `config.go` (clientConfig + composeHTTPClient + buildTransport), `transport.go` (headerTransport + loggingTransport), `countries.go` (Countries endpoint method).
- Test files: `client_test.go`, `options_test.go`, `transport_header_test.go`, `transport_logging_test.go`, `countries_test.go`, plus existing `_test.go` files extended where needed.
- Every exported symbol gets a godoc starting with the symbol name.
- Error message strings start with `"openholidays: "`.
- All Go tests use `testify` (assert + require) with one `TestXxx` per exported prod function and every case wrapped in `t.Run` (Gold Rule 3).

### Deferred Ideas (OUT OF SCOPE)

- **Environment-driven base URL override** (e.g., `OPENHOLIDAYS_BASE_URL`) — explicitly rejected; callers use `WithBaseURL`.
- **`WithMiddleware(mw Middleware) Option`** for pluggable transports — deferred indefinitely.
- **Configurable 10 MiB cap via option** — Out of scope; PROJECT.md fixes the cap.
- **Phase 1 follow-ups W-02 / W-03 / W-04** — remain on the follow_ups list.
- **`runtime.NumGoroutine` vs `go.uber.org/goleak` for D-49** — picks runtime-delta; revisit if leaks become recurring.
- **Logging body preview at Debug** — not in Phase 2. Phase 4 may add `body_preview` if useful.
- **`WithRequestHook` (TRANS-05)** — Lands in Phase 4.
- **`WithStrictDecoding(bool)` (OBS-03)** — Lands in Phase 4.
- **Retry semantics (RESIL-*)** — Phase 3+ territory. Phase 2 leaves `attempt: 1` hardcoded.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CLIENT-01 | `NewClient(opts ...Option) *Client` constructs a client; never returns an error | Functional Options pattern (ARCHITECTURE.md Pattern 1) — Standard Stack `client.go` + `options.go` + `config.go` triad. |
| CLIENT-02 | `WithHTTPClient(*http.Client)` shallow-copies caller's client and wraps Transport | Pitfall HTTP-1 (`.planning/research/PITFALLS.md`) drives the shallow-copy in `composeHTTPClient`. Test pattern at "Test pattern: shallow-copy isolation". |
| CLIENT-03 | `WithBaseURL(string)` overrides default base URL | Trivial Option; verify by httptest URL injection. Live default `https://openholidaysapi.org` confirmed 200 OK on 2026-05-27. |
| CLIENT-04 | `WithUserAgent(string)` overrides default `go-openholidays/<version>` | D-30/D-38 — empty string = no-op (CDN reject mitigation per Pitfall HTTP-5). |
| CLIENT-05 | `WithLogger(*slog.Logger)` injects structured logger; defaults to `slog.Default()` | D-39. `slog.Default()` is the Go 1.21+ stdlib answer; never call `slog.SetDefault` from library code. |
| CLIENT-06 | `WithTimeout(time.Duration)` sets per-request timeout (default 15s) | D-26..D-28 — applied as `context.WithTimeout` inside endpoint methods, NOT as `httpClient.Timeout` (avoids the body-close race in golang/go#49521). |
| CLIENT-07 | `Client` is goroutine-safe — verified by `TestClient_ConcurrentAccess` under `-race` | D-47 + Pitfall CONC-1 — Client is immutable after construction; no mutable fields. |
| CLIENT-08 | `Client.Close() error` is idempotent stub | D-40 — `atomic.Bool` flip + return nil. Phase 4 will hook the cache sweeper. |
| CLIENT-09 | Context cancellation interrupts in-flight HTTP within ≤ 100 ms | D-48 — measured via httptest server that hangs, cancel after 50 ms, assert elapsed < 200 ms (2x slack for CI). Pattern: "Context cancellation test pattern". |
| ENDPT-01 | `Countries(ctx) ([]Country, error)` fetches the supported-countries list | D-41..D-45. Live response shape verified: 36 countries, 6055 bytes, `application/json; charset=utf-8`. |
| TRANS-01 | All requests include `Accept: application/json` and `User-Agent: go-openholidays/<version>` | D-30 — `headerTransport` injects via `req.Clone(req.Context())` (canonical RoundTripper idiom, see "RoundTripper request mutation"). |
| TRANS-02 | All response bodies are read through `io.LimitReader` capped at 10 MiB; oversized responses return a typed error | D-25/D-49 + Pitfall HTTP-4 — the limit+1 sentinel-byte technique detects truncation. New `ErrResponseTooLarge` sentinel. |
| TRANS-03 | Response bodies are always drained then closed via `defer`, including on early returns and parse errors | D-45 + Pitfall HTTP-3 — `defer func() { io.Copy(io.Discard, resp.Body); resp.Body.Close() }()`. Drain-before-close is required for HTTP/1.1 keep-alive reuse. |
| TRANS-04 | Custom RoundTripper chain composes header injection, logging; each RoundTripper independently unit-tested | D-29/D-50 — two transports in Phase 2 (header + logging); each tested via `roundTripperFunc` adapter. |
| OBS-01 | HTTP requests/responses logged at `slog.LevelDebug` only; response body never logged | D-31 + Pitfall OBS-1 — `loggingTransport` never reads body. |
| OBS-02 | Structured fields: `method`, `path`, `status`, `duration_ms`, `attempt`, `bytes_in` | D-31 — exact field names, `attempt` hardcoded to 1, `bytes_in = resp.ContentLength` (which is `-1` for HTTP/2 chunked responses — verified live). |
| TEST-04 | `TestClient_ConcurrentAccess` runs N parallel requests under `-race` | D-47 — 50 goroutines, fan-in via `errgroup.Wait`-free `sync.WaitGroup`; assert all 50 return identical payloads. |
</phase_requirements>

## Summary

Phase 2 stands up a small, idiomatic stdlib-only HTTP transport that proves the entire `NewClient → buildTransport → Countries → decode → typed return` pipeline against an `httptest.Server`. Every architectural choice is locked in CONTEXT.md (D-24..D-50). The research task here is therefore narrow: **verify** the assumed patterns against current Go stdlib documentation and against the live OpenHolidays API; surface any drift, gap, or implementation gotcha; and supply concrete code shapes for the planner to slice into tasks.

Live probe of `https://openholidaysapi.org/Countries` (2026-05-27): returns HTTP/2 200, `Content-Type: application/json; charset=utf-8`, 6055 bytes, 36 countries. Cold response in ~89 ms over the public internet — comfortably under the 500 ms cold-call target. Crucially the response is HTTP/2 chunked: **`Content-Length` header is absent**, so `resp.ContentLength == -1` in `loggingTransport` — D-31's "`-1` when unknown" semantics is the correct default. Error responses use RFC 7807 `application/problem+json` with fields `{type, title, status, detail, instance}` — **the `Message` extractor in D-43 must check `detail` first, then `title`** (the literal `{"error": "..."}` shape D-43 lists does NOT appear in OpenHolidays responses; document both for future-proofing but `detail` is the live winner).

**Primary recommendation:** Implement the file layout from Claude's Discretion verbatim. Write the W-01 fix plan last in the sequence. Capture `testdata/countries.json` as a small 2-country slice (PL + DE) of the live response captured during this phase — both are present, both are upper-case ISO codes, both ship 3-language `Name` arrays. Use `req.Clone(req.Context())` in `headerTransport`. Detect oversize with the limit+1 sentinel-byte read after a successful `Decode`. For D-49 goroutine-leak guard, `runtime.NumGoroutine()` delta with a 100 ms settle pause is acceptable for Phase 2's scope (single oversize test) — promote to `go.uber.org/goleak` only when Phase 4's sweeper-goroutine work surfaces real leaks.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| HTTP request construction | Endpoint method (`countries.go`) | — | Endpoint owns URL assembly, headers list, `http.NewRequestWithContext`. RoundTripper layer must NOT know which endpoint a request belongs to (Pattern 6 from ARCHITECTURE.md). |
| Header injection (`Accept`, `User-Agent`) | RoundTripper (`headerTransport`) | — | Cross-cutting concern; uniform across every endpoint that ever ships. Belongs in the chain per Pattern 2. |
| HTTP-level logging (method, status, duration) | RoundTripper (`loggingTransport`) | — | Same reasoning; the chain is the only place to see every request uniformly. |
| Context propagation / cancellation | Endpoint method (`context.WithTimeout`) → `http.NewRequestWithContext` → chain → base `Transport` | — | Endpoint owns the per-request timeout; the chain is ctx-transparent. Base `http.Transport` honors `req.Context()` cancellation via `Transport.CancelRequest` (deprecated form) / `http2Transport` directly. |
| Response decoding + 10 MiB cap | Endpoint method (`countries.go`) | — | Decoder semantics (strict-vs-lenient, body cap) belong with the endpoint — the chain should not consume the body (Pitfall OBS-1 + D-31 explicit). |
| 4xx/5xx → `*APIError` construction | Endpoint method (`countries.go`) | — | RoundTripper layer is HTTP-pure; constructing a domain error requires knowing the endpoint Path (`/Countries`). Per ARCHITECTURE.md Pattern 6. |
| `*http.Client` lifecycle | `clientConfig` builder + `composeHTTPClient(cfg)` | — | Shallow-copy of caller's `*http.Client` (D-37); chain assembled in `buildTransport` (D-29). Avoids hidden mutability (Pitfall HTTP-1). |
| Validation (W-01 fix) | Unexported `validate.go` (existing Phase 1 file) | — | The W-01 fix plan operates entirely within `validate.go` + `validate_test.go`. Does NOT touch transport code. |
| `Client.Close()` lifecycle | `client.go` Client method | — | Atomic flag flip in Phase 2; Phase 4 will hook cache-sweeper cancel. |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `net/http` | stdlib (Go 1.23+) | HTTP client (Client, Transport, RoundTripper, Request, Response) | Canonical Go HTTP. Zero-dep mandate (PROJECT.md) and STACK.md HIGH confidence. `req.Clone(ctx)` is the canonical pattern for RoundTripper request mutation. [VERIFIED: pkg.go.dev/net/http] |
| `net/http/httptest` | stdlib | In-process test server | `httptest.NewServer` returns a server bound to a random port. Use `t.Cleanup(srv.Close)` (Pitfall TEST-2). [VERIFIED: pkg.go.dev/net/http/httptest] |
| `context` | stdlib | Cancellation & deadlines | `context.WithTimeout` + `http.NewRequestWithContext` is the locked pattern (D-27). Never use deprecated `http.NewRequest` + `req.WithContext`. [VERIFIED: pkg.go.dev/context] |
| `encoding/json` | stdlib (v1) | JSON decoding | `json.NewDecoder(io.LimitReader(...)).Decode(&v)` is the bounded-decode form. Strict decoding deferred to Phase 4. [VERIFIED: pkg.go.dev/encoding/json] |
| `log/slog` | stdlib (Go 1.21+) | Structured logging | OBS-02 fields emitted via `logger.LogAttrs(ctx, slog.LevelDebug, ...)` — preferred over `logger.Debug(...)` for hot paths since it avoids the variadic-allocations cost. [VERIFIED: pkg.go.dev/log/slog] |
| `io` | stdlib | `io.LimitReader`, `io.Discard`, `io.Copy` | All three are stdlib essentials for the bounded-decode + drain-and-close pattern. [VERIFIED: pkg.go.dev/io] |
| `sync/atomic` | stdlib | `atomic.Bool` for `Client.closed` flag (D-40) | `atomic.Bool` (Go 1.19+) is the modern typed wrapper around `atomic.{Load,Store}Uint32`. [VERIFIED: pkg.go.dev/sync/atomic] |
| `time` | stdlib | Per-request timeout + duration measurement in `loggingTransport` | `time.Since(start)` returns `time.Duration`; convert via `int64(d / time.Millisecond)` for `duration_ms`. [VERIFIED: pkg.go.dev/time] |
| `runtime` | stdlib | `runtime.NumGoroutine()` for D-49's leak audit | Approximate goroutine count; non-deterministic settle-time required (100 ms `time.Sleep` between baseline and post-check is sufficient for Phase 2's single oversize test). [VERIFIED: pkg.go.dev/runtime] |

**Verification commands (run before adding any of these to go.mod imports):**

```bash
# All stdlib — no go.mod additions needed for Phase 2 production code.
# Confirm Go toolchain supports the floor:
go version                              # expect ≥ go1.23
grep '^go ' /data/git/private/holidays/go.mod   # expect: go 1.23
```

### Supporting (test-only)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/stretchr/testify` | v1.11.1 (already in go.mod) | Assertion library (`assert`, `require`) | Gold Rule 3 mandates testify. Already pinned by Phase 1. No new dependency. |
| `net/http/httptest` | stdlib | In-process HTTP servers for unit tests | All Phase 2 tests; no live network. |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `io.LimitReader(body, max+1)` + sentinel-byte read | `http.MaxBytesReader(nil, body, max)` | `MaxBytesReader` returns a typed `*http.MaxBytesError`, is a `ReadCloser`, and is technically more idiomatic. BUT — D-24/D-25 already pin `ErrResponseTooLarge` as the sentinel callers branch on, and the limit+1 technique is well-understood in the Go community. Stick with `io.LimitReader` per D-44, document the sentinel-byte read clearly. |
| `goleak.VerifyNone(t)` (D-49 alternative) | `go.uber.org/goleak` | Higher signal (per-goroutine stack inspection, allow-list filtering for testify+runtime baseline goroutines). Adds a test-only dep that needs a `Key Decisions` entry. D-49 picks the runtime-delta approach; revisit in Phase 4 if cache sweeper produces leaks. |
| `slog.Default()` (D-39 default) | `discard logger` (`slog.New(slog.NewTextHandler(io.Discard, nil))`) | Hidden default means library logs go to whatever the calling app set as default — usually fine; sometimes surprises tests. Mitigated by `WithLogger(...)`. |
| `req.Clone(req.Context())` | `req.WithContext(req.Context())` (no-op clone) | `WithContext` returns a shallow copy of `*Request` whose `Header` map shares with the original — mutating it would mutate the caller's. `Clone` deep-copies Header. **Use Clone**. [VERIFIED: pkg.go.dev/net/http#Request.Clone] |
| `context.WithTimeout` per call (D-27) | Client-level `httpClient.Timeout` (D-26 rejects this) | `httpClient.Timeout` is known to cause body-close races (golang/go#49521); ctx-based timeouts are the modern answer. [CITED: github.com/golang/go/issues/49521] |

**Installation:** None required for Phase 2 production code. testify is already in `go.mod` from Phase 1.

**Version verification:**
```bash
go list -m github.com/stretchr/testify              # expect v1.11.1
grep '^go ' /data/git/private/holidays/go.mod        # expect: go 1.23
```

## Package Legitimacy Audit

> **Not applicable** for Phase 2. Phase 2 introduces zero new dependencies (production or test). All packages used are either stdlib or already in `go.mod` from Phase 1 (testify v1.11.1). Phase 1's verification report (`01-VERIFICATION.md`) recorded zero non-stdlib production imports and testify as the only test-only dep — that posture is preserved in Phase 2.

If a future plan in this phase proposes adding `go.uber.org/goleak` (deferred — see D-49), the legitimacy gate applies at that time:

| Package | Registry | Disposition | Notes |
|---------|----------|-------------|-------|
| `go.uber.org/goleak` | Go module proxy (proxy.golang.org) | DEFERRED | Pre-approved test deps per PROJECT.md are testify + go-cmp. goleak would need a new `Key Decisions` entry. |

## Architecture Patterns

### System Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│ Library Consumer                                                         │
│   c := openholidays.NewClient(                                           │
│       openholidays.WithBaseURL("https://openholidaysapi.org"),           │
│       openholidays.WithTimeout(15*time.Second),                          │
│   )                                                                      │
│   countries, err := c.Countries(ctx)                                     │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │ (1) public method call with ctx
                               ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ countries.go : Client.Countries(ctx)                                     │
│                                                                          │
│  ctx == nil ? → errors.New("openholidays: nil context")  // D-42 guard  │
│  ctx, cancel := context.WithTimeout(ctx, c.timeout)                      │
│  defer cancel()                                                          │
│  req, _ := http.NewRequestWithContext(ctx, GET, c.baseURL+"/Countries")  │
│  resp, err := c.http.Do(req)                                             │
│  if err != nil { return nil, fmt.Errorf("openholidays: ...: %w", err) }  │
│  defer func() { io.Copy(io.Discard, resp.Body); resp.Body.Close() }()    │
│  if resp.StatusCode >= 400 { return nil, buildAPIError(resp, "/Countries") }
│  limited := io.LimitReader(resp.Body, maxResponseBytes)                  │
│  if err := json.NewDecoder(limited).Decode(&countries); err != nil { … } │
│  // Sentinel byte: detect truncation                                     │
│  one := make([]byte, 1); n, _ := io.ReadFull(resp.Body, one)             │
│  if n > 0 { return nil, fmt.Errorf("...: %w", ErrResponseTooLarge) }     │
│  return countries, nil                                                   │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │ (2) c.http.Do enters chain
                               ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ headerTransport (outermost — first to see the request)                   │
│   reqCopy := req.Clone(req.Context())                                    │
│   if reqCopy.Header.Get("Accept") == "" {                                │
│       reqCopy.Header.Set("Accept", "application/json")                   │
│   }                                                                      │
│   if reqCopy.Header.Get("User-Agent") == "" {                            │
│       reqCopy.Header.Set("User-Agent", "go-openholidays/"+Version)       │
│   }                                                                      │
│   return h.next.RoundTrip(reqCopy)                                       │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ loggingTransport                                                         │
│   start := time.Now()                                                    │
│   resp, err := l.next.RoundTrip(req)                                     │
│   l.logger.LogAttrs(req.Context(), slog.LevelDebug,                      │
│       "openholidays http",                                               │
│       slog.String("method", req.Method),                                 │
│       slog.String("path", req.URL.Path),                                 │
│       slog.Int("status", statusOf(resp)),  // -1 on err                  │
│       slog.Int64("duration_ms", time.Since(start).Milliseconds()),       │
│       slog.Int("attempt", 1),  // D-31 hardcoded                         │
│       slog.Int64("bytes_in", bytesIn(resp)),  // resp.ContentLength or -1│
│   )                                                                      │
│   return resp, err                                                       │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ underlying http.RoundTripper                                             │
│   - http.DefaultTransport if WithHTTPClient not used                     │
│   - else the user's *http.Client.Transport (shallow-copied client)       │
│   - honors req.Context() cancellation via http2 internals                │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │ network
                               ▼
                  openholidaysapi.org/Countries
                               │ HTTP/2 chunked response, application/json
                               ▼
              [response flows back up: log records duration,
               header layer is a no-op on the response,
               endpoint method drains+decodes]
```

### Component Responsibilities

| Component | File | Owns |
|-----------|------|------|
| `Client` (struct + `NewClient` + `Close`) | `client.go` | Immutable Client fields (`http`, `baseURL`, `userAgent`, `logger`, `timeout`, `closed atomic.Bool`); `NewClient` orchestration; `Close` atomic flag flip. |
| `Option` + functional Option constructors | `options.go` | `type Option func(*clientConfig)` + `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithLogger`, `WithTimeout`. |
| `clientConfig` + `composeHTTPClient` + `buildTransport` | `config.go` | Unexported config struct; default config builder; shallow-copy of user's `*http.Client`; chain composition. |
| `headerTransport` + `loggingTransport` | `transport.go` | Two `http.RoundTripper` implementations; each is one struct with a `next http.RoundTripper` field. |
| `Countries` endpoint + `buildAPIError` helper | `countries.go` | The endpoint method; `*APIError` construction from a 4xx/5xx response (D-43). |

### Recommended Project Structure

```
go-openholidays/                  # already exists at repo root
├── client.go              [NEW]  # Client struct, NewClient, Close
├── options.go             [NEW]  # Option type + 5 WithX functions
├── config.go              [NEW]  # clientConfig, defaultConfig(), composeHTTPClient, buildTransport
├── transport.go           [NEW]  # headerTransport, loggingTransport, statusOf, bytesIn helpers
├── countries.go           [NEW]  # Countries endpoint + buildAPIError helper
├── errors.go              [MOD]  # ADD ErrResponseTooLarge to existing var ( ... ) block
├── validate.go            [MOD]  # W-01 fix: reorder ASCII check before canonicalization
├── internal_test.go       [MOD]  # ADD "ErrResponseTooLarge" to allowedVars; remove "internal" from skipDirs (W-03 deferred — Phase 2 only fixes W-01)
│
├── client_test.go         [NEW]  # TestNewClient + TestClient_Close + TestClient_ConcurrentAccess
├── options_test.go        [NEW]  # one TestXxx per WithX function
├── transport_header_test.go [NEW] # TestHeaderTransport_RoundTrip
├── transport_logging_test.go [NEW] # TestLoggingTransport_RoundTrip
├── countries_test.go      [NEW]  # TestClient_Countries (happy + 4xx + 5xx + ctx-cancel + oversize)
├── validate_test.go       [MOD]  # ADD regression cases for W-01 (KK, İ-prefix, ı-prefix, ſ-prefix)
│
└── testdata/              [NEW]
    └── countries.json     [NEW]  # 2-country slice (PL + DE) captured from live API 2026-05-27
```

### Pattern 1: Functional Options with Internal Builder (locked, D-35)

**What:** `NewClient(opts ...Option) *Client` applies each `Option` to a fresh `clientConfig`, then constructs an immutable `Client`. Options never touch the `Client` after construction.

**When to use:** Always — this is the canonical Go SDK construction pattern (gRPC, Kubernetes client-go, AWS SDK, stripe-go, google/go-github). Confirmed by ARCHITECTURE.md Pattern 1.

**Implementation skeleton (planner will slice into tasks):**

```go
// options.go
package openholidays

import (
    "log/slog"
    "net/http"
    "time"
)

// Option configures a Client. Options compose via NewClient.
type Option func(*clientConfig)

// clientConfig is the internal builder state. Unexported — never escapes.
type clientConfig struct {
    httpClient *http.Client
    baseURL    string
    userAgent  string
    logger     *slog.Logger
    timeout    time.Duration
}

func defaultConfig() *clientConfig {
    return &clientConfig{
        httpClient: &http.Client{}, // empty; Transport supplied by buildTransport
        baseURL:    "https://openholidaysapi.org",
        userAgent:  "go-openholidays/" + Version,
        logger:     slog.Default(),
        timeout:    15 * time.Second,
    }
}

// WithHTTPClient supplies a pre-configured *http.Client. The SDK shallow-copies
// it; mutations to the original after NewClient returns do not affect the SDK.
func WithHTTPClient(c *http.Client) Option {
    return func(cfg *clientConfig) {
        if c != nil { cfg.httpClient = c }
    }
}

// WithBaseURL overrides the default base URL (https://openholidaysapi.org).
// The base URL must NOT have a trailing slash; endpoint paths begin with "/".
func WithBaseURL(u string) Option { /* trim trailing slash, set */ }

// WithUserAgent overrides the default User-Agent. Empty string is treated as
// "use default" — the SDK never sends an empty UA (some CDNs reject it).
func WithUserAgent(s string) Option { /* if s != "" then cfg.userAgent = s */ }

// WithLogger injects a structured logger. A nil logger is replaced with slog.Default().
func WithLogger(l *slog.Logger) Option { /* if l == nil then cfg.logger = slog.Default() else cfg.logger = l */ }

// WithTimeout sets the per-request timeout (default 15s). A zero duration
// disables the SDK-imposed timeout; the caller's ctx becomes the only deadline.
func WithTimeout(d time.Duration) Option { /* cfg.timeout = d */ }
```

### Pattern 2: RoundTripper Decorator Chain — minimal Phase 2 shape (D-29)

**What:** Two `http.RoundTripper` structs (`headerTransport`, `loggingTransport`) compose via a `next` field. `buildTransport(cfg)` returns the outermost wrapper.

**Chain order (Phase 2):**

```
req → headerTransport → loggingTransport → http.DefaultTransport (or user's Transport)
```

**Phase 3 will add `retryTransport` outermost. Phase 4 will add `cacheTransport` between retry and hook. Plan accordingly — do NOT abstract the chain composition into a generic middleware list pre-1.0 (D-29 explicitly rejects this).**

**Implementation skeleton:**

```go
// config.go
package openholidays

import (
    "net/http"
)

func composeHTTPClient(cfg *clientConfig) *http.Client {
    // Shallow-copy the user's *http.Client to neutralize hidden mutability
    // (Pitfall HTTP-1). The Transport on the copy is replaced with our chain.
    cp := *cfg.httpClient
    cp.Transport = buildTransport(cfg)
    return &cp
}

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
```

### Pattern 3: `headerTransport` with `req.Clone` (D-30)

**Key insight:** `http.RoundTripper`'s contract says *"RoundTrip should not modify the request, except for consuming and closing the Request's Body."* Mutating the inbound `req.Header` would violate this and cause races when the same `*http.Request` is reused.

**Canonical idiom** (verified against Go stdlib docs and google/go-github PR #805):

```go
// transport.go
package openholidays

import "net/http"

type headerTransport struct {
    userAgent string
    next      http.RoundTripper
}

func (h *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    // Clone the request so we do not mutate the caller's *http.Request
    // (http.RoundTripper contract — Pitfall HTTP-2). req.Clone deep-copies
    // the Header map; req.WithContext does not.
    reqCopy := req.Clone(req.Context())

    // Caller override wins (D-30): only set defaults when absent.
    if reqCopy.Header.Get("Accept") == "" {
        reqCopy.Header.Set("Accept", "application/json")
    }
    if reqCopy.Header.Get("User-Agent") == "" {
        reqCopy.Header.Set("User-Agent", h.userAgent)
    }
    return h.next.RoundTrip(reqCopy)
}
```

[VERIFIED: pkg.go.dev/net/http#RoundTripper documentation; google/go-github PR #805 — "RoundTrip: avoid modifying the original request"]

### Pattern 4: `loggingTransport` with `slog.LogAttrs` (D-31)

**Key insight:** `logger.LogAttrs(ctx, level, msg, attrs...)` is faster than `logger.Debug(msg, "k", v, "k", v)` because it avoids the variadic-key-value parsing path. For a hot path that fires on every HTTP call, prefer `LogAttrs`. [CITED: pkg.go.dev/log/slog#Logger.LogAttrs]

**Implementation skeleton:**

```go
// transport.go (continued)

import (
    "context"
    "log/slog"
    "net/http"
    "time"
)

type loggingTransport struct {
    logger *slog.Logger
    next   http.RoundTripper
}

func (l *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    start := time.Now()
    resp, err := l.next.RoundTrip(req)
    l.logger.LogAttrs(req.Context(), slog.LevelDebug,
        "openholidays http",
        slog.String("method", req.Method),
        slog.String("path", req.URL.Path),
        slog.Int("status", statusOf(resp)),
        slog.Int64("duration_ms", time.Since(start).Milliseconds()),
        slog.Int("attempt", 1), // D-31: hardcoded in Phase 2; Phase 3 retry will inject via ctx
        slog.Int64("bytes_in", bytesIn(resp)),
    )
    return resp, err
}

// statusOf returns resp.StatusCode when resp != nil, else -1 (so a network
// failure logs as "status=-1" rather than a nil-deref panic).
func statusOf(resp *http.Response) int {
    if resp == nil {
        return -1
    }
    return resp.StatusCode
}

// bytesIn returns resp.ContentLength when resp != nil, else -1.
// Note: HTTP/2 chunked responses (which the live OpenHolidays API uses)
// have ContentLength == -1 by design — this is the documented stdlib
// semantic for "unknown length", not a bug.
func bytesIn(resp *http.Response) int64 {
    if resp == nil {
        return -1
    }
    return resp.ContentLength
}
```

**Note on `attempt: 1`:** A future-proof alternative is `ctxKeyAttempt{}` context value injection, but Phase 2 deliberately hardcodes `1` per D-31. When Phase 3's retry transport lands, it will wrap with a ctx-attached counter; the planner must update `loggingTransport` then (one-line read of `req.Context().Value(...)`). Pre-planning for that hook now adds complexity Phase 2 doesn't need.

### Pattern 5: Endpoint method — `Countries(ctx)` (D-41..D-45)

**Implementation skeleton:**

```go
// countries.go
package openholidays

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
)

const maxResponseBytes = 10 << 20 // 10 MiB (D-25)

// Countries fetches the list of supported countries from the upstream API.
//
// Every country includes its ISO 3166-1 alpha-2 isoCode, a multi-language Name
// array, and the country's official languages (ISO 639-1 codes). Use
// Country.NameFor(lang) to look up the localized name for a given language.
//
// The per-request timeout configured via WithTimeout (default 15s) is honored;
// the caller's context cancellation interrupts in-flight HTTP within ≤ 100ms.
func (c *Client) Countries(ctx context.Context) ([]Country, error) {
    if ctx == nil {
        return nil, errors.New("openholidays: nil context")
    }

    // D-26..D-28: per-request timeout via context.WithTimeout. When c.timeout
    // is zero, skip the wrap so the caller's ctx is the only deadline.
    if c.timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, c.timeout)
        defer cancel()
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/Countries", nil)
    if err != nil {
        return nil, fmt.Errorf("openholidays: build request: %w", err)
    }

    resp, err := c.http.Do(req)
    if err != nil {
        // Preserve ctx sentinels (Pitfall CTX-3) — they must survive wrap.
        return nil, fmt.Errorf("openholidays: GET /Countries: %w", err)
    }
    defer func() {
        // Drain-then-close (D-45 + Pitfall HTTP-3) for keep-alive reuse.
        // Cap the drain at maxResponseBytes+1 so a malicious server cannot
        // make the close path block on an unbounded stream.
        _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
        _ = resp.Body.Close()
    }()

    if resp.StatusCode >= 400 {
        return nil, buildAPIError(resp, "/Countries")
    }

    var countries []Country
    limited := io.LimitReader(resp.Body, maxResponseBytes)
    if err := json.NewDecoder(limited).Decode(&countries); err != nil {
        return nil, fmt.Errorf("openholidays: decode /Countries: %w", err)
    }

    // Sentinel-byte read: if any data remains after Decode consumed the
    // maximum allowed bytes, the response was truncated and we must surface
    // it as ErrResponseTooLarge (D-24).
    var one [1]byte
    if n, _ := resp.Body.Read(one[:]); n > 0 {
        return nil, fmt.Errorf("openholidays: response exceeded %d bytes: %w",
            maxResponseBytes, ErrResponseTooLarge)
    }

    return countries, nil
}

// buildAPIError reads the (already-error) response body up to 4 KiB (Phase 1
// D-17 cap) and constructs an *APIError. The Message is best-effort parsed
// from the upstream JSON envelope, supporting both RFC 7807 ProblemDetails
// ({"detail": ...} / {"title": ...} — confirmed live 2026-05-27) and the
// fallback {"error": ...} shape.
func buildAPIError(resp *http.Response, path string) *APIError {
    const apiErrorBodyCap = 4 << 10 // 4 KiB (Phase 1 D-17)
    body, _ := io.ReadAll(io.LimitReader(resp.Body, apiErrorBodyCap))
    msg := parseAPIMessage(body)
    return &APIError{
        StatusCode: resp.StatusCode,
        Path:       path,
        Body:       body,
        Message:    msg,
    }
}

// parseAPIMessage decodes upstream error envelopes opportunistically.
// Tries detail (RFC 7807, OpenHolidays confirmed live), then title (RFC 7807),
// then error (generic). Returns "" when no recognizable field is present.
func parseAPIMessage(body []byte) string {
    var env struct {
        Detail string `json:"detail"`
        Title  string `json:"title"`
        Error  string `json:"error"`
    }
    if err := json.Unmarshal(body, &env); err != nil {
        return ""
    }
    switch {
    case env.Detail != "":
        return env.Detail
    case env.Title != "":
        return env.Title
    case env.Error != "":
        return env.Error
    default:
        return ""
    }
}
```

### Anti-Patterns to Avoid (specific to Phase 2)

- **Setting `cfg.httpClient.Timeout`** (instead of `context.WithTimeout`): Known to produce spurious "context canceled" errors on `resp.Body.Close()` per golang/go#49521. D-26 explicitly says leave Timeout at the caller's value. Use `context.WithTimeout` per call.
- **Mutating `req.Header` directly in `headerTransport`**: Violates RoundTripper contract; `Header` map is shared with caller via `req.WithContext`. Always `req.Clone(req.Context())` first.
- **Reading `resp.Body` inside `loggingTransport`**: Would consume the bytes before the endpoint decoder runs. D-31 explicit. The `bytes_in` field is `resp.ContentLength`, NOT a byte count we measured.
- **Returning `*APIError` for transport-level errors** (network failure, ctx cancel): Loses `errors.Is(err, context.Canceled)` semantics. `buildAPIError` is only called when `resp.StatusCode >= 400`.
- **Skipping the sentinel-byte read after Decode**: `json.Decoder.Decode` stops at the end of the first valid JSON value; if the response is `[..., ...]` followed by 12 MiB of trailing garbage, Decode returns success and the trailing garbage is never observed. The 1-byte read after Decode is the truncation gate.
- **Using `goroutine` to drain on `defer`**: The drain must be synchronous so the deferred Close happens *after* the drain finishes. A goroutine'd drain races with the function return.
- **`http.NewRequest` without `WithContext`**: Deprecated form since Go 1.13. Always `http.NewRequestWithContext`.
- **`json.Unmarshal(body, &v)` with `io.ReadAll(resp.Body)`**: Loads the whole body into memory first, defeating the streaming-decode advantage and giving no truncation visibility. Use `json.NewDecoder(io.LimitReader(...)).Decode(&v)` instead.
- **Setting both `Accept` and `User-Agent` from the endpoint method**: Belongs in `headerTransport` — uniform across every endpoint Phase 3+ will add.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP client transport chain | A custom `chain []middleware` framework with reflection or per-middleware registration | Plain `http.RoundTripper` decorator structs composed in `buildTransport` | Stdlib interface; every Go developer knows it; no abstraction tax. D-29 explicit. |
| Bounded response read | A custom `LimitedReader` with overflow callbacks | `io.LimitReader` + 1-byte sentinel read | Stdlib; well-understood. `http.MaxBytesReader` works client-side too but D-24 already names the sentinel `ErrResponseTooLarge` and the limit+1 technique is the idiomatic match. |
| Drain-and-close pattern | A `drainAndClose(io.ReadCloser)` helper file just for this | Single-line deferred `io.Copy(io.Discard, ...); body.Close()` | The pattern appears once per endpoint method (Phase 2: 1 endpoint, Phase 3: 4 more). A 3-line helper saves nothing and adds an indirection level. |
| Per-request timeout | A wall-clock timer goroutine | `context.WithTimeout` + `defer cancel()` | Stdlib; honors cancellation propagation through the entire RoundTripper chain and into `http.Transport`. |
| User-Agent injection | Setting on every request manually | `headerTransport.RoundTrip` clones-and-sets once | Uniform across endpoints; Pitfall HTTP-5 mitigation built into the chain. |
| Structured logging | `log.Printf("method=%s status=%d", ...)` | `slog.LogAttrs(ctx, slog.LevelDebug, ...)` | Stdlib since Go 1.21; OBS-01 mandates `slog.LevelDebug`. Never import `log`. |
| Goroutine leak detection | A polling loop checking `runtime.NumGoroutine` every 10ms | A single baseline / 100ms-settle / post-check delta | D-49 specifies the runtime-delta approach. goleak is the "real" answer but deferred to Phase 4. |

**Key insight:** Phase 2's transport is small enough (header transport: ~12 lines; logging transport: ~18 lines; Countries endpoint: ~50 lines) that the "hand-rolled" code IS the recommendation. Adding a framework for two RoundTrippers is over-engineering. The architectural value comes from the chain *shape* (D-29 explicit), not from a layer of helpers.

## Runtime State Inventory

> **Not applicable** for Phase 2. Phase 2 is a greenfield additive change — no renames, no refactors, no data migrations. The Phase 1 file allowlist in `internal_test.go` (`allowedVars`) needs one new entry (`ErrResponseTooLarge`) to keep the CLIENT-10 AST audit green; this is a code edit, not a runtime-state migration. There is no live service, no database, no OS-registered state, no env-var-named-thing that Phase 2 touches.

## Common Pitfalls

### Pitfall 1: HTTP/2 chunked responses have `Content-Length == -1`

**What goes wrong:** `loggingTransport` emits `bytes_in: -1` for every live OpenHolidays response (verified 2026-05-27: chunked encoding, no Content-Length header on the wire). Operators see `bytes_in: -1` in logs, mis-interpret as "the library failed to count bytes," file a bug.

**Why it happens:** HTTP/2 uses framing with length-per-frame; servers commonly do not pre-compute or send a total Content-Length header. `http.Response.ContentLength` is `-1` by stdlib design when the value is unknown — this is documented in `net/http`. The same applies to HTTP/1.1 with `Transfer-Encoding: chunked`.

**How to avoid:** Document the `-1` semantic in the OBS-02 field documentation (godoc on Client or in `doc.go`):
> `bytes_in` is the declared response content length, or `-1` when the response uses chunked transfer encoding (HTTP/1.1 chunked or HTTP/2). For OpenHolidays, expect `-1` on every successful call against the live upstream.

If a future requirement needs the actual byte count, wrap `resp.Body` in a `countingReader` *inside the endpoint method* (NOT in `loggingTransport` — body reading there breaks decode). That's Phase 4+ territory.

**Warning signs:** A test that asserts `bytes_in > 0` for a live response (or against `httptest.NewServer` which DOES set Content-Length by default — so tests pass but production logs are misleading).

### Pitfall 2: `context.WithTimeout` AND `http.Client.Timeout` both set causes spurious cancel errors

**What goes wrong:** Caller passes `WithHTTPClient(&http.Client{Timeout: 5*time.Second})` and the SDK also wraps with `context.WithTimeout(ctx, 15*time.Second)`. The shorter Client.Timeout fires first; `http.Transport` cancels the request via an internal context. On `resp.Body.Close()` after a slow read, the close path returns "context canceled" because the cancellation flag is set even though decode finished successfully.

**Why it happens:** `http.Client.Timeout` is implemented internally via a context that wraps the request — so it composes with the caller's ctx. The race window is the body close path. Documented in golang/go#49521.

**How to avoid:** D-26 explicitly says **do not set `cfg.httpClient.Timeout`** — leave it at the caller's value. The SDK uses `context.WithTimeout` per call. If a caller's `*http.Client` has a non-zero Timeout, that's the caller's contract; document the recommendation against it in `WithHTTPClient` godoc:

> WithHTTPClient supplies a pre-configured *http.Client. The library shallow-
> copies the client and wraps its Transport. NOTE: setting Timeout on the
> supplied *http.Client may cause spurious "context canceled" errors on body
> close (see github.com/golang/go/issues/49521); prefer WithTimeout(d) to
> bound per-request duration via context.

**Warning signs:** Tests that pass against a slow httptest server but flake under high CI load. Operator reports of "context canceled" on successful decodes.

### Pitfall 3: Body drain happens AFTER body close (resource leak)

**What goes wrong:** Naive `defer resp.Body.Close()` followed by an early return on a status-code check. The Close fires, the body is never drained, the connection cannot be returned to the keep-alive pool. Transport thrashes new TCP connections; latency rises 5-10× under load. Documented in Pitfall HTTP-3.

**Why it happens:** Go 1.24 added an auto-drain of up to 256 KiB / 50 ms in `Body.Close()`, which masks the bug for small responses. The live OpenHolidays `/Countries` response is 6 KiB — well under 256 KiB — so the auto-drain succeeds and the bug stays hidden until a larger response (e.g., `/Subdivisions` for 30 countries) hits.

**How to avoid:** D-45 specifies the order: `io.Copy(io.Discard, resp.Body)` THEN `resp.Body.Close()`. Single deferred func:

```go
defer func() {
    _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
    _ = resp.Body.Close()
}()
```

The `LimitReader` cap on the drain itself is belt-and-braces against a malicious server that streams forever — the drain bound is independent of the decoder bound.

**Warning signs:** `netstat -an | grep TIME_WAIT | wc -l` climbing in a long-running test. Throughput regression when responses grow.

### Pitfall 4: `req.Header` mutation in `headerTransport` (race + caller-visible)

**What goes wrong:** `headerTransport.RoundTrip` does `req.Header.Set("Accept", "application/json")` directly on `req`. Caller's original `*http.Request` (or a goroutine using the same request) now sees the mutated header. Race detector flags it; `-race` CI run fails.

**Why it happens:** `*http.Request.Header` is a shared `http.Header` map. `req.WithContext` returns a shallow copy whose Header points to the same map. Only `req.Clone(ctx)` deep-copies Header.

**How to avoid:** D-30 mandates `req.Clone(req.Context())` before any header mutation. See the implementation skeleton above. [VERIFIED: pkg.go.dev/net/http#Request.Clone — "Clone returns a deep copy of r with its context changed to ctx. The provided ctx must be non-nil."]

**Warning signs:** Any line of the form `req.Header.Set` in `headerTransport.RoundTrip` BEFORE a `Clone` call. Linter rule (gocritic / staticcheck) does not catch this — code review must.

### Pitfall 5: `errors.Is(err, ErrResponseTooLarge)` fails because Decode masks the error

**What goes wrong:** The endpoint method returns `fmt.Errorf("decode /Countries: %w", err)` where `err` is `*json.SyntaxError` from a truncated mid-array decode (the 12 MiB body got cut at byte 10485760 in the middle of a country object). Caller's `errors.Is(err, ErrResponseTooLarge)` returns `false` because the sentinel wasn't wrapped.

**Why it happens:** `json.NewDecoder` reading from `io.LimitReader` does NOT return a special "limit reached" error — it returns the JSON syntax error from the truncated payload. The limit is invisible to the decoder.

**How to avoid:** The sentinel-byte read AFTER a successful Decode is the only reliable signal of oversize. For the case where Decode fails mid-truncation, the planner has two options:

1. **Accept "JSON syntax error" as the message for mid-truncation oversize.** Callers who care can still distinguish by reading `resp.ContentLength` upstream of the SDK (when known). Simpler.
2. **Re-check on Decode failure:** If `Decode` returns `*json.SyntaxError` AND a sentinel-byte read succeeds, prefer `ErrResponseTooLarge` over the syntax error. Adds 4 lines.

Recommend **option 1** for Phase 2: simpler, and the spec test (D-49) uses a server that returns a valid 11 MiB JSON array — Decode succeeds, sentinel-byte read fires, `ErrResponseTooLarge` wraps. The mid-truncation case is an edge case worth a follow-up note in the godoc for `ErrResponseTooLarge`:

> ErrResponseTooLarge is returned when an upstream response exceeds the
> 10 MiB cap and the truncation is detected after JSON decode completes.
> A response truncated mid-JSON-value returns a decode error wrapping
> *json.SyntaxError instead.

### Pitfall 6: `Client.Close()` raced from multiple goroutines

**What goes wrong:** Two goroutines call `c.Close()` concurrently. Phase 2's stub is `c.closed.Store(true)` — `atomic.Bool` makes this safe. But Phase 4 will hook a cache-sweeper-cancel onto Close; the naive Phase 4 implementation might cancel the sweeper twice, panic on a closed channel, etc.

**Why it happens:** `Client.Close()` is documented idempotent and safe from any goroutine (CLIENT-08). Phase 2's stub satisfies this trivially; Phase 4 needs to satisfy it nontrivially.

**How to avoid:** Phase 2 implements `Close` with `atomic.Bool` so Phase 4 can wrap the additional teardown in `sync.Once`:

```go
// client.go (Phase 2 stub — Phase 4 will extend the body)
type Client struct {
    http      *http.Client
    baseURL   string
    userAgent string
    logger    *slog.Logger
    timeout   time.Duration
    closed    atomic.Bool
    // Phase 4 will add: closeOnce sync.Once; cacheSweeper context.CancelFunc
}

// Close is the idempotent shutdown hook. In v0.1.0 it is a no-op that flips
// an internal closed flag; future versions will stop background goroutines
// (cache sweeper) here. Safe to call from any goroutine; subsequent calls
// also return nil.
func (c *Client) Close() error {
    c.closed.Store(true)
    return nil
}
```

**Warning signs:** A future PR that adds `c.cacheCancel()` inline (not behind `sync.Once`) after Phase 4's sweeper goroutine lands.

## Code Examples

Verified patterns from official sources, ready for the planner to slice into task `actions`.

### Test pattern: shallow-copy isolation (CLIENT-02)

```go
// client_test.go
func TestClient_WithHTTPClient_isolation(t *testing.T) {
    t.Parallel()
    custom := &http.Client{Timeout: 5 * time.Second}
    c := NewClient(WithHTTPClient(custom))

    // After NewClient returns, mutate the user's client.
    custom.Timeout = 100 * time.Millisecond

    // The SDK's internal client must NOT have observed the mutation.
    // Reach in via a test-only accessor or compare Transport identity.
    require.NotNil(t, c)
    // Concrete assertion shape depends on how the planner exposes the field
    // for testing — either a //test_only getter or unexported-package test.
}
```

### Test pattern: header transport sets defaults (TRANS-01)

```go
// transport_header_test.go
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
    return f(r)
}

func TestHeaderTransport_RoundTrip(t *testing.T) {
    t.Parallel()
    var got *http.Request
    h := &headerTransport{
        userAgent: "go-openholidays/0.1.0",
        next: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
            got = r
            return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
        }),
    }

    t.Run("sets defaults when absent", func(t *testing.T) {
        req, _ := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
        _, err := h.RoundTrip(req)
        require.NoError(t, err)
        assert.Equal(t, "application/json", got.Header.Get("Accept"))
        assert.Equal(t, "go-openholidays/0.1.0", got.Header.Get("User-Agent"))
        // Caller's original req.Header is untouched.
        assert.Empty(t, req.Header.Get("Accept"))
    })

    t.Run("preserves caller-supplied headers", func(t *testing.T) {
        req, _ := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
        req.Header.Set("Accept", "application/vnd.custom+json")
        req.Header.Set("User-Agent", "my-app/2.0")
        _, err := h.RoundTrip(req)
        require.NoError(t, err)
        assert.Equal(t, "application/vnd.custom+json", got.Header.Get("Accept"))
        assert.Equal(t, "my-app/2.0", got.Header.Get("User-Agent"))
    })
}
```

### Test pattern: logging transport captures slog records (OBS-01, OBS-02)

```go
// transport_logging_test.go
func TestLoggingTransport_RoundTrip(t *testing.T) {
    t.Parallel()

    t.Run("emits Debug record with all OBS-02 fields", func(t *testing.T) {
        var buf bytes.Buffer
        logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
            Level: slog.LevelDebug,
        }))
        l := &loggingTransport{
            logger: logger,
            next: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
                return &http.Response{
                    StatusCode:    200,
                    ContentLength: 6055,
                    Body:          io.NopCloser(strings.NewReader("")),
                }, nil
            }),
        }
        req, _ := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
        _, err := l.RoundTrip(req)
        require.NoError(t, err)

        var rec map[string]any
        require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
        assert.Equal(t, "DEBUG", rec["level"])
        assert.Equal(t, "GET", rec["method"])
        assert.Equal(t, "/Countries", rec["path"])
        assert.EqualValues(t, 200, rec["status"])
        assert.EqualValues(t, 1, rec["attempt"])
        assert.EqualValues(t, 6055, rec["bytes_in"])
        // duration_ms is non-deterministic; assert it is present and non-negative.
        dur, ok := rec["duration_ms"].(float64)
        require.True(t, ok, "duration_ms missing or wrong type: %v", rec["duration_ms"])
        assert.GreaterOrEqual(t, dur, 0.0)
    })

    t.Run("logs ContentLength as -1 when response is chunked", func(t *testing.T) {
        // HTTP/2 / chunked responses arrive with ContentLength == -1.
        // The library MUST forward this, not coerce to 0.
        var buf bytes.Buffer
        logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
        l := &loggingTransport{logger: logger, next: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
            return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(strings.NewReader(""))}, nil
        })}
        req, _ := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
        _, _ = l.RoundTrip(req)
        var rec map[string]any
        require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
        assert.EqualValues(t, -1, rec["bytes_in"])
    })
}
```

### Test pattern: context cancellation under 100 ms (CLIENT-09)

```go
// client_test.go
func TestClient_ContextCancel(t *testing.T) {
    t.Parallel()

    // Server hangs forever (10s) — caller cancellation MUST interrupt.
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        select {
        case <-time.After(10 * time.Second):
            w.WriteHeader(http.StatusOK)
        case <-r.Context().Done():
            // Server observes the client cancellation. No write needed.
        }
    }))
    t.Cleanup(srv.Close)

    c := NewClient(WithBaseURL(srv.URL), WithTimeout(30*time.Second)) // SDK timeout NOT in play here

    t.Run("ctx cancel interrupts in-flight HTTP within 200ms", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        time.AfterFunc(50*time.Millisecond, cancel)
        start := time.Now()
        _, err := c.Countries(ctx)
        elapsed := time.Since(start)

        require.Error(t, err)
        // 100ms target + 100ms CI slack = 200ms ceiling (D-48).
        assert.Less(t, elapsed, 200*time.Millisecond,
            "ctx cancel must interrupt in-flight HTTP within 200ms; took %v", elapsed)
        assert.True(t, errors.Is(err, context.Canceled),
            "expected context.Canceled, got %v", err)
    })
}
```

### Test pattern: oversize response with goroutine-leak audit (TRANS-02, D-49)

```go
// countries_test.go
func TestClient_Countries_oversize(t *testing.T) {
    // NOT t.Parallel — this test reads runtime.NumGoroutine() and must
    // not race with other tests' goroutine churn.

    // Server streams an 11 MiB JSON array of valid Country entries.
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        // Write [{...one_country...}, {...one_country...}, ...] until 11 MiB.
        w.Write([]byte("["))
        sample := []byte(`{"isoCode":"PL","name":[{"language":"EN","text":"Poland"}],"officialLanguages":["PL"]}`)
        target := 11 << 20
        written := 1 // for the "["
        first := true
        for written < target {
            if !first {
                w.Write([]byte(","))
                written++
            }
            w.Write(sample)
            written += len(sample)
            first = false
        }
        w.Write([]byte("]"))
    }))
    t.Cleanup(srv.Close)

    c := NewClient(WithBaseURL(srv.URL))

    t.Run("returns ErrResponseTooLarge for 11 MiB body", func(t *testing.T) {
        baseGoroutines := runtime.NumGoroutine()
        _, err := c.Countries(context.Background())
        require.Error(t, err)
        assert.True(t, errors.Is(err, ErrResponseTooLarge),
            "expected ErrResponseTooLarge, got %v", err)

        // D-49 goroutine-leak gate: settle for 100ms, then re-measure.
        // Allow +2 slack for httptest's own internal goroutines.
        time.Sleep(100 * time.Millisecond)
        afterGoroutines := runtime.NumGoroutine()
        assert.LessOrEqual(t, afterGoroutines, baseGoroutines+2,
            "goroutine leak suspected: baseline=%d after=%d (drain failed?)",
            baseGoroutines, afterGoroutines)
    })
}
```

### Test pattern: concurrent access (TEST-04)

```go
// client_test.go
func TestClient_ConcurrentAccess(t *testing.T) {
    t.Parallel()

    // Synthetic delay simulates real network latency without flake risk.
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(time.Duration(5+rand.Intn(15)) * time.Millisecond) // 5-20ms
        w.Header().Set("Content-Type", "application/json")
        body, _ := os.ReadFile(filepath.Join("testdata", "countries.json"))
        w.Write(body)
    }))
    t.Cleanup(srv.Close)

    c := NewClient(WithBaseURL(srv.URL))
    const N = 50
    var wg sync.WaitGroup
    errs := make([]error, N)
    results := make([][]Country, N)

    for i := 0; i < N; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            results[idx], errs[idx] = c.Countries(context.Background())
        }(i)
    }
    wg.Wait()

    t.Run("all 50 calls succeed with identical payloads", func(t *testing.T) {
        for i := 0; i < N; i++ {
            require.NoError(t, errs[i], "call %d failed", i)
            require.NotEmpty(t, results[i], "call %d returned empty", i)
            if i > 0 {
                assert.Equal(t, results[0], results[i], "call %d payload differs from call 0", i)
            }
        }
    })
}
```

### Test pattern: testdata fixture (PL + DE)

`testdata/countries.json` — capture from live API on the day the fixture lands:

```bash
# Capture command (run during plan execution):
curl -s -H "Accept: application/json" -H "User-Agent: go-openholidays-test/0.1.0" \
    https://openholidaysapi.org/Countries \
  | jq '[.[] | select(.isoCode == "PL" or .isoCode == "DE")]' \
  > testdata/countries.json
```

**Expected content** (verified live 2026-05-27):

```json
[
  {
    "isoCode": "PL",
    "name": [
      {"language": "EN", "text": "Poland"},
      {"language": "PL", "text": "Polska"},
      {"language": "DE", "text": "Polen"}
    ],
    "officialLanguages": ["PL"]
  },
  {
    "isoCode": "DE",
    "name": [
      {"language": "EN", "text": "Germany"},
      {"language": "DE", "text": "Deutschland"}
    ],
    "officialLanguages": ["DE"]
  }
]
```

**Fixture capture metadata:** Add a const in `countries_test.go`:

```go
// countriesFixtureCapturedAt records the date testdata/countries.json was
// captured from the live API. Re-capture when the upstream schema is
// suspected to have drifted. The fixture is not the authoritative shape —
// the live API is.
const countriesFixtureCapturedAt = "2026-05-27"
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `http.NewRequest(method, url, body)` then `req.WithContext(ctx)` | `http.NewRequestWithContext(ctx, method, url, body)` | Go 1.13 (Oct 2019) | Single call; ctx is non-optional and cannot be forgotten. PROJECT.md mandates this form. |
| `log.Printf("...")` for library logging | `slog.LogAttrs(ctx, slog.LevelDebug, "msg", attrs...)` | Go 1.21 (Aug 2023) | Structured fields, ctx-aware, per-record level. Project never imports `"log"`. |
| `req.Header.Set(...)` on the inbound request inside `RoundTrip` | `reqCopy := req.Clone(req.Context()); reqCopy.Header.Set(...)` | Documented stdlib contract; Go 1.13 added `Clone` | Avoids race against caller reuse of `*http.Request`. Canonical idiom in go-github, hashicorp/go-retryablehttp, oauth2 RoundTrippers. |
| `*rand.Rand` seeded from `time.Now().UnixNano()` for jitter | `math/rand/v2` (no seed required) | Go 1.22 (Feb 2024) | Not strictly used in Phase 2 (retry is Phase 3) but worth noting now for chain-extension planning. |
| `atomic.{Load,Store}Uint32` for boolean flags | `atomic.Bool` typed wrapper | Go 1.19 (Aug 2022) | D-40's `closed atomic.Bool` is the modern idiom. |
| `for range time.After(d)` retry loop (uninterruptible) | `select { case <-ctx.Done(): case <-time.After(d): }` | Always — but routinely forgotten | Honors Pitfall CTX-2; not needed in Phase 2 (no retry) but flagged for Phase 3. |
| `encoding/json` v1 only | `GOEXPERIMENT=jsonv2` (Go 1.25 experimental); future `encoding/json/v2` | Go 1.25 (likely Aug 2025) — still experimental in May 2026 | STACK.md HIGH confidence: stick with v1 for Phase 2; revisit when v2 stabilizes (Go 1.26+). |

**Deprecated / outdated patterns to avoid:**

- `http.DefaultClient.Do(req)` — global state, no timeout, Pitfall HTTP-1.
- `req.Cancel` channel — superseded by `req.Context()` since Go 1.7.
- `httputil.DumpRequest` for logging — would log the body; OBS-01 violation.
- Manual `time.AfterFunc` instead of `context.WithTimeout` for per-request deadlines.

## Assumptions Log

Phase 2 research relied on a small number of unverified-in-session claims; each is recorded here so the planner and discuss-phase can confirm them before they harden into locked implementation choices.

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `runtime.NumGoroutine() + 2` slack is sufficient for D-49's leak guard | Test patterns: oversize | If the slack is too tight, the test flakes on slow CI. If too loose, real leaks pass. Risk: low — single-test scope, settle pause of 100 ms is generous. Can tighten/loosen empirically during execution. |
| A2 | The mid-truncation oversize case (Decode returns syntax error before sentinel-byte runs) is acceptable to ship without special handling in Phase 2 | Common Pitfalls #5 | Caller cannot distinguish "valid JSON, wire was truncated" from "garbage JSON" in this edge case. Risk: low — D-49's spec test uses a valid 11 MiB array, which Decode WILL accept; only the trailing-byte branch fires. The mid-truncation case requires a hostile server that sends invalid JSON; that's a "your upstream is broken" scenario, not a normal oversize. Document in godoc. |
| A3 | `time.AfterFunc(50*time.Millisecond, cancel)` reliably triggers cancellation within the 200 ms total budget on shared CI | Test patterns: context cancellation | If runner scheduling delays `cancel()` invocation by > 150 ms, the test flakes. Risk: low — `AfterFunc` is goroutine-based and fires in a separate goroutine; 50 ms is well above kernel timer resolution. |
| A4 | OpenHolidays will not change the `/Countries` response shape between fixture capture and merge | testdata fixture | If the upstream drops `officialLanguages` or renames `isoCode`, the fixture lies. Risk: very low — Phase 1 already verified the schema via the live OpenAPI spec; v0.1.0 ships with the date-stamped fixture and a re-capture script. |
| A5 | The 5-language `LocalizedText` array in PL/DE entries is stable enough that `Country.NameFor("pl")` returning "Polska" is a permanent test invariant | Test patterns: testdata fixture | If upstream removes the PL or DE translation, table assertions break. Risk: very low — "Polska" is the official Polish name; "Deutschland" likewise. The upstream is community-curated; deletions are unlikely. |
| A6 | The deferred drain-and-close pattern (`io.Copy(io.Discard, body); body.Close()` in one deferred func) does NOT introduce a measurable latency penalty for sub-MB responses | Implementation skeleton: Countries | If the drain blocks on a slow-streaming server, total request time increases. Risk: low — the live `/Countries` is 6 KB; drain completes in microseconds. The `LimitReader(body, maxResponseBytes+1)` cap on the drain itself is the safety bound. |
| A7 | Adding `ErrResponseTooLarge` to the `internal_test.go` `allowedVars` set is sufficient to keep CLIENT-10 AST audit green | Specifics callout (re: CONTEXT.md) | If a future contributor adds an unlisted sentinel and the audit was already known-broken, the regression slips. Risk: low — the audit fails closed; adding the entry is mechanical. |

**Bottom line:** All seven assumptions are low-risk and verifiable during plan execution. None is load-bearing on a third-party dependency or a future Go release.

## Open Questions (RESOLVED)

1. **Should `Countries` validate the decoded slice (`len(countries) == 0` ⇒ `ErrEmptyResponse`)?**
   - What we know: D-43 ships `ErrEmptyResponse` for 2xx + empty body; CONTEXT.md does not lock the empty-slice case explicitly.
   - What's unclear: Is `[]` (valid JSON empty array) the same as "empty response"? Phase 1 `ErrEmptyResponse` godoc says "non-empty payload was required" — could be read either way.
   - RESOLVED: Recommendation: Phase 2's `Countries` accepts an empty array as a valid response (returns `[]Country{}, nil`). Document explicitly. `ErrEmptyResponse` is reserved for the "literal empty body" case (no JSON at all), which would surface as `json.Decoder.Decode` returning `io.EOF`. Wrap that as `ErrEmptyResponse`. The planner should add a `errors.Is(err, io.EOF) ⇒ ErrEmptyResponse` branch on Decode failure.

2. **Should `composeHTTPClient` honor `cfg.httpClient.CheckRedirect` and `cfg.httpClient.Jar`?**
   - What we know: D-37 specifies shallow-copy of `*cfg.httpClient`. A shallow copy preserves both fields verbatim.
   - What's unclear: Whether the test plan should explicitly cover "CheckRedirect on user client is preserved."
   - RESOLVED: Recommendation: Yes — add a one-case `t.Run` to `TestClient_WithHTTPClient` that sets a custom `CheckRedirect` and verifies it is invoked on a redirect from the httptest server. Cheap insurance.

3. **Should `parseAPIMessage` log a debug record when JSON parsing fails?**
   - What we know: D-43 says "best-effort parsed ... returns empty string when unparseable."
   - What's unclear: Whether operators benefit from a `slog.Debug("api error body unparseable", "bytes", len(body))` line.
   - RESOLVED: Recommendation: NO for Phase 2 — keep `parseAPIMessage` pure (no I/O, no logging). The `*APIError.Body` already carries the raw bytes; operators with `Debug` enabled see the bytes via the logging transport's response-status record anyway. Avoids extra log noise.

4. **`WithBaseURL` — trailing slash handling**
   - What we know: D-36 sets `defaultBaseURL = "https://openholidaysapi.org"` (no trailing slash). Endpoint method appends `"/Countries"`.
   - What's unclear: Whether `WithBaseURL("https://mirror.example.com/")` (with trailing slash) should be auto-trimmed or rejected.
   - RESOLVED: Recommendation: Auto-trim. The most caller-friendly behavior. Document in `WithBaseURL` godoc. Add a test case `WithBaseURL("https://example.test/")` → internal `baseURL` is `"https://example.test"`.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All Phase 2 production + test code | ✓ | go1.26.3 (matches ≥1.23 floor per Phase 1 verification) | — |
| testify v1.11.1 | Test files (Gold Rule 3) | ✓ | v1.11.1 (already in go.mod from Phase 1) | — |
| `net/http`, `net/http/httptest`, `context`, `encoding/json`, `log/slog`, `io`, `sync/atomic`, `time`, `runtime` | Phase 2 production + test code | ✓ | stdlib | — |
| `curl` (for fixture capture only) | One-time `testdata/countries.json` capture | ✓ | system curl (any) | `wget` or in-tree `go run scripts/fetch-countries.go` |
| Live `https://openholidaysapi.org` reachability | Fixture capture; never invoked from tests | ✓ | verified 2026-05-27 (HTTP/2 200, 6055 bytes, 89 ms) | If upstream is down on capture day, defer fixture capture until next day; Phase 2 does NOT block on it (the canonical 2-country shape is documented in the test pattern above and can be typed directly). |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** None — every dependency is stdlib or already in go.mod.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` stdlib + `github.com/stretchr/testify` v1.11.1 (Gold Rule 3) |
| Config file | None — `go test` reads CLI flags only |
| Quick run command | `go test -race ./...` |
| Full suite command | `go test -race -cover ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CLIENT-01 | `NewClient` returns usable client, never errors | unit | `go test -run TestNewClient ./...` | ❌ Wave 0 |
| CLIENT-02 | `WithHTTPClient` shallow-copies | unit | `go test -run TestWithHTTPClient ./...` | ❌ Wave 0 |
| CLIENT-03 | `WithBaseURL` overrides | unit | `go test -run TestWithBaseURL ./...` | ❌ Wave 0 |
| CLIENT-04 | `WithUserAgent` overrides | unit | `go test -run TestWithUserAgent ./...` | ❌ Wave 0 |
| CLIENT-05 | `WithLogger` injects | unit | `go test -run TestWithLogger ./...` | ❌ Wave 0 |
| CLIENT-06 | `WithTimeout` sets per-request timeout | unit | `go test -run TestWithTimeout ./...` | ❌ Wave 0 |
| CLIENT-07 | Goroutine-safe under -race | unit (concurrency) | `go test -race -run TestClient_ConcurrentAccess ./...` | ❌ Wave 0 |
| CLIENT-08 | `Close()` idempotent stub | unit | `go test -run TestClient_Close ./...` | ❌ Wave 0 |
| CLIENT-09 | Ctx cancellation within ≤ 100 ms | unit (timing) | `go test -run TestClient_ContextCancel ./...` | ❌ Wave 0 |
| ENDPT-01 | `Countries(ctx)` returns `[]Country` | unit (httptest) | `go test -run TestClient_Countries ./...` | ❌ Wave 0 |
| TRANS-01 | Accept + User-Agent headers sent | unit | `go test -run TestHeaderTransport_RoundTrip ./...` | ❌ Wave 0 |
| TRANS-02 | 10 MiB cap returns `ErrResponseTooLarge` | unit | `go test -run TestClient_Countries/oversize ./...` | ❌ Wave 0 |
| TRANS-03 | Body drained + closed on every path | unit (goroutine delta) | `go test -run TestClient_Countries/oversize ./...` | ❌ Wave 0 |
| TRANS-04 | Each RoundTripper unit-tested in isolation | unit | `go test -run "TestHeaderTransport\|TestLoggingTransport" ./...` | ❌ Wave 0 |
| OBS-01 | Debug-level logging; no body | unit (captured slog) | `go test -run TestLoggingTransport_RoundTrip ./...` | ❌ Wave 0 |
| OBS-02 | OBS-02 field set in every record | unit (captured slog) | `go test -run TestLoggingTransport_RoundTrip ./...` | ❌ Wave 0 |
| TEST-04 | N parallel calls under -race | unit (concurrency) | `go test -race -run TestClient_ConcurrentAccess ./...` | ❌ Wave 0 |
| (W-01 fix) | Unicode case-fold bypass rejected | unit (regression) | `go test -run TestValidateCountry ./...` and `TestValidateLanguage` | ⚠️ partial (file exists; cases to add) |

### Sampling Rate

- **Per task commit:** `go test -race ./...` (fast — full suite is small at this stage; under 2s expected)
- **Per wave merge:** `go test -race -cover ./...` (coverage gate ≥ 85% per TEST-10, though TEST-10 itself is Phase 5; tracking it now is good hygiene)
- **Phase gate:** `go test -race -cover ./... && go vet ./... && go build ./...` all green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `client.go` + `client_test.go` — TestNewClient, TestClient_Close, TestClient_ConcurrentAccess, TestClient_ContextCancel
- [ ] `options.go` + `options_test.go` — one TestXxx per WithX (5 functions = 5 tests)
- [ ] `config.go` (no public symbols — covered by `client_test.go` and integration via `countries_test.go`)
- [ ] `transport.go` + `transport_header_test.go` + `transport_logging_test.go`
- [ ] `countries.go` + `countries_test.go` — TestClient_Countries with happy + 4xx + 5xx + ctx-cancel + oversize subtests
- [ ] `testdata/countries.json` — 2-country PL+DE fixture
- [ ] `errors.go` extension — add `ErrResponseTooLarge` to existing var block + `TestErrResponseTooLarge_sentinel` regression test in `errors_test.go`
- [ ] `internal_test.go` modification — add `"ErrResponseTooLarge"` to `allowedVars`
- [ ] `validate.go` + `validate_test.go` — W-01 fix: reorder ASCII check; add 4 regression cases (KK, İ-prefix, ı-prefix, ſ-prefix)

*No framework install needed — `go test` is stdlib; testify is already in go.mod.*

## Security Domain

> Phase 2 inherits PROJECT.md's posture: `govulncheck` clean in CI, no secrets in repo, structured logging that never emits bodies above Debug. The phase introduces an HTTP client; the applicable ASVS categories are below.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | OpenHolidays is keyless / unauthenticated. |
| V3 Session Management | no | No session state in the SDK. |
| V4 Access Control | no | No multi-tenant or scoped resources. |
| V5 Input Validation | yes | Phase 1 validators + W-01 fix (D-32) — case-fold bypass closed; ASCII-only shape enforced before canonicalization. |
| V6 Cryptography | no (delegated) | TLS handled by `crypto/tls` via `net/http` (default verify; no custom certs). |
| V7 Error Handling | yes | `*APIError.Body` capped at 4 KiB (Phase 1 D-17); response logging never includes body above Debug (OBS-01). |
| V8 Data Protection | yes | No PII in OpenHolidays data; no secrets in logs (LOG-1 mitigation). |
| V12 Files / Resources | yes | `io.LimitReader` 10 MiB cap on response decode (TRANS-02); drain bound at `maxResponseBytes+1`. |
| V13 API & Web Service | yes | `Accept: application/json` + `User-Agent` headers set on every request (TRANS-01); RFC 7807 ProblemDetails parsed for `Message`. |

### Known Threat Patterns for Go HTTP SDKs

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Hostile / compromised upstream OOMs the client with a huge response | Denial of Service | `io.LimitReader(body, maxResponseBytes)` + sentinel-byte truncation detection (D-25/D-44). |
| Hostile upstream streams forever, blocking `Close()` drain indefinitely | Denial of Service | Drain bound: `io.Copy(io.Discard, io.LimitReader(body, maxResponseBytes+1))` (Pitfall HTTP-3 + D-45). |
| Hostile upstream returns a 4xx body containing a giant error envelope | Denial of Service / Information Disclosure | `*APIError.Body` capped at 4 KiB (Phase 1 D-17 / D-43). |
| Server omits `User-Agent` → CDN classifies as bot → 403/429 cascade | Repudiation | `headerTransport` always sets a UA matching `^go-openholidays/` (TRANS-01 + Pitfall HTTP-5). |
| Caller-supplied `*http.Client` later mutated → SDK silently inherits | Tampering | Shallow-copy of `*http.Client` in `composeHTTPClient` (Pitfall HTTP-1 + D-37). |
| `req.Header` race when RoundTripper mutates inbound request | Tampering | `req.Clone(req.Context())` in `headerTransport` (D-30 + Pitfall HTTP-2). |
| Caller cancellation observed > 100 ms late → wasted upstream work, possible upstream rate-limit signal | Denial of Service (self-inflicted) | `context.WithTimeout` per call + ctx propagation through entire chain (D-27 + CLIENT-09 + Pitfall CTX-2). |
| Validator accepts Unicode-folding to ASCII → wrong client-side reject | Tampering / Input Validation | W-01 fix (D-32): ASCII shape check BEFORE `ToUpper`/`ToLower`. |
| Response body in operator logs above Debug | Information Disclosure | `loggingTransport` never reads body; only emits status/duration/contentlength (D-31 + OBS-01 + Pitfall LOG-1). |

## Sources

### Primary (HIGH confidence)

- `pkg.go.dev/net/http` — `http.RoundTripper`, `Request.Clone`, `Request.WithContext`, `http.NewRequestWithContext`, `*http.Response.ContentLength` semantics. [VERIFIED via WebSearch + stdlib docs lookup, 2026-05-27]
- `pkg.go.dev/net/http/httptest` — `httptest.NewServer`, server cleanup pattern with `t.Cleanup`. [CITED]
- `pkg.go.dev/log/slog` — `slog.Default()`, `Logger.LogAttrs` performance characteristic. [CITED]
- `pkg.go.dev/io` — `io.LimitReader`, `io.Discard`, `io.Copy` semantics. [CITED]
- `pkg.go.dev/sync/atomic` — `atomic.Bool` typed wrapper (Go 1.19+). [CITED]
- `pkg.go.dev/encoding/json` — `json.NewDecoder` streaming semantics; `Decode` stops at end of first valid value. [CITED]
- `pkg.go.dev/context` — `context.WithTimeout`, ctx propagation through `http.Transport`. [CITED]
- `openholidaysapi.org/Countries` live response — verified 2026-05-27 via `curl` from the dev machine. HTTP/2 200, 6055 bytes, application/json, 36 countries, PL and DE entries verified. [VERIFIED live]
- `openholidaysapi.org/swagger/v1/swagger.json` — verified via WebFetch 2026-05-27: `/Countries` returns array of `CountryResponse` with required `isoCode`, `name`, `officialLanguages`; 400 + 500 use RFC 7807 `ProblemDetails` (`type, title, status, detail, instance`). [VERIFIED]
- `.planning/research/ARCHITECTURE.md` — Patterns 1 (Functional Options), 2 (RoundTripper chain), 6 (error layering). [CITED in-repo]
- `.planning/research/PITFALLS.md` — HTTP-1..6, CTX-1..3, JSON-1..4, OBS-1, LOG-1..2, TEST-1..2, CONC-1..2. [CITED in-repo]

### Secondary (MEDIUM confidence)

- google/go-github PR #805 — "RoundTrip: avoid modifying the original request" — confirms `req.Clone(req.Context())` canonical idiom. [github.com/google/go-github/pull/805]
- golang/go#49521 — `net/http` Timeout produces unexpected "context canceled" on body.Close — drives D-26 not setting `httpClient.Timeout`. [github.com/golang/go/issues/49521]
- DataDog/dd-trace-go#1090 — "RoundTripper should not modify request" — reinforces stdlib contract. [github.com/DataDog/dd-trace-go/issues/1090]
- golang/go#39533 — "net/http: RoundTrip unexpectedly changes Request" — historical context for the contract. [github.com/golang/go/issues/39533]
- arashtaher.com — "Closing Response body in Go is not enough" — drain-then-close pattern background. [arashtaher.com/blog/closing-response-body-in-go-is-not-enough/]
- manishrjain.com — "TIL: Go Response Body MUST be closed, even if you don't read it" — Pitfall HTTP-2 background. [manishrjain.com/must-close-golang-http-response]
- leapcell.io — "Understanding and Managing the Go HTTP Request Body" — keep-alive reuse and drain. [leapcell.io/blog/understanding-and-managing-the-go-http-request-body]
- uber-go/goleak README — confirms goleak is the "gold standard" for goroutine leak detection in Go tests; runtime-delta is acceptable for simple cases (informs D-49 deferral). [github.com/uber-go/goleak]
- blog.cloudflare.com — "The complete guide to Go net/http timeouts" — defense-in-depth timeouts including the body close race. [blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/]

### Tertiary (LOW confidence — corroborating)

- groups.google.com/g/golang-nuts/c/-j6p12SSpXI — "Correct use of http.RoundTripper" — community discussion confirming `Clone` pattern.
- rednafi.com — "What canceled my Go context?" — ctx cancellation causes / inspection.
- engineering.grab.com — "Context Deadlines and How to Set Them" — context.WithTimeout patterns.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all stdlib, all verified against Go 1.23 docs + Phase 1 go.mod state.
- Architecture patterns: HIGH — every pattern locked by CONTEXT.md D-24..D-50 and corroborated by ARCHITECTURE.md.
- Pitfalls: HIGH — every Phase 2 pitfall is already in PITFALLS.md; the Phase 2-specific gotchas (HTTP/2 chunked → ContentLength=-1, Decode masks oversize sentinel, body drain ordering) are corroborated by stdlib docs and verified live.
- Test patterns: HIGH for unit tests; MEDIUM for the goroutine-leak delta technique (works for one-test scope; revisit if Phase 4's cache sweeper introduces more goroutines).
- Live API shape: HIGH — verified via direct curl on the day of research; fixture should be re-captured if more than 7 days elapse before Phase 2 lands.

**Research date:** 2026-05-27
**Valid until:** 2026-06-27 (one month — Go 1.23 + net/http behavior is highly stable; the live OpenHolidays response shape is the only fast-moving piece, hence the fixture-recapture recommendation).
