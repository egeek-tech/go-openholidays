# Phase 2: Transport - Context

**Gathered:** 2026-05-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver the HTTP transport layer that every subsequent endpoint phase will plug into:

- `Client` struct constructed via `NewClient(opts ...Option) *Client` (never returns an error).
- Functional `Option` pattern over an internal `clientConfig` builder: `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithLogger`, `WithTimeout`.
- `Client.Close() error` as an idempotent no-op stub (background goroutines arrive with the Phase 4 cache).
- Custom RoundTripper chain `req → headerTransport → loggingTransport → underlying http.Transport` composed in an unexported `buildTransport(cfg) http.RoundTripper`.
- `headerTransport` injects `Accept: application/json` + `User-Agent: go-openholidays/<Version>` on every request.
- `loggingTransport` emits structured `slog` records at `Debug` level with fields `method, path, status, duration_ms, attempt, bytes_in`. Never logs response bodies above `Debug`.
- `Countries(ctx context.Context) ([]Country, error)` as the first endpoint — proves NewClient → chain → decode → typed return end-to-end against `httptest.Server`.
- 10 MiB response cap via `io.LimitReader`; oversized responses return a new exported sentinel `ErrResponseTooLarge` (CL-07).
- Per-request `context.WithTimeout(ctx, c.timeout)` inside each endpoint method to honor CLIENT-09's ≤ 100 ms cancellation guarantee.
- Goroutine safety verified by `TestClient_ConcurrentAccess` under `-race`.
- Fold in a small fix plan for Phase 1's W-01 (validator Unicode case-fold bypass) since validators must be sound before Phase 3 wires them into endpoint dispatch.

What this phase does NOT deliver: retry, cache, `WithRequestHook`, observability MetricsHook, additional endpoints (Languages, Subdivisions, PublicHolidays, SchoolHolidays), helpers (`Holiday.Name`, `IsInRegion`, `Range`), CLI, CI workflows, release tooling. All of those depend on the transport contract being stable.

</domain>

<decisions>
## Implementation Decisions

### A1 — Oversized-body error (CL-07)

- **D-24:** A new exported sentinel `ErrResponseTooLarge` is added to `errors.go`. Phase 2's transport returns this wrapped via `%w` (with the actual byte count in the message) when `io.LimitReader` truncates a response above 10 MiB. Callers branch via `errors.Is(err, openholidays.ErrResponseTooLarge)`. This expands the public sentinel surface from 5 to 6 — must be recorded as **CL-07** in PROJECT.md `Key Decisions` before Phase 2 closes. Same rationale as CL-01: distinct, recoverable failure mode (caller may retry against a mirror, or skip-and-continue) deserves a distinct `errors.Is`-discoverable identity.
- **D-25:** The 10 MiB ceiling lives as an unexported `const maxResponseBytes = 10 << 20` in `transport.go`. Not configurable in v0.1.0 (PROJECT.md cap, locked).

### A2 — Timeout enforcement

- **D-26:** `WithTimeout(d time.Duration) Option` sets `cfg.timeout` only; `cfg.httpClient.Timeout` is left at the caller's value (default 0 = no Go-level timer).
- **D-27:** Every endpoint method wraps the incoming `ctx` with `ctx, cancel := context.WithTimeout(ctx, c.timeout); defer cancel()` immediately after the nil-ctx guard. `http.NewRequestWithContext(ctx, ...)` then propagates ctx through the RoundTripper chain. Caller-supplied tighter deadlines compose naturally (the shorter of caller's ctx and `c.timeout` wins).
- **D-28:** Default `cfg.timeout = 15 * time.Second` (CLIENT-06; PROJECT.md). `WithTimeout(0)` is interpreted as "no SDK-imposed timeout" — caller's ctx is the only deadline.

### A3 — Transport scaffold scope (minimal)

- **D-29:** `buildTransport(cfg *clientConfig) http.RoundTripper` returns exactly:
  ```
  req → headerTransport{next: loggingTransport{next: underlying}} → underlying
  ```
  Where `underlying = cfg.httpClient.Transport` if non-nil else `http.DefaultTransport`. No retry, cache, hook, or observability transports — those are added in Phase 3 (retry) and Phase 4 (cache + hook) by editing `buildTransport` directly. Pre-1.0 phases prefer one constructor edit over middleware-list abstraction.
- **D-30:** `headerTransport.RoundTrip` clones `req` via `req.Clone(req.Context())` before mutating headers (Pitfall HTTP-2 — never mutate the caller's `*http.Request`). Sets `Accept: application/json` and `User-Agent: go-openholidays/<Version>` only when the caller hasn't already supplied them (caller override wins).
- **D-31:** `loggingTransport.RoundTrip` wraps the round-trip in `start := time.Now()` and emits a single `slog.LogAttrs` Debug record with the OBS-02 fields (`method, path, status, duration_ms, attempt, bytes_in`). `attempt` is hardcoded to `1` in Phase 2 (the retry transport supplied in Phase 3 will increment via context-attached counter). `bytes_in` is `resp.ContentLength` when known (>= 0), else `-1`. Response body is **never** read inside `loggingTransport` (would interfere with downstream decoding).

### A4 — Phase 1 W-01 fix folded into Phase 2

- **D-32:** A dedicated plan in Phase 2 (sequenced after the transport plans) fixes `validate.go`'s Unicode case-fold bypass:
  - Reorder: ASCII-shape check (regex `^[A-Za-z]{2}$`) runs **before** `strings.ToUpper` / `strings.ToLower` canonicalization, not after.
  - The shape predicate uses byte-level `b >= 'A' && b <= 'Z'` / `b >= 'a' && b <= 'z'` checks (no `unicode` package) to guarantee ASCII-only.
  - Extend `validate_test.go` tables with these cases that previously passed: `"KK"` (U+212A x2), `"İ"` + 1 char, `"ı"` + 1 char, `"ſ"` + 1 char (all should now reject). Add a permanent regression case for each.
- **D-33:** No CL row for the W-01 fix — it is a defect fix against the locked VALID-01/04 contract (the godoc already says "non-ASCII rejected"; the implementation now matches). PROJECT.md `Key Decisions` does not need a row.
- **D-34:** This plan does NOT touch the other Phase 1 advisory follow-ups (W-02, W-03, W-04). They remain on the backlog with documented status in `01-VERIFICATION.md` `follow_ups`.

### Client surface, defaults, and lifecycle

- **D-35:** `type Option func(*clientConfig)` (per ARCHITECTURE.md Pattern 1). Each `WithX` returns one. `NewClient` applies them to a fresh `clientConfig`, then constructs an immutable `Client`. Options never touch `Client` after construction.
- **D-36:** `defaultBaseURL = "https://openholidaysapi.org"` (PROJECT.md; ARCHITECTURE.md). No environment-variable override (ARCHITECTURE.md explicitly rejects `init()` reading env). Callers who want a mirror use `WithBaseURL`.
- **D-37:** `WithHTTPClient(c *http.Client) Option` performs a shallow copy of `*c` inside `composeHTTPClient(cfg)` (Pitfall HTTP-1: hidden mutability via `*http.Client` pointer). The Client wraps the copy's `Transport` with the chain from D-29. Caller mutations of the original `*http.Client` are no longer visible to the SDK.
- **D-38:** `WithUserAgent(s string)` overrides the default `go-openholidays/<Version>`. Empty string is treated as "use default" (no-op), not "send empty UA" — empty UAs are bad citizenship and some CDNs reject them.
- **D-39:** `WithLogger(l *slog.Logger)` defaults to `slog.Default()` when not supplied or when nil is passed. The library never calls `slog.SetDefault` (forbidden — would mutate global state of consuming applications).
- **D-40:** `Client.Close() error` is implemented as a struct method that flips a single `closed atomic.Bool` and returns `nil` always. Safe to call from any goroutine; idempotent (subsequent calls also return `nil`). Phase 4's cache will hook a `sync.Once` + sweeper-goroutine cancel onto this method.

### Countries endpoint

- **D-41:** `(c *Client) Countries(ctx context.Context) ([]Country, error)`. Signature matches ENDPT-01 verbatim. The internal HTTP request is `GET <baseURL>/Countries` with no query parameters — callers receive the full multi-language `Name` arrays and use `Country.NameFor(lang)` (Phase 1 D-23/CL-05) to localize.
- **D-42:** Error handling order inside `Countries`: nil-ctx guard (return `errors.New("openholidays: nil context")` — defensive, not a sentinel), then `context.WithTimeout` wrap (D-27), then `http.NewRequestWithContext`, then `c.http.Do`, then defer-drain-and-close, then status-code check, then JSON decode.
- **D-43:** On `resp.StatusCode >= 400` the method constructs `*APIError{StatusCode, Path: "/Countries", Body, Message}` where `Body` is the truncated upstream body (capped at 4 KiB per Phase 1 D-17) and `Message` is best-effort parsed from upstream `{"error": ...}` / `{"detail": ...}` / `{"title": ...}` shapes (returns empty string when unparseable). Returns `nil, apiErr` directly (not wrapped — APIError is a leaf type per D-16).
- **D-44:** On `2xx` the method uses `json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&countries)`. Lenient default per Phase 1 JSON-1 decision (strict decoding ships in Phase 4 via `WithStrictDecoding(true)`).
- **D-45:** Drain-and-close pattern (Pitfall HTTP-3): `defer func() { io.Copy(io.Discard, resp.Body); resp.Body.Close() }()` placed immediately after the `c.http.Do(req)` call returns no error. Drain MUST run before Close (single-line defer is fine).

### Test architecture

- **D-46:** All Phase 2 HTTP tests use `httptest.NewServer` with hand-crafted `http.HandlerFunc` per case. No live network in tests. A canonical 2-country fixture (`testdata/countries.json` containing at least PL and DE entries from the live upstream as of 2026-05-27) backs `TestClient_Countries` happy-path.
- **D-47:** `TestClient_ConcurrentAccess` (TEST-04) runs 50 parallel `Countries` calls against a `httptest.Server` that synthetically delays each response by 5-20ms. Race detector active. Asserts all 50 return identical typed payloads and no errors.
- **D-48:** `TestClient_ContextCancel` (CLIENT-09) creates a server that hangs for 10s on the response, then calls `cancel()` 50ms in, asserts `errors.Is(err, context.Canceled)` and total elapsed < 200 ms (giving 100 ms target a 2x slack for CI machine variance).
- **D-49:** `TestClient_OversizedResponse` (TRANS-02 + D-24) starts a server that streams 11 MiB of valid JSON, asserts the returned error satisfies `errors.Is(err, ErrResponseTooLarge)`. Also verifies the response body was drained + closed (no leaked goroutines per `goleak`-style sentinel — use `runtime.NumGoroutine` delta or the actual `go.uber.org/goleak` if approved as a test dep; recommend the runtime delta approach to avoid adding a test dep).
- **D-50:** Per-RoundTripper unit tests live alongside transport code: `transport_header_test.go` and `transport_logging_test.go`. Each tests the RoundTripper in isolation via a tiny `roundTripperFunc` adapter for the `next` slot.

### Claude's Discretion

The following are inferred from already-locked architecture and conventions; no need to re-ask:

- File layout: `client.go` (Client struct + NewClient + Close), `options.go` (Option + WithX), `config.go` (clientConfig + composeHTTPClient + buildTransport), `transport.go` (headerTransport + loggingTransport), `countries.go` (Countries endpoint method).
- Test files: `client_test.go`, `options_test.go`, `transport_header_test.go`, `transport_logging_test.go`, `countries_test.go`, plus existing `_test.go` files extended where needed.
- Every exported symbol gets a godoc starting with the symbol name (PROJECT.md / Gold Rule 1).
- Error message strings start with `"openholidays: "` (Phase 1 convention).
- All Go tests use `testify` (assert + require) with one `TestXxx` per exported prod function and every case wrapped in `t.Run` (Gold Rule 3).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project baseline (read first)
- `.planning/PROJECT.md` — locked constraints (zero runtime deps, 10 MiB cap, default timeout 15s, slog Debug-only HTTP logging, ≤ 100 ms ctx cancellation), Key Decisions table (CL-01..CL-06; CL-07 will be added by this phase).
- `.planning/REQUIREMENTS.md` — Phase 2 owns: CLIENT-01..09 (9), ENDPT-01 (1), TRANS-01..04 (4), OBS-01..02 (2), TEST-04 (1) = 17 requirements.
- `.planning/ROADMAP.md` §"Phase 2: Transport" — goal + 5 success criteria.
- `.planning/STATE.md` — running ledger of decisions inherited from prior phases.
- `.planning/codebase/CONVENTIONS.md` — Gold Project Rules.

### Architecture and patterns (read before writing transport)
- `.planning/research/ARCHITECTURE.md` §"Pattern 1: Functional Options with Internal Builder" (lines 209-264) — Option signature, NewClient flow, shallow-copy fix for WithHTTPClient mutability.
- `.planning/research/ARCHITECTURE.md` §"Transport Layer (RoundTripper decorator chain)" (lines 55-90, 135-137) — chain order, per-transport file layout, buildTransport composer.
- `.planning/research/ARCHITECTURE.md` §"What people do" (line 975) — explicit rejection of `init()` reading env vars for defaults.
- `.planning/research/STACK.md` §"Core Technologies" — net/http stdlib, log/slog, encoding/json v1 baseline.

### Pitfalls (read before writing transport code)
- `.planning/research/PITFALLS.md` §"Pitfall HTTP-1: hidden mutability via *http.Client" — drives D-37's shallow-copy.
- `.planning/research/PITFALLS.md` §"Pitfall HTTP-2: mutating caller's *http.Request" — drives D-30's req.Clone.
- `.planning/research/PITFALLS.md` §"Pitfall HTTP-3: response body drain + close" — drives D-45's defer pattern.
- `.planning/research/PITFALLS.md` §"Pitfall HTTP-4: unbounded JSON decode" — drives the 10 MiB cap.
- `.planning/research/PITFALLS.md` §"Pitfall OBS-1: response body in logs" — drives D-31's no-body-read rule.

### Phase 1 anchors (read for state inherited from Phase 1)
- `.planning/phases/01-foundation/01-CONTEXT.md` — D-01..D-23 + CL-01..CL-06 (decisions Phase 2 builds on, especially D-13..D-19 errors, D-05..D-12 Date, D-14 APIError shape).
- `.planning/phases/01-foundation/01-VERIFICATION.md` §"follow_ups" — W-01 (Unicode bypass, Phase 2 fixes), W-02 (version.go ldflags doc), W-03 (AST audit skips internal/), W-04 (TestValidators_NoSensitiveData shape). Only W-01 is addressed in this phase.
- `.planning/phases/01-foundation/01-REVIEW.md` — full advisory review.
- Phase 1 source: `errors.go` (5 sentinels + APIError type), `date.go` (Date wrapper), `validate.go` (validators to be hardened in W-01 plan), `types.go` (Country struct + NameFor accessor), `version.go` (Version const).

### Upstream API (verify shape before writing Countries decode)
- `https://openholidaysapi.org/swagger/v1/swagger.json` — Researcher confirmed live on 2026-05-27 from Phase 1 research. Re-verify the `/Countries` response envelope for Phase 2: should be `[]CountryResponse` where `CountryResponse` has fields `isoCode` (string, ISO 3166-1 alpha-2), `name` ([]LocalizedText), `officialLanguages` ([]string). Test fixture `testdata/countries.json` MUST contain real upstream payloads (at least PL and DE) captured during this phase.

### Gold Project Rules (apply everywhere)
- `CLAUDE.md` §"Project Rules (Gold)" — Rule 1 (English-only), Rule 2 (verify-or-ask), Rule 3 (testify + t.Run + one-test-per-prod-function).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (from Phase 1)

- `errors.go` — Phase 2 extends this file with the new `ErrResponseTooLarge` sentinel (D-24). The 5 existing sentinels remain untouched; the new var is appended to the existing `var (...)` block.
- `errors.go` — `*APIError` type, `Error()`, `Is()` already exist (Phase 1 D-14, D-15, D-18). Phase 2 is the first phase that **constructs** `*APIError` from a real `*http.Response` (Phase 1 only constructed literals in tests per D-19).
- `validate.go` — `validateCountry`, `validateLanguage`, `validateDateRange` are unexported. None of them are called from `Countries(ctx)` (no params) but the W-01 fix plan hardens them in place ahead of Phase 3.
- `types.go` — `Country` struct exists with verified upstream field names. Phase 2 decodes `[]Country` directly. `Country.NameFor(lang)` accessor (CL-05) is already in place.
- `version.go` — `const Version = "0.1.0"` powers the default User-Agent (D-30 reads `openholidays.Version`).
- `date.go` — Phase 2 does not import or use `Date` directly (Countries response has no date fields). Phase 3+ holiday endpoints will.

### Established Patterns (continue using)

- testify-based tests (Gold Rule 3): one `TestXxx` per exported prod function, every case in `t.Run`, `require` for preconditions, `assert` for verifications.
- English-only invariant (Gold Rule 1) — testdata fixtures may contain non-English strings only when they reflect real upstream responses (e.g., a Polish country name).
- Error message convention — every error string starts with `"openholidays: "` (Phase 1 D-23).
- File layout: production source at repo root, tests as `*_test.go` siblings, no `internal/` subpackage until clearly justified (Phase 1 W-03 left the AST audit's `internal/` skip in place — that bug remains until W-03 is addressed in a later phase).

### Integration Points

- Phase 2's `Client` and `Option` types are consumed by every Phase 3 endpoint method (`Languages`, `Subdivisions`, `PublicHolidays`, `SchoolHolidays`) — the signature `(c *Client) X(ctx context.Context, ...) (..., error)` is the contract Phase 3 must follow.
- Phase 3 will add `retryTransport` to `buildTransport`'s chain (after `loggingTransport`, before `underlying`).
- Phase 4 will add `cacheTransport` and `hookTransport` similarly.
- The `attempt` slog field (D-31) is hardcoded to 1 in Phase 2 and bumped via a context value `ctxKeyAttempt{}` by Phase 3's retry transport.
- `*APIError.Body` 4 KiB cap (Phase 1 D-17) is enforced inside the Phase 2 endpoint method when building APIError from `resp.Body` — phrase as `bodyTrunc := make([]byte, 4096); n, _ := io.ReadFull(resp.Body, bodyTrunc); apiErr.Body = bodyTrunc[:n]`.

</code_context>

<specifics>
## Specific Ideas

- The W-01 fix plan should arrive last in the Phase 2 plan sequence, after the transport scaffolding and Countries plans. Reason: the transport work is the load-bearing piece; the validator fix is a small known-quantity hardening that should not block the riskier transport work from starting in Wave 1.
- `testdata/countries.json` should be captured from the live upstream on the day the test fixture lands, via a small `make` target or one-shot `curl` command. Document the capture date in a comment at the top of the fixture (Go's JSON parser ignores comments, so this needs to be a sibling `testdata/countries.json.README` or a Go const in the test file referencing the date). Recommend a Go const in `countries_test.go`: `const countriesFixtureCapturedAt = "2026-05-27"`.
- Phase 2's `composeHTTPClient(cfg) *http.Client` should be unit-testable in isolation: pass a `clientConfig`, get back a `*http.Client`, verify its `Transport` is the expected chain via type-assertion walks.
- The 5 existing exported sentinels + 1 new `ErrResponseTooLarge` = 6 exported sentinels. Phase 1's `TestNoInitOrGlobalState` allowlist (Phase 1 D-25 / Plan 06) is currently a hardcoded list of 5 + `errEmptyDate`. The W-01 fix plan or a sibling plan must extend that allowlist to include `ErrResponseTooLarge`, otherwise the CLIENT-10 AST audit fails after Phase 2 lands.

</specifics>

<deferred>
## Deferred Ideas

- **Environment-driven base URL override** (e.g., `OPENHOLIDAYS_BASE_URL`) — explicitly rejected per ARCHITECTURE.md line 975 (no `init()` reading env). Callers wanting mirror support use `WithBaseURL`. May be revisited if a real ops need surfaces.
- **`WithMiddleware(mw Middleware) Option`** for pluggable transports — deferred indefinitely. Pre-1.0 surface stays narrow.
- **Configurable 10 MiB cap via option** — Out of scope; PROJECT.md fixes the cap.
- **Phase 1 follow-ups W-02 / W-03 / W-04** — remain on the `01-VERIFICATION.md` follow_ups list. W-02 is a documentation fix; W-03 needs reconsideration once a real `internal/` package lands; W-04 is a test-shape refactor with no behavioral impact. Address in a later cleanup phase (e.g., before tagging v0.1.0).
- **`runtime.NumGoroutine` vs `go.uber.org/goleak` for D-49's leak audit** — D-49 picks the runtime-delta approach to avoid adding `go.uber.org/goleak` as a test dep. If goroutine leaks become a recurring problem in later phases, revisit and add goleak as a test-only dep with a Key Decisions entry.
- **Logging body preview at Debug** — OBS-01 permits but does not require logging body at Debug. Phase 2 doesn't log bodies. Phase 4 may add a Debug-only `body_preview` (truncated to ~256 bytes) once that signal becomes useful for debugging upstream schema drift.
- **`WithRequestHook` (TRANS-05)** — Not in Phase 2's requirement list. Lands in Phase 4 alongside cache + hook.
- **`WithStrictDecoding(bool)` (OBS-03)** — Not in Phase 2. Lands in Phase 4.
- **Retry semantics (RESIL-*)** — Phase 3 territory. Phase 2 leaves `attempt: 1` hardcoded in `loggingTransport`.

</deferred>

---

*Phase: 02-transport*
*Context gathered: 2026-05-27*
