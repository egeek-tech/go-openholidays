# Phase 2: Transport - Pattern Map

**Mapped:** 2026-05-27
**Files analyzed:** 13 new/modified production + test files + 1 fixture
**Analogs found:** 13 / 13 (all production/test files have strong Phase 1 analogs; fixture is greenfield with documented capture process)

## File Classification

| New / Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---------------------|------|-----------|----------------|---------------|
| `client.go` [NEW] | client / constructor + lifecycle | request-response (host of `http.Client` chain) | `errors.go` (exported-type-with-methods scaffold) + `types.go` (struct with godoc-ed exported fields) | role-match (no prior Client type exists; combine the two scaffolds) |
| `options.go` [NEW] | builder / functional options | config-build-time | `validate.go` (small, focused, function-per-purpose file with detailed godoc and per-func error wrapping) | role-match |
| `config.go` [NEW] | internal builder / unexported helpers | config-build-time | `date.go` lines 1-77 (unexported sentinel + constructor pattern) + `validate.go` (unexported helper pair `isTwoASCIIUppers/Lowers`) | role-match |
| `transport.go` [NEW] | middleware / RoundTripper decorators | request-response (cross-cutting) | `validate.go` (two small structs/funcs in one file, each with focused godoc + byte-level mechanical contract) | role-match |
| `countries.go` [NEW] | endpoint / HTTP client method | request-response (CRUD-Read) | `date.go` ParseDate (input → typed result + wrapped sentinel error) — closest existing pattern for "parse external input into a typed Go value with sentinels" | partial (no prior HTTP endpoint exists) |
| `errors.go` [MOD] | error surface | sentinel registry | self (existing `var (...)` block) | exact (append to existing block) |
| `validate.go` [MOD] | input validator | request-response (pre-network) | self (`validateCountry`, `validateLanguage` — W-01 fix reorders ASCII check before canonicalization) | exact (self-modification) |
| `internal_test.go` [MOD] | AST audit | meta-test | self (existing `allowedVars` map) | exact (one-line allowlist extension) |
| `client_test.go` [NEW] | unit test (client + concurrency + ctx) | request-response | `errors_test.go` (TestSentinelErrors table-driven shape; TestAPIError_Is t.Run-per-case) | role-match |
| `options_test.go` [NEW] | unit test (one per WithX) | config-build-time | `validate_test.go` (one `TestValidateXxx` per exported function with `accept/` + `reject/` table-driven subtests) | role-match |
| `transport_header_test.go` [NEW] | unit test (RoundTripper isolation) | request-response | `errors_test.go` TestAPIError_Is (struct-driven case table + t.Run-per-case with require/assert split) | role-match |
| `transport_logging_test.go` [NEW] | unit test (slog capture) | request-response (observability) | `errors_test.go` TestAPIError_Error (Sprintf-format contract via table + `assert.Equal`) | role-match |
| `countries_test.go` [NEW] | unit test (httptest end-to-end) | request-response | `types_test.go` TestHoliday_JSON (large-payload happy-path + nested t.Run subtests; mirrors testdata fixtures) | role-match |
| `validate_test.go` [MOD] | regression cases for W-01 | request-response (pre-network) | self (existing `successCases` / `rejectCases` tables — append 4 Unicode-fold cases to the reject table) | exact (self-modification) |
| `testdata/countries.json` [NEW] | fixture | static read input | (none — first fixture in repo) | no analog — follow types_test.go's "non-English strings reflect real upstream" convention |

---

## Pattern Assignments

### `errors.go` [MOD] — append `ErrResponseTooLarge` (D-24, CL-07)

**Self-analog:** existing `var (...)` block in `errors.go` lines 17-36.

**Copy this exact block style** when appending — same `var (...)` block, same one-blank-line separation, same godoc shape `"ErrXxx is returned ..."`:

```go
// errors.go — lines 17-36 (verbatim today)
var (
	// ErrInvalidCountry is returned for malformed country codes
	// (not exactly two ASCII letters after canonicalization).
	ErrInvalidCountry = errors.New("openholidays: invalid country code")

	// ErrInvalidLanguage is returned for malformed language codes
	// (not exactly two ASCII letters after canonicalization).
	ErrInvalidLanguage = errors.New("openholidays: invalid language code")

	// ErrDateRangeTooLarge is returned when the validFrom..validTo window
	// spans more than 3 calendar years inclusive.
	ErrDateRangeTooLarge = errors.New("openholidays: date range too large")

	// ErrInvalidDateRange is returned when validFrom is strictly after validTo.
	ErrInvalidDateRange = errors.New("openholidays: invalid date range")

	// ErrEmptyResponse is returned when the upstream returns a 2xx with an
	// empty body where a non-empty payload was required.
	ErrEmptyResponse = errors.New("openholidays: empty response body")
)
```

**Action for Phase 2:** Insert a sixth entry inside the same `var (...)` block, before the closing paren. Godoc shape must match: name + "is returned" + when. Pitfall 5 note (Decode masks oversize) belongs in the godoc per RESEARCH §"Pitfall 5":

```go
	// ErrResponseTooLarge is returned when an upstream response exceeds the
	// 10 MiB cap and the truncation is detected after JSON decode completes.
	// A response truncated mid-JSON-value returns a decode error wrapping
	// *json.SyntaxError instead.
	ErrResponseTooLarge = errors.New("openholidays: response too large")
```

**Error-string convention to copy:** every sentinel string begins with literal `"openholidays: "` (lower-case `openholidays`, colon, single space). The `errors_test.go` `TestSentinelErrors` block at lines 30-55 hard-codes that exact prefix check — any new sentinel must satisfy it.

---

### `internal_test.go` [MOD] — extend `allowedVars` (one-line append)

**Self-analog:** existing `allowedVars` map at lines 50-57.

```go
// internal_test.go — lines 50-57 (verbatim today)
var allowedVars = map[string]struct{}{
	"ErrInvalidCountry":    {},
	"ErrInvalidLanguage":   {},
	"ErrDateRangeTooLarge": {},
	"ErrInvalidDateRange":  {},
	"ErrEmptyResponse":     {},
	"errEmptyDate":         {},
}
```

**Action for Phase 2:** Add `"ErrResponseTooLarge": {},` to the map. The map is alphabetized loosely by introduction order (exported first, then unexported) — append the new entry after `ErrEmptyResponse` to match. The audit test (`TestNoInitOrGlobalState`) walks every `*.go` file at the repo root and fails closed if an unknown var is encountered. Adding the entry is mechanical and required to keep CLIENT-10 green after `errors.go` extension.

---

### `validate.go` [MOD] — W-01 fix (D-32)

**Self-analog:** existing `validateCountry` / `validateLanguage` at lines 28-55, plus the byte-arithmetic helpers `isTwoASCIIUppers` / `isTwoASCIILowers` at lines 102-116.

**Current (W-01 vulnerable) shape — `validate.go` lines 28-34:**

```go
func validateCountry(code string) (string, error) {
	canon := strings.ToUpper(code)            // <-- ToUpper FIRST (vulnerable)
	if !isTwoASCIIUppers(canon) {             // <-- shape check on canonical form
		return "", fmt.Errorf("%w: %q", ErrInvalidCountry, code)
	}
	return canon, nil
}
```

**The bug:** `strings.ToUpper("ﬁ")` folds Unicode through case mapping; `"KK"` (Kelvin sign U+212A twice) ToUpper-canonicalizes into something that `len(canon) != 2` does not catch the way the godoc claims. The fix reorders: shape check runs against the *original* bytes before any canonicalization.

**Target (W-01 fixed) shape:**

```go
func validateCountry(code string) (string, error) {
	// ASCII-shape check on ORIGINAL bytes BEFORE any case canonicalization
	// (W-01 fix: closes the Unicode case-fold bypass where, e.g., "KK"
	// — U+212A twice — would survive ToUpper and pass an over-permissive
	// len-2 check.)
	if !isTwoASCIILetters(code) {
		return "", fmt.Errorf("%w: %q", ErrInvalidCountry, code)
	}
	return strings.ToUpper(code), nil
}
```

**Helper to add (byte-level, no `unicode` package — D-32 explicit):**

```go
// isTwoASCIILetters reports whether s is exactly 2 bytes and each byte is
// an ASCII letter (A-Z or a-z). Byte arithmetic (rather than unicode.IsLetter)
// is intentional and mandatory: the W-01 fix requires that Unicode characters
// that fold to ASCII through strings.ToUpper / strings.ToLower (e.g. U+212A
// Kelvin sign → K) are rejected here, BEFORE canonicalization runs.
func isTwoASCIILetters(s string) bool {
	if len(s) != 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		b := s[i]
		if !((b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')) {
			return false
		}
	}
	return true
}
```

**`validateLanguage` mirrors the same reorder** — shape check first, then `strings.ToLower(code)`. The two existing helpers (`isTwoASCIIUppers`, `isTwoASCIILowers`) can stay or be removed — they are still useful as defense-in-depth assertions, but the W-01 fix makes them unreachable from `validateCountry`/`validateLanguage`. Recommend keeping them and adding a comment, or removing them with a single grep to confirm no other callers exist.

**Error-string convention preserved:** `fmt.Errorf("%w: %q", ErrInvalidCountry, code)` — `%w` wraps the sentinel; `%q` quotes the offending *original* input (not the canonicalized form) per D-23 / TestValidators_NoSensitiveData lock. **Do not pass `canon` to `%q`** — it must be the original `code` so callers see what they passed.

---

### `validate_test.go` [MOD] — append 4 W-01 regression cases

**Self-analog:** existing `rejectCases` table in `TestValidateCountry` at lines 51-62 and `TestValidateLanguage` at lines 112-122.

**Action:** Append 4 cases to *each* of the `rejectCases` slices (8 total). Use the same struct literal style, same `name` convention (kebab-or-space lowercase phrase describing the input). Example for `TestValidateCountry.rejectCases`:

```go
{name: "Kelvin-sign x2 must not fold to KK", input: "KK"},
{name: "Latin capital I with dot above", input: "İA"},   // İ + A
{name: "Latin small dotless i", input: "ıA"},            // ı + A
{name: "long s folds to S but is not ASCII", input: "ſA"}, // ſ + A
```

For `TestValidateLanguage.rejectCases`, the same characters in single-byte-letter contexts. **The same `errors.Is(err, ErrInvalidCountry)` / `ErrInvalidLanguage` assertion at lines 69-70 / 130-131 passes unchanged** — these are sentinel-identity assertions, not message assertions. The `assert.Contains(t, err.Error(), wantSub)` quoted-original-value check at lines 73-76 / 132-134 continues to hold because the offending value (`code`) is what gets `%q`-quoted, *not* the canonical form (D-23 + ERR-04 invariant).

---

### `client.go` [NEW] — `Client` struct + `NewClient` + `Close` (D-35, D-40)

**Closest analog for the file-level structure:** `errors.go` (exported type with methods, package-level godoc paragraph at top, every exported symbol with godoc starting with the symbol name).

**Closest analog for struct field godoc style:** `types.go` `Country` lines 143-151 and `Holiday` lines 94-138 — every exported field gets a `// FieldName ...` godoc immediately above it. **`Client` has no exported fields**, so the godoc lives on the struct as a whole + on each method.

**Copy this file-header style** from `errors.go` lines 1-7:

```go
// Package openholidays — client construction and lifecycle.
//
// This file declares the Client struct (immutable after NewClient returns),
// the NewClient constructor that applies functional Options to a fresh
// clientConfig, and the Close method (Phase 2: atomic flag flip; Phase 4
// will hook the cache-sweeper cancel here).
package openholidays
```

**Godoc shape for `NewClient`** — copy the style of `errors.go` `APIError.Is` (lines 83-95) which opens with "Symbol verb …" and includes a `Semantics:` block when multiple branches matter. For `NewClient`:

```go
// NewClient constructs an *openholidays.Client by applying the supplied
// Options to a fresh internal configuration and returning the resulting
// immutable client. NewClient never returns an error: all Options either
// silently accept any well-formed input (e.g. WithTimeout(0) means "no
// SDK-imposed timeout") or fall back to a documented default (e.g.
// WithLogger(nil) falls back to slog.Default()).
//
// Defaults applied when no Option supplies the field:
//
//   - HTTP client: &http.Client{} (no caller-supplied Timeout)
//   - Base URL:    https://openholidaysapi.org
//   - User-Agent:  go-openholidays/<Version>
//   - Logger:      slog.Default()
//   - Timeout:     15 * time.Second (per-request, applied via context.WithTimeout)
//
// The returned Client is safe for concurrent use from any goroutine
// (verified by TestClient_ConcurrentAccess under the race detector).
func NewClient(opts ...Option) *Client {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return &Client{
		http:      composeHTTPClient(cfg),
		baseURL:   cfg.baseURL,
		userAgent: cfg.userAgent,
		logger:    cfg.logger,
		timeout:   cfg.timeout,
	}
}
```

**Godoc shape for `Close`** — copy `APIError.Is` "documented invariant" style. From RESEARCH lines 760-767:

```go
// Close is the idempotent shutdown hook. In v0.1.0 it is a no-op that flips
// an internal closed flag; future versions will stop background goroutines
// (cache sweeper) here. Safe to call from any goroutine; subsequent calls
// also return nil.
func (c *Client) Close() error {
	c.closed.Store(true)
	return nil
}
```

**Struct shape** (D-40, RESEARCH §"Pitfall 6"):

```go
// Client is the immutable HTTP client for the OpenHolidays API. Construct
// one via NewClient and reuse it across goroutines for the lifetime of the
// program; Client carries no per-call state.
type Client struct {
	http      *http.Client
	baseURL   string
	userAgent string
	logger    *slog.Logger
	timeout   time.Duration
	closed    atomic.Bool
	// Phase 4 will add: closeOnce sync.Once; cacheSweeper context.CancelFunc
}
```

All Client fields are unexported (locked Phase 2 surface). Imports follow Phase 1's grouped-by-origin style — see `errors.go` lines 10-13 / `validate.go` lines 11-14.

---

### `options.go` [NEW] — `Option` type + 5 `WithX` constructors (D-35..D-39)

**Closest analog:** `validate.go` — small, focused file; one short godoc per exported symbol; per-function defensive guards (analogous to `WithUserAgent("")` no-op = `validateCountry("") → error`).

**Copy this header-and-imports style** from `validate.go` lines 1-14:

```go
// Package openholidays — functional Option constructors and the Option type.
//
// This file declares Option (the functional-option signature) and the five
// public WithX constructors that callers compose at NewClient time. Options
// mutate only the internal *clientConfig (declared in config.go); they never
// touch the Client after construction (D-35).

package openholidays

import (
	"log/slog"
	"net/http"
	"time"
)
```

**Godoc shape for `Option`** — copy the typed-string-with-constants pattern from `types.go` lines 14-23 (`HolidayType`):

```go
// Option configures a Client at construction time. Options compose via
// NewClient: each Option mutates a private *clientConfig builder, and the
// final *Client is constructed from that builder. After NewClient returns,
// the Client is immutable; further Option calls on a constructed Client
// have no effect by design (no setter exists).
type Option func(*clientConfig)
```

**Godoc shape for each `WithX`** — copy `validateLanguage` shape (lines 36-48): one paragraph stating what it does, one paragraph stating accepted input + documented sentinel behavior (e.g., empty / nil fallback). Example for `WithUserAgent`:

```go
// WithUserAgent overrides the default User-Agent header
// ("go-openholidays/<Version>") sent on every HTTP request.
//
// An empty string is treated as "use the default" — the library never sends
// an empty User-Agent because some CDNs reject empty-UA requests. To
// suppress the User-Agent entirely, the caller must use WithHTTPClient and
// a custom http.RoundTripper.
func WithUserAgent(s string) Option {
	return func(cfg *clientConfig) {
		if s != "" {
			cfg.userAgent = s
		}
	}
}
```

**Consistency rule across all 5 WithX:** every constructor has the same one-liner body shape — a `return func(cfg *clientConfig) { ... }` closure that guards "is the caller input usable?" then assigns. **No constructor returns an error** (per D-35: Options never fail).

`WithBaseURL` should auto-trim the trailing slash per RESEARCH §"Open Question 4":

```go
// WithBaseURL overrides the default base URL. A trailing slash, if present,
// is trimmed so endpoint paths (always beginning with "/") concatenate
// cleanly. WithBaseURL("") is treated as "use the default".
func WithBaseURL(u string) Option {
	return func(cfg *clientConfig) {
		if u == "" {
			return
		}
		cfg.baseURL = strings.TrimRight(u, "/")
	}
}
```

(Requires importing `"strings"` — already a Phase 1 import pattern.)

---

### `config.go` [NEW] — `clientConfig` + `defaultConfig` + `composeHTTPClient` + `buildTransport` (D-29, D-37)

**Closest analog for unexported state + constructor:** `date.go` lines 18-46 — package-level `const dateLayout`, unexported package-level sentinel (`errEmptyDate`), constructor functions (`NewDate`, `ParseDate`) — all with concise godoc.

**Closest analog for unexported helper pairing:** `validate.go` lines 102-116 — two parallel small helpers (`isTwoASCIIUppers` / `isTwoASCIILowers`) sharing the same byte-arithmetic rationale in their godoc.

**Copy this file-header style:**

```go
// Package openholidays — internal client configuration and HTTP transport
// composition.
//
// This file declares the unexported clientConfig builder that holds every
// option-supplied value before NewClient finalizes the *Client; the
// default-values constructor; the *http.Client shallow-copy helper that
// neutralizes hidden-mutability (Pitfall HTTP-1, D-37); and the RoundTripper
// chain composer that wires headerTransport → loggingTransport → underlying
// transport (D-29).

package openholidays

import (
	"log/slog"
	"net/http"
	"time"
)
```

**`clientConfig` field godoc style** — copy `types.go` `Country` (lines 143-151), one comment per field, even unexported:

```go
// clientConfig is the internal builder state filled by Options between
// NewClient's start and Client construction. Unexported — never escapes
// the package. Field-by-field semantics mirror the public WithX godoc.
type clientConfig struct {
	httpClient *http.Client  // wrapped in composeHTTPClient via shallow copy
	baseURL    string        // trailing-slash-trimmed; concatenated with "/EndpointPath"
	userAgent  string        // non-empty; injected by headerTransport
	logger     *slog.Logger  // non-nil; falls back to slog.Default()
	timeout    time.Duration // 0 disables the SDK-imposed timeout
}
```

**`defaultConfig` shape** — copy `RESEARCH §"Pattern 1"` skeleton (matches Phase 1 minimalism):

```go
func defaultConfig() *clientConfig {
	return &clientConfig{
		httpClient: &http.Client{},
		baseURL:    "https://openholidaysapi.org",
		userAgent:  "go-openholidays/" + Version,
		logger:     slog.Default(),
		timeout:    15 * time.Second,
	}
}
```

`Version` is the existing const in `version.go` line 10 — no import needed (same package).

**`composeHTTPClient` (D-37 shallow copy, Pitfall HTTP-1)** — godoc must call out the mutability gate explicitly:

```go
// composeHTTPClient shallow-copies cfg.httpClient so that caller mutations of
// the original *http.Client after NewClient returns do not affect the SDK
// (Pitfall HTTP-1). The Transport on the copy is replaced with the chain
// returned by buildTransport (D-29).
func composeHTTPClient(cfg *clientConfig) *http.Client {
	cp := *cfg.httpClient
	cp.Transport = buildTransport(cfg)
	return &cp
}
```

**`buildTransport`** — copy RESEARCH §"Pattern 2" verbatim (chain order is load-bearing):

```go
// buildTransport composes the RoundTripper chain for Phase 2:
//
//	req → headerTransport → loggingTransport → underlying
//
// Where underlying is cfg.httpClient.Transport if non-nil, else
// http.DefaultTransport. Phase 3 will add retryTransport outermost; Phase 4
// will add cacheTransport and hookTransport. Pre-1.0, this constructor is
// edited in place rather than abstracted into a generic middleware list
// (D-29 explicit).
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

---

### `transport.go` [NEW] — `headerTransport` + `loggingTransport` (D-30, D-31)

**Closest analog:** `validate.go` — two parallel small structs each with focused godoc + mechanical (byte/clone/log) contract. Use the same "two helpers with shared rationale in their godocs" approach.

**Copy header-style:**

```go
// Package openholidays — HTTP RoundTripper decorators.
//
// This file declares two http.RoundTripper implementations that compose via
// a `next` field:
//
//   - headerTransport injects Accept: application/json and User-Agent on
//     every request, cloning the inbound *http.Request first to honor the
//     stdlib RoundTripper contract (Pitfall HTTP-2, D-30).
//   - loggingTransport emits one structured slog.LevelDebug record per
//     round-trip with the OBS-02 fields (method, path, status, duration_ms,
//     attempt, bytes_in). Response body is never read here (Pitfall OBS-1,
//     OBS-01, D-31).

package openholidays

import (
	"log/slog"
	"net/http"
	"time"
)
```

**`headerTransport` (D-30)** — RESEARCH §"Pattern 3" verbatim. Critical contract notes in the godoc per Pitfall HTTP-2:

```go
// headerTransport injects standard request headers (Accept, User-Agent) into
// every outgoing request. Caller-supplied values for either header are
// preserved — only absent values are populated with the SDK defaults.
//
// Implementation note: the inbound *http.Request is cloned via
// req.Clone(req.Context()) BEFORE any header mutation. The stdlib
// RoundTripper contract forbids mutating the caller's request; the
// Clone deep-copies the Header map (req.WithContext does not).
type headerTransport struct {
	userAgent string
	next      http.RoundTripper
}

func (h *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqCopy := req.Clone(req.Context())
	if reqCopy.Header.Get("Accept") == "" {
		reqCopy.Header.Set("Accept", "application/json")
	}
	if reqCopy.Header.Get("User-Agent") == "" {
		reqCopy.Header.Set("User-Agent", h.userAgent)
	}
	return h.next.RoundTrip(reqCopy)
}
```

**`loggingTransport` (D-31, OBS-02)** — RESEARCH §"Pattern 4" verbatim:

```go
// loggingTransport emits a single slog.LevelDebug record per HTTP round-trip.
//
// Fields (OBS-02): method, path, status, duration_ms, attempt, bytes_in.
// `attempt` is hardcoded to 1 in Phase 2; Phase 3's retry transport will
// inject the real attempt counter via a context value. `bytes_in` is
// resp.ContentLength, which is -1 for HTTP/2 chunked responses (the live
// OpenHolidays API uses HTTP/2 — expect -1 in production logs).
//
// The response body is NEVER read here (OBS-01 + Pitfall OBS-1); doing so
// would consume bytes before the endpoint decoder runs.
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
		slog.Int("attempt", 1),
		slog.Int64("bytes_in", bytesIn(resp)),
	)
	return resp, err
}

func statusOf(resp *http.Response) int {
	if resp == nil {
		return -1
	}
	return resp.StatusCode
}

func bytesIn(resp *http.Response) int64 {
	if resp == nil {
		return -1
	}
	return resp.ContentLength
}
```

`statusOf` / `bytesIn` follow the `isTwoASCIIUppers` / `isTwoASCIILowers` *parallel-helper* pattern from `validate.go` lines 102-116: small, single-purpose, defensive against nil, paired in the same file. Note: `slog` log message string `"openholidays http"` keeps the same `openholidays` prefix as the error strings — consistency with the brand.

---

### `countries.go` [NEW] — `Countries` endpoint + `buildAPIError` helper (D-41..D-45)

**Closest analog for the endpoint method:** `date.go` `ParseDate` (lines 54-63) — same shape: input → parse → wrapped error on failure, typed value on success, sentinel quoting via `%q` for failures. Apply that template to the bigger HTTP scenario.

**Closest analog for godoc shape of an exported method:** `types.go` `Country.NameFor` (lines 153-165) — `// MethodName verb …` opening, multi-paragraph body covering match semantics, single example invocation. For `Countries`:

```go
// Countries fetches the list of supported countries from the upstream API.
//
// Every country includes its ISO 3166-1 alpha-2 IsoCode, a multi-language
// Name array, and the country's official languages (ISO 639-1 codes). Use
// Country.NameFor(lang) to look up the localized name for a given language.
//
// The per-request timeout configured via WithTimeout (default 15s) is honored
// via context.WithTimeout; caller-supplied cancellation interrupts in-flight
// HTTP within ≤ 100ms (CLIENT-09). On a non-2xx response, the returned error
// is an *APIError carrying the upstream status, the request path
// ("/Countries"), the first 4 KiB of the response body, and a best-effort
// Message parsed from RFC 7807 ProblemDetails. Network and decode failures
// return fmt.Errorf-wrapped errors that preserve ctx sentinels
// (errors.Is(err, context.Canceled) holds across the wrap).
func (c *Client) Countries(ctx context.Context) ([]Country, error) {
```

**Endpoint body** — copy RESEARCH §"Pattern 5" / §"Architectural Responsibility Map" verbatim. Critical pieces and their analog:

1. **Nil-ctx guard** — D-42: `if ctx == nil { return nil, errors.New("openholidays: nil context") }`. The error string convention `"openholidays: "` is the same as Phase 1 sentinels.
2. **`context.WithTimeout` wrap** — D-26/D-27: only when `c.timeout > 0` (D-28 — zero means no SDK timeout).
3. **`http.NewRequestWithContext`** (not the deprecated `http.NewRequest`) — RESEARCH §"State of the Art" / Pitfall against Go 1.13 deprecation.
4. **Defer drain-then-close** — D-45 verbatim:
   ```go
   defer func() {
       _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
       _ = resp.Body.Close()
   }()
   ```
   The `LimitReader(resp.Body, maxResponseBytes+1)` cap on the drain itself is belt-and-braces against a malicious infinite stream (Pitfall 3 in RESEARCH).
5. **4xx/5xx → `buildAPIError`** — D-43.
6. **Bounded decode** — D-44: `json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&countries)`.
7. **Sentinel-byte truncation gate** — D-24/D-49: 1-byte read post-Decode; `n > 0` means truncation; wrap `ErrResponseTooLarge`.
8. **`io.EOF → ErrEmptyResponse`** — RESEARCH §"Open Question 1" recommendation: empty array `[]` is valid; literal empty body (Decode returns `io.EOF`) wraps `ErrEmptyResponse`.

**Error-wrap shape** consistent with `date.go` line 60-61: `fmt.Errorf("openholidays: <where>: %w", err)`. Example failure points:

| Step | Wrap |
|------|------|
| build request | `fmt.Errorf("openholidays: build /Countries request: %w", err)` |
| `c.http.Do` failed | `fmt.Errorf("openholidays: GET /Countries: %w", err)` |
| decode failed | `fmt.Errorf("openholidays: decode /Countries: %w", err)` |
| oversize sentinel | `fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)` |

**Crucial:** every wrap preserves `errors.Is(err, context.Canceled)` / `errors.Is(err, ErrResponseTooLarge)` semantics (verified at `errors_test.go` TestSentinels_ErrorsIs as the existing test pattern).

**`buildAPIError` helper + `parseAPIMessage`** — RESEARCH §"Pattern 5" verbatim. `parseAPIMessage` priority order **must be `detail` → `title` → `error`** per RESEARCH §"Summary": OpenHolidays returns RFC 7807 `ProblemDetails` where `detail` is the winning live field on 2026-05-27.

**Imports for this file:**

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)
```

**Constant placement:** `const maxResponseBytes = 10 << 20` (D-25) — declared at file top, immediately after imports. Follows `date.go` line 18 `const dateLayout = "2006-01-02"` pattern. Comment matches `date.go` style:

```go
// maxResponseBytes is the hard ceiling on any decoded response body (D-25).
// 10 MiB. Not configurable in v0.1.0 — PROJECT.md fixes the cap.
const maxResponseBytes = 10 << 20
```

---

### `client_test.go` [NEW] — TestNewClient, TestClient_Close, TestClient_ConcurrentAccess, TestClient_ContextCancel

**Closest analog:** `errors_test.go` (multiple `TestXxx` per file, each owning one exported prod function/area; struct-driven case tables; `require` for preconditions / `assert` for verifications). Phase 2's `client_test.go` shares the same shape because `client.go` exports multiple symbols (`NewClient`, `Close`, the `Client` type itself) and the concurrency / cancel invariants are per-test functions.

**Copy this header style** from `errors_test.go` lines 1-10 (test file header is minimal — package, imports, no extra godoc comment block):

```go
package openholidays

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

**Test-function-per-prod-function rule (Gold Rule 3):** one `TestXxx` per exported prod function plus `TestClient_*` umbrella tests for cross-cutting behaviors:

| Prod symbol | Test function |
|-------------|---------------|
| `NewClient` | `TestNewClient` |
| `Client.Close` | `TestClient_Close` |
| (CLIENT-07) | `TestClient_ConcurrentAccess` |
| (CLIENT-09) | `TestClient_ContextCancel` |

**`TestNewClient` shape** — copy `TestSentinelErrors` table-iteration (errors_test.go lines 18-55) with a name+expected-default record per subcase:

```go
func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("defaults applied when no Option supplied", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		// internal package test: directly inspect fields
		assert.Equal(t, "https://openholidaysapi.org", c.baseURL)
		assert.Equal(t, "go-openholidays/"+Version, c.userAgent)
		assert.Equal(t, 15*time.Second, c.timeout)
		require.NotNil(t, c.logger)
		require.NotNil(t, c.http)
	})

	t.Run("WithBaseURL trims trailing slash", func(t *testing.T) { ... })
	t.Run("WithHTTPClient shallow-copies (mutation isolation)", func(t *testing.T) { ... })
	// ... one t.Run per documented invariant
}
```

**`TestClient_ConcurrentAccess`** — RESEARCH §"Test pattern: concurrent access" verbatim, 50 goroutines, `sync.WaitGroup`, identical-payload assertion via `assert.Equal(results[0], results[i])`. Use `t.Cleanup(srv.Close)` for the `httptest.NewServer` (Pitfall TEST-2).

**`TestClient_ContextCancel`** — RESEARCH §"Test pattern: context cancellation under 100 ms" verbatim. Asserts `errors.Is(err, context.Canceled)` (the wrap must preserve the sentinel) and `elapsed < 200ms` (2× CI slack per D-48).

---

### `options_test.go` [NEW] — one `TestWithX` per Option

**Closest analog:** `validate_test.go` — exactly one `TestValidateXxx` per exported (well, unexported here) validator, each with `successCases`/`rejectCases` struct tables and per-case `t.Run`. Phase 2's `options_test.go` follows the same shape: one `TestWithX` function per `WithX`, each exercising the documented happy path and the documented "no-op" fallback.

**Tests required** (one per Option — 5 tests):

| Test function | Locks |
|--------------|-------|
| `TestWithHTTPClient` | CLIENT-02 shallow-copy (mutation isolation per Pitfall HTTP-1) |
| `TestWithBaseURL` | CLIENT-03 + trailing-slash trim (RESEARCH OQ-4) |
| `TestWithUserAgent` | CLIENT-04 + empty-string no-op (D-38) |
| `TestWithLogger` | CLIENT-05 + nil-logger fallback (D-39) |
| `TestWithTimeout` | CLIENT-06 + zero-duration "no SDK timeout" (D-28) |

**Test case style** — copy `validate_test.go` lines 26-78 verbatim:

```go
func TestWithBaseURL(t *testing.T) {
	t.Parallel()

	type tc struct {
		name string
		in   string
		want string
	}
	cases := []tc{
		{name: "no-trailing-slash passes through", in: "https://example.test", want: "https://example.test"},
		{name: "trailing slash trimmed", in: "https://example.test/", want: "https://example.test"},
		{name: "multiple trailing slashes trimmed", in: "https://example.test///", want: "https://example.test"},
		{name: "empty string is no-op (keeps default)", in: "", want: "https://openholidaysapi.org"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cli := NewClient(WithBaseURL(c.in))
			require.NotNil(t, cli)
			assert.Equal(t, c.want, cli.baseURL)
		})
	}
}
```

`require.NotNil` for the precondition (the client must be constructed), `assert.Equal` for the verification. Same `t.Parallel()` etiquette as `validate_test.go`: every top-level test and every leaf `t.Run` calls it.

---

### `transport_header_test.go` [NEW] — `TestHeaderTransport_RoundTrip`

**Closest analog:** `errors_test.go` `TestAPIError_Is` (lines 151-222) — struct-table-driven, multiple `t.Run` subcases each asserting one branch of an `Is`-like contract. The header transport has two documented branches (sets-when-absent / preserves-when-present); each is a subcase.

**Copy the `tc` struct literal style** from `errors_test.go` lines 162-207:

```go
// roundTripperFunc is the test-only adapter that lets a plain func satisfy
// http.RoundTripper. Used as the `next` slot for unit-isolating a single
// transport in the chain (RESEARCH §"Test pattern: header transport").
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestHeaderTransport_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("sets defaults when caller did not supply them", func(t *testing.T) {
		t.Parallel()
		var captured *http.Request
		h := &headerTransport{
			userAgent: "go-openholidays/0.1.0",
			next: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				captured = r
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
			}),
		}
		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)

		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, "application/json", captured.Header.Get("Accept"))
		assert.Equal(t, "go-openholidays/0.1.0", captured.Header.Get("User-Agent"))
		assert.Empty(t, req.Header.Get("Accept"),
			"caller's original req.Header must be untouched (Pitfall HTTP-2)")
	})

	t.Run("preserves caller-supplied Accept and User-Agent", func(t *testing.T) { ... })
}
```

The `roundTripperFunc` adapter is the test-only middleware shim. Phase 1 has no equivalent because Phase 1 has no transport code; declare it once here and reuse in `transport_logging_test.go` (same-package tests share unexported helpers — see `errors_test.go` `namedSentinel` struct reused inside the same file).

---

### `transport_logging_test.go` [NEW] — `TestLoggingTransport_RoundTrip`

**Closest analog:** `errors_test.go` `TestAPIError_Error` (lines 107-146) — Sprintf-format contract verified by string comparison; same shape, but here the "format" is the JSON record emitted by `slog.NewJSONHandler` and the contract is field presence + values.

**Copy this pattern:** capture slog records to a `bytes.Buffer` via a `slog.NewJSONHandler`, then assert per-field via `json.Unmarshal` into a `map[string]any`. RESEARCH §"Test pattern: logging transport" verbatim:

```go
func TestLoggingTransport_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("emits Debug record with all OBS-02 fields", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		l := &loggingTransport{
			logger: logger,
			next: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200, ContentLength: 6055,
					Body: io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}
		req, err := http.NewRequest(http.MethodGet, "https://example.test/Countries", nil)
		require.NoError(t, err)
		_, err = l.RoundTrip(req)
		require.NoError(t, err)

		var rec map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
		assert.Equal(t, "DEBUG", rec["level"])
		assert.Equal(t, "GET", rec["method"])
		assert.Equal(t, "/Countries", rec["path"])
		assert.EqualValues(t, 200, rec["status"])
		assert.EqualValues(t, 1, rec["attempt"])
		assert.EqualValues(t, 6055, rec["bytes_in"])
		dur, ok := rec["duration_ms"].(float64)
		require.True(t, ok)
		assert.GreaterOrEqual(t, dur, 0.0)
	})

	t.Run("ContentLength=-1 forwarded as bytes_in=-1 (HTTP/2 chunked)", func(t *testing.T) { ... })
}
```

**`require` for preconditions** (`NoError` on request building, `NoError` on JSON unmarshal, `True` on type assertion) and **`assert` for verifications** (each field's expected value) — exactly the split used in `errors_test.go` lines 30-55.

---

### `countries_test.go` [NEW] — `TestClient_Countries` (happy + 4xx + 5xx + ctx-cancel + oversize)

**Closest analog:** `types_test.go` `TestHoliday_JSON` (lines 120+) — large wire-shape contract test with multiple `t.Run` subcases against a JSON payload that mirrors a verified live response. Phase 2 elevates this to `httptest.NewServer` (which the validate test helpers never used because there was no HTTP code yet).

**One TestXxx per exported prod function (Gold Rule 3):** Phase 2's `Countries` is one exported method on `Client`; therefore exactly one `TestClient_Countries` function in `countries_test.go`. All scenarios (happy, 4xx, 5xx, ctx-cancel, oversize) live as `t.Run` subcases inside it.

**Header style** mirrors `types_test.go` lines 1-18:

```go
// Package openholidays — tests for the Countries endpoint method.
//
// One TestXxx per exported production function per Gold Rule 3.
// Every scenario lives in a t.Run subtest. Non-English strings in the
// fixture (e.g. "Polska", "Deutschland") mirror real upstream OpenHolidays
// responses and are admitted per CONVENTIONS.md Rule 1 testdata-fixture
// exception.
package openholidays

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

**Fixture-capture-date const** — RESEARCH §"Test pattern: testdata fixture" specifies a package-level test-only const documenting capture date:

```go
// countriesFixtureCapturedAt records the date testdata/countries.json was
// captured from the live API. Re-capture when the upstream schema is
// suspected to have drifted. The fixture is not the authoritative shape —
// the live API is.
const countriesFixtureCapturedAt = "2026-05-27"
```

**Subtest layout** (every case inside `TestClient_Countries`):

| Subtest name | Behavior | Asserts |
|--------------|----------|---------|
| `"happy path returns PL+DE"` | httptest serves `testdata/countries.json` | 2 entries, `IsoCode=="PL"` first, `Country.NameFor("pl")=="Polska"` |
| `"4xx returns *APIError with parsed Message"` | server returns 404 + RFC 7807 `{"detail": "Country not supported"}` | `errors.As(err, &apiErr)`; `apiErr.StatusCode == 404`; `apiErr.Path == "/Countries"`; `apiErr.Message == "Country not supported"` |
| `"5xx returns *APIError with title fallback"` | server returns 500 + `{"title": "Internal Server Error"}` | `apiErr.Message == "Internal Server Error"` (parseAPIMessage fallback chain detail → title) |
| `"4xx body truncated at 4 KiB"` | server returns 4xx + 8 KiB body | `len(apiErr.Body) == 4096` (Phase 1 D-17 cap) |
| `"ctx cancel interrupts within 200ms"` | server hangs 10s; cancel at 50ms | `errors.Is(err, context.Canceled)` AND `elapsed < 200ms` (D-48) |
| `"oversize triggers ErrResponseTooLarge"` | server streams 11 MiB; **not** `t.Parallel` (reads `runtime.NumGoroutine`) | `errors.Is(err, ErrResponseTooLarge)`; post-settle `NumGoroutine` delta ≤ 2 (D-49) |
| `"empty body wraps ErrEmptyResponse"` | server returns 200 with `""` body | `errors.Is(err, ErrEmptyResponse)` (RESEARCH §OQ-1) |

**Fixture-read pattern** — files in `testdata/` are read with `os.ReadFile(filepath.Join("testdata", "countries.json"))` (RESEARCH §"Test pattern: concurrent access"). Go's `testing` framework guarantees tests run with the package directory as CWD, so the relative path works.

---

### `testdata/countries.json` [NEW] — 2-country fixture (PL + DE)

**No prior analog** — first testdata fixture in the repo. Follow the convention `types_test.go` lines 6-7 documents: **non-English strings (e.g. "Polska", "Deutschland", "Wigilia Bożego Narodzenia") are admitted ONLY when they mirror real upstream responses** (CONVENTIONS.md Rule 1 exception).

**Capture method** (RESEARCH §"Test pattern: testdata fixture"):

```bash
curl -s -H "Accept: application/json" -H "User-Agent: go-openholidays-test/0.1.0" \
    https://openholidaysapi.org/Countries \
  | jq '[.[] | select(.isoCode == "PL" or .isoCode == "DE")]' \
  > testdata/countries.json
```

**Expected content** (verified live 2026-05-27 per RESEARCH lines 1042-1061):

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

**Capture-date documentation:** Go's `encoding/json` does not allow comments inside JSON; the capture date lives in `countries_test.go` as the `countriesFixtureCapturedAt` const (see above).

---

## Shared Patterns

### Pattern: Error string convention `"openholidays: "` (Phase 1 D-23 / Rule 1)

**Source:** `errors.go` lines 19-35 (every sentinel `errors.New("openholidays: ...")`), `validate.go` line 31 (`fmt.Errorf("%w: %q", ErrInvalidCountry, code)` — wrap inherits the prefix), `date.go` line 60 (`fmt.Errorf("openholidays: invalid date %q: %w", s, err)`).

**Apply to:** every error string created in Phase 2 (sentinel literal in `errors.go`, every `fmt.Errorf` in `countries.go`, the nil-ctx guard `errors.New("openholidays: nil context")` in `Countries`). Locked by `errors_test.go` TestSentinelErrors lines 37-40 — the prefix check is mechanical.

```go
// Pattern (literal sentinel)
ErrXxx = errors.New("openholidays: <human-readable lowercase phrase>")

// Pattern (wrap of caller-input quote)
fmt.Errorf("%w: %q", ErrXxx, originalCallerInput)

// Pattern (wrap of upstream/stdlib error with context phrase)
fmt.Errorf("openholidays: <what we were doing>: %w", err)
```

---

### Pattern: Testify testing — `require` for preconditions, `assert` for verifications, one `TestXxx` per exported prod function, every case in `t.Run`, `t.Parallel()` per level (Gold Rule 3)

**Source:** every Phase 1 `_test.go` file:
- `errors_test.go` lines 31-32 (`require.NotNil(t, s.err); ... assert.True(t, len(msg) > ...)` — require for the precondition, assert for the verification),
- `validate_test.go` lines 31-44 (`successCases` struct table, `t.Run(prefix+name, func(t *testing.T) { t.Parallel(); ... })`),
- `types_test.go` lines 23-49 (`HolidayType_constants` — one `t.Run` per constant, parallel).

**Apply to:** every `_test.go` file Phase 2 adds. Rules in order:

1. **Top-level `TestXxx` calls `t.Parallel()` immediately** (the exception is `TestClient_Countries/oversize` per RESEARCH D-49: `runtime.NumGoroutine` racy → not parallel).
2. **Every leaf `t.Run` calls `t.Parallel()` immediately** (Phase 1 etiquette — `validate_test.go` lines 40, 65, 100, 125, etc.).
3. **`require.X` aborts the case** on a failed precondition (NotNil, NoError on setup, type-assertion ok); **`assert.X` reports without aborting** on each verification. Mixing them is per `errors_test.go` line 142 (`require.NotNil(t, c.err, ...)` then `require.Equal(t, c.want, c.err.Error())` — both Equal calls require here because the test cannot continue if one fails, but in lower-stakes cases use `assert`).
4. **One `TestXxx` function per exported production function** — see the file-by-file table above. The audit-style enforcement isn't mechanical, but the file shape (`countries_test.go` has exactly one `TestClient_Countries`) is the contract.
5. **Table-driven by default when ≥ 2 cases share setup** — copy `validate_test.go` `successCase` / `rejectCase` struct shape (lines 26-31 + 51-62). Each table case has a `name` field used as the `t.Run` argument.

---

### Pattern: File-level package-godoc header (Rule 1 + Phase 1 style)

**Source:** `doc.go` (package overview at the package level), then every `errors.go` / `validate.go` / `types.go` / `date.go` opens with a `// Package openholidays — <subject>.` line followed by a one-paragraph context block ending before `package openholidays`.

**Apply to:** every new `.go` file Phase 2 adds (production and test). Skeleton:

```go
// Package openholidays — <one-line subject of this file>.
//
// <one paragraph explaining what this file ships and why it exists,
// referencing the relevant phase decisions / requirements where useful.>

package openholidays
```

Real examples to mimic verbatim:
- `errors.go` lines 1-7 (error surface),
- `validate.go` lines 1-7 (input validators),
- `types.go` lines 1-9 (domain types).

**Test file headers** are lighter (`types_test.go` lines 1-7 is the canonical shape); a single `// Package openholidays — tests for X.` line suffices, optionally with a note when the file admits non-English testdata strings.

---

### Pattern: Godoc for exported symbols (Rule 1 + Gold Rule 1)

**Source:** every exported symbol in Phase 1.

**Apply to:** every Phase 2 exported symbol (`NewClient`, `Client`, `Option`, `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithLogger`, `WithTimeout`, `Client.Close`, `Client.Countries`, `ErrResponseTooLarge`). Rules:

1. **Opening line starts with the symbol name** — `// NewClient ...`, `// Client ...`, `// ErrResponseTooLarge is returned when ...`. See `types.go` line 153: `// NameFor returns the localized ...`.
2. **Verb form is third-person present** — "returns", "constructs", "is" — see `errors.go` line 18 `// ErrInvalidCountry is returned for ...`.
3. **Multi-paragraph when needed** — blank-line-separated paragraphs, no Markdown. See `errors.go` lines 38-56 (`APIError`).
4. **Examples inside godoc use indented code blocks** (4 spaces / a tab), not Markdown fences — see `errors.go` lines 44-56.

---

### Pattern: Test cleanup with `t.Cleanup` (Pitfall TEST-2)

**Source:** no Phase 1 file uses `t.Cleanup` yet (Phase 1 has no resources to release). Phase 2 is the first introduction.

**Apply to:** every `httptest.NewServer` call in Phase 2 tests:

```go
srv := httptest.NewServer(handler)
t.Cleanup(srv.Close)
```

`t.Cleanup` runs even when a subtest fails (and runs in reverse order of registration), which `defer` does not when the goroutine running the test ends abruptly. RESEARCH §"Standard Stack / Supporting" calls this out as the canonical pattern. **Do not use `defer srv.Close()`** — `t.Cleanup(srv.Close)` is the test-library-aware form.

---

### Pattern: Imports ordering (Phase 1 stdlib convention)

**Source:** every Phase 1 `_test.go` file uses two import groups:
1. **stdlib imports** (alphabetical),
2. **third-party imports** (alphabetical, only testify in Phase 1, separated by blank line).

Production files use only stdlib (zero-dep mandate), so they have one import group.

**Apply to:** every Phase 2 file. Production files: stdlib only, single group. Test files: stdlib group, blank line, `github.com/stretchr/testify/{assert,require}` second group.

Reference: `errors_test.go` lines 3-10, `validate_test.go` lines 10-18, `internal_test.go` lines 18-31.

---

## No Analog Found

| File | Role | Reason |
|------|------|--------|
| `testdata/countries.json` | static fixture | First testdata fixture in the repo — no prior file to copy from. The convention documented in `types_test.go` lines 5-7 (non-English strings admitted when they mirror live upstream) is followed; the capture script and 2-country shape are documented in RESEARCH §"Test pattern: testdata fixture". |
| `countries.go` (HTTP endpoint method) | endpoint | First HTTP-endpoint method in the repo. Closest pattern shift is `date.go` `ParseDate` (input → typed result + wrapped sentinel) but the network / RoundTripper / drain-close / oversize-sentinel layering is brand-new. Planner should treat the RESEARCH §"Pattern 5" skeleton as the authoritative shape. |

Both gaps are well-covered by RESEARCH.md skeletons (verbatim code in §"Pattern 5" and §"Test pattern: testdata fixture"). Planner can copy those into plan actions directly.

---

## Metadata

**Analog search scope:** repo root (`/data/git/private/holidays/`) — Phase 1 ships five production files (`doc.go`, `errors.go`, `date.go`, `types.go`, `validate.go`, `version.go`) and five test files (`errors_test.go`, `date_test.go`, `types_test.go`, `validate_test.go`, `internal_test.go`).

**Files scanned (read in full):** `errors.go`, `errors_test.go`, `validate.go`, `validate_test.go`, `types.go`, `internal_test.go`, `version.go`, `doc.go`. Targeted reads on `date.go` (lines 1-77 — pattern excerpts), `types_test.go` (lines 1-120 — test header + table-test shape).

**Pattern extraction date:** 2026-05-27.

**Lints in scope:** PROJECT.md golangci-lint v2 with `govet`, `errcheck`, `staticcheck`, `gosec`, `revive`, `gocritic`. All Phase 1 source passes; copying these patterns keeps Phase 2 in the same state.

**Phase 2 tightenings layered on top of Phase 1 patterns:**

1. `slog` log-message string `"openholidays http"` (no colon) is the only deviation from the `"openholidays: "` *error* string convention — log messages are not errors, and slog conventionally uses a short, no-punctuation event name as the first positional arg.
2. The non-parallel test (`TestClient_Countries/oversize`) is the first Phase 2 deviation from "every test is parallel"; rationale (D-49 reads `runtime.NumGoroutine`) is documented in the godoc on that subcase.
3. `t.Cleanup(srv.Close)` is introduced for the first time (Pitfall TEST-2); no Phase 1 file uses it.
