# Phase 3: Endpoints & Helpers - Pattern Map

**Mapped:** 2026-05-27
**Files analyzed:** 20 (8 new source, 8 new test, 1 build-tagged integration test, 3 modified source/test, plus 5+ new fixture JSON files; fixtures are data not code so are not classified by role)
**Analogs found:** 20 / 20 (every new/modified Go file has a strong analog in the existing tree)

## File Classification

### New source files

| New File | Role | Data Flow | Closest Analog | Match Quality |
|----------|------|-----------|----------------|---------------|
| `request.go` | internal HTTP helper (generic) | request-response (transform + decode) | `countries.go` (lines 78-191; the very pipeline being extracted) | exact — `doJSONGet[T]` is the lines 78-138 pipeline lifted into a generic function; `buildAPIError` + `parseAPIMessage` move verbatim from countries.go lines 149-191 |
| `languages.go` | endpoint method on `*Client` | request-response (1 required-free GET → `[]Language`) | `countries.go` lines 50-138 (Countries shape, no required params) | exact (same Request-struct + single optional filter shape after D-52 retrofit) |
| `subdivisions.go` | endpoint method on `*Client` | request-response (1 required + 1 optional GET → `[]Subdivision`) | `countries.go` lines 50-138 + validator wiring in `validate.go` lines 28-38 | role-match (same endpoint shape, adds `validateCountry` required + `validateLanguage` optional) |
| `public_holidays.go` | endpoint method on `*Client` | request-response (3 required + 2 optional GET → `[]Holiday`, post-decode validation) | `countries.go` + `validate.go` lines 89-103 (`validateDateRange`) | role-match (same endpoint shape + post-decode `validateHolidays` injection) |
| `school_holidays.go` | endpoint method on `*Client` | request-response (3 required + 3 optional GET → `[]Holiday`, post-decode validation) | `public_holidays.go` (this phase) | exact (same shape + extra optional `groupCode` field) |
| `holiday.go` | pure-value method group on `Holiday` (no I/O) | transform (Holiday value → string/bool/int/iter.Seq[Date]) | `types.go` lines 153-165 (`Country.NameFor`), `types.go` lines 178-182 (`Language.NameFor`), `types.go` lines 218-222 (`Subdivision.NameFor`), `date.go` lines 144-164 (`DaysUntil`) | exact (Holiday helpers are siblings of existing `NameFor` accessors; `Days()` directly delegates to `DaysUntil`; only new pattern is `Range()` yielding `iter.Seq[Date]`) |
| `client_isinregion.go` (or appended to `client.go`) | endpoint-like method on `*Client` (I/O via Subdivisions) | request-response + tree-walk (calls `c.Subdivisions`, walks `Subdivision.Children`, returns `bool`) | `countries.go` (for the I/O dispatch idiom) + `types.go` Subdivision recursive shape (lines 184-216) | role-match (only Client method that issues hidden I/O; pattern combines endpoint dispatch with in-memory recursion) |

### New test files

| New File | Role | Data Flow | Closest Analog | Match Quality |
|----------|------|-----------|----------------|---------------|
| `request_test.go` | unit test for generic helper | request-response (httptest server stubs) | `countries_test.go` (the post-refactor caller exercising the same pipeline) | exact (same `httptest.NewServer` + table-driven `t.Run` pattern) |
| `languages_test.go` | per-endpoint table test | request-response (httptest server stubs + fixture) | `countries_test.go` | exact |
| `subdivisions_test.go` | per-endpoint table test | request-response + query-param contract assertions | `countries_test.go` + need to inspect `r.URL.Query()` for required `countryIsoCode` | role-match (same skeleton, plus handler-side query inspection) |
| `public_holidays_test.go` | per-endpoint table test | request-response + Holiday-validation error paths | `countries_test.go` + add `ErrMalformedResponse` error subtest | role-match |
| `school_holidays_test.go` | per-endpoint table test | request-response | `public_holidays_test.go` (this phase) | exact |
| `holiday_test.go` | pure-method unit tests | transform | `types_test.go` lines 1-100 (`TestHolidayType_constants` + `TestLocalizedText_JSON`), `date_test.go` (for `Days`/`Range` calendar-correctness pattern) | exact (sibling helpers tested in `types_test.go` follow identical `t.Run`/testify shape) |
| `client_isinregion_test.go` | endpoint+tree test | request-response + tree-walk | `countries_test.go` (for the httptest dispatch path); synthetic Subdivision tree literal for the recursive walk | role-match |
| `update_fixtures_test.go` | build-tagged integration test | request-response (live HTTP, fixture write) | No prior analog in tree; pattern fully specified in RESEARCH.md §"Pattern 5" lines 616-765 | no-analog (new pattern — see "No Analog Found" below) |

### Modified files

| Modified File | Modification | Closest Analog (for the change shape) |
|---------------|--------------|----------------------------------------|
| `countries.go` | Retrofit signature to `Countries(ctx, CountriesRequest)`; refactor body to call `doJSONGet[[]Country]`; remove `maxResponseBytes`/`apiErrorBodyCap`/`buildAPIError`/`parseAPIMessage` (moved to `request.go`) | The file itself is the analog (D-63 refactor in place) |
| `countries_test.go` | Update test calls from `c.Countries(ctx)` to `c.Countries(ctx, CountriesRequest{})`; refresh fixture | Itself (mechanical signature update) |
| `errors.go` | Append `ErrMalformedResponse` to the existing `var (...)` block (lines 17-44) | The 6 existing sentinels in the same block are the analog shape |
| `internal_test.go` | Extend `allowedVars` map (lines 54-62) with `"ErrMalformedResponse": {}` | The map's existing 7 entries are the analog shape |
| `testdata/countries.json` | Re-capture from live API (Countries retrofit + Phase 3 fixture refresh date) | The existing 38-line file is the analog shape |

## Pattern Assignments

### `request.go` (internal HTTP helper, request-response)

**Analog:** `countries.go` (lines 78-191 — the exact pipeline being extracted into `doJSONGet[T]`)

**Imports pattern** (verbatim from `countries.go` lines 29-38; `net/url` added because `doJSONGet` takes `url.Values`):

```go
package openholidays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)
```

For `request.go` the import block adds `"net/url"` because the function signature takes `q url.Values`. No new third-party imports — zero-runtime-dep invariant preserved.

**Constants to relocate** (verbatim from `countries.go` lines 40-48):

```go
// maxResponseBytes is the hard ceiling on any decoded response body (D-25).
// 10 MiB. Not configurable in v0.1.0 — PROJECT.md fixes the cap.
const maxResponseBytes = 10 << 20

// apiErrorBodyCap is the maximum number of upstream body bytes copied into
// APIError.Body (Phase 1 D-17). 4 KiB. The cap bounds the byte cost of
// echoing a hostile multi-MB error envelope into operator logs while still
// preserving enough context for diagnostics.
const apiErrorBodyCap = 4 << 10
```

**Core generic helper body — derived verbatim from `countries.go` lines 78-138** (only the slice-typed locals become `T`, the path becomes a parameter, and an extra `req.URL.RawQuery = q.Encode()` line is added before `c.http.Do`):

The Countries pipeline shape to lift (`countries.go` lines 78-138):

```go
func (c *Client) Countries(ctx context.Context) ([]Country, error) {
	if ctx == nil {
		return nil, errors.New("openholidays: nil context")
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/Countries", nil)
	if err != nil {
		return nil, fmt.Errorf("openholidays: build /Countries request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openholidays: GET /Countries: %w", err)
	}
	defer func() {
		// Drain before close so the keep-alive connection can be reused
		// (PITFALLS HTTP-3). LimitReader bounds the drain at
		// maxResponseBytes+1 so a hostile infinite stream cannot block the
		// drain indefinitely (T-02-12).
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return nil, buildAPIError(resp, "/Countries")
	}
	var countries []Country
	limited := &io.LimitedReader{R: resp.Body, N: maxResponseBytes}
	decoder := json.NewDecoder(limited)
	if decodeErr := decoder.Decode(&countries); decodeErr != nil {
		if errors.Is(decodeErr, io.EOF) {
			return nil, fmt.Errorf("openholidays: empty /Countries response: %w", ErrEmptyResponse)
		}
		if limited.N == 0 {
			return nil, fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)
		}
		return nil, fmt.Errorf("openholidays: decode /Countries: %w", decodeErr)
	}
	if decoder.More() {
		return nil, fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)
	}
	return countries, nil
}
```

The generic translation rules:
- `[]Country` → type parameter `T`
- `"/Countries"` → parameter `path string`
- All `nil` returns become `var zero T; return zero, err` (see Pitfall 1 / Anti-Patterns in RESEARCH.md)
- Add `if len(q) > 0 { req.URL.RawQuery = q.Encode() }` between `NewRequestWithContext` and `c.http.Do` (the only structural addition; required because the new endpoints carry query params)
- The error messages keep the `"openholidays: <verb> <path>: %w"` shape from `countries.go` lines 89, 93, 111, 123, 125, 136 — substitute `path` parameter for the literal `/Countries`

**Helpers to move verbatim from `countries.go` lines 141-191:**

```go
// buildAPIError constructs an *APIError from a non-2xx *http.Response.
func buildAPIError(resp *http.Response, path string) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, apiErrorBodyCap))
	msg := parseAPIMessage(body)
	return &APIError{
		StatusCode: resp.StatusCode,
		Path:       path,
		Body:       body,
		Message:    msg,
	}
}

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

These move byte-for-byte (D-63); their godoc and signatures are unchanged.

**`validateHolidays` helper** (D-65 / CL-12; placement: `request.go` per RESEARCH.md §"Pattern 6 placement" recommendation — "response pipeline concern, not Holiday type concern"). Pattern excerpt (no existing analog — new code; the shape mirrors the validator-style of `validate.go` lines 89-103):

```go
// validateHolidays runs the post-decode Holiday schema-drift checks
// mandated by D-65 / Pitfall JSON-4. Returns the first violation wrapping
// ErrMalformedResponse.
func validateHolidays(hs []Holiday, path string) error {
	for i := range hs {
		h := &hs[i]
		if h.StartDate.IsZero() {
			return fmt.Errorf("openholidays: malformed holiday %q at %s: zero StartDate: %w",
				h.ID, path, ErrMalformedResponse)
		}
		if h.EndDate.IsZero() {
			return fmt.Errorf("openholidays: malformed holiday %q at %s: zero EndDate: %w",
				h.ID, path, ErrMalformedResponse)
		}
		if h.EndDate.Before(h.StartDate) {
			return fmt.Errorf("openholidays: malformed holiday %q at %s: EndDate %s before StartDate %s: %w",
				h.ID, path, h.EndDate, h.StartDate, ErrMalformedResponse)
		}
	}
	return nil
}
```

The error-string convention is verbatim from `errors.go` line 20 / `validate.go` lines 35, 59, 91, 100: every error string starts with `"openholidays: "` (Phase 1 D-23). The `%w` wrap of the sentinel is the same idiom used in `validate.go` line 35 (`fmt.Errorf("%w: %q", ErrInvalidCountry, code)`).

---

### `languages.go` (endpoint, request-response)

**Analog:** `countries.go` (lines 50-138 — the canonical endpoint shape, post-refactor it will be ~20 lines of pure dispatch)

**Imports pattern** (derived from `countries.go` line 31-38, slimmed because `doJSONGet` owns http/io/json):

```go
package openholidays

import (
	"context"
	"net/url"
)
```

**Godoc pattern** (verbatim shape from `countries.go` lines 50-77 — symbol-named first sentence, sections for per-request timeout / errors / concurrency):

```go
// Languages fetches the list of supported languages from the upstream
// OpenHolidays API. Each returned Language carries an IsoCode (ISO 639-1
// lowercase) and a per-language localized Name array (look up a specific
// language via Language.NameFor).
//
// Per-request timeout: when the Client was constructed with WithTimeout(d)
// and d > 0, Languages wraps ctx via context.WithTimeout(ctx, d) before
// dispatching.
//
// Errors:
//   - validateLanguage failure on a non-empty LanguageIsoCode wraps
//     ErrInvalidLanguage.
//   - Transport, decode, and HTTP errors surface verbatim via the
//     doJSONGet contract (see request.go).
```

**Core endpoint body** (template from RESEARCH.md §"Endpoint method body — Subdivisions" lines 944-987; this is the trimmed shape every endpoint follows):

```go
type LanguagesRequest struct {
	LanguageIsoCode string
}

func (c *Client) Languages(ctx context.Context, req LanguagesRequest) ([]Language, error) {
	q := url.Values{}
	if req.LanguageIsoCode != "" {
		lang, err := validateLanguage(req.LanguageIsoCode)
		if err != nil { return nil, err }
		q.Set("languageIsoCode", lang)
	}
	return doJSONGet[[]Language](ctx, c, "/Languages", q)
}
```

The validator-wiring matrix per endpoint comes from CONTEXT.md D-56. The empty-string optional-omission guard is from CONTEXT.md D-55 (and Pitfall 2 in RESEARCH.md): one `if v != "" { q.Set(...) }` block per optional Request field.

---

### `subdivisions.go` (endpoint, request-response)

**Analog:** `countries.go` lines 50-138 + `validate.go` lines 28-38 (`validateCountry`)

**Core pattern** (RESEARCH.md lines 944-987):

```go
type SubdivisionsRequest struct {
	CountryIsoCode  string
	LanguageIsoCode string
}

func (c *Client) Subdivisions(ctx context.Context, req SubdivisionsRequest) ([]Subdivision, error) {
	country, err := validateCountry(req.CountryIsoCode)
	if err != nil { return nil, err }
	q := url.Values{}
	q.Set("countryIsoCode", country)
	if req.LanguageIsoCode != "" {
		lang, err := validateLanguage(req.LanguageIsoCode)
		if err != nil { return nil, err }
		q.Set("languageIsoCode", lang)
	}
	return doJSONGet[[]Subdivision](ctx, c, "/Subdivisions", q)
}
```

`validateCountry` is called verbatim from `validate.go` lines 28-38; its canonicalized uppercase output is passed to the wire param `countryIsoCode` exactly as in research D-56's matrix.

---

### `public_holidays.go` (endpoint, request-response + post-decode validation)

**Analog:** `countries.go` lines 50-138 + `validate.go` lines 89-103 (`validateDateRange`) + `request.go` `validateHolidays` (this phase)

**Core pattern** (RESEARCH.md lines 991-1033, verbatim from research):

```go
type PublicHolidaysRequest struct {
	CountryIsoCode  string
	ValidFrom       Date
	ValidTo         Date
	LanguageIsoCode string
	SubdivisionCode string
}

func (c *Client) PublicHolidays(ctx context.Context, req PublicHolidaysRequest) ([]Holiday, error) {
	country, err := validateCountry(req.CountryIsoCode)
	if err != nil { return nil, err }
	if err := validateDateRange(req.ValidFrom, req.ValidTo); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("countryIsoCode", country)
	q.Set("validFrom", req.ValidFrom.String())
	q.Set("validTo", req.ValidTo.String())
	if req.LanguageIsoCode != "" {
		lang, err := validateLanguage(req.LanguageIsoCode)
		if err != nil { return nil, err }
		q.Set("languageIsoCode", lang)
	}
	if req.SubdivisionCode != "" {
		q.Set("subdivisionCode", req.SubdivisionCode)
	}
	holidays, err := doJSONGet[[]Holiday](ctx, c, "/PublicHolidays", q)
	if err != nil { return nil, err }
	if err := validateHolidays(holidays, "/PublicHolidays"); err != nil {
		return nil, err
	}
	return holidays, nil
}
```

`req.ValidFrom.String()` reuses `date.go` lines 107-115 (`String() string` returning `YYYY-MM-DD` via `dateLayout`). The post-decode `validateHolidays` call is the second-pass injection point per D-65.

---

### `school_holidays.go` (endpoint, request-response + post-decode validation)

**Analog:** `public_holidays.go` (this phase) — sibling endpoint with one extra optional field

**Difference from `public_holidays.go`:**

```go
type SchoolHolidaysRequest struct {
	CountryIsoCode  string
	ValidFrom       Date
	ValidTo         Date
	LanguageIsoCode string
	SubdivisionCode string
	GroupCode       string // additional optional field
}
```

The endpoint method body is the same five-validator + query-builder + `doJSONGet[[]Holiday]("/SchoolHolidays", q)` + `validateHolidays(..., "/SchoolHolidays")` flow, with one extra `if req.GroupCode != "" { q.Set("groupCode", req.GroupCode) }` block. Per D-56, `GroupCode` is "shape-tolerant — pass through to upstream and let it reject" so no client-side validator runs on it.

---

### `holiday.go` (pure-value method group on `Holiday`)

**Analog:** `types.go` lines 153-243 (the three existing `NameFor` accessors + `pickLocalized`) plus `date.go` lines 144-164 (`DaysUntil`).

**Imports pattern** (derived from `types.go` line 12 + the new `iter` import for Go 1.23 range-over-func):

```go
package openholidays

import (
	"iter"
	"strings"
)
```

**`Holiday.NameFor` pattern** — direct sibling of `Country.NameFor` (`types.go` lines 153-165), `Language.NameFor` (lines 178-182), `Subdivision.NameFor` (lines 218-222). All four delegate to `pickLocalized`:

Verbatim from `types.go` lines 163-165 (`Country.NameFor`):

```go
func (c Country) NameFor(lang string) string {
	return pickLocalized(c.Name, lang)
}
```

`Holiday.NameFor` follows the identical shape, substituting receiver type:

```go
func (h Holiday) NameFor(lang string) string {
	return pickLocalized(h.Name, lang)
}
```

Godoc follows the verbatim Country.NameFor pattern (`types.go` lines 153-162) — case-insensitive matching, first-entry fallback, empty-on-empty.

**`Holiday.IsInRegion` pattern** — flat-only per D-58 (RESEARCH.md lines 1037-1049, verbatim):

```go
func (h Holiday) IsInRegion(code string) bool {
	if code == "" { return false }
	if h.Nationwide { return true }
	for _, s := range h.Subdivisions {
		if strings.EqualFold(s.Code, code) {
			return true
		}
	}
	return false
}
```

`strings.EqualFold` matches the case-insensitive idiom used by `pickLocalized` (`types.go` line 235) and by `validate.go`'s canonicalization-then-compare pattern.

**`Holiday.Days` pattern** — direct delegation to `Date.DaysUntil` (`date.go` lines 144-164):

```go
func (h Holiday) Days() int {
	return h.StartDate.DaysUntil(h.EndDate)
}
```

The DaysUntil contract from `date.go` lines 144-164 specifies: same day → 1, +1 day → 2, negative span → negative result. RESEARCH.md D-60 verified this produces 14 for the Polish ferie zimowe Śląskie 2025 (Jan 18 – Jan 31) test case.

**`Holiday.Range` pattern** — new `iter.Seq[Date]` shape per CL-11, with NewDate rebuild every step (RESEARCH.md lines 340-379):

```go
func (h Holiday) Range() iter.Seq[Date] {
	return func(yield func(Date) bool) {
		if h.EndDate.Before(h.StartDate) {
			return
		}
		d := h.StartDate
		for {
			if !yield(d) {
				return
			}
			if !d.Before(h.EndDate) {
				return
			}
			next := d.AddDate(0, 0, 1)
			d = NewDate(next.Year(), next.Month(), next.Day())
		}
	}
}
```

`NewDate` is `date.go` lines 41-46. `AddDate` is inherited from `time.Time` (embedded in Date via `date.go` lines 37-39). The rebuild via `NewDate(next.Year(), next.Month(), next.Day())` preserves the UTC-midnight invariant unconditionally (RESEARCH.md Pitfall 3, lines 893-902).

---

### `client_isinregion.go` (endpoint+tree, request-response + recursion)

**Analog:** `countries.go` (for the I/O dispatch idiom — but the I/O is delegated to `c.Subdivisions` from this phase, so no direct HTTP code) + `types.go` lines 184-216 (`Subdivision.Children []Subdivision` recursive shape) + `Holiday.IsInRegion` flat fast-path from this phase.

**Core pattern** (RESEARCH.md lines 534-605, verbatim):

```go
func (c *Client) IsInRegion(ctx context.Context, h Holiday, code string) (bool, error) {
	if code == "" { return false, nil }
	if h.Nationwide { return true, nil }
	// Fast path: flat match on Holiday.Subdivisions directly.
	for _, s := range h.Subdivisions {
		if strings.EqualFold(s.Code, code) {
			return true, nil
		}
	}
	if len(h.Subdivisions) == 0 {
		return false, nil
	}
	countryCode, ok := splitCountryFromSubdivision(h.Subdivisions[0].Code)
	if !ok {
		return false, nil
	}
	tree, err := c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: countryCode})
	if err != nil {
		return false, err
	}
	parentIdx := buildParentIndex(tree)
	current := code
	for i := 0; i <= len(parentIdx); i++ {
		for _, s := range h.Subdivisions {
			if strings.EqualFold(s.Code, current) {
				return true, nil
			}
		}
		parent, found := parentIdx[strings.ToUpper(current)]
		if !found { return false, nil }
		current = parent
	}
	return false, nil
}

func splitCountryFromSubdivision(code string) (string, bool) {
	if i := strings.IndexByte(code, '-'); i > 0 {
		return code[:i], true
	}
	return "", false
}

func buildParentIndex(tree []Subdivision) map[string]string {
	idx := make(map[string]string)
	var walk func(parent string, nodes []Subdivision)
	walk = func(parent string, nodes []Subdivision) {
		for _, n := range nodes {
			if parent != "" {
				idx[strings.ToUpper(n.Code)] = parent
			}
			if len(n.Children) > 0 {
				walk(n.Code, n.Children)
			}
		}
	}
	walk("", tree)
	return idx
}
```

The defensive iteration cap `i <= len(parentIdx)` is the cycle protection mandated by RESEARCH.md Pitfall 4 (lines 904-915). The `Subdivision.Children` field is the existing recursive shape from `types.go` lines 210-212.

---

### `*_test.go` files (all per-endpoint tests)

**Analog:** `countries_test.go` (lines 1-300 — the entire file is the canonical pattern).

**Test file header pattern** (verbatim from `countries_test.go` lines 1-32):

```go
// Package openholidays — tests for the <X> endpoint method.
//
// One TestXxx per exported production function per Gold Rule 3. Every
// scenario lives in a t.Run subtest. Non-English strings in the fixture
// (e.g. "Polska", "Deutschland", "Polen") mirror real upstream OpenHolidays
// responses and are admitted per CONVENTIONS.md Rule 1 testdata-fixture
// exception.

package openholidays

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const <endpoint>FixtureCapturedAt = "2026-05-27"
```

The `FixtureCapturedAt` const is the D-69 pattern: per CONTEXT.md, names are `languagesFixtureCapturedAt`, `subdivisionsPLFixtureCapturedAt`, `publicHolidaysPL2025FixtureCapturedAt`, `schoolHolidaysPL2025FixtureCapturedAt`.

**Happy-path subtest pattern** (verbatim from `countries_test.go` lines 48-81):

```go
t.Run("happy path returns ... from fixture", func(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join("testdata", "<fixture>.json"))
	require.NoError(t, err, "fixture missing — re-capture per ...")
	t.Logf("fixture captured %s", <endpoint>FixtureCapturedAt)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithBaseURL(srv.URL))
	got, err := c.<Endpoint>(context.Background(), <Endpoint>Request{...})
	require.NoError(t, err)
	require.Len(t, got, <D-70 locked count>)
	// per-fixture sanity assertions
})
```

**Query-param contract assertion pattern** (RESEARCH.md lines 406-414 — adds query-inspection inside the handler, required for endpoints with required params; not present in `countries_test.go` because Countries has no required params):

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	assert.Equal(t, "PL", q.Get("countryIsoCode"))
	assert.Equal(t, "2025-01-01", q.Get("validFrom"))
	assert.Equal(t, "2025-12-31", q.Get("validTo"))
	assert.Equal(t, "/PublicHolidays", r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}))
```

**4xx error subtest pattern** (verbatim from `countries_test.go` lines 83-105):

```go
t.Run("4xx returns *APIError with detail Message", func(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail": "Country not supported"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithBaseURL(srv.URL))
	got, err := c.<Endpoint>(context.Background(), <Endpoint>Request{<valid required fields>})
	require.Error(t, err)
	assert.Nil(t, got)

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr),
		"expected *APIError, got %T: %v", err, err)
	assert.Equal(t, 404, apiErr.StatusCode)
	assert.Equal(t, "/<Path>", apiErr.Path)
	assert.Equal(t, "Country not supported", apiErr.Message)
})
```

The four error-path subtests in `countries_test.go` (4xx, 5xx, error fallback, body-truncated, empty body, nil context, oversize) — public_holidays/school_holidays tests should mirror these plus add the new `ErrMalformedResponse` subtest (RESEARCH.md lines 467-493).

**Validation-error subtest pattern** (RESEARCH.md lines 440-461, novel — no analog in countries_test.go because Countries has no client-side validators today):

```go
t.Run("validation error: empty CountryIsoCode wraps ErrInvalidCountry", func(t *testing.T) {
	t.Parallel()
	c := NewClient(WithBaseURL("http://example.invalid")) // no server reached
	_, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{
		ValidFrom: NewDate(2025, time.January, 1),
		ValidTo:   NewDate(2025, time.December, 31),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCountry))
})
```

`http://example.invalid` is the RFC 6761 reserved test domain (RESEARCH.md line 499) — accidental HTTP would fail loudly.

**ErrMalformedResponse subtest pattern** (RESEARCH.md lines 467-493) — specific to public_holidays and school_holidays:

```go
t.Run("malformed holiday (EndDate before StartDate) wraps ErrMalformedResponse", func(t *testing.T) {
	t.Parallel()
	bad := `[{
		"id":"bad-uuid","startDate":"2025-12-25","endDate":"2025-01-01",
		"type":"Public","name":[{"language":"en","text":"X"}],
		"nationwide":true,"regionalScope":"National","temporalScope":"FullDay"
	}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(bad))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(WithBaseURL(srv.URL))
	_, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{
		CountryIsoCode: "PL",
		ValidFrom:      NewDate(2025, time.January, 1),
		ValidTo:        NewDate(2025, time.December, 31),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMalformedResponse))
})
```

The 4xx-body-cap, empty-body, nil-context, and oversize-streaming-handler subtests carry over verbatim from `countries_test.go` lines 149-263 (signatures updated to the new Request struct). The CR-01 trailing-whitespace regression subtest (`countries_test.go` lines 265-299) can apply to any endpoint and is a candidate for `request_test.go` since it tests the generic helper, not the endpoint-specific shape.

---

### `holiday_test.go` (pure-method unit tests)

**Analog:** `types_test.go` lines 1-100 (testify+t.Run+t.Parallel pattern for type-method tests).

**File header pattern** (verbatim from `types_test.go` lines 1-18):

```go
// Package openholidays — tests for the Holiday helpers in holiday.go.
//
// One TestXxx function per exported production function per Gold Rule 3.
// Every test case is wrapped in t.Run; require for preconditions, assert for
// verifications. Non-English strings in fixtures (e.g. "Wigilia Bożego
// Narodzenia", "Śląskie") mirror real upstream OpenHolidays responses and
// are admitted per CONVENTIONS.md Rule 1 testdata-fixture exception.

package openholidays

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

**Per-method test skeleton** (verbatim shape from `types_test.go` lines 22-49 — `TestHolidayType_constants`):

```go
func TestHoliday_NameFor(t *testing.T) {
	t.Parallel()
	t.Run("matches Polish entry case-insensitively", func(t *testing.T) {
		t.Parallel()
		h := Holiday{Name: []LocalizedText{{Language: "pl", Text: "Wigilia"}, {Language: "en", Text: "Christmas Eve"}}}
		assert.Equal(t, "Wigilia", h.NameFor("pl"))
		assert.Equal(t, "Wigilia", h.NameFor("PL"))
	})
	t.Run("falls back to first entry on miss", func(t *testing.T) { /* ... */ })
	t.Run("returns empty on empty Name", func(t *testing.T) { /* ... */ })
}
```

`TestHoliday_Range` should assert against the Polish ferie zimowe Śląskie case (RESEARCH.md success criterion verification): 14 yielded Dates with the first equal to `NewDate(2025, time.January, 18)` and the last equal to `NewDate(2025, time.January, 31)`.

## Shared Patterns

### Error-string convention — `"openholidays: "` prefix

**Source:** `errors.go` lines 17-43 (every sentinel message starts with the prefix); `validate.go` lines 35, 59, 91, 100; `countries.go` lines 80, 89, 93, 111, 123, 125, 136.

**Apply to:** Every error returned by every new file in this phase. Including the new `ErrMalformedResponse` sentinel in `errors.go` and the `validateHolidays` wrapper.

Verbatim from `errors.go` lines 17-44 (the var block `ErrMalformedResponse` will be appended to):

```go
var (
	ErrInvalidCountry = errors.New("openholidays: invalid country code")
	ErrInvalidLanguage = errors.New("openholidays: invalid language code")
	ErrDateRangeTooLarge = errors.New("openholidays: date range too large")
	ErrInvalidDateRange = errors.New("openholidays: invalid date range")
	ErrEmptyResponse = errors.New("openholidays: empty response body")
	ErrResponseTooLarge = errors.New("openholidays: response too large")
	// Phase 3 appends:
	// ErrMalformedResponse = errors.New("openholidays: malformed response")
)
```

### Sentinel wrapping with `fmt.Errorf("...: %w", sentinel)`

**Source:** `validate.go` line 35 (`fmt.Errorf("%w: %q", ErrInvalidCountry, code)`); `countries.go` lines 111, 123, 136 (`fmt.Errorf("openholidays: ... %w", ErrXxx)`).

**Apply to:** Every error site in every new endpoint file. The `errors.Is(err, ErrMalformedResponse)` test pattern (RESEARCH.md line 492) depends on the `%w` wrap being present at the construction site.

### `httptest.NewServer` + `WithBaseURL` test idiom

**Source:** `countries_test.go` lines 55-62 (verbatim — reused by every endpoint test in this phase):

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}))
t.Cleanup(srv.Close)

c := NewClient(WithBaseURL(srv.URL))
```

**Apply to:** Every new `*_test.go` file (request_test, languages_test, subdivisions_test, public_holidays_test, school_holidays_test, client_isinregion_test). Phase 2 D-46 enforces no live HTTP in unit tests.

### testify + one-TestXxx-per-prod-function + `t.Run` per case

**Source:** `countries_test.go` (one TestClient_Countries with 8 t.Run cases); `types_test.go` (one TestXxx per exported type).

**Apply to:** Every new test file. Gold Rule 3 is non-negotiable. `require` for preconditions (fixture read, JSON decode, http.NewRequest); `assert` for verifications (field values, slice lengths, error chains).

### `t.Parallel()` at top of TestXxx and inside each t.Run

**Source:** `countries_test.go` lines 46-47 (`t.Parallel()` at top) + inside each subtest. Note the goroutine-leak subtest (lines 200-203) explicitly does NOT call `t.Parallel()` — same exception applies if new tests inspect runtime state.

**Apply to:** Every new test file except subtests that inspect goroutine count or mutate process-global state.

### Pointer iteration over Holiday slices in helpers — `for i := range hs { h := &hs[i] }`

**Source:** New pattern from RESEARCH.md lines 791-808 (`validateHolidays` body); rationale at RESEARCH.md lines 811-812 (`rangeValCopy` linter recommendation).

**Apply to:** `validateHolidays` in request.go. NOT to be applied to Holiday-receiver methods (`NameFor`, `IsInRegion`, `Days`, `Range`) — those take a value receiver because Holiday helpers are pure-value transforms following the existing `Country.NameFor` value-receiver shape.

### Subdivision recursive tree walk — `Subdivision.Children` field

**Source:** `types.go` lines 184-216 (Subdivision struct + recursive `Children []Subdivision`).

**Apply to:** `buildParentIndex` (in `client_isinregion.go`) and any tree-walking helper. The defensive cycle bound (RESEARCH.md Pitfall 4) is applied at `Client.IsInRegion`'s upward walk, not inside `buildParentIndex` — but the visited-set discipline ("walk every node exactly once") is the implementation contract.

### `internal_test.go::allowedVars` extension protocol

**Source:** `internal_test.go` lines 54-62 (the closed map) + RESEARCH.md lines 930-938 (Pitfall 6 — protocol for adding a new sentinel).

**Apply to:** Plan 4 (or wherever `ErrMalformedResponse` lands). The change is one-line:

```go
var allowedVars = map[string]struct{}{
	"ErrInvalidCountry":    {},
	"ErrInvalidLanguage":   {},
	"ErrDateRangeTooLarge": {},
	"ErrInvalidDateRange":  {},
	"ErrEmptyResponse":     {},
	"ErrResponseTooLarge":  {},
	"ErrMalformedResponse": {}, // NEW (CL-12 / D-66)
	"errEmptyDate":         {},
}
```

The sentinel addition to `errors.go` and the allowlist extension MUST land in the same commit per RESEARCH.md Pitfall 6 (otherwise `TestNoInitOrGlobalState` fails).

### godoc convention — first sentence starts with the symbol name

**Source:** `client.go` line 21 ("Client is..."), line 40 ("NewClient constructs..."), line 73 ("Close is..."); `countries.go` line 50 ("Countries fetches..."); `types.go` lines 14, 42, 54, 73, 80 (every exported type), 153 ("NameFor returns..."), 178, 218.

**Apply to:** Every exported symbol declared in this phase. Gold Rule 1 + PROJECT.md "Public surface area: minimize. Every exported symbol must have a doc comment."

## No Analog Found

Files with no close match in the existing codebase (planner should use RESEARCH.md patterns instead):

| File | Role | Data Flow | Reason / Reference Pattern |
|------|------|-----------|-----------------------------|
| `update_fixtures_test.go` (build-tagged `//go:build integration`) | live-API fixture refresh utility | request-response (live HTTP) + file I/O (atomic write) | No prior `integration`-tagged test file exists in the tree. Pattern fully specified in RESEARCH.md §"Pattern 5 — `-update` fixture refresh mechanism" lines 616-765 (full body). Key new patterns: `flag.Bool("update", ...)` declared in build-tagged file only; `os.CreateTemp` + `os.Rename` atomic write; `nonEmptyJSONArray` sanity check before overwrite; double-gate `//go:build integration` + `OPENHOLIDAYS_LIVE=1` env. |
| `subdivisions_de.json` fixture (testdata) | data fixture | n/a | Net-new file; required only to exercise the hierarchical `Client.IsInRegion` path against the live-grounded DE-BY/Augsburg children (PL is flat — see RESEARCH.md lines 502-513 + Assumption A3). Captured live during Phase 3 via the `-update` mechanism. |

These two items have well-defined patterns in RESEARCH.md but no existing analog to copy verbatim. The planner should reference RESEARCH.md directly for their implementation.

## Metadata

**Analog search scope:** Repo root (`/data/git/private/holidays/*.go`) and `testdata/`. Skipped `.planning/`, `.git/`, `.golangci.yml_backup`.
**Files scanned:** 22 (21 `.go` files + 1 JSON fixture). All Phase 1 + Phase 2 production and test files were read; the planning artifacts (CONTEXT.md, RESEARCH.md) were read in full.
**Pattern extraction date:** 2026-05-27
**Verification level:** Every code excerpt above is quoted verbatim from the indicated source file and line range in the working tree (Gold Rule 2 compliance); no excerpt was paraphrased or reconstructed from memory.
