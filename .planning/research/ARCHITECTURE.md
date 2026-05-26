# Architecture Research

**Domain:** Idiomatic Go SDK / HTTP client library wrapping the public OpenHolidays REST API
**Researched:** 2026-05-26
**Confidence:** HIGH (corroborated by Go standard library conventions, multiple well-known SDKs — go-github, stripe-go, hashicorp/go-retryablehttp — and recent 2024-2026 community guidance)

---

## TL;DR

- **Single root package** (`openholidays`) for the public surface. Do **not** split `transport`, `cache`, `types` into separate exported sub-packages — that fragments the public API for no benefit and forces consumers to import 3-5 paths to do one thing. This matches go-github, AWS SDK v2 service packages, and most idiomatic Go clients.
- **`internal/` for genuinely private machinery** (validators, retry math, JSON helpers, build version). Keep it small — most things callers should *never* depend on still don't need to be in `internal/` if they're unexported in the root package.
- **RoundTripper decorator chain** for cross-cutting concerns (retry → cache → hook → logging → user-agent → base transport). Cache **inside** the chain, not at method call sites. This is the canonical Go pattern (`http.RoundTripper` is *the* extensibility seam).
- **Custom `Date` type** (`type Date time.Time` or a wrapper struct) used as a field of `Holiday`, not a free-standing `UnmarshalJSON` method on `Holiday`. Localizes the date-format concern and lets callers reason about `Holiday.StartDate.Time()` once.
- **`*APIError` constructed inside the method handler** after the RoundTripper chain returns — the RoundTripper sees only `http.Response` and shouldn't carry endpoint semantics. Sentinels live as package-level `var Err... = errors.New(...)`; method handlers wrap with `%w` and `errors.Is/As` traversal works through the chain.
- **`cmd/ohcli` MUST import the library at its module path**, not via relative paths or `internal/`. The CLI is a first-class consumer that exercises the public surface — that's its main value beyond demo.
- **Build order DAG:** types → transport scaffold (Client + RoundTripper chain skeleton) → first endpoint (Countries, the smallest) → remaining endpoints in parallel → helpers/validators → retry + cache (composable, added without method-signature churn) → CLI → docs/release. The transport scaffold gates everything else; the rest is a wide layer.

---

## Standard Architecture

### System Overview

```
                          ┌──────────────────────────────────────────────────────┐
                          │              Library Consumer (caller code)          │
                          │  client := openholidays.NewClient(WithCache(...))    │
                          │  hs, err := client.PublicHolidays(ctx, req)          │
                          └─────────────────────────┬────────────────────────────┘
                                                    │ (1) public method call
                                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Public API Layer (package openholidays)             │
│  ┌──────────────────────────────────────────────────────────────────────────┐   │
│  │ Client (struct)                                                          │   │
│  │   - methods: Countries, Languages, Subdivisions, PublicHolidays,         │   │
│  │              SchoolHolidays                                              │   │
│  │   - fields:  http (*http.Client w/ chained Transport), baseURL, ua,      │   │
│  │              logger, strictDecoding, retry cfg, hook                     │   │
│  └────────────────┬─────────────────────────────────────────────────────────┘   │
│                   │ (2) validate request → build *http.Request                  │
│                   │     (validators are unexported funcs in root pkg)           │
│                   ▼                                                             │
│  ┌──────────────────────────────────────────────────────────────────────────┐   │
│  │ Request builder + Response decoder                                       │   │
│  │   - URL assembly, query params, Accept/UA headers                        │   │
│  │   - io.LimitReader(10 MiB), json.Decoder w/ DisallowUnknownFields opt    │   │
│  │   - on non-2xx → constructs *APIError                                    │   │
│  └────────────────┬─────────────────────────────────────────────────────────┘   │
└───────────────────┼─────────────────────────────────────────────────────────────┘
                    │ (3) http.Client.Do(req)
                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                  Transport Layer (http.RoundTripper decorator chain)             │
│                                                                                 │
│   request ──►  retryTransport                                                   │
│                  │   (retries on 5xx/429/network err w/ exp backoff+jitter,     │
│                  │    honors Retry-After; ctx-bound)                            │
│                  ▼                                                              │
│                cacheTransport (only if WithCache; bypassed for non-GET)         │
│                  │   (TTL keyed by method+URL; serves stale from store on hit)  │
│                  ▼                                                              │
│                hookTransport                                                    │
│                  │   (calls user hook(req, resp, err) post-roundtrip)           │
│                  ▼                                                              │
│                loggingTransport                                                 │
│                  │   (slog.Debug with method, URL, status, duration)            │
│                  ▼                                                              │
│                headerTransport                                                  │
│                  │   (sets Accept, User-Agent — last layer before base)         │
│                  ▼                                                              │
│                http.DefaultTransport (or user-supplied via WithHTTPClient)      │
│                  │                                                              │
│                  ▼                                                              │
│                network ──► openholidaysapi.org                                  │
│                                                                                 │
│   response flows back up the same stack, each layer post-processing             │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| `Client` | Holds composed `*http.Client`, base URL, version-ed UA, slog logger, strict-decoding flag, hook. Each endpoint method is thin: validate → request → decode → return. | Exported struct in root pkg. Goroutine-safe (no mutable state after `NewClient`). |
| Functional `Option` | Mutates an internal `clientConfig` builder during `NewClient`. Never mutates `Client` after construction. | `type Option func(*clientConfig)`. Each `WithX` returns one. |
| Request builder | URL assembly, query-param encoding, body serialization, header set. Takes `context.Context`. | Unexported helpers (`newRequest(ctx, method, path, q url.Values) (*http.Request, error)`) on `Client`. |
| Response decoder | `io.LimitReader` → `json.Decoder` → typed value. Handles `Content-Type` check and strict mode. | Unexported `decode[T any](resp, v *T, strict bool) error` generic helper. |
| RoundTripper chain | Cross-cutting HTTP concerns: retry, cache, hook, logging, headers. Each is a small struct implementing `http.RoundTripper` and wrapping a `next`. | One file per transport in root pkg (`transport_retry.go`, `transport_cache.go`, ...); composed in `NewClient` via `buildTransport(cfg)`. |
| Validators | Pre-flight client-side checks: country code shape, date-range bounds, language code. Return sentinel errors. | Unexported funcs in root pkg (`validateCountry`, `validateDateRange`). |
| Domain types | `Country`, `Language`, `Subdivision`, `Holiday`, `LocalizedText`, `SubdivisionRef`, `Date`. Plus request structs (`PublicHolidaysRequest`). | Exported structs with stable JSON tags; `Date` carries the custom Unmarshal. |
| Sentinel errors | `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`. | `var ErrX = errors.New("openholidays: ...")` at root. |
| `*APIError` | Carries `StatusCode`, `Path`, `Body` (capped slice), `Message`. Implements `Error()` + `Is(target) bool` and is matched via `errors.As`. | Exported struct, returned by method handlers on non-2xx. |
| `cmd/ohcli` | Demo CLI consuming the public lib at the module's import path. Uses `flag` (stdlib) — keep zero deps. | Separate `main` package; does **not** import `internal/`. |

---

## Recommended Project Structure

The brief's §7 layout is largely sound but slightly over-specified. Here is the verified, idiomatic version:

```
go-openholidays/
├── go.mod                          # module github.com/<owner>/go-openholidays
├── go.sum
├── LICENSE                         # MIT, single root file
├── README.md                       # ≤20-line quickstart + links
├── CHANGELOG.md
├── CONTRIBUTING.md
├── .golangci.yml
├── .goreleaser.yaml
├── .github/
│   └── workflows/
│       ├── ci.yml                  # matrix: go 1.22, 1.23, stable; vet, test -race, lint, govulncheck
│       └── release.yml             # goreleaser on v* tags
│
├── doc.go                          # package doc comment (godoc shows this first)
├── client.go                       # Client struct, NewClient, options application
├── options.go                      # all WithX functional options + clientConfig builder
├── errors.go                       # sentinels + *APIError
├── types.go                        # Country, Language, Subdivision, LocalizedText, SubdivisionRef
├── date.go                         # custom Date type + UnmarshalJSON / MarshalJSON
├── holiday.go                      # Holiday struct + helpers (Name, IsInRegion, Days, Range)
│
├── countries.go                    # Client.Countries + CountriesRequest (if any)
├── languages.go                    # Client.Languages
├── subdivisions.go                 # Client.Subdivisions + SubdivisionsRequest
├── public_holidays.go              # Client.PublicHolidays + PublicHolidaysRequest
├── school_holidays.go              # Client.SchoolHolidays + SchoolHolidaysRequest
│
├── request.go                      # unexported newRequest, decode[T], buildURL helpers
├── validate.go                     # unexported validateCountry, validateDateRange, etc.
│
├── transport.go                    # buildTransport(cfg) chain composer + headerTransport
├── transport_retry.go              # retryTransport (exp backoff + jitter, Retry-After)
├── transport_cache.go              # cacheTransport (in-memory TTL store, opt-in)
├── transport_hook.go               # hookTransport (WithRequestHook)
├── transport_logging.go            # loggingTransport (slog.Debug)
│
├── version.go                      # const Version = "0.1.0" (single-source for UA + CLI --version)
│
├── countries_test.go               # httptest-backed unit tests for Countries
├── languages_test.go
├── subdivisions_test.go
├── public_holidays_test.go
├── school_holidays_test.go
├── client_test.go                  # NewClient + options + concurrency tests
├── transport_retry_test.go
├── transport_cache_test.go
├── transport_logging_test.go
├── date_test.go                    # incl. fuzz target FuzzDateUnmarshal
├── validate_test.go
├── errors_test.go                  # errors.Is/As traversal tests
├── example_test.go                 # one runnable Example per public method
├── integration_test.go             # //go:build integration ; OPENHOLIDAYS_LIVE=1
│
├── testdata/                       # golden fixtures (excluded from build by `go build`)
│   ├── countries.json
│   ├── languages.json
│   ├── subdivisions_pl.json
│   ├── public_holidays_pl_2025.json
│   └── school_holidays_pl_2025.json
│
├── internal/
│   └── testhttp/                   # shared test helper (httptest server fixture loader)
│       └── server.go
│
├── cmd/
│   └── ohcli/
│       ├── main.go                 # imports github.com/<owner>/go-openholidays
│       ├── public.go               # `ohcli public PL 2025`
│       ├── school.go               # `ohcli school PL 2025 --region PL-SL`
│       └── main_test.go            # CLI smoke tests (run binary in-process)
│
├── docs/
│   └── design.md                   # architectural decisions, RFC-style
│
└── bench/                          # optional: dedicated benchmark package if hot paths warrant
    └── bench_test.go               # //go:build bench
```

### Structure Rationale

- **Root package `openholidays`** owns the entire public API. Callers write one import, get `Client`, types, errors, options. This matches `github.com/google/go-github/v60/github`, `github.com/stripe/stripe-go/v85`, `github.com/aws/aws-sdk-go-v2/service/*` (each service is *one* package). Splitting `types`, `transport`, `cache` as exported sub-packages is a Java-ism — it forces consumers to import 4 paths and creates synthetic boundaries that don't help anyone.
- **Flat root with per-endpoint files** is the idiomatic Go alternative to subdirectories. Files are an organizational tool; packages are an API tool. `countries.go` + `countries_test.go` next to each other beats `endpoints/countries/countries.go`.
- **No `pkg/` directory.** The `pkg/` convention from `golang-standards/project-layout` is widely considered an anti-pattern by Go core contributors (rsc, bradfitz have said as much). The root *is* the public package.
- **`internal/testhttp/`** is genuinely shared test machinery — it must compile in test mode but cannot leak. This is the one strong case for `internal/` in this project. Most "should be internal" code in this SDK is already unexported in the root package and needs nothing more.
- **`cmd/ohcli/`** is a sibling, not a child. It imports `github.com/<owner>/go-openholidays` at the module path — exactly as an external consumer would. This is critical: the CLI dogfoods the public API and surfaces any usability flaws *before* external users hit them. It MUST NOT import `internal/` (Go would actually allow it since they share an ancestor, but doing so would invalidate the dogfooding).
- **`testdata/` at root** is the Go stdlib convention: `go build` ignores it automatically. Sub-tests can subdir under it (`testdata/poland_2025/`) if fixtures grow.
- **`docs/design.md`** is the long-form architecture record. README stays short. The brief asks for this explicitly.
- **`bench/` is optional.** If hot-path benchmarks live in regular `_test.go` files with `func BenchmarkX`, they run with `go test -bench`. A separate `bench/` package only earns its keep if benchmark setup is heavy or shouldn't load on every `go test`.

### Deltas From the Brief's §7 Proposal

| Brief proposes | Recommendation | Why |
|----------------|----------------|-----|
| Single root pkg + `internal/` + `cmd/ohcli` | Confirmed — keep this | Matches go-github, stripe-go conventions |
| (unspecified) sub-packages for transport/cache/types | Do **not** create them | Fragments public surface; users import one path |
| `transport.go` as single file | Split per concern (`transport_retry.go`, `transport_cache.go`, ...) | Each RoundTripper is independently testable; one file per concern is friendlier than a 600-line `transport.go` |
| `pkg/` mention | Avoid entirely | Anti-pattern in modern Go |
| Custom `UnmarshalJSON` on `Holiday` | Use a `Date` type as a `Holiday` field instead | Localizes parsing; reusable across `StartDate`/`EndDate` and other date-bearing types |
| `internal/` for generic helpers | Use unexported root-pkg funcs unless a helper is shared across test+main+cmd | Most "private helper" needs are met by lowercase names in the root pkg |

---

## Architectural Patterns

### Pattern 1: Functional Options with Internal Builder

**What:** `NewClient(opts ...Option) *Client` applies each option to an unexported `clientConfig` struct, then constructs an immutable `Client`. Options never touch `Client` directly — they only configure the builder.

**When to use:** Always. This is the canonical Go pattern for SDK construction (used by gRPC, Kubernetes client-go, AWS SDK).

**Trade-offs:**
- ✅ Stable constructor signature forever — new options never break callers
- ✅ Defaults are explicit and in one place
- ✅ Hides the surface area of `Client` fields (callers can't poke at them)
- ⚠️ Slightly more code than a config struct, but worth it for the API stability guarantee

**Example:**
```go
// options.go
type clientConfig struct {
    httpClient     *http.Client
    baseURL        string
    userAgent      string
    logger         *slog.Logger
    timeout        time.Duration
    retry          retryConfig
    cache          *memoryCache  // nil = disabled
    hook           RequestHookFunc
    strictDecoding bool
}

type Option func(*clientConfig)

func WithHTTPClient(c *http.Client) Option {
    return func(cfg *clientConfig) { cfg.httpClient = c }
}

func WithCache(ttl time.Duration, maxEntries int) Option {
    return func(cfg *clientConfig) { cfg.cache = newMemoryCache(ttl, maxEntries) }
}

// client.go
func NewClient(opts ...Option) *Client {
    cfg := defaultConfig()
    for _, opt := range opts {
        opt(&cfg)
    }
    // Build the immutable Client. http.Client is COPIED so caller mutations
    // post-NewClient don't leak in. See "hidden mutability concerns" below.
    return &Client{
        http:    composeHTTPClient(cfg),
        baseURL: cfg.baseURL,
        ua:      cfg.userAgent,
        logger:  cfg.logger,
        strict:  cfg.strictDecoding,
    }
}
```

**Hidden mutability concern (and fix):** If a caller does `c := &http.Client{Timeout: 5*time.Second}; client := openholidays.NewClient(openholidays.WithHTTPClient(c))` and later mutates `c.Timeout = 0`, the SDK is affected because `*http.Client` is a pointer. **Fix:** In `composeHTTPClient`, shallow-copy the user's `http.Client` (`copy := *cfg.httpClient`) before wrapping its `Transport` with the RoundTripper chain. Now mutations on the original don't leak. The user's `Transport` is reused as the chain's terminal — that's intentional and documented.

### Pattern 2: RoundTripper Decorator Chain

**What:** Each cross-cutting concern (retry, cache, hook, logging, headers) is a `struct { next http.RoundTripper }` implementing `RoundTrip(req *http.Request) (*http.Response, error)`. They compose by wrapping `next`.

**When to use:** This is the **only** idiomatic way to layer HTTP-level concerns in Go. Both `golang.org/x/oauth2` and `hashicorp/go-retryablehttp` use it; `go-chi/transport`, `gregjones/httpcache`, and the `chi` framework's transport package all expose this exact shape.

**Trade-offs vs per-method wrapping:**
- ✅ Concerns are independently testable (give a `headerTransport{next: fakeRT{}}` a request, assert it set headers)
- ✅ Concerns can be reordered or removed without touching endpoint methods
- ✅ Standard interface (`http.RoundTripper`) — callers can supply their own transports and they slot in
- ✅ Works for *every* HTTP call uniformly — no risk that a new endpoint forgets to call `withRetry(...)`
- ⚠️ Cache key generation for non-GET requests needs care (we cache only GET, which OpenHolidays uses exclusively)
- ⚠️ Retry must `Body.Close()` and rewind any request body — for this SDK all calls are GET with no body, simplifying enormously
- ⚠️ Stacking order matters (see chain order below)

**Chain order (outer → inner, so this is the order in which they SEE the request):**

```
Outer (sees request first)                      Inner (sees request last)
┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐  ┌──────────┐
│ retry  │→ │ cache  │→ │  hook  │→ │ log    │→ │ header │→ │ base     │
└────────┘  └────────┘  └────────┘  └────────┘  └────────┘  │ transport│
                                                            └──────────┘
                                                                  ↓
                                                                network
```

**Order rationale:**
1. **retry outermost** — retries should re-enter the full chain (so retried calls are re-cached, re-logged, etc.). Putting retry inside cache would let the cache return stale failed responses; putting retry inside logging would hide retry attempts from logs.
2. **cache before hook** — a cache hit short-circuits the network; the hook should see the synthetic "from-cache" response. Document this clearly.
3. **hook before logging** — the hook is observability *injection* by the caller; the SDK's own logging is downstream of that.
4. **logging before header** — log the canonical request as it'll go on the wire (with our UA and Accept).
5. **header innermost (just above base)** — last chance to set headers before network. Putting it outermost would let retry add headers, then later layers theoretically modify them — error-prone.
6. **base transport** — `http.DefaultTransport` or `cfg.httpClient.Transport` if user provided one. This is the actual network.

**Example:**
```go
// transport.go
func composeHTTPClient(cfg clientConfig) *http.Client {
    base := cfg.httpClient.Transport
    if base == nil {
        base = http.DefaultTransport
    }
    var rt http.RoundTripper = base
    rt = &headerTransport{ua: cfg.userAgent, next: rt}
    rt = &loggingTransport{logger: cfg.logger, next: rt}
    if cfg.hook != nil {
        rt = &hookTransport{hook: cfg.hook, next: rt}
    }
    if cfg.cache != nil {
        rt = &cacheTransport{cache: cfg.cache, next: rt}
    }
    if cfg.retry.enabled {
        rt = &retryTransport{cfg: cfg.retry, next: rt}
    }
    httpCopy := *cfg.httpClient // shallow copy
    httpCopy.Transport = rt
    return &httpCopy
}
```

### Pattern 3: Custom `Date` Type Over Per-Field Unmarshal

**What:** Define `type Date struct { time.Time }` (wrapper) or `type Date time.Time` (named alias). Implement `UnmarshalJSON`/`MarshalJSON` on it. Use as `Holiday.StartDate Date` instead of writing a custom `UnmarshalJSON` on `Holiday` itself.

**When to use:** Any time you have a non-RFC3339 date/time format and the same format appears in multiple fields or multiple types. OpenHolidays uses `YYYY-MM-DD` for every date field across `Holiday`, query parameters, and likely future endpoints — this is a slam-dunk for a custom type.

**Trade-offs — wrapper struct vs named alias:**

| Approach | Pros | Cons |
|----------|------|------|
| `type Date struct { time.Time }` | Embedding promotes all `time.Time` methods (`Year()`, `Format()`, etc.) — callers write `h.StartDate.Year()` naturally. No type conversion needed. | Slightly more verbose to construct: `Date{time.Date(...)}` |
| `type Date time.Time` | Concise: `Date(time.Now())`. Compact JSON tags. | Loses `time.Time` methods unless you re-declare them or callers do `time.Time(d).Year()` — friction at every call site. |

**Recommendation:** **wrapper struct** (`type Date struct { time.Time }`). Idiomatic Go promotes embedding for exactly this reason; consumer ergonomics matter for an SDK. Callers can do `h.StartDate.Format("2006-01-02")` directly.

**Why not per-struct `UnmarshalJSON` on `Holiday`:**
- Forces re-implementation on every type that has a date field
- Couples the date-format concern to every domain struct
- Harder to test in isolation
- Tedious to extend if the API gains more date-bearing types

**Example:**
```go
// date.go
type Date struct{ time.Time }

func (d Date) MarshalJSON() ([]byte, error) {
    return []byte(`"` + d.Format("2006-01-02") + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
    s := strings.Trim(string(b), `"`)
    if s == "" || s == "null" {
        return nil
    }
    t, err := time.Parse("2006-01-02", s)
    if err != nil {
        return fmt.Errorf("openholidays: invalid date %q: %w", s, err)
    }
    d.Time = t
    return nil
}

// holiday.go
type Holiday struct {
    ID           string          `json:"id"`
    StartDate    Date            `json:"startDate"`
    EndDate      Date            `json:"endDate"`
    Type         string          `json:"type"`
    Name         []LocalizedText `json:"name"`
    Nationwide   bool            `json:"nationwide"`
    Subdivisions []SubdivisionRef `json:"subdivisions,omitempty"`
    // ...
}
```

### Pattern 4: Cache Inside the RoundTripper (not at the Method Call Site)

**What:** Cache lookup/storage happens in `cacheTransport.RoundTrip(req)`, keyed by `req.Method + req.URL.String()`. Endpoint methods don't know caching exists.

**When to use:** Always preferred for HTTP-level caching of idempotent GETs. `gregjones/httpcache` (the de-facto Go HTTP cache) does this.

**Trade-offs vs per-method cache:**

| Cache-in-RoundTripper (recommended) | Cache-at-method-call-site |
|--------------------------------------|---------------------------|
| ✅ Endpoint methods stay thin — no caching boilerplate to repeat | ✅ Can cache *decoded* values, saving the decode cost too |
| ✅ Cache scope = "any GET to this URL" — simple, predictable | ✅ Can use richer cache keys (typed request structs) |
| ✅ Caller-supplied transports compose with cache trivially | ⚠️ Five endpoints × cache boilerplate = repetition, drift risk |
| ✅ Easier to add cache control headers later (RFC 7234 alignment if desired) | ⚠️ Each method must remember to participate — easy to forget |
| ⚠️ Caches raw bytes (small overhead to re-decode on hit) | ⚠️ Cache stores live objects, larger memory footprint, GC-unfriendly for large slices |
| ⚠️ Caller can't easily bypass per-call without an option | ⚠️ Doesn't compose with other transports |

For this SDK — small responses, network is the bottleneck, reference data dominates the cacheable set — **cache-in-RoundTripper wins clearly**. The "re-decode on hit" cost is negligible (a 14-element holiday array is microseconds).

**Cache key:** `req.Method + " " + req.URL.String()`. URL includes path + sorted query string (use `req.URL.Query().Encode()` to canonicalize). Vary on `Accept-Language` if you ever localize.

**What to cache:** the brief says reference endpoints only (`Countries`, `Languages`, `Subdivisions`). Easiest enforcement: the cacheTransport caches *every* GET, but the Client constructs a non-cacheable request for holiday endpoints by skipping the cacheTransport entirely — actually no, simpler: cacheTransport reads a context value `cacheBypassCtxKey` set by holiday-endpoint methods. Or — simplest of all — the `clientConfig.cache` is constructed only with a list of cacheable paths (URL prefix match: `/Countries`, `/Languages`, `/Subdivisions`), and the transport bypasses for anything else. Pick path-prefix matching for v0.1.0; it's two lines.

### Pattern 5: Validation as Unexported Package Functions

**What:** `validateCountry(code string) error`, `validateDateRange(from, to time.Time) error`, etc., are unexported funcs in the root package. Each endpoint method calls them at the top before building a request.

**When to use:** Whenever the cost of a misuse is a network round-trip + a parsed `*APIError` from upstream. Pre-flight validation is cheaper and gives clearer errors.

**Why not methods on request types?** A request type is a passive struct; behavior on it bloats the API surface unnecessarily. `req.Validate()` looks fine but you've added a public method for every request type. Free functions stay private.

**Why not a separate `validate` package?** Over-engineering. Five validators total; they live with the types they validate.

**Example:**
```go
// validate.go
var ErrInvalidCountry = errors.New("openholidays: invalid country code")
var ErrDateRangeTooLarge = errors.New("openholidays: date range exceeds 3 years")

func validateCountry(code string) error {
    if len(code) != 2 || strings.ToUpper(code) != code {
        return fmt.Errorf("%w: %q (want 2-letter uppercase ISO 3166-1 alpha-2)", ErrInvalidCountry, code)
    }
    return nil
}

func validateDateRange(from, to time.Time) error {
    if from.After(to) {
        return fmt.Errorf("%w: from=%s > to=%s", ErrInvalidDateRange, from.Format("2006-01-02"), to.Format("2006-01-02"))
    }
    if to.Sub(from) > 3*365*24*time.Hour+24*time.Hour { // ~3 years, leap-tolerant
        return fmt.Errorf("%w: %s to %s spans more than 3 years", ErrDateRangeTooLarge, from.Format("2006-01-02"), to.Format("2006-01-02"))
    }
    return nil
}

// public_holidays.go
func (c *Client) PublicHolidays(ctx context.Context, req PublicHolidaysRequest) ([]Holiday, error) {
    if err := validateCountry(req.CountryCode); err != nil {
        return nil, err
    }
    if err := validateDateRange(req.ValidFrom, req.ValidTo); err != nil {
        return nil, err
    }
    // ... build *http.Request, decode, return
}
```

### Pattern 6: Error Construction at the Method Layer, Sentinels at Package Level

**What:**
- Sentinel errors are package-level `var Err... = errors.New(...)`.
- `*APIError` is constructed by the endpoint method after the RoundTripper chain returns, when it sees `resp.StatusCode >= 400`.
- Validation errors wrap sentinels with `%w` so `errors.Is(err, ErrInvalidCountry)` works.
- Network errors propagate as-is from the chain; the method may wrap with endpoint context (`fmt.Errorf("openholidays: %s: %w", "/PublicHolidays", err)`).

**When to use:** This is the modern Go idiom — `errors.Is`/`errors.As` traversal works through `%w` chains, so callers can branch on identity or type cleanly.

**Why the RoundTripper should NOT construct `*APIError`:** The RoundTripper sees only `*http.Response`; it doesn't know which endpoint or what the request meant. Constructing a domain error there leaks semantic context out of the network layer. Keep the chain HTTP-pure: it returns `(*http.Response, error)` with transport-level errors only (timeouts, network failures, retries-exhausted).

**Example:**
```go
// errors.go
var (
    ErrInvalidCountry    = errors.New("openholidays: invalid country code")
    ErrInvalidLanguage   = errors.New("openholidays: invalid language code")
    ErrDateRangeTooLarge = errors.New("openholidays: date range too large")
    ErrInvalidDateRange  = errors.New("openholidays: invalid date range")
    ErrEmptyResponse     = errors.New("openholidays: empty response body")
)

type APIError struct {
    StatusCode int
    Path       string
    Body       []byte  // capped at e.g. 4 KiB
    Message    string  // parsed from upstream JSON if available
}

func (e *APIError) Error() string {
    return fmt.Sprintf("openholidays: api error %d at %s: %s", e.StatusCode, e.Path, e.Message)
}

// Allow errors.Is(err, &APIError{StatusCode: 404}) — match by status if target has one.
func (e *APIError) Is(target error) bool {
    t, ok := target.(*APIError)
    if !ok {
        return false
    }
    if t.StatusCode != 0 && t.StatusCode != e.StatusCode {
        return false
    }
    return true
}

// In each endpoint method:
resp, err := c.http.Do(req)
if err != nil {
    return nil, fmt.Errorf("openholidays: %s: %w", req.URL.Path, err)
}
defer resp.Body.Close()
if resp.StatusCode >= 400 {
    body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
    return nil, &APIError{
        StatusCode: resp.StatusCode,
        Path:       req.URL.Path,
        Body:       body,
        Message:    parseAPIMessage(body), // best-effort
    }
}
```

Callers then do:
```go
hs, err := client.PublicHolidays(ctx, req)
if errors.Is(err, openholidays.ErrInvalidCountry) {
    // bad input
}
var apiErr *openholidays.APIError
if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
    // upstream said not found
}
```

### Pattern 7: Test Architecture — Per-File + Build-Tagged Integration

**What:**
- Unit tests live next to source: `countries.go` ↔ `countries_test.go`. Same package (`package openholidays`) so they can see unexported helpers.
- Integration tests in one file: `integration_test.go` guarded by `//go:build integration` and check `OPENHOLIDAYS_LIVE=1`.
- Shared test infrastructure (httptest server fixture loader) in `internal/testhttp/`.
- Golden fixtures in `testdata/` at root.
- Fuzz targets in `date_test.go` and `validate_test.go` (use Go 1.18+ `func FuzzX(f *testing.F)`).
- Examples in `example_test.go` for godoc rendering.

**When to use:** Idiomatic for Go. Tests-as-siblings is the convention; a separate `tests/` directory is a foreign Java/Python pattern that breaks `go test ./...` ergonomics.

**Trade-offs vs separate `tests/` directory:**
- ✅ `_test.go` files are auto-excluded from the consumer's build
- ✅ Same-package tests access unexported helpers (use `package openholidays_test` for example_test.go to keep them in the external-consumer view)
- ✅ Single command runs everything: `go test ./...`
- ✅ Coverage works out of the box: `go test -cover ./...`
- ⚠️ Per-file co-location can crowd a directory — mitigated by clear naming and one file per logical concern

**Build-tag pattern:**
```go
//go:build integration

package openholidays_test

import (
    "os"
    "testing"
)

func TestLive_PublicHolidays_PL_2025(t *testing.T) {
    if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
        t.Skip("OPENHOLIDAYS_LIVE=1 required for live integration tests")
    }
    // ...
}
```

Run with: `go test -tags=integration ./...` (and `OPENHOLIDAYS_LIVE=1`).

**Shared golden-file infrastructure:**
```go
// internal/testhttp/server.go
package testhttp

import (
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"
)

// NewServer returns a test server that serves fixture files from testdata/<route>.
// E.g. GET /PublicHolidays?... → testdata/public_holidays_pl_2025.json
func NewServer(t *testing.T, routes map[string]string) *httptest.Server {
    t.Helper()
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        fixture, ok := routes[r.URL.Path]
        if !ok {
            http.NotFound(w, r)
            return
        }
        data, err := os.ReadFile(filepath.Join("testdata", fixture))
        if err != nil {
            t.Fatalf("fixture %q: %v", fixture, err)
        }
        w.Header().Set("Content-Type", "application/json")
        w.Write(data)
    }))
}
```

Note: this uses `internal/testhttp` precisely because it's shared by both `*_test.go` files in the root package and by `cmd/ohcli/main_test.go`. The `internal/` boundary keeps it from leaking to consumers.

### Pattern 8: CLI as an External Consumer

**What:** `cmd/ohcli/main.go` does `import "github.com/<owner>/go-openholidays"` and uses only the public API — never `internal/` and never relative paths.

**When to use:** Any time a library ships a CLI in the same module.

**Trade-offs:**
- ✅ CLI exercises the public API end-to-end — bugs in usability surface immediately
- ✅ Acts as living documentation for what the library can do
- ✅ Single repo, single `go install github.com/<owner>/go-openholidays/cmd/ohcli@latest`
- ⚠️ The CLI is built as part of `go test ./...` — keep it dep-light (stdlib `flag` is fine; don't pull in cobra unless we want it as a runtime dep)
- ⚠️ Avoid the temptation to share types/helpers via `internal/` — if something is genuinely shared (e.g. version constant), put it in the root package as an exported `const Version`

**Why no relative imports:** Go modules don't support them. The CLI imports at the module path. As a side benefit, if you ever extract the CLI to its own repo, no code changes needed.

---

## Data Flow

### Request Flow: `client.PublicHolidays(ctx, req)` traversal

```
User code
   │ client.PublicHolidays(ctx, PublicHolidaysRequest{CountryCode:"PL", ValidFrom:..., ValidTo:...})
   ▼
┌─ Client.PublicHolidays (public_holidays.go) ────────────────────────────────────┐
│  1. validateCountry("PL")                                                       │
│     └─ returns nil (good) OR wraps ErrInvalidCountry with %w                    │
│  2. validateDateRange(req.ValidFrom, req.ValidTo)                               │
│  3. q := url.Values{"countryIsoCode":"PL", "validFrom":"2025-01-01", ...}       │
│  4. req2, _ := c.newRequest(ctx, "GET", "/PublicHolidays", q)                   │
│     - sets req.Header Accept=application/json (later) and embeds ctx            │
│  5. resp, err := c.http.Do(req2)  ◄─── enters RoundTripper chain                │
└────────────────────────────────┬───────────────────────────────────────────────┘
                                 │
                                 ▼
┌─ retryTransport.RoundTrip ─────────────────────────────────────────────────────┐
│  for attempt := 0; attempt < max; attempt++ {                                  │
│    resp, err := r.next.RoundTrip(req)  ──┐                                     │
│    if !shouldRetry(resp, err) { return } │ (cache→hook→log→header→base)        │
│    wait := backoff(attempt) +/- jitter   │                                     │
│    honor Retry-After header              │                                     │
│    select {                              │                                     │
│      case <-time.After(wait): continue   │                                     │
│      case <-ctx.Done(): return ctx.Err() │                                     │
│    }                                     │                                     │
│  }                                       ▼                                     │
└──────────────────────────────────────┬─────────────────────────────────────────┘
                                       │
                                       ▼
┌─ cacheTransport.RoundTrip ─────────────────────────────────────────────────────┐
│  if !isCacheable(req.URL.Path) { return c.next.RoundTrip(req) }                │
│  key := req.Method + " " + req.URL.String()                                    │
│  if entry, ok := c.store.Get(key); ok && !entry.Expired() {                    │
│    return entry.Response.Clone(), nil  ◄─── SHORT-CIRCUITS THE NETWORK         │
│  }                                                                             │
│  resp, err := c.next.RoundTrip(req)                                            │
│  if err == nil && resp.StatusCode == 200 {                                     │
│    body, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))                    │
│    resp.Body = io.NopCloser(bytes.NewReader(body))                             │
│    c.store.Put(key, cachedEntry{Response: resp, Body: body, Expiry:...})       │
│  }                                                                             │
│  return resp, err                                                              │
└──────────────────────────────────────┬─────────────────────────────────────────┘
                                       │ (For /PublicHolidays this layer is bypassed)
                                       ▼
┌─ hookTransport.RoundTrip ──────────────────────────────────────────────────────┐
│  resp, err := h.next.RoundTrip(req)                                            │
│  if h.hook != nil { h.hook(req, resp, err) }                                   │
│  return resp, err                                                              │
└──────────────────────────────────────┬─────────────────────────────────────────┘
                                       │
                                       ▼
┌─ loggingTransport.RoundTrip ───────────────────────────────────────────────────┐
│  start := time.Now()                                                           │
│  resp, err := l.next.RoundTrip(req)                                            │
│  l.logger.Debug("openholidays request",                                        │
│    "method", req.Method, "url", req.URL.String(),                              │
│    "status", statusOf(resp), "dur", time.Since(start), "err", err)             │
│  return resp, err                                                              │
└──────────────────────────────────────┬─────────────────────────────────────────┘
                                       │
                                       ▼
┌─ headerTransport.RoundTrip ────────────────────────────────────────────────────┐
│  req = req.Clone(req.Context())  // so callers can't observe header mutation   │
│  req.Header.Set("Accept", "application/json")                                  │
│  req.Header.Set("User-Agent", h.ua)  // e.g. "go-openholidays/0.1.0"           │
│  return h.next.RoundTrip(req)                                                  │
└──────────────────────────────────────┬─────────────────────────────────────────┘
                                       │
                                       ▼
┌─ http.DefaultTransport (or user-supplied) ─────────────────────────────────────┐
│  - DNS, TCP, TLS, HTTP/1.1 or HTTP/2                                           │
│  - Honors req.Context() — cancellation interrupts in flight                    │
└──────────────────────────────────────┬─────────────────────────────────────────┘
                                       │  network
                                       ▼
                              openholidaysapi.org
                                       │
                                       ▼  HTTP response (JSON body)
                              [resp travels back UP the chain;
                               each layer post-processes:
                               header→noop, log→records duration,
                               hook→fires, cache→stores, retry→checks status]
                                       │
                                       ▼
┌─ Back in Client.PublicHolidays ────────────────────────────────────────────────┐
│  6. if err != nil { return nil, fmt.Errorf("openholidays: /PublicHolidays: %w", err) }
│  7. defer resp.Body.Close()                                                    │
│  8. if resp.StatusCode >= 400 {                                                │
│       body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))                  │
│       return nil, &APIError{StatusCode:..., Path:..., Body:body, Message:...} │
│     }                                                                          │
│  9. var holidays []Holiday                                                     │
│     dec := json.NewDecoder(io.LimitReader(resp.Body, 10<<20))                  │
│     if c.strict { dec.DisallowUnknownFields() }                                │
│     if err := dec.Decode(&holidays); err != nil {                              │
│       return nil, fmt.Errorf("openholidays: decode /PublicHolidays: %w", err)  │
│     }                                                                          │
│     // Date fields populated via Date.UnmarshalJSON during decode              │
│ 10. return holidays, nil                                                       │
└─────────────────────────────────────────────────────────────────────────────────┘
   │
   ▼
User code receives []Holiday
```

### Key Data Flow Properties

1. **Context propagation:** `ctx` is set on the request at step 4. Every subsequent layer respects `req.Context()`. The retry layer explicitly selects on `ctx.Done()`. The base transport observes ctx cancellation and aborts in-flight I/O within ~100ms (Go runtime guarantee for `http.Transport`).
2. **No global state:** Every layer is instance-scoped via the `Client`. Two `Client`s never share anything mutable.
3. **Body consumption:** Only one layer (cache) reads the body; it replaces `resp.Body` with a `bytes.Reader` so the decode step still sees a fresh stream. Without cache, the body stream goes directly to the decoder.
4. **Failure surfaces:**
   - Validation errors return before step 5 (no network call).
   - Network errors return from step 6 with `%w`-wrapped context.
   - HTTP errors return `*APIError` from step 8.
   - Decode errors return wrapped errors from step 9.
   - All are inspectable via `errors.Is` / `errors.As`.

---

## Build Order (Dependency DAG)

The library has natural layers. Build them in this order, with several wide-fanout opportunities:

```
                          ┌────────────────────────────┐
                          │ Phase 1: Foundation         │
                          │                            │
                          │  types.go, date.go         │
                          │  errors.go (sentinels +    │
                          │            *APIError)      │
                          │  doc.go                    │
                          │  validate.go               │
                          │  + per-file unit tests     │
                          └─────────────┬──────────────┘
                                        │
                                        ▼
                          ┌────────────────────────────┐
                          │ Phase 2: Transport scaffold│
                          │                            │
                          │  options.go (clientConfig, │
                          │     Option, defaults)      │
                          │  client.go (Client struct, │
                          │     NewClient)             │
                          │  request.go (newRequest,   │
                          │     decode[T])             │
                          │  transport.go (compose,    │
                          │     headerTransport)       │
                          │  transport_logging.go      │
                          │  internal/testhttp/        │
                          │  + tests for all of above  │
                          └─────────────┬──────────────┘
                                        │
                                        │ (gates everything below)
                                        │
              ┌─────────────────────────┴────────────────────────────┐
              ▼                                                      ▼
   ┌────────────────────────┐                          ┌────────────────────────┐
   │ Phase 3a: First         │                          │ (parallelizable once    │
   │ endpoint (smoke test)   │                          │  3a lands; below can    │
   │                         │                          │  proceed concurrently)  │
   │  countries.go +         │                          │                         │
   │  countries_test.go      │                          │                         │
   │                         │                          │                         │
   │ Validates the entire    │                          │                         │
   │ Phase 1+2 stack with    │                          │                         │
   │ smallest endpoint.      │                          │                         │
   └───────────┬─────────────┘                          │                         │
               │                                        │                         │
               ▼                                        │                         │
   ┌────────────────────────┐                           │                         │
   │ Phase 3b: Remaining    │                           │                         │
   │ endpoints (parallel)   │                           │                         │
   │                        │                           │                         │
   │  languages.go          │                           │                         │
   │  subdivisions.go       │                           │                         │
   │  public_holidays.go    │                           │                         │
   │  school_holidays.go    │                           │                         │
   │  + tests for each      │                           │                         │
   └───────────┬────────────┘                           │                         │
               │                                        │                         │
               ▼                                        │                         │
   ┌────────────────────────┐                           │                         │
   │ Phase 4: Helpers       │                           │                         │
   │                        │                           │                         │
   │  holiday.go            │                           │                         │
   │   - Name(lang)         │                           │                         │
   │   - IsInRegion(code)   │                           │                         │
   │   - Days()             │                           │                         │
   │   - Range() iter.Seq   │                           │                         │
   │  + helper tests        │                           │                         │
   └───────────┬────────────┘                           │                         │
               │                                        │                         │
               ▼                                        │                         │
   ┌────────────────────────┐    ┌────────────────────────┐                       │
   │ Phase 5a: Retry        │    │ Phase 5b: Cache        │                       │
   │ (parallel with 5b)     │    │ (parallel with 5a)     │                       │
   │                        │    │                        │                       │
   │ transport_retry.go     │    │ transport_cache.go     │                       │
   │ + WithRetry option     │    │ + WithCache option     │                       │
   │ + tests w/ flaky srv   │    │ + tests w/ TTL clock   │                       │
   └───────────┬────────────┘    └───────────┬────────────┘                       │
               │                             │                                    │
               └──────────────┬──────────────┘                                    │
                              ▼                                                   │
                ┌────────────────────────┐                                        │
                │ Phase 6: Hook + strict │                                        │
                │                        │                                        │
                │  transport_hook.go     │                                        │
                │  WithRequestHook       │                                        │
                │  WithStrictDecoding    │                                        │
                │  + tests               │                                        │
                └───────────┬────────────┘                                        │
                            │                                                     │
                            ▼                                                     │
                ┌────────────────────────┐                                        │
                │ Phase 7: CLI           │                                        │
                │                        │                                        │
                │  cmd/ohcli/main.go     │                                        │
                │  cmd/ohcli/public.go   │                                        │
                │  cmd/ohcli/school.go   │                                        │
                │  + main_test.go        │                                        │
                └───────────┬────────────┘                                        │
                            │                                                     │
                            ▼                                                     │
                ┌────────────────────────┐                                        │
                │ Phase 8: Polish        │◄───────────────────────────────────────┘
                │                        │
                │  example_test.go       │
                │  fuzz targets          │
                │  benchmarks            │
                │  integration_test.go   │
                │  docs/design.md        │
                │  README, CHANGELOG     │
                │  CI workflows          │
                │  goreleaser            │
                │  v0.1.0 tag            │
                └────────────────────────┘
```

### Critical Path

The longest chain (cannot be parallelized) is:

```
Foundation → Transport scaffold → First endpoint → Remaining endpoints → Helpers → CLI → Polish/release
   ~3h            ~4h                ~2h               ~5h                 ~3h     ~3h     ~6h
```

Total critical-path estimate: ~26h. Retry+cache+hook (Phase 5/6) are off the critical path because the endpoint methods don't depend on them — they're transparent middleware. That's the architectural payoff of the RoundTripper chain: features compose without method-signature churn.

### Why this order

1. **Types first** — every test, every endpoint, every helper needs them. They're trivially decoupled and verifiable.
2. **Transport scaffold before any endpoint** — the first endpoint that lands proves the entire `Client → Request → RoundTripper → Response → Decode` pipeline. After that, each new endpoint is mechanical.
3. **Countries first among endpoints** — smallest payload, no date fields, no validation complexity. It's the canary that proves the scaffold.
4. **Retry/cache after endpoints** — they're transparent middleware. Adding them later doesn't churn endpoint code. This is *the* test of the architecture.
5. **CLI after the library is feature-complete** — the CLI is dogfooding, and it only finds bugs if the API is stable enough to demonstrate. Doing the CLI early would lock in a half-baked API.
6. **Polish last** — examples, benchmarks, integration tests, docs all depend on the public API being settled.

### Build-order DAG Implications for Roadmap Phases

A reasonable phase breakdown:

- **Phase 1: Foundation** — types, dates, errors, validation, doc.go.
- **Phase 2: Transport** — Client, options, RoundTripper scaffold (header + logging), one smoke-test endpoint (Countries).
- **Phase 3: Endpoints** — remaining four endpoints + helpers.
- **Phase 4: Resilience** — retry, cache, hook, strict decoding (all transparent additions).
- **Phase 5: Distribution** — CLI, examples, integration tests, fuzz, benchmarks, CI, goreleaser, v0.1.0.

Each phase ends with a working library at a meaningful capability level. Specifically, after Phase 2 we have a usable `client.Countries(ctx)`; after Phase 3 we have all advertised endpoints; after Phase 4 we have production-grade resilience; after Phase 5 we ship.

---

## Scaling Considerations

This is a client SDK, not a server, so "scaling" means usage shape, not request load.

| Scale | Architecture adjustments |
|-------|--------------------------|
| **Single caller, occasional use** (e.g. one cron job/day) | Default config. Cache adds little value at this scale. Retry helps against transient network blips. |
| **Embedded in a web service** (10s of requests/sec to upstream) | Enable cache for `Countries`/`Languages`/`Subdivisions`. Use a shared `Client` (it's goroutine-safe). Default `http.Transport` connection pooling handles concurrency. |
| **High-fanout batch job** (thousands of distinct holiday queries) | Add caller-side worker pool with bounded concurrency. The SDK doesn't need to know — it's just `client.PublicHolidays(ctx, req)` called in N goroutines. The `MaxIdleConnsPerHost` default of 2 may bottleneck; users can pass a tuned `*http.Client` via `WithHTTPClient`. |
| **Long-running daemon with millions of cached entries** | Brief defers persistent cache to M3. Until then, document the in-memory cache's max-entries cap and recommend periodic restart or a manual cache reset method. |

### Scaling Priorities — what breaks first

1. **First bottleneck: `MaxIdleConnsPerHost = 2`** in `http.DefaultTransport`. Under concurrent load, callers will see connection thrash. Document this; callers can supply their own `http.Client` with a tuned transport.
2. **Second bottleneck: in-memory cache unbounded growth.** Bound it (e.g. `WithCache(ttl, maxEntries)`). When full, evict LRU. The brief commits to in-memory only for v0.1.0 — that's fine; just don't make it infinite.
3. **Third bottleneck: rate limiting by upstream.** OpenHolidays POC observed no rate limit headers, but bursts of 1000s of requests could hit one. Retry + jitter mitigates; document conservative-by-default retry policy.

---

## Anti-Patterns

### Anti-Pattern 1: Sub-packages for "transport", "cache", "types"

**What people do:** Create `openholidays/transport/`, `openholidays/cache/`, `openholidays/types/` and export pieces from each. Often inherited from Java/C# muscle memory.

**Why it's wrong:** Consumers now import 3-5 paths to do one thing. Public surface area triples. Internal refactors (e.g. moving a helper) become breaking API changes. No idiomatic Go SDK does this.

**Do this instead:** One root package `openholidays`. Use files for organization; the package boundary is the API contract. If a sub-package emerges *organically* (e.g. a totally separate concept like "ical export" that users opt into), give it a directory then — but never for "categories of things in the main package."

### Anti-Pattern 2: Caching at the Method Call Site Instead of in a RoundTripper

**What people do:** In each `Client.Countries`, `Client.Languages` method, wrap the call with cache lookup/store logic.

**Why it's wrong:** Five endpoints × cache boilerplate = five places where someone forgets to bypass cache for write requests, forgets to refresh on errors, gets cache key generation subtly wrong. Cache concern bleeds into every endpoint method.

**Do this instead:** Put the cache in a RoundTripper. Endpoint methods stay one-screen long. Cache logic exists in one file and is tested in isolation.

### Anti-Pattern 3: Custom UnmarshalJSON on Every Date-Bearing Struct

**What people do:** Implement `func (h *Holiday) UnmarshalJSON(b []byte) error` that hand-parses every field including dates. Repeat for `Subdivision`, `Country`, etc.

**Why it's wrong:** Catastrophically tedious. Easy to forget a field. JSON struct tag drift not caught by the compiler. Adding a new field is a foot-gun.

**Do this instead:** A custom `Date` type with `UnmarshalJSON`. The standard `json.Unmarshal` then handles `Holiday`, picks up `Date` fields, and calls `Date.UnmarshalJSON` automatically. New fields just need the right type, no extra code.

### Anti-Pattern 4: CLI Importing `internal/`

**What people do:** Treat `cmd/ohcli` as "part of the same module so it can reach into `internal/` for helpers."

**Why it's wrong:** Defeats the purpose of dogfooding. The CLI is supposed to prove the library is usable as an external consumer would use it. If the CLI needs something, that thing should be public (and probably wanted by other consumers too).

**Do this instead:** CLI imports only the public root package. If you find yourself wanting an internal helper from the CLI, that's a signal to export it.

### Anti-Pattern 5: Storing User's `*http.Client` Pointer Directly

**What people do:** `Client.http = cfg.httpClient` — directly reuse the user's pointer.

**Why it's wrong:** User mutates `cfg.httpClient.Timeout` later → SDK's timeout changes silently. Race conditions on `Transport` field if user also wraps it.

**Do this instead:** Shallow-copy the user's `*http.Client` in `NewClient`, then attach the RoundTripper chain to the copy. Document this behavior. The user keeps full control of their original; the SDK has its own immutable view.

### Anti-Pattern 6: `errors.New` for Every Error With Context Embedded in the String

**What people do:** `return errors.New(fmt.Sprintf("invalid country code: %s", code))` — loses identity, can't be matched.

**Why it's wrong:** Callers can't branch on error type. They'd have to string-match, which is fragile.

**Do this instead:** Define sentinels at package level. Wrap with `%w`: `fmt.Errorf("%w: %q", ErrInvalidCountry, code)`. Callers use `errors.Is(err, openholidays.ErrInvalidCountry)`.

### Anti-Pattern 7: Construction-Time Network Calls

**What people do:** In `NewClient`, ping the upstream to validate connectivity, fetch a token, etc.

**Why it's wrong:** Constructor that takes a context is a code smell; one that doesn't take a context but does I/O is a footgun (no timeout, no cancellation). Also: tests now need a live server just to instantiate a client.

**Do this instead:** `NewClient` is pure — it sets up config and returns. The first network call happens in the first endpoint method, with the caller's `ctx`. (The brief is already aligned with this — keep it that way.)

### Anti-Pattern 8: Global Mutable State / `init()` Side Effects

**What people do:** Package-level `var DefaultClient = NewClient()`, `init()` that reads env vars to configure defaults.

**Why it's wrong:** Two consumers in the same binary fight over config. Tests can't isolate state. The brief explicitly forbids this — good.

**Do this instead:** Every consumer constructs their own `Client`. Defaults are computed inside `NewClient` from a pure function.

---

## Integration Points

### External Services

| Service | Integration pattern | Notes |
|---------|---------------------|-------|
| openholidaysapi.org REST API | Plain HTTP/JSON via `http.Client` + RoundTripper chain | Keyless, public. No rate-limit headers observed. 3-year query window cap. 5 endpoints used: `/Countries`, `/Languages`, `/Subdivisions`, `/PublicHolidays`, `/SchoolHolidays`. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| Public API ↔ internal helpers | Function calls in same package (lowercase unexported) | Most "internal" helpers live in the root package as unexported identifiers. Only genuinely cross-package-shared test machinery uses `internal/testhttp/`. |
| Client method ↔ RoundTripper chain | Standard `http.Client.Do(req)` interface | The method doesn't know about retry, cache, hook, logging — that's the architectural payoff. |
| RoundTripper ↔ RoundTripper | `http.RoundTripper` interface, `next` field | Each layer is independently testable by giving it a `next` that's a stub. |
| Library ↔ CLI (cmd/ohcli) | Module-path import of public API only | CLI must NOT import `internal/`. Acts as external-consumer dogfooding. |
| Test infrastructure ↔ tests | `internal/testhttp` package | Shared between root-pkg tests and CLI tests; not exported to consumers. |

---

## Confidence Notes

- **HIGH:** Single-package layout, RoundTripper chain, custom Date type, sentinel errors, build order. All cross-checked against go-github, stripe-go, hashicorp/go-retryablehttp, gregjones/httpcache, and recent (2024-2026) community articles.
- **MEDIUM-HIGH:** Cache-in-RoundTripper vs cache-at-method-site. Both work; the chain version is cleaner and more idiomatic for an SDK whose responses are small. Could be reversed if response decoding ever becomes the bottleneck (it won't, for these payload sizes).
- **MEDIUM:** Chain order (retry outermost). There's a defensible argument for putting `hook` outermost (so it sees retries as separate events). The recommended order makes "from-cache" hits visible to the hook, which is the more useful default. If users want per-attempt observability, they can attach a logger to the retry transport directly.
- **MEDIUM:** Whether `bench/` deserves its own directory. The brief asks for benchmarks; co-located `*_test.go` Benchmark funcs are the simplest answer. Promote to `bench/` only if benchmark setup grows heavy.

---

## Sources

- [Standard Go Project Layout (golang-standards/project-layout)](https://github.com/golang-standards/project-layout)
- [Organizing a Go module (go.dev)](https://go.dev/doc/modules/layout)
- [google/go-github — package layout reference](https://pkg.go.dev/github.com/google/go-github/github)
- [stripe/stripe-go — package layout reference](https://pkg.go.dev/github.com/stripe/stripe-go/v81)
- [hashicorp/go-retryablehttp — retry as RoundTripper](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp)
- [hashicorp/go-cleanhttp — clean http.Client without shared state](https://github.com/hashicorp/go-cleanhttp)
- [gregjones/httpcache — cache as RoundTripper](https://pkg.go.dev/github.com/gregjones/httpcache)
- [Middleware and RoundTrippers in Go (Calvin McLean, DEV)](https://dev.to/calvinmclean/middleware-and-roundtrippers-in-go-30pa)
- [Adding middleware to Go HTTP client requests (Jon Friesen)](https://jonfriesen.ca/articles/go-http-client-middleware)
- [Tripperwares: http.Client Middleware - chaining RoundTrippers (Steven Coffman, DEV)](https://dev.to/stevenacoffman/tripperwares-http-client-middleware-chaining-roundtrippers-3o00)
- [Writing HTTP client middleware in Go (echorand.me)](https://echorand.me/posts/go-http-client-middleware/)
- [Robust HTTP Client Design in Go (Leapcell)](https://leapcell.io/blog/robust-http-client-design-in-go)
- [Deep Dive into Go's HTTP Client Transport Layer (Leapcell)](https://leapcell.io/blog/deep-dive-into-go-s-http-client-transport-layer)
- [How we add off-the-shelf caching to our Go HTTP clients (Typeform)](https://medium.com/typeforms-engineering-blog/how-we-add-off-the-shelf-caching-to-our-go-http-clients-07d5d6b71600)
- [Unmarshaling Time values from JSON (Eli Bendersky)](https://eli.thegreenplace.net/2020/unmarshaling-time-values-from-json/)
- [Change JSON Time and Date format in Go (willem.dev)](https://www.willem.dev/articles/change-time-format-json/)
- [JSON, time, and golang (Roman Garanin)](https://romangaranin.net/posts/2021-02-19-json-time-and-golang/)
- [Go Error Handling: errors.Is, errors.As, Wrapping, and Custom Types (BackendBytes)](https://backendbytes.com/articles/go-error-handling-patterns/)
- [Error wrapping in Go (Bitfield Consulting)](https://bitfieldconsulting.com/posts/wrapping-errors)
- [Eleven Tips for Structuring Your Go Projects (Alex Edwards)](https://www.alexedwards.net/blog/11-tips-for-structuring-your-go-projects)
- [No nonsense guide to Go projects layout (laurentsv.com)](https://laurentsv.com/blog/2024/10/19/no-nonsense-go-package-layout.html)
- [Testing in Go: Golden Files (Ilija Eftimov)](https://ieftimov.com/posts/testing-in-go-golden-files/)
- [The Functional Options Pattern in Go (David Bacisin)](https://davidbacisin.com/writing/golang-options-pattern)
- [Understanding the Options Pattern in Go (Kittipat T., DEV)](https://dev.to/kittipat1413/understanding-the-options-pattern-in-go-390c)
- [go-chi/transport — Go HTTP Client Middleware](https://github.com/go-chi/transport)

---

*Architecture research for: idiomatic Go SDK library wrapping OpenHolidays REST API*
*Researched: 2026-05-26*
