# Phase 5: Distribution - Pattern Map

**Mapped:** 2026-05-28
**Files analyzed:** 26 new/modified files
**Analogs found:** 12 / 26 (12 have strong in-repo analogs; 14 have NO existing analog — see "No Analog Found" section; for those the planner uses 05-RESEARCH.md reference implementations).

---

## Scope and inputs

This pattern map covers every file Phase 5 creates or modifies, as enumerated in `05-CONTEXT.md` §"Integration Points" and `05-RESEARCH.md` §"Recommended Project Structure" + §"Wave 0 Gaps". Files are bucketed by role; each row points at the closest existing analog (or flags "no analog — use RESEARCH.md reference").

The dominant existing-analog cluster is **`*_test.go` files at the repo root** — every test pattern (testify `require`/`assert`, one-`TestXxx`-per-prod-function, `t.Parallel()`, `t.Run` subtests, `httptest.NewServer` per case, `os.ReadFile("testdata/...")` fixture loading, `t.Cleanup(srv.Close)`) is already established. CLI tests under `cmd/ohcli/` will inherit that shape verbatim; the only delta is "test calls `run(args, stdout, stderr)`" instead of "test calls `c.PublicHolidays(ctx, req)`".

The cluster with **no in-repo analog** is **GitHub Actions YAML, goreleaser config, dependabot config, and `cmd/ohcli/*.go` itself** — these files have never existed in this repo (`ls .github/` returns nothing; `cmd/` is absent). For those, the planner copies from `05-RESEARCH.md` §"Code Examples" verbatim.

---

## File Classification

### Files WITH in-repo analog

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `cmd/ohcli/main_test.go` | cli-test | request-response | `public_holidays_test.go` | role-match (testify + `httptest.NewServer` + `t.Run`) |
| `cmd/ohcli/public_test.go` | cli-test | request-response | `public_holidays_test.go` | role-match |
| `cmd/ohcli/school_test.go` | cli-test | request-response | `school_holidays_test.go` (same shape) | role-match |
| `cmd/ohcli/countries_test.go` | cli-test | request-response | `countries_test.go` | role-match |
| `cmd/ohcli/format_test.go` | unit-test (pure) | transform | `holiday_test.go` (pure-helper testify pattern) | role-match |
| `fuzz_test.go` | fuzz-target | transform | `date_test.go` (existing `FuzzDateUnmarshal` if present; see Pattern 3) | partial-match (Go fuzz idiom) |
| `bench_test.go` | bench-test | request-response | `public_holidays_test.go` (`httptest.NewServer` + fixture body) | role-match |
| `integration_test.go` | integration-test | request-response | `update_fixtures_test.go` (build-tagged, env-var-gated) | exact (same `//go:build integration` + env-var pattern) |
| `example_test.go` | godoc-example | request-response | `doc.go` + existing godoc style in `holiday.go`, `client.go` | role-match (godoc voice; no existing `Example_*`) |
| `doc.go` (extend) | package-doc | n/a | existing `doc.go` | exact (extending existing file) |
| `.golangci.yml` | lint-config | n/a | `.golangci.yml_backup` | exact (rewrite of, see Pitfall 3) |
| `go.mod` (verify) | module-manifest | n/a | existing `go.mod` | exact (verify, no rewrite expected) |

### Files with NO in-repo analog

| New/Modified File | Role | Data Flow | Notes |
|-------------------|------|-----------|-------|
| `cmd/ohcli/main.go` | cli-entrypoint | request-response | No `cmd/` dir exists. Use `05-RESEARCH.md` §"Pattern 1" reference. |
| `cmd/ohcli/public.go` | cli-subcommand | request-response | Use `05-RESEARCH.md` §"Code Examples → CLI subcommand handler" reference. |
| `cmd/ohcli/school.go` | cli-subcommand | request-response | Mirror `public.go` (adds `--region` flag). |
| `cmd/ohcli/countries.go` | cli-subcommand | request-response | Mirror `public.go` (no positional year). |
| `cmd/ohcli/format.go` | format-renderer | transform | Use `05-RESEARCH.md` §"Pattern 2" reference (tabwriter / json / csv). |
| `testdata/fuzz/FuzzParseLocalizedText/` | fuzz-corpus | n/a | New directory; seed files. |
| `testdata/fuzz/FuzzUnmarshalHoliday/` | fuzz-corpus | n/a | New directory; seed files. |
| `.github/workflows/ci.yml` | ci-workflow | n/a | No `.github/` dir exists. Use `05-RESEARCH.md` §"Code Examples → ci.yml" reference. |
| `.github/workflows/integration.yml` | ci-workflow | n/a | Use `05-RESEARCH.md` reference. |
| `.github/workflows/release.yml` | ci-workflow | n/a | Use `05-RESEARCH.md` reference. |
| `.github/dependabot.yml` | ci-workflow | n/a | Use `05-RESEARCH.md` reference. |
| `.goreleaser.yaml` | release-config | n/a | Use `05-RESEARCH.md` §"Code Examples → .goreleaser.yaml" reference. |
| `README.md` | project-doc | n/a | No README currently. Pure authoring; reference badges + quickstart from CONTEXT.md §D-09 and PROJECT.md. |
| `docs/design.md` | project-doc | n/a | No `docs/` dir; ASCII transport-chain diagrams from `05-RESEARCH.md` §"System Architecture Diagram". |
| `CONTRIBUTING.md` | project-doc | n/a | New; minimal dev-loop content per CONTEXT.md "Claude's Discretion". |
| `CHANGELOG.md` | project-doc | n/a | New; one-line pointer file per CONTEXT.md D-12 + 05-RESEARCH.md §"Open Questions" Q3. |

---

## Pattern Assignments

### `cmd/ohcli/*_test.go` (cli-test, request-response)

**Analog:** `public_holidays_test.go`

The CLI test files are the heaviest pattern-reuse target in this phase. Every published convention — file-level godoc preamble explaining the Gold Rule 3 application, `t.Parallel()` at top, `t.Run` per case, `t.Parallel()` inside each subtest, `httptest.NewServer` + `t.Cleanup(srv.Close)`, fixture loading via `os.ReadFile(filepath.Join("testdata", ...))`, `require` for preconditions / `assert` for verifications, error-path assertions via `errors.Is` and `errors.As` — already lives in `public_holidays_test.go` and is mechanically copyable.

The CLI delta is shape: `c.PublicHolidays(ctx, req)` becomes `run([]string{"public", "PL", "2025"}, stdout, stderr)` where `stdout`/`stderr` are `*bytes.Buffer` substitutes for the real `os.Stdout`/`os.Stderr`. Otherwise the file shape is identical.

**File-level godoc preamble pattern** (`public_holidays_test.go` lines 1-15):

```go
// Package openholidays — tests for the PublicHolidays endpoint method and
// the validateHolidays post-decode helper.
//
// One TestXxx per exported production function per Gold Rule 3:
// TestClient_PublicHolidays covers the endpoint method;
// TestValidateHolidays covers the unexported validateHolidays helper
// (the helper is private to the package and Gold Rule 3 still applies for
// every package function the rest of the package exercises — the test
// pins the invariant set in isolation from the HTTP pipeline).
//
// Every scenario lives in a t.Run subtest. Non-English strings in the
// fixture (e.g. "Wigilia Bożego Narodzenia", "Trzech Króli") mirror real
// upstream OpenHolidays responses and are admitted per CONVENTIONS.md Rule
// 1 testdata-fixture exception.
```

**Imports pattern** (`public_holidays_test.go` lines 16-30):

```go
package openholidays

import (
    "context"
    "errors"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
```

For `cmd/ohcli/*_test.go` the package declaration is `package main` (or `package main_test` if a hermetic external test) and an extra `import "bytes"` is needed for buffer capture.

**Happy-path test shape** (`public_holidays_test.go` lines 44-98):

```go
func TestClient_PublicHolidays(t *testing.T) {
    t.Parallel()

    t.Run("happy path PL 2025 returns 14 holidays incl. Wigilia 2025-12-24", func(t *testing.T) {
        t.Parallel()

        body, err := os.ReadFile(filepath.Join("testdata", "public_holidays_pl_2025.json"))
        require.NoError(t, err, "fixture missing — re-capture from live API per Plan 03-04 Task 2 (captured %s)",
            publicHolidaysPL2025FixtureCapturedAt)
        t.Logf("fixture captured %s", publicHolidaysPL2025FixtureCapturedAt)

        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            assert.Equal(t, "/PublicHolidays", r.URL.Path)
            q := r.URL.Query()
            assert.Equal(t, "PL", q.Get("countryIsoCode"),
                "country code must be uppercased canonical form")
            assert.Equal(t, "2025-01-01", q.Get("validFrom"))
            assert.Equal(t, "2025-12-31", q.Get("validTo"))
            w.Header().Set("Content-Type", "application/json")
            _, _ = w.Write(body)
        }))
        t.Cleanup(srv.Close)

        c := NewClient(WithBaseURL(srv.URL))
        holidays, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{ ... })
        require.NoError(t, err)
        require.Len(t, holidays, 14, "fixture captured %s — re-capture if upstream shape drifted",
            publicHolidaysPL2025FixtureCapturedAt)
        ...
    })
```

For `cmd/ohcli/public_test.go`, the equivalent body is:

```go
func TestCmdPublic(t *testing.T) {
    t.Parallel()

    t.Run("text output for PL 2025 prints 14-row table", func(t *testing.T) {
        t.Parallel()

        body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "public_holidays_pl_2025.json"))
        require.NoError(t, err)
        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            _, _ = w.Write(body)
        }))
        t.Cleanup(srv.Close)

        var stdout, stderr bytes.Buffer
        // Use OPENHOLIDAYS_BASE_URL env or a test-only --base-url flag, OR
        // construct the Client inside cmd/ohcli with WithBaseURL plumbing.
        // (Planner decides the seam shape — recommended: env var read once
        //  in newClient(); test sets t.Setenv to srv.URL.)
        t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)
        code := run([]string{"ohcli", "public", "PL", "2025"}, &stdout, &stderr)

        require.Equal(t, 0, code, "stderr: %s", stderr.String())
        assert.Contains(t, stdout.String(), "2025-01-01")
        assert.Contains(t, stdout.String(), "DATE")  // header row
    })

    t.Run("usage error: missing year returns exit 2", func(t *testing.T) {
        t.Parallel()
        var stdout, stderr bytes.Buffer
        code := run([]string{"ohcli", "public", "PL"}, &stdout, &stderr)
        assert.Equal(t, 2, code)
        assert.Contains(t, stderr.String(), "ohcli:")
    })

    t.Run("empty result prints to stderr and exits 0", func(t *testing.T) {
        t.Parallel()
        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
            _, _ = w.Write([]byte(`[]`))
        }))
        t.Cleanup(srv.Close)
        t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

        var stdout, stderr bytes.Buffer
        code := run([]string{"ohcli", "public", "PL", "2025"}, &stdout, &stderr)
        assert.Equal(t, 0, code)
        assert.Empty(t, stdout.String(), "stdout must be empty so pipes don't break")
        assert.Contains(t, stderr.String(), "no public holidays found")
    })
}
```

**Error-path testify pattern** (`public_holidays_test.go` lines 100-127):

```go
t.Run("validation error: empty CountryIsoCode wraps ErrInvalidCountry", func(t *testing.T) {
    t.Parallel()
    c := NewClient(WithBaseURL("http://example.invalid"))
    holidays, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{ ... })
    require.Error(t, err)
    assert.Nil(t, holidays)
    assert.True(t, errors.Is(err, ErrInvalidCountry),
        "expected ErrInvalidCountry via errors.Is, got %v", err)
})
```

**Fixture path note for `cmd/ohcli/*_test.go`:** existing tests live at repo root and read `filepath.Join("testdata", ...)`. CLI tests live at `cmd/ohcli/` (two directories deeper) and must read `filepath.Join("..", "..", "testdata", ...)`. Alternative: copy fixtures into `cmd/ohcli/testdata/` (Go's standard testdata convention — each package has its own). Recommended: relative path (`../../testdata/...`) — keeps one source of truth.

---

### `cmd/ohcli/format_test.go` (unit-test, transform)

**Analog:** `holiday_test.go`

Pure-helper testify pattern — no HTTP, no fixtures (or only via tiny constructed `[]Holiday` literals). One `TestXxx` per renderer function (`TestRenderText`, `TestRenderJSON`, `TestRenderCSV`).

**Pattern** (`holiday_test.go` lines 24-53):

```go
func TestHoliday_NameFor(t *testing.T) {
    t.Parallel()

    t.Run("matches Polish entry case-insensitively", func(t *testing.T) {
        t.Parallel()
        h := Holiday{Name: []LocalizedText{
            {Language: "pl", Text: "Wigilia"},
            {Language: "en", Text: "Christmas Eve"},
        }}
        assert.Equal(t, "Wigilia", h.NameFor("pl"))
        assert.Equal(t, "Wigilia", h.NameFor("PL"))
    })
    ...
}
```

Mirrors directly to:

```go
func TestRenderCSV(t *testing.T) {
    t.Parallel()

    t.Run("header row + one data row for single-holiday slice", func(t *testing.T) {
        t.Parallel()
        var buf bytes.Buffer
        err := renderCSV(&buf, []openholidays.Holiday{{
            StartDate: openholidays.NewDate(2025, time.January, 1),
            EndDate:   openholidays.NewDate(2025, time.January, 1),
            Name:      []openholidays.LocalizedText{{Language: "en", Text: "New Year"}},
        }}, "en")
        require.NoError(t, err)
        lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
        require.Len(t, lines, 2)
        assert.Contains(t, lines[0], "start_date")
        assert.Contains(t, lines[1], "2025-01-01,2025-01-01,New Year")
    })
}
```

---

### `fuzz_test.go` (fuzz-target, transform)

**Analog:** Closest in-repo touchpoint is the testify-imports + same-package convention from any existing root `_test.go` file (e.g. `date_test.go`); the actual `testing.F` + `F.Add` + `F.Fuzz` shape has no existing in-repo precedent. Use `05-RESEARCH.md` §"Pattern 3" reference.

**Imports pattern** (consistent with existing tests):

```go
package openholidays

import (
    "encoding/json"
    "os"
    "testing"
)
```

**Reference body** (from `05-RESEARCH.md` lines 411-435):

```go
func FuzzUnmarshalHoliday(f *testing.F) {
    // Load Phase 3 fixtures as initial seeds.
    for _, name := range []string{
        "testdata/public_holidays_pl_2025.json",
        "testdata/school_holidays_pl_2025.json",
    } {
        b, err := os.ReadFile(name)
        if err != nil {
            f.Fatal(err)
        }
        f.Add(b)
    }
    // Adversarial seeds.
    f.Add([]byte(`{}`))
    f.Add([]byte(`{"id":"","startDate":"2025-01-01","endDate":"2024-12-31","type":"Public","name":[]}`))
    f.Add([]byte(`[{"id":"x","startDate":null,"endDate":"2025-01-01","type":"","name":null}]`))

    f.Fuzz(func(t *testing.T, data []byte) {
        var hs []Holiday
        _ = json.Unmarshal(data, &hs)
    })
}
```

**Notes:**
- File lives at repo root (`fuzz_test.go`), in package `openholidays` — gives direct access to unexported helpers (`pickLocalized`) for `FuzzParseLocalizedText`.
- Auto-discovered seed corpus under `testdata/fuzz/FuzzParseLocalizedText/` and `testdata/fuzz/FuzzUnmarshalHoliday/` per Go's stdlib convention.
- Same testify-free style as `clock_test.go` is fine when the fuzz body needs no assertions beyond "must not panic" — the runtime fuzzer reports panics directly.

---

### `bench_test.go` (bench-test, request-response)

**Analog:** `public_holidays_test.go` (for `httptest.NewServer` + fixture body) + Pitfall 5 reinterpretation (`Countries` instead of `PublicHolidays` for the cached sub-benchmark).

**Reference body** (from `05-RESEARCH.md` lines 489-535):

```go
func BenchmarkClient_PublicHolidays(b *testing.B) {
    body, err := os.ReadFile("testdata/public_holidays_pl_2025.json")
    require.NoError(b, err)
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write(body)
    }))
    b.Cleanup(srv.Close)
    ...
    b.Run("cold (no cache)", func(b *testing.B) { ... })
    b.Run("cached", func(b *testing.B) { ... })  // measures Countries per A2 reinterpretation
}
```

**Notes:**
- Use `*testing.B`'s `b.Cleanup`, `b.ResetTimer`, `b.Run` (parallels the `t.Cleanup` / `t.Run` etiquette).
- Per CONTEXT.md `< 5 ms cached` target only applies to cached endpoints (`Countries`/`Languages`/`Subdivisions`) — not `PublicHolidays` (RESIL-07 / D-83). 05-RESEARCH.md Pitfall 5 ratifies this reinterpretation as CL-18 candidate.

---

### `integration_test.go` (integration-test, request-response)

**Analog:** `update_fixtures_test.go` — EXACT match.

**Build-tag + env-gate preamble** (`update_fixtures_test.go` lines 1-71):

```go
//go:build integration

// Package openholidays — fixture refresh utility (live API).
//
// This file is compiled only when -tags=integration is supplied to go test
// AND has effect only when OPENHOLIDAYS_LIVE=1 is also set. Both gates must
// be true to issue any HTTP request to the live upstream; either being unset
// causes the test to skip silently (compile-time exclusion in the first
// case, runtime t.Skip in the second).
...

package openholidays

import (
    "context"
    "os"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)
```

**Body pattern** (mirror `05-RESEARCH.md` §"Pattern 4" lines 457-477):

```go
func TestIntegration_PublicHolidays_PL_2025(t *testing.T) {
    if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
        t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
    }
    c := NewClient(WithTimeout(15 * time.Second))
    t.Cleanup(func() { _ = c.Close() })

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    t.Cleanup(cancel)

    t.Run("14 public holidays for PL 2025", func(t *testing.T) {
        hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
            CountryIsoCode: "PL",
            ValidFrom:      NewDate(2025, time.January, 1),
            ValidTo:        NewDate(2025, time.December, 31),
        })
        require.NoError(t, err)
        require.Len(t, hs, 14, "PL 2025 has 14 public holidays per Phase 3 golden fixture")
    })
}
```

**Critical note from 05-RESEARCH.md Pitfall 1:** Use `context.Background()` + `context.WithTimeout(...)`, NOT `t.Context()`. The latter is Go 1.24+ and breaks the Go 1.23 CI leg.

---

### `example_test.go` (godoc-example, request-response)

**Analog:** No existing `Example_*` in the repo. Closest analog is the godoc voice in `doc.go`, `holiday.go`, `client.go`. The shape comes from `05-RESEARCH.md` §"Code Examples → Example_*" reference.

**Godoc voice pattern to mirror** (`holiday.go` lines 27-39):

```go
// NameFor returns the localized holiday name for the given ISO 639-1
// language code. Language matching is case-insensitive (strings.EqualFold)
// so "PL" matches a "pl" entry. When the requested language is not found,
// NameFor falls back to the first entry in the Name slice. Returns the
// empty string only when Name is empty.
//
// The accessor is named NameFor (not Name) because Holiday already has a
// Name field of type []LocalizedText — a method named Name(lang) would
// collide with the field. The same shape is used by Country.NameFor,
// Language.NameFor, and Subdivision.NameFor (CL-05 / CL-10).
func (h Holiday) NameFor(lang string) string {
```

Match this voice in the doc comments above each `Example_*`.

**Reference body** (`05-RESEARCH.md` lines 783-819):

```go
package openholidays_test

import (
    "context"
    "fmt"
    "time"

    "github.com/egeek-tech/go-openholidays"
)

// ExampleClient_PublicHolidays demonstrates the canonical
// "fetch one year of public holidays" call against a Client.
//
// The // Output: block is intentionally omitted because the live API would
// be hit at `go test -run Example` time; the example is therefore compile-only.
func ExampleClient_PublicHolidays() {
    c := openholidays.NewClient()
    defer c.Close()
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    hs, err := c.PublicHolidays(ctx, openholidays.PublicHolidaysRequest{
        CountryIsoCode: "PL",
        ValidFrom:      openholidays.NewDate(2025, time.January, 1),
        ValidTo:        openholidays.NewDate(2025, time.December, 31),
    })
    if err != nil {
        fmt.Println("error:", err)
        return
    }
    fmt.Printf("got %d Polish public holidays\n", len(hs))
}

// ExampleHoliday_NameFor demonstrates language fallback when the requested
// language is not present in the LocalizedText slice.
func ExampleHoliday_NameFor() {
    h := openholidays.Holiday{
        Name: []openholidays.LocalizedText{
            {Language: "pl", Text: "Boże Narodzenie"},
            {Language: "en", Text: "Christmas Day"},
        },
    }
    fmt.Println(h.NameFor("xx"))
    // Output: Boże Narodzenie
}
```

**Notes:**
- File lives at repo root in package `openholidays_test` (external test package — exercises the public surface only). This is the pattern `pkg.go.dev` expects.
- Per CONTEXT.md "Claude's Discretion" + Pitfall 7: doctest-style with `// Output:` blocks where deterministic (pure helpers like `Holiday.NameFor`, `HolidayType.IsKnown`); compile-only (no `// Output:`) where output depends on the live API (`Client.*` methods).

---

### `doc.go` (package-doc, n/a — extend existing)

**Analog:** Existing `doc.go` (current contents below).

```go
// Package openholidays is a Go client for the OpenHolidays public-holidays
// and school-holidays API (https://www.openholidaysapi.org).
//
// The library exposes public holidays, school holidays, country and language
// metadata, and administrative subdivisions through a clean, well-tested
// Go-first API. It is designed for backend engineers building HR, scheduling,
// education, and leave-management applications — including those that need
// regional school-break granularity (for example, Polish ferie per
// województwo) that competing libraries do not cover.
//
// Design principles:
//   - Zero runtime dependencies (no non-stdlib import outside *_test.go).
//   - Full context.Context propagation on every exported call.
//   - Typed errors inspectable via errors.Is / errors.As.
package openholidays
```

**Phase 5 extension scope** (per DOC-02): leave the existing godoc preamble intact; append a short "Quickstart" block referencing `ExampleClient_PublicHolidays` (the rest of the examples render automatically beneath `Client.PublicHolidays` etc. on `pkg.go.dev`).

**Em-dash preservation:** the existing `—` characters on lines 7-8 must NOT be mass-replaced to `--` per CONVENTIONS.md / CL-17.

---

### `.golangci.yml` (lint-config, n/a)

**Analog:** `.golangci.yml_backup` — but only as a **starting point**; per Pitfall 3 the file requires a substantial rewrite.

**What to keep from the backup** (verified by reading `.golangci.yml_backup` 2026-05-28):

Lines 1-3 (version + run block):

```yaml
version: "2"
run:
  modules-download-mode: readonly
```

Lines 4-43 (linters.default + the curated `enable:` list — but DROP `tagliatelle` per Pitfall 4):

```yaml
linters:
  default: none
  enable:
    - asciicheck
    - durationcheck
    - copyloopvar
    - contextcheck
    - errcheck
    - errchkjson
    - errname
    - fatcontext
    - gocritic
    - gomoddirectives
    - gosec
    - govet
    - ineffassign
    - intrange
    - mirror
    - musttag
    - nilerr
    - revive
    - sloglint
    - staticcheck
    # - tagliatelle — DROPPED per Pitfall 4 (upstream is camelCase; enforcing
    #   case style on JSON tags adds noise without value).
    - testifylint
    - thelper
    - unconvert
    # - unparam — disabled 2026-05-11 (preserve rationale comment from backup
    #   lines 35-39 verbatim).
    - unused
    - usestdlibvars
    - usetesting
    - wastedassign
```

Lines 44-51 (gosec excludes + revive.exported off — keep as-is):

```yaml
  settings:
    gosec:
      excludes:
        - G101
    revive:
      rules:
        - name: exported
          disabled: true
```

Lines 69-76 (exclusions.presets — keep as-is):

```yaml
  exclusions:
    generated: lax
    presets:
      - comments
      - common-false-positives
      - legacy
      - std-error-handling
```

Lines 133-151 (`_test.go` gosec exemptions for G124 + G122 — keep as-is):

```yaml
    rules:
      - linters:
          - gosec
        text: "G124"
        path: _test\.go
      - linters:
          - gosec
        text: "G122"
        path: _test\.go
```

**What to STRIP from the backup** (all cross-project debris — verified absent from this repo):

- Lines 52-68 (the `tagliatelle.case.rules` block with the GDPR / OIDC justification — entire block goes).
- Lines 77-132 (every `exclusions.rules.path:` entry referencing `internal/gdpr/*`, `internal/web/handlers/*`, `*_templ.go`, `web/templates/layout/*`, `internal/app/app.go`, `internal/web/middleware/recover.go`, `internal/web/middleware/request_logger.go` — none of these paths exist in this repo).
- Lines 152-162 (`paths: third_party$ / builtin$ / examples$` — none of these dirs exist; harmless to keep but adds noise).

**Treat the rewrite as "author a new `.golangci.yml` using the backup as a checklist"** — not "copy and modify." After the new file lands, delete `.golangci.yml_backup` (per 05-RESEARCH.md §"Open Questions" Q2).

---

### `go.mod` (module-manifest, n/a — verify only)

**Analog:** Existing `go.mod`.

Current contents:

```go
module github.com/egeek-tech/go-openholidays

go 1.23

require github.com/stretchr/testify v1.11.1

require (
    github.com/davecgh/go-spew v1.1.1 // indirect
    github.com/pmezard/go-difflib v1.0.0 // indirect
    gopkg.in/yaml.v3 v3.0.1 // indirect
)
```

**Phase 5 verification** (per CI-03 + REL-04):
- `head -1 go.mod` equals `module github.com/egeek-tech/go-openholidays` ✓ (already satisfied).
- `go 1.23` floor ✓ (already satisfied).
- `go mod tidy && git diff --exit-code -- go.mod go.sum` — clean.
- `cmd/ohcli/*.go` imports MUST NOT add new runtime deps (PROJECT.md zero-dep rule); only stdlib + the library at its own module path.
- No `tool` directives needed for v0.1.0 (golangci-lint, govulncheck, goreleaser are CI-installed, not `go run`-ed via `tool` block).

---

## Shared Patterns

### Test conventions (Gold Rule 3) — applied to every new `_test.go`

**Source:** `public_holidays_test.go`, `holiday_test.go`, `clock_test.go`, `internal_test.go`, `update_fixtures_test.go`

**Apply to:** `cmd/ohcli/*_test.go`, `fuzz_test.go`, `bench_test.go`, `integration_test.go`, `example_test.go`

1. **File-level godoc preamble** explaining the file's role + Gold Rule 3 application (see `public_holidays_test.go` lines 1-15 above).
2. **Imports order**: stdlib first (one group), then `github.com/stretchr/testify/{assert,require}` (testify last group, alphabetical within).
3. **One `TestXxx` per exported production function** — for CLI, one per subcommand (`TestCmdPublic`, `TestCmdSchool`, `TestCmdCountries`); for fuzz, one per fuzz target (`FuzzParseLocalizedText`, `FuzzUnmarshalHoliday`); for benchmarks, one per measured boundary.
4. **`t.Parallel()` at top of `TestXxx`** + on every leaf `t.Run` subtest (unless the subtest mutates global state like `t.Setenv`, in which case omit per `internal_test.go` etiquette).
5. **`t.Run(name, func(t *testing.T) { ... })` per scenario** — no top-level assertions in the outer function body.
6. **`require` for preconditions, `assert` for verifications** — `require.NoError` before any subsequent assertion; `assert.Equal` etc. for the actual contract checks.
7. **`t.Cleanup(srv.Close)`** instead of `defer srv.Close()` (Go 1.14+ idiom; survives panics; CI-friendly).
8. **Error-path assertion**: `assert.True(t, errors.Is(err, ErrXxx), "expected ErrXxx via errors.Is, got %v", err)` (mirror `public_holidays_test.go` line 111).
9. **API-error assertion via `errors.As`**: `require.True(t, errors.As(err, &apiErr), "expected *APIError, got %T: %v", err, err)` (mirror line 177).

### Em-dash style (CL-17 / CONVENTIONS.md) — applied to every new doc

**Source:** Throughout the codebase — see `doc.go` line 7 (`— including`), `holiday.go` line 35 (`—`), `client.go` lines 49, 53, 56-57.

**Apply to:** All new godoc comments, README.md, docs/design.md, CONTRIBUTING.md, CHANGELOG.md, example_test.go doc comments.

Em-dashes (`—`, U+2014) are deliberate prose-style choices throughout this repo. No mass-replace to `--` (double-hyphen). Em-dash in godoc renders correctly on `pkg.go.dev` and in `go doc`.

### English-only (Gold Rule 1)

**Source:** Repo-wide invariant (CONVENTIONS.md, `.planning/codebase/CONVENTIONS.md` Rule 1)

**Apply to:** Every new source file, doc, comment, commit message, README, CHANGELOG.

The only acceptable non-English strings are:
- `testdata/*.json` fixtures (real upstream API responses).
- `Example_*` doctest output (e.g. `// Output: Boże Narodzenie` from `ExampleHoliday_NameFor`).
- Identifiers in test names that quote upstream payloads (e.g. `t.Run("matches Polish entry case-insensitively", ...)` is fine because the assertion key — the `pl` language tag — is the upstream value, not a Polish-language identifier).

### Fixture loading (testdata convention)

**Source:** `public_holidays_test.go` lines 50-52, `school_holidays_test.go`, every endpoint test.

**Apply to:** All new tests that need real upstream payloads.

```go
body, err := os.ReadFile(filepath.Join("testdata", "public_holidays_pl_2025.json"))
require.NoError(t, err, "fixture missing — re-capture from live API per <relevant plan>")
```

For tests in `cmd/ohcli/*_test.go`, either:
- (a) `filepath.Join("..", "..", "testdata", "public_holidays_pl_2025.json")` — relative path back to the root testdata dir, OR
- (b) Copy/symlink fixtures into `cmd/ohcli/testdata/` (Go's standard testdata convention — each package gets its own).

**Recommended: (a)** to keep one source of truth and avoid fixture-drift between two directories.

### Error wrapping (PROJECT.md ERR-04 / Phase 1 D-13)

**Source:** `errors.go` lines 16-62.

**Apply to:** Any error-producing code in `cmd/ohcli/*.go` (CLI error messages). Specifically: when wrapping a library error in a CLI subcommand, do NOT re-wrap library sentinels — print the chain as-is. The library's wrapping (`fmt.Errorf("...: %w", ErrInvalidCountry)`) renders correctly via `err.Error()` and CLI just prepends `ohcli: ` prefix:

```go
fmt.Fprintf(stderr, "ohcli: %v\n", err)
```

(`%v` walks the wrap chain and produces the full `"endpoint X: openholidays: invalid country code"` string. No additional wrapping required at the CLI layer.)

### Functional options + `NewClient(WithXxx(...))` (CLIENT-02)

**Source:** `client.go` lines 95-100, `options.go` (every `WithXxx`).

**Apply to:** Every CLI subcommand's `openholidays.NewClient(...)` call site.

```go
c := openholidays.NewClient(
    openholidays.WithUserAgent("ohcli/" + openholidays.Version),
    openholidays.WithTimeout(15 * time.Second),
    // Test-only seam: planner adds WithBaseURL(srv.URL) reading from
    // OPENHOLIDAYS_BASE_URL env var so cmd/ohcli/*_test.go can plumb
    // httptest.NewServer URLs without forking the public surface.
)
defer c.Close()
```

### Same-package test helpers stay in same-package files (D-94 / D-95)

**Source:** `clock_test.go` lines 1-26 (package `openholidays`, NOT `openholidays_test`), `transport_header_test.go` line 1 (same), `internal_test.go` line 1 (same).

**Apply to:** `fuzz_test.go`, `bench_test.go`, `integration_test.go` — all live in package `openholidays` (NOT `openholidays_test`) so they can exercise unexported helpers (`pickLocalized`, `validateHolidays`) directly.

**Exception:** `example_test.go` lives in package `openholidays_test` (external test package) so the examples exercise only the public surface — this is what `pkg.go.dev` expects.

---

## No Analog Found

Files with no close in-repo analog. Planner uses `05-RESEARCH.md` reference implementations directly.

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `cmd/ohcli/main.go` | cli-entrypoint | request-response | No `cmd/` exists in repo. Use 05-RESEARCH.md §"Pattern 1" lines 291-342 verbatim. |
| `cmd/ohcli/public.go` | cli-subcommand | request-response | Use 05-RESEARCH.md §"Code Examples → CLI subcommand handler" lines 686-765 verbatim. |
| `cmd/ohcli/school.go` | cli-subcommand | request-response | Mirror `public.go` reference; add `--region` flag (string, optional). |
| `cmd/ohcli/countries.go` | cli-subcommand | request-response | Mirror `public.go` reference; drop year positional. |
| `cmd/ohcli/format.go` | format-renderer | transform | Use 05-RESEARCH.md §"Pattern 2" lines 348-400 verbatim (text/json/csv renderers). |
| `testdata/fuzz/FuzzParseLocalizedText/` | fuzz-corpus | n/a | Empty dir; runtime fuzzer populates after first failure. Seed via `F.Add` in `fuzz_test.go`. |
| `testdata/fuzz/FuzzUnmarshalHoliday/` | fuzz-corpus | n/a | Same. |
| `.github/workflows/ci.yml` | ci-workflow | n/a | No `.github/` exists. Use 05-RESEARCH.md §"Code Examples → ci.yml" lines 990-1079 verbatim. **Pin action versions per Source-current notes**: `setup-go@v5`, `codecov-action@v5` (NOT v4), `golangci-lint-action@v7`, `govulncheck-action@v1`, `checkout@v4`. |
| `.github/workflows/integration.yml` | ci-workflow | n/a | Use 05-RESEARCH.md lines 1081-1108 verbatim. |
| `.github/workflows/release.yml` | ci-workflow | n/a | Use 05-RESEARCH.md lines 1110-1151 verbatim. **Pin `actions/attest-build-provenance@v4` (NOT v1)** per Source-current notes. |
| `.github/dependabot.yml` | ci-workflow | n/a | Use 05-RESEARCH.md lines 1153-1164 verbatim. |
| `.goreleaser.yaml` | release-config | n/a | Use 05-RESEARCH.md §"Code Examples → .goreleaser.yaml" lines 904-987 verbatim. |
| `README.md` | project-doc | n/a | New authoring. Badges per CONTEXT.md "Claude's Discretion" + 05-RESEARCH.md §"Open Questions" Q4 (`[CI] [codecov] [report card] [godoc] [license]`). Quickstart ≤ 20 lines per DOC-01. |
| `docs/design.md` | project-doc | n/a | New authoring. ASCII transport-chain diagram from 05-RESEARCH.md §"System Architecture Diagram" lines 183-243. |
| `CONTRIBUTING.md` | project-doc | n/a | Minimal v0.1.0 — dev loop (`go test`, `go test -race`, `golangci-lint run`, `go test -tags=integration`, `go test -fuzz`) + conventional-commit note per Q1. |
| `CHANGELOG.md` | project-doc | n/a | One-line pointer per CONTEXT.md D-12 + 05-RESEARCH.md §"Open Questions" Q3. |

---

## Metadata

**Analog search scope:** repo root `*.go` files (all 50+ files listed in initial `ls`), `testdata/` directory, `.golangci.yml_backup`, `.planning/phases/05-distribution/{CONTEXT,RESEARCH}.md`.

**Files scanned:** 13 source files read (`public_holidays_test.go`, `holiday.go`, `holiday_test.go`, `clock_test.go`, `internal_test.go`, `update_fixtures_test.go`, `client.go`, `types.go`, `errors.go`, `transport_header_test.go`, `doc.go`, `version.go`, `go.mod`); 1 config file (`.golangci.yml_backup`); 2 phase documents (CONTEXT.md, RESEARCH.md lines 1-1405).

**Pattern extraction date:** 2026-05-28

**Key insight:** Phase 5 splits cleanly into two clusters:
1. **Test-shape cluster** (10 files: all `_test.go` additions + `doc.go` extension + `go.mod` verify) — every pattern already exists in-repo; mechanical copy.
2. **Greenfield-config cluster** (16 files: `cmd/ohcli/*.go`, `.github/`, `.goreleaser.yaml`, README/docs/CONTRIBUTING/CHANGELOG, `.golangci.yml` rewrite) — no in-repo analog; 05-RESEARCH.md reference implementations are the authoritative source.

The planner should structure plans around this split: one plan per cluster boundary, or one plan per `Wave 0 Gap` checklist item in 05-RESEARCH.md lines 1305-1320.
