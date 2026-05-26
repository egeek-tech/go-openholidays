# Stack Research

**Domain:** Go HTTP/JSON client SDK (OSS library, zero runtime dependencies)
**Researched:** 2026-05-26
**Confidence:** HIGH (most recommendations align with official Go stdlib or established 2025/2026 community defaults; confidence levels noted per recommendation)

## Executive Summary

For a small, dependency-light Go HTTP client SDK in 2025/2026, the standard
Go community answer is: **lean on the standard library**. Functional options
for construction, custom `http.RoundTripper` for cross-cutting concerns,
`context.Context` everywhere, `encoding/json` v1 (with `GOEXPERIMENT=jsonv2`
optionally available in tests for Go 1.25+), `log/slog` for logging, a small
hand-rolled retry loop with full jitter, and a `sync.Map`/`sync.RWMutex`-based
TTL cache. For the demo CLI, `flag` is still acceptable in 2025/2026 for a
genuinely small surface, with `cobra` as the upgrade path only if the CLI
grows. Tooling: `golangci-lint` v2.x, `goreleaser` v2.x, `govulncheck`
latest, GitHub Actions matrix across Go 1.22 / 1.23 / stable. `iter.Seq`
(Go 1.23+) is increasingly idiomatic for stream-style helpers in libraries
that already require Go 1.23+, and a single helper method exposing
`iter.Seq[time.Time]` is well-justified given this project's stated Go 1.22+
floor and 1.23 CI cell.

This stack honors the project's zero-runtime-dep constraint with **only one
test-only dependency required**: `github.com/google/go-cmp` (already
pre-approved in PROJECT.md) for deep-equal diffing in tests. Everything else
ships from the standard library.

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go (toolchain) | 1.22, 1.23, `stable` matrix; module declares `go 1.22` | Language runtime, stdlib | PROJECT.md fixes Go 1.22 floor to keep one version of backwards compat; `iter.Seq` features (Go 1.23+) gated by build tags or simply guaranteed by the requirement that one helper method needs Go 1.23 at compile time. **HIGH confidence** — fixed by the project brief. |
| `net/http` | stdlib | HTTP transport | The community-default HTTP client. No reason to pull in `fasthttp`, `resty`, or `go-resty` for a small SDK: the stdlib client is fast enough, integrates with `context.Context`, and pulling third-party clients is a recurring complaint among Go SDK reviewers in 2025. **HIGH confidence**. |
| `encoding/json` (v1) | stdlib | JSON marshaling/unmarshaling | Zero-dep mandatory. v1 is fully sufficient for this API (small response shapes, no hot-path serialization). Go 1.25 ships `encoding/json/v2` as an opt-in experiment via `GOEXPERIMENT=jsonv2`; **do not** require v2 — v1 remains the API the library targets. Optionally add a benchmark cell with `GOEXPERIMENT=jsonv2` to measure when v2 lands stably (likely Go 1.26). **HIGH confidence**. |
| `context` | stdlib | Cancellation & deadlines | Every exported endpoint method takes `ctx context.Context` as first param. Use `http.NewRequestWithContext` (never `http.NewRequest` + `req.WithContext` — the former is preferred since Go 1.13). **HIGH confidence**. |
| `log/slog` | stdlib (Go 1.21+) | Structured logging | The 2025 Go answer. No external logger needed. Library accepts `*slog.Logger` via `WithLogger(...)`; defaults to `slog.Default()`. Library code must **never** log full response bodies (PROJECT.md explicitly prohibits this above Debug). **HIGH confidence**. |
| `time` | stdlib | Time and date handling | `time.Time` is the underlying type for the custom `Date` wrapper. **HIGH confidence**. |
| `errors` | stdlib | Sentinel + wrapping | `errors.Is` / `errors.As` for typed errors per PROJECT.md. Use `fmt.Errorf("%w", err)` for wrapping; declare sentinels as `var ErrX = errors.New("...")`. **HIGH confidence**. |
| `iter` (Go 1.23+) | stdlib | Range-over-func helpers | Used for `Holiday.Range() iter.Seq[time.Time]` per PROJECT.md. Requires Go 1.23 at build for that method. Strategy: either (a) bump module's `go` directive to `1.23` (drops 1.22 from compile target but matches the CI matrix's reality), or (b) keep `go 1.22` and put the `iter.Seq` helper behind `//go:build go1.23` build tag. Recommend **(a)** for simplicity — Go 1.22 has been off mainline support for over a year by 2026-05. **MEDIUM confidence** (depends on user's tolerance for that go-directive bump; flag this for the roadmap phase). |
| `flag` | stdlib | CLI argument parsing for `cmd/ohcli` | The demo CLI is small (two subcommands per PROJECT.md). Stdlib `flag` works with manual subcommand dispatch (`os.Args[1]` switch + per-subcommand `flag.NewFlagSet`). This is the zero-dep correct choice. If the CLI grows beyond ~4 subcommands, revisit `cobra` — but not for v0.1.0. **HIGH confidence** (stdlib `flag` remains acceptable for small CLIs in 2025/2026; it's only "limiting" when POSIX-style `--long=value`, command trees, or auto-generated help/man pages are needed). |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/google/go-cmp/cmp` | latest (v0.6.x or newer) | Deep-equal diffing in tests | **Test-only.** Pre-approved in PROJECT.md. Use `cmp.Diff(want, got)` for table-driven tests producing readable diffs. `reflect.DeepEqual` is acceptable but produces unreadable failure output; `cmp` is the de-facto Go testing standard in 2025. **HIGH confidence**. |

**That is the entire dependency graph.** No production deps. One test dep.
This is unusual and a selling point for the library.

#### Libraries explicitly NOT added (with rationale)

| Library | Why not |
|---------|---------|
| `github.com/cenkalti/backoff/v4`, `github.com/avast/retry-go` | Retry logic for this library is ~80 lines of code (exponential backoff + full jitter + `Retry-After` parsing + ctx cancellation). Pulling a dep for that adds supply-chain surface, conflicts with PROJECT.md's zero-runtime-dep rule, and surrenders control over the exact retry semantics consumers see. **HIGH confidence — write our own.** |
| `github.com/hashicorp/go-retryablehttp` | Same as above, plus it wraps the `*http.Client` and obscures the user's own transport chain — bad for a library that exposes `WithHTTPClient`. **HIGH confidence.** |
| `github.com/spf13/cobra`, `github.com/urfave/cli` | Demo CLI has two subcommands. Stdlib `flag` handles this cleanly. Adding `cobra` would pull `spf13/pflag`, `inconshreveable/mousetrap` (Windows-only but always in graph) and several others. **HIGH confidence.** |
| `github.com/bytedance/sonic`, `github.com/goccy/go-json`, `github.com/mailru/easyjson` | Faster than stdlib but with cost: `sonic` requires CGO on some paths and amd64-favoring assembly; `go-json` has had subtle correctness regressions historically; `easyjson` requires code generation. JSON parsing is not the bottleneck for an SDK that calls a public REST API across the public internet — network is. **HIGH confidence — stdlib `encoding/json` is correct.** |
| `cloud.google.com/go/civil`, `github.com/jjeffery/civil`, `github.com/rickb777/date` | All solve "I want a date type without timezone". Each is well-designed, but each is a runtime dependency. Project's `Holiday.Date` field is parsed from `YYYY-MM-DD` strings — a 10-line custom `Date` type wrapping `time.Time` with `MarshalJSON`/`UnmarshalJSON` and `String()` is sufficient. Document that internal representation is `time.Time` at midnight UTC for ergonomics with `time.Date`/`time.Before`/etc. **HIGH confidence.** |
| `github.com/stretchr/testify` | Convenient `assert.Equal` etc., but a heavy dep tree and the Go community has been moving away from it in 2025 in favor of `cmp.Diff` + plain `t.Errorf`. **HIGH confidence — stick with stdlib `testing` + `cmp`.** |
| `github.com/patrickmn/go-cache`, `github.com/coocood/freecache`, `VictoriaMetrics/fastcache` | TTL cache patterns are simple — `sync.RWMutex` + `map[string]entry{value, expiresAt}` + janitor goroutine is ~50 lines. Pulling an external cache surrenders control over eviction policy and TTL semantics. **HIGH confidence — write our own.** |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `golangci-lint` | Aggregated linter runner | Use **v2.x** (released March 2025, current stable as of May 2026 is ~v2.12.x). Migration from v1 is one-command (`golangci-lint migrate`). Configure `.golangci.yml` with `version: "2"`, `linters.default: standard` plus the PROJECT.md-required additions: `govet`, `errcheck`, `staticcheck`, `gosec`, `revive`, `gocritic` (note: `staticcheck` is in the `standard` default set in v2, so this is mostly explicit-enabling for `gosec`, `revive`, `gocritic`). **HIGH confidence**. |
| `govulncheck` | Vulnerability scanner | `go install golang.org/x/vuln/cmd/govulncheck@latest` in CI. Run as `govulncheck ./...`. Only reports vulnerabilities the binary's call graph actually reaches, which keeps the signal-to-noise ratio high. **HIGH confidence**. |
| `goreleaser` | Release pipeline (CLI binaries only — library doesn't need it) | Use **v2.x** (current stable as of May 2026 is ~v2.15.x). `.goreleaser.yaml` with `version: 2`. Cross-compile cmd binaries for linux/darwin/windows × amd64/arm64, set `CGO_ENABLED=0` for static binaries, generate checksums + SLSA-style provenance. Library itself is consumed via `go get` and needs no release artifacts beyond the git tag. **HIGH confidence**. |
| `gofmt` / `gofumpt` | Code formatting | `gofmt` is the floor (required). `gofumpt` (stricter formatter, used as a `golangci-lint` linter) is optional but commonly added in 2025/2026 OSS Go libraries for consistency. Recommend enabling via `gofumpt` linter in `.golangci.yml`. **MEDIUM confidence** (community split — some projects skip it). |
| `go test -race -cover` | Race detector + coverage | Standard tool. PROJECT.md requires `-race`-clean. CI runs `go test -race -coverprofile=cover.out ./...`. **HIGH confidence**. |
| GitHub Actions | CI | Matrix on `go-version: [1.22.x, 1.23.x, stable]`. `actions/setup-go@v5` (current major). Cache modules with the action's built-in caching. **HIGH confidence**. |
| Dependabot or Renovate | Dep update bot | Trivially small dep graph (just `cmp` test-only + Actions versions), but worth turning on for security updates on `actions/setup-go` etc. **HIGH confidence**. |

## Installation

```bash
# Library has zero runtime dependencies — no `go get` needed for users beyond
# the library itself.

# In the repo, the test-only dep:
go get -t github.com/google/go-cmp/cmp@latest

# Dev tooling (one-time install per developer machine):
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/goreleaser/goreleaser/v2@latest      # only needed for releases
# Optional but recommended:
go install mvdan.cc/gofumpt@latest
```

In CI, these tools are typically installed via the official actions
(`golangci/golangci-lint-action`, `goreleaser/goreleaser-action`) which manage
version pinning.

---

## Detailed Pattern Recommendations

The questions in the research request span ten specific design points. Each
maps to a concrete pattern below.

### 1. `net/http` patterns (HIGH confidence)

**Client construction — functional Options.** Functional options are the
2025 Go community consensus for SDK constructors. They preserve a stable
`NewClient(opts ...Option) *Client` signature and let new features land
additively without breaking callers. Pattern:

```go
type Option func(*Client)

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }
func WithBaseURL(u string) Option          { return func(c *Client) { c.baseURL = u } }
// ...

func NewClient(opts ...Option) *Client {
    c := &Client{
        http:     &http.Client{Timeout: 15 * time.Second},
        baseURL:  "https://openholidaysapi.org",
        logger:   slog.Default(),
        ua:       "go-openholidays/" + version,
    }
    for _, opt := range opts { opt(c) }
    return c
}
```

Reject the "builder" pattern (chained `c.WithX().WithY()`) — less idiomatic in
Go, complicates the type once you add validation. Functional options can
return errors via a small variation (`type Option func(*Client) error`),
but it adds friction at every call site; prefer documenting invariants and
validating inside `NewClient` after applying options if needed.

**Context propagation.** Every endpoint method's first arg is
`ctx context.Context`. Always construct requests with
`http.NewRequestWithContext(ctx, ...)`. The stdlib client honors `ctx`
cancellation within roughly one TCP read interval, well within PROJECT.md's
100 ms target.

**Custom `RoundTripper` for retry/logging/hooks.** This is the right hook
point. `http.RoundTripper` middleware ("tripperware") is the 2025 idiomatic
pattern for cross-cutting HTTP concerns. Chain pattern:

```go
type roundTripperFunc func(*http.Request) (*http.Response, error)
func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func chain(base http.RoundTripper, middlewares ...func(http.RoundTripper) http.RoundTripper) http.RoundTripper {
    if base == nil { base = http.DefaultTransport }
    for i := len(middlewares) - 1; i >= 0; i-- { base = middlewares[i](base) }
    return base
}
```

Use middlewares for:
- **Headers** (User-Agent, Accept) — set once via a header-injecting RoundTripper.
- **Logging** (slog Debug-level method+URL+status+duration; never the body).
- **Hook** (`WithRequestHook` invokes user callback after each round trip).

**Do NOT** put retry in the RoundTripper for an HTTP client SDK that
exposes `WithHTTPClient`. Reason: if the user supplies their own
`*http.Client` with their own retry transport, doubling up creates
geometric retry storms. Retry belongs in the **endpoint method** wrapper,
above the HTTP client. This is a subtle but important point — see section 5.

**Response-body cleanup.** Canonical pattern, applied at every call site:

```go
resp, err := c.http.Do(req)
if err != nil { return ... }
defer func() {
    // Drain remaining body so the connection can be reused, then close.
    _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
    _ = resp.Body.Close()
}()

// Decode with a size cap to prevent resource exhaustion from a hostile server.
lr := io.LimitReader(resp.Body, 10<<20) // 10 MiB cap from PROJECT.md
dec := json.NewDecoder(lr)
if c.strict { dec.DisallowUnknownFields() }
```

The drain-then-close pattern ensures connection reuse for keepalive. Naive
`defer resp.Body.Close()` without draining leaks the connection back to the
pool in a "still has unread data" state, which sometimes prevents reuse.

### 2. JSON handling (HIGH confidence)

**Use stdlib `encoding/json` v1.** Tradeoffs accepted:

- **Speed:** `sonic` is ~5x faster on unmarshal, `go-json` ~1.4x faster.
  For an SDK calling a public REST API across the internet, JSON parse time
  is dwarfed by network RTT (typically 50–300 ms for openholidaysapi.org).
  Stdlib JSON at ~1 μs/holiday-record is invisible.
- **Reflection cost:** `encoding/json` uses reflection. For ~50 holidays per
  call, this is negligible. If the library ever moves to streaming millions
  of records, revisit.
- **Memory:** v1 allocates intermediate `interface{}` values. Same conclusion
  — not the bottleneck.

**Go 1.25's `GOEXPERIMENT=jsonv2`:** Worth a single optional CI benchmark
cell (e.g., `GOEXPERIMENT=jsonv2 go test -bench=. ./...`) to start
characterizing the migration cost. **Do not** require v2 — it's experimental
in 1.25 and the public surface targets Go 1.22.

**Strict decoding mode.** Wire `WithStrictDecoding(true)` to
`dec.DisallowUnknownFields()`. This is the standard mechanism, well-known to
Go developers, and exposes upstream schema drift early. The PROJECT.md POC
already observed optional fields appearing/disappearing across responses, so
strict mode is opt-in (default off) for forward compatibility.

**Date handling — custom `Date` wrapper (HIGH confidence).** Define:

```go
// Date is a calendar date in YYYY-MM-DD with no time-of-day or timezone.
type Date struct{ time.Time }

func (d *Date) UnmarshalJSON(b []byte) error {
    s := strings.Trim(string(b), `"`)
    t, err := time.Parse("2006-01-02", s)
    if err != nil { return fmt.Errorf("date: parsing %q: %w", s, err) }
    d.Time = t
    return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
    return []byte(`"` + d.Format("2006-01-02") + `"`), nil
}
```

Reasons over raw `time.Time`:
1. **Self-documenting at API boundary** — calling code reading `h.StartDate`
   sees `Date`, not `time.Time`, and knows there's no time-of-day component.
2. **Marshal symmetry** — round-trip `JSON → struct → JSON` produces
   `"2025-12-24"`, not `"2025-12-24T00:00:00Z"`.
3. **No external dep** vs `cloud.google.com/go/civil.Date` or
   `rickb777/date` — same value, no supply chain cost.

The wrapping-`time.Time` form (vs `civil.Date`'s `struct{ Year int; Month time.Month; Day int }`) makes arithmetic ergonomic: `d.AddDate(0, 0, 1)`, `d.Before(...)`, `d.Weekday()` all work directly.

### 3. Logging — `log/slog` (HIGH confidence)

**Library gotchas (must follow):**

1. **Accept a `*slog.Logger`, don't create one.** Library code does not
   choose handlers (text vs JSON); the caller does. Default to `slog.Default()`.
2. **Never log response bodies at `Info` or above.** PROJECT.md mandates this.
   At `Debug`, log method + URL + status + duration; if a body must be logged
   at all (for debugging a 4xx), log only at `Debug` and truncate to a
   small byte cap (e.g., 1 KiB).
3. **Log structured attrs, not formatted strings.** Use
   `logger.Debug("http call", "method", req.Method, "url", req.URL.String(), "status", resp.StatusCode, "duration_ms", dur.Milliseconds())`.
4. **Respect the level.** Library should never log at `Info` for normal
   operation. Reserve `Info` for one-shot lifecycle events the caller will
   actually want to see (which, for this SDK, is essentially nothing).
   `Warn` for retried-but-eventually-succeeded calls. `Error` for
   user-visible failures (and even then — the caller has the error return;
   re-logging it is redundant).
5. **Avoid `slog.SetDefault` from library code.** That mutates global state
   for the entire process, breaking everyone else's logging configuration.
6. **No `LogValuer` traps.** If you implement `slog.LogValuer` on internal
   types, make sure attributes accessed lazily don't perform network I/O.

**Performance note:** `slog` benchmarks at ~650 ns/op vs `zap` at ~420 ns/op.
At Debug-level logging frequency for an SDK (a few logs per HTTP call),
this is irrelevant. Trade the 50% throughput for stdlib status and zero dep.

### 4. Retry/backoff — hand-rolled (HIGH confidence)

**Decision: write a small retry loop.** ~80 lines including tests.

**Pattern:** retry only at the endpoint-method layer (not in RoundTripper),
so user-supplied `*http.Client` is unaffected.

```go
// Pseudocode skeleton.
func (c *Client) doRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
    var lastErr error
    for attempt := 0; attempt <= c.maxRetries; attempt++ {
        // Clone request so body (if any) can be replayed.
        r := req.Clone(ctx)
        resp, err := c.http.Do(r)

        // Success or non-retryable failure
        if err == nil && !isRetryable(resp.StatusCode) { return resp, nil }
        if err == nil { resp.Body.Close() }

        if !shouldRetry(err, resp) { return resp, err }
        lastErr = err

        delay := backoff(attempt, c.baseDelay, c.maxDelay)
        if resp != nil {
            if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
                delay = ra
            }
        }

        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(delay):
        }
    }
    return nil, lastErr
}
```

**Patterns:**

- **Full jitter** (not equal jitter, not decorrelated jitter): formula
  `delay = rand.Int63n(min(base * 2^attempt, cap))`. Industry literature
  (notably AWS Architecture Blog's "exponential backoff and jitter" post)
  concludes full jitter has the lowest contention and competition in
  simulations. It's also the simplest to code correctly.
- **`Retry-After` parsing** — accept both forms per RFC 7231:
  integer seconds (`Retry-After: 120`) and HTTP-date
  (`Retry-After: Wed, 21 Oct 2026 07:28:00 GMT`). Cap at a sane maximum
  (e.g., 60 s) so a hostile server can't ask us to sleep for an hour.
- **Retryable conditions:** network errors (`net.Error.Timeout()`, conn
  reset), HTTP `408`, `425`, `429`, `500`, `502`, `503`, `504`. Not `400`,
  `401`, `403`, `404` — those are client errors that won't change on retry.
- **`ctx` cancellation:** use `select { case <-ctx.Done(): ... case <-time.After(delay): }` — never bare `time.Sleep`, which is uninterruptible.
- **Idempotency:** all five endpoints are GETs and safely retryable. Document
  in code that retry is GET-safe and would need rework if POST/mutation
  endpoints are ever added.
- **Random source:** use `math/rand/v2` (Go 1.22+) for jitter — no seeding
  needed, goroutine-safe by default, removes the `rand.Seed` deprecation
  footgun from Go 1.20+. **HIGH confidence**.

### 5. In-memory TTL cache (HIGH confidence)

**Pattern: `sync.RWMutex` + `map[string]entry` + lazy expiry + periodic
janitor.** Choose over `sync.Map` because:

- `sync.Map` is optimized for write-once read-many or for goroutines that
  read-mostly-disjoint key sets. Our cache has bounded keys (per country
  code) and TTL-driven writes.
- `sync.Map` has no convenient iteration for the janitor; its `Range` works
  but is awkward to combine with type-asserted values.
- A plain map under `RWMutex` is easier to reason about, easier to add an
  LRU later if needed, and the lock contention argument doesn't matter for
  a low-frequency reference-data cache.

Skeleton:

```go
type cache struct {
    mu      sync.RWMutex
    items   map[string]cacheEntry
    ttl     time.Duration
    janitor *time.Ticker
    done    chan struct{}
}

type cacheEntry struct {
    value     any
    expiresAt time.Time
}

func (c *cache) Get(key string) (any, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    e, ok := c.items[key]
    if !ok || time.Now().After(e.expiresAt) { return nil, false }
    return e.value, true
}

func (c *cache) Set(key string, value any) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.items[key] = cacheEntry{value: value, expiresAt: time.Now().Add(c.ttl)}
}
```

The janitor goroutine sweeps every `ttl / 2` and is shut down via a
`Close()` method on Client. **Important:** if the cache holds a janitor
goroutine, the Client must expose a `Close()` so the caller can stop it
during shutdown — otherwise the goroutine outlives the Client and leaks.
Alternative: only check-on-read (no janitor). Drawback: stale entries
linger until next read of that key. For reference data (countries, languages,
subdivisions — never delete), check-on-read is fine and avoids the
`Close()` complication. **Recommend check-on-read** for v0.1.0 simplicity.

Generics (Go 1.18+) let you make this type-safe per cache (e.g.,
`cache[[]Country]`), but the cache is internal — `any` plus type assertion
at the few call sites is fine. **Use generics if it doesn't cost test
complexity**; otherwise it's a wash.

### 6. Testing (HIGH confidence)

**`httptest.NewServer`** for the unit tests — start a fake server that
returns canned JSON (loaded from `testdata/*.json` golden files) and point
the client at it. Pattern:

```go
func TestPublicHolidays(t *testing.T) {
    t.Parallel()

    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // assertions on r.URL.Query()
        w.Header().Set("Content-Type", "application/json")
        body, _ := os.ReadFile("testdata/public_pl_2025.json")
        w.Write(body)
    }))
    defer ts.Close()

    c := NewClient(WithBaseURL(ts.URL))
    got, err := c.PublicHolidays(context.Background(), ...)
    if err != nil { t.Fatal(err) }
    want := []Holiday{...}
    if diff := cmp.Diff(want, got); diff != "" {
        t.Errorf("PublicHolidays mismatch (-want +got):\n%s", diff)
    }
}
```

**Table-driven tests** are mandatory for the typed-error sentinels and the
validation logic (country code shape, date window size). Standard form:

```go
for _, tc := range []struct{ name, country string; wantErr error }{ ... } {
    tc := tc // shadow for parallel safety (still recommended even with Go 1.22 loop scoping fix)
    t.Run(tc.name, func(t *testing.T) {
        t.Parallel()
        _, err := c.PublicHolidays(ctx, tc.country, ...)
        if !errors.Is(err, tc.wantErr) { t.Errorf("...") }
    })
}
```

Note: Go 1.22 made loop variables per-iteration scoped, so `tc := tc` is
strictly speaking unnecessary on Go 1.22+. Keep it as defensive style for
clarity and for any older toolchain — but linters may flag it as redundant.
Recommendation: **drop the `tc := tc` shadow** if module `go` directive
is 1.22+; it's noise. **MEDIUM confidence** — purely style.

**Golden files** under `testdata/` (the magic directory name `testdata` is
ignored by `go build`). Capture real responses from POC runs, redact
nothing (this is a public API). Compare with `cmp.Diff`. Provide an update
flag pattern: `var update = flag.Bool("update", false, "...")` — run
`go test -update` to refresh fixtures after intentional schema changes.

**Fuzz tests** for the JSON parsers — PROJECT.md requires this. Stdlib
`testing.F.Fuzz` is the right tool (since Go 1.18). Fuzz the `Date.UnmarshalJSON`
and the `Holiday.UnmarshalJSON` entry points with garbage byte slices to
ensure no panic, no infinite loop, and graceful error returns.

**`testing/quick`** is older and less convenient than `testing.F.Fuzz` —
skip it.

**`t.Parallel()` etiquette:**
- Top-level tests: yes, parallel by default. Each unit test starts its own
  `httptest.Server` so there's no shared state.
- Subtests inside table-driven loops: yes, parallel, with proper variable
  capture as above.
- Tests that mutate global state (e.g., `os.Setenv`): **no parallel** —
  use `t.Setenv` (Go 1.17+) which auto-restores and refuses to run on a
  parallel test.

**`-race` requirement:** PROJECT.md mandates `-race`-clean. CI runs
`go test -race -coverprofile=cover.out ./...`. Set `count=1` in CI to defeat
the test cache for nightly runs.

**Coverage:** `≥ 85 %` per PROJECT.md. Realistic for an HTTP-stub-driven
test suite; nothing to special-case.

**Integration tests:** behind `//go:build integration` build tag and
`OPENHOLIDAYS_LIVE=1` env gate per PROJECT.md. Standard Go pattern. Run
on a nightly cron in CI; not on every PR.

**Benchmarks:** in `*_test.go` next to the code under test, named
`BenchmarkX`. Cold listing benchmark uses `httptest.Server` and disables
the cache. Cached benchmark hits the cache repeatedly. Both committed.

### 7. CLI in `cmd/ohcli` — stdlib `flag` (HIGH confidence)

**Pattern:**

```go
func main() {
    if len(os.Args) < 2 { usage(); os.Exit(2) }
    switch os.Args[1] {
    case "public": publicCmd(os.Args[2:])
    case "school": schoolCmd(os.Args[2:])
    case "-h", "--help": usage()
    default: usage(); os.Exit(2)
    }
}

func publicCmd(args []string) {
    fs := flag.NewFlagSet("public", flag.ExitOnError)
    region := fs.String("region", "", "optional subdivision code")
    // positional: country, year
    fs.Parse(args)
    // ...
}
```

This handles PROJECT.md's CLI shape (`ohcli public PL 2025`, `ohcli school PL 2025 --region PL-SL`) cleanly.

**Is stdlib `flag` still acceptable in 2025/2026?** Yes, for small CLIs.
The complaints about `flag` — no POSIX-style `--long`, no built-in
subcommands, no auto-help — are real but immaterial for two-subcommand
tools. The community has not "moved on" from `flag` so much as: large CLIs
(kubectl, helm, hugo) use cobra; tiny CLIs (most internal tools, demos,
example binaries in OSS libraries) use `flag`. This is in the "tiny" bucket.

If the CLI grows past four subcommands or needs nested commands (e.g.,
`ohcli countries list`), migrate to `cobra` — but cleanly, after the v0.1.0
ship.

### 8. Build/release — `goreleaser` v2 + `golangci-lint` v2 (HIGH confidence)

**`.goreleaser.yaml`** skeleton:

```yaml
version: 2
project_name: go-openholidays
builds:
  - id: ohcli
    main: ./cmd/ohcli
    binary: ohcli
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}
archives:
  - formats: [tar.gz]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        formats: [zip]
checksum:
  name_template: "checksums.txt"
changelog:
  sort: asc
  filters:
    exclude: ["^docs:", "^test:", "^chore:"]
```

Run via `goreleaser/goreleaser-action@v6` on `v*` tags. **The library
itself doesn't need GoReleaser** — `go get github.com/owner/go-openholidays@v0.1.0`
just uses the git tag. GoReleaser is purely for the CLI binary.

**`.golangci.yml`** skeleton (v2 format):

```yaml
version: "2"
run:
  timeout: 5m
linters:
  default: standard
  enable:
    - govet
    - errcheck
    - staticcheck
    - gosec
    - revive
    - gocritic
    - gofumpt
    - misspell
    - unused
  settings:
    gosec:
      excludes: [G104] # if needed; review case-by-case
    revive:
      rules:
        - name: exported
```

Note: `staticcheck`, `govet`, `errcheck`, `unused` are all in the
`standard` default in v2; listing them explicitly is harmless but
unnecessary. The minimum compliant set for PROJECT.md is what's shown above.

**GitHub Actions** matrix:

```yaml
strategy:
  matrix:
    go: ["1.22.x", "1.23.x", "stable"]
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-go@v5
    with: { go-version: ${{ matrix.go }} }
  - run: go vet ./...
  - run: go build ./...
  - run: go test -race -coverprofile=cover.out ./...
  - uses: golangci/golangci-lint-action@v6
    with: { version: v2.12 }
  - run: |
      go install golang.org/x/vuln/cmd/govulncheck@latest
      govulncheck ./...
```

### 9. `iter.Seq` / range-over-func (MEDIUM-HIGH confidence)

PROJECT.md already commits to `Holiday.Range() iter.Seq[time.Time]`.
Status of adoption in the broader Go ecosystem as of 2025/2026:

- **Stdlib has adopted it heavily:** `slices.All`, `slices.Values`,
  `maps.All`, `maps.Keys`, `maps.Values` (Go 1.23); `bytes.Lines`,
  `strings.Lines`, `strings.SplitSeq`, `strings.FieldsSeq` (Go 1.24);
  more in Go 1.25. This is no longer experimental at the stdlib level.
- **Third-party adoption is mixed but rising.** Newer libraries treat it
  as a first-class API; older libraries are sticking with slice-returning
  helpers until they ship a major version bump.
- **For this library:** exposing one `iter.Seq[time.Time]` helper
  (`Holiday.Range()`) is well-justified. It's the ergonomic Go-idiomatic
  way to iterate a date range without materializing a slice, and PROJECT.md
  lists it as a target feature.

Implication for module `go` directive: bumping to `go 1.23` is the cleanest
path. Alternative is `//go:build go1.23` on the file containing
`Holiday.Range()` to preserve 1.22 compile-target compatibility. The
roadmap should pick one explicitly; my recommendation is **bump to `go 1.23`**
because:
1. Go 1.22 left mainline support in early 2025; by May 2026 it's been
   off-support for over a year.
2. CI already tests 1.22 in the matrix — passing CI on 1.22 means the code
   compiles there even with `go 1.23` in `go.mod` (the `go` directive sets
   minimum semantic version, not compile floor for stdlib features
   below 1.23-introduced symbols).

Wait — that's nuanced. The `go` directive in `go.mod` (post-1.21) does
control which stdlib features the compiler will accept. With `go 1.23`,
the file using `iter.Seq` compiles fine; with `go 1.22`, it won't. So
either bump the directive or build-tag the file. **Recommend bump.**

### 10. Errors (mentioned in retry section, expanded here for completeness)

**Sentinels:** declare as package-level `var Err... = errors.New("...")`.
PROJECT.md lists `ErrInvalidCountry`, `ErrInvalidLanguage`,
`ErrDateRangeTooLarge`, `ErrEmptyResponse`. Compose for context via
`fmt.Errorf("country %q: %w", code, ErrInvalidCountry)` so callers can
`errors.Is(err, ErrInvalidCountry)` while still seeing the offending value.

**`*APIError`:** typed struct with `StatusCode`, `Path`, `Body` fields.
Implement `Error()`, optional `Unwrap()` if it embeds a cause. Callers use
`errors.As(err, &apiErr)` to inspect status. Cap `Body` capture at e.g.
1 KiB so an `APIError` doesn't drag a 10 MiB body into the heap.

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Functional options | Config struct (`type Config struct { Timeout time.Duration; ... }` passed to `New(cfg)`) | Fewer options (≤ 3), or when callers benefit from struct literal syntax with named fields. For an SDK with growing optionality, options scale better. |
| stdlib `flag` | `cobra` | CLI grows past 4 subcommands or needs nested commands, completion scripts, auto-generated man pages. |
| stdlib `encoding/json` | `encoding/json/v2` (Go 1.25 experiment) | When v2 stabilizes (likely Go 1.26 promotion); evaluate then. Today's experimental v2 is fine to benchmark but not require. |
| stdlib `encoding/json` | `sonic` / `go-json` | When JSON parse is provably the bottleneck (typically server-side hot paths processing 10K+ records/sec). Not this SDK. |
| Custom `Date` wrapping `time.Time` | `cloud.google.com/go/civil.Date` | If multiple types in the API needed civil dates and the consumer base was already on Google Cloud (the import path is well-known there). For zero-dep, hand-rolled wins. |
| Hand-rolled retry | `hashicorp/go-retryablehttp` | If the SDK ever needs to support arbitrary user-supplied retry policies pluggably. For one canonical policy with reasonable knobs, hand-roll. |
| `sync.RWMutex` + map cache | `sync.Map` cache | If access pattern shifts to many readers / very rare writes on disjoint keys (the workload sync.Map was designed for). Not this SDK. |
| `cmp.Diff` for tests | `testify/assert.Equal` | If the team is already on testify and migration cost outweighs benefit. New code in 2025: prefer `cmp`. |
| `log/slog` | `zap`, `zerolog` | When sub-microsecond logging perf matters (high-throughput servers). Not a client SDK with a few debug logs per HTTP call. |
| Custom RoundTripper chain | Middleware framework (e.g., `go-chi/chi` for clients) | These don't really exist for clients — chi is server-side. Custom chain is the idiom. |
| Hand-rolled small cache | `patrickmn/go-cache`, `karlseguin/ccache` | If LRU eviction or memory bounding becomes a requirement. v0.1.0 doesn't need either. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `http.Get` / `http.Post` package-level helpers | Don't propagate `ctx`; use `http.DefaultClient`; impossible to instrument | `http.NewRequestWithContext` + `c.http.Do(req)` |
| `http.NewRequest` (no ctx) | Deprecated form since Go 1.13 in favor of `NewRequestWithContext`; cancellation can't be wired | `http.NewRequestWithContext` |
| `json.Unmarshal(body, &v)` without size cap | Hostile server can return 10 GiB; library OOMs | `io.LimitReader` then `json.NewDecoder(lr).Decode(&v)` |
| `defer resp.Body.Close()` without draining | Drained connection-pool semantics — sometimes the connection isn't returned to pool | Drain with `io.Copy(io.Discard, ...)` then close |
| `time.Sleep(delay)` in retry loop | Not interruptible by `ctx` cancellation | `select { case <-ctx.Done(): case <-time.After(delay): }` |
| `math/rand` (v1) for jitter | Requires manual seeding (footgun); not concurrent-safe without `rand.New(rand.NewSource(...))` boilerplate | `math/rand/v2` (Go 1.22+) |
| `init()` for client construction | PROJECT.md forbids; also bad practice (untestable, hidden side effects) | Constructor pattern |
| Global mutable state (e.g., shared default cache) | PROJECT.md forbids; thread-safety footgun | Per-Client state |
| `interface{}`-typed `Get/Set` cache | Loses type safety, every read needs a type assertion | Generic cache `cache[T]` if type erasure cost matters; otherwise scoped internal caches per call type |
| `panic` for input validation | Libraries should return errors; panic is reserved for "programmer error" | Return typed errors |
| `slog.SetDefault` from library code | Mutates global state for the entire process | Accept `*slog.Logger` via option |
| `tc := tc` shadow in table-driven loops (Go 1.22+ code) | Redundant since Go 1.22 scopes loop vars per iteration | Drop it; linters may flag it |
| Logging full response bodies | Leaks data into operator logs (PROJECT.md prohibits above Debug) | Log method/URL/status/duration only; truncate body if at Debug |
| `time.Now().Unix()` for deadlines | Loses sub-second precision | `time.Now().Add(d)` for deadlines |

## Stack Patterns by Variant

**If `cmd/ohcli` ever grows beyond 4 subcommands:**
- Migrate to `github.com/spf13/cobra`
- Because: subcommand routing, POSIX flags, auto-generated help, completion scripts all become real wins past that complexity threshold.

**If JSON v2 promotes to stable (Go 1.26+):**
- Adopt `encoding/json/v2` for `Holiday.UnmarshalJSON` if benchmarks show >20% improvement
- Because: stdlib still wins for zero-dep, v2 is faster, and the migration is mostly mechanical with v2's option-driven API.

**If `Holiday.Range()` becomes too restrictive and callers need `iter.Seq2[int, time.Time]`:**
- Add a parallel method, don't break the existing one
- Because: pre-1.0 the library can break things, but the friction is high enough that adding a new method is cheaper.

**If integration test volume grows:**
- Move from `httptest.NewServer` per-test to a recorded fixture replay tool (e.g., implement a thin `http.RoundTripper` that reads from `testdata/recordings/`)
- Because: maintenance of canned fixtures scales poorly past ~30 distinct request shapes.

**If the API ever gains rate-limit headers:**
- Add a `WithRateLimit(rps int)` option backed by `golang.org/x/time/rate.Limiter`
- Because: `x/time/rate` is the de-facto stdlib-adjacent token bucket; it's the only `golang.org/x/*` dependency that's nearly universal in HTTP clients. **Important:** this would break the zero-dep rule — current scope doesn't need it (POC observed no rate-limit headers).

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| Go 1.22 module | `cmp` (latest), Go 1.22+ stdlib | `cmp` requires Go 1.21+; safe. |
| Go 1.23 features (`iter.Seq`) | Module `go` directive 1.23+ | Bump directive when adopting `Holiday.Range()`. |
| `math/rand/v2` | Go 1.22+ | Available throughout the support matrix; no compat concern. |
| `t.Setenv` | Go 1.17+ | Safe across matrix. |
| `testing.F.Fuzz` | Go 1.18+ | Safe across matrix. |
| `slog` | Go 1.21+ | Safe across matrix. |
| `GOEXPERIMENT=jsonv2` | Go 1.25+ | Optional/experimental; not on the support floor. |
| `golangci-lint` v2 | Any Go in the matrix | v2.x supports linting code built with Go 1.21+. |
| `goreleaser` v2 | Any Go in the matrix | v2.x builds with any modern Go toolchain. |
| `actions/setup-go@v5` | Latest Actions runner | Major v5 is stable and current as of 2026. |

## Sources

- [Go 1.23 Release Notes](https://go.dev/doc/go1.23) — `iter.Seq` package, range-over-func, stdlib iterator helpers. HIGH confidence.
- [Go 1.24 Release Notes](https://go.dev/doc/go1.24) — `bytes`/`strings` iterator helpers (`Lines`, `SplitSeq`, etc.), generic type aliases. HIGH confidence.
- [Go 1.25 Release Notes](https://tip.golang.org/doc/go1.25) — `encoding/json/v2` experiment, container-native improvements. HIGH confidence.
- [A new experimental Go API for JSON](https://go.dev/blog/jsonv2-exp) — official rationale for json v2, perf and API tradeoffs. HIGH confidence.
- [`log/slog` package docs](https://pkg.go.dev/log/slog) — official slog reference. HIGH confidence.
- [Structured Logging with slog (go.dev blog)](https://go.dev/blog/slog) — official intro and gotchas. HIGH confidence.
- [Welcome to golangci-lint v2 (ldez)](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) — v2 launch post, configuration changes, migration. HIGH confidence (project maintainer).
- [golangci-lint Configuration](https://golangci-lint.run/docs/configuration/) — current v2 config schema. HIGH confidence.
- [Announcing GoReleaser v2](https://goreleaser.com/blog/goreleaser-v2/) — v2 launch and config schema. HIGH confidence.
- [GoReleaser Build docs](https://goreleaser.com/customization/builds/) — build matrix for Go binaries. HIGH confidence.
- [`govulncheck` docs (pkg.go.dev)](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) — official vuln scanner. HIGH confidence.
- [hashicorp/go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp) — reference for Retry-After parsing semantics (used as design reference, not a dep). MEDIUM confidence (third-party project's docs).
- [cenkalti/backoff/v4](https://pkg.go.dev/github.com/cenkalti/backoff/v4) — reference for jitter strategy patterns (design reference, not a dep). MEDIUM confidence.
- [Middleware and RoundTrippers in Go (DEV)](https://dev.to/calvinmclean/middleware-and-roundtrippers-in-go-30pa) — community summary of tripperware pattern. MEDIUM confidence (community blog).
- [Tripperwares: http.Client Middleware (DEV)](https://dev.to/stevenacoffman/tripperwares-http-client-middleware-chaining-roundtrippers-3o00) — chaining pattern. MEDIUM confidence.
- [Implementing an In-Memory Cache in Go (Alex Edwards)](https://www.alexedwards.net/blog/implementing-an-in-memory-cache-in-go) — canonical small Go cache pattern. MEDIUM confidence (well-respected Go author).
- [Parallel Table-Driven Tests in Go (glukhov.org)](https://www.glukhov.org/post/2025/12/parallel-table-driven-tests-in-go/) — 2025 update on Go 1.22 loop-scoping fix and t.Parallel etiquette. MEDIUM confidence.
- [`pkg.go.dev` jjeffery/civil](https://pkg.go.dev/github.com/jjeffery/civil) — comparison reference for civil-date alternatives. MEDIUM confidence.
- [Choosing a Go CLI Library (mt165.co.uk)](https://mt165.co.uk/blog/golang-cli-library/) — survey of Go CLI library tradeoffs. MEDIUM confidence.
- [JetBrains "The Go Ecosystem in 2025"](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025/) — 2025 ecosystem snapshot (frameworks, tools, practices). MEDIUM confidence (industry survey).
- [go-json-experiment/jsonbench](https://github.com/go-json-experiment/jsonbench) — official jsonv2 vs alternatives benchmarks. HIGH confidence (run by Go team / json v2 authors).
- [Go 1.25 JSON v2 Benchmarks (dev.to/ryansgi)](https://dev.to/ryansgi/go-125-json-v2-benchmarks-raptor-escapes-and-a-18-speedup-5cf3) — third-party benchmark write-up. MEDIUM confidence.

---

*Stack research for: Go HTTP/JSON client SDK (OSS, dependency-light, Go 1.22+)*
*Researched: 2026-05-26*
