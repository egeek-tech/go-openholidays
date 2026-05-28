---
phase: 05-distribution
plan: 02
subsystem: cli
tags: [cli, testing, testify, httptest, ohcli]

# Dependency graph
requires:
  - phase: 05-distribution
    plan: 01
    provides: cmd/ohcli main.go / public.go / school.go / countries.go / format.go production code; run / cmdPublic / cmdSchool / cmdCountries / renderText / renderJSON / renderCSV / renderCountries entry points; OPENHOLIDAYS_BASE_URL env-var test seam
  - phase: 01-foundation
    provides: openholidays.Holiday, openholidays.Country, openholidays.LocalizedText, openholidays.SubdivisionRef types
  - phase: 03-endpoints
    provides: testdata/public_holidays_pl_2025.json (14 entries), testdata/school_holidays_pl_2025.json (7 periods), testdata/countries.json (2 entries) — captured 2026-05-27
provides:
  - TestRun (cmd/ohcli/main_test.go) — dispatch coverage for no-args, unknown command, version, help/-h/--help
  - TestCmdPublic (cmd/ohcli/public_test.go) — happy paths for text/json/csv; D-07 empty-result; usage errors (missing year, invalid year, out-of-bounds year, invalid --format); runtime error (server 500 -> exit 1); --lang lowercase-on-wire assertion
  - TestCmdSchool (cmd/ohcli/school_test.go) — --region PL-SL CLI-02 wire assertion; D-07 parenthesized empty-result wording; no-region path with subdivisionCode absent from URL; missing-year and server 500 paths
  - TestCmdCountries (cmd/ohcli/countries_test.go) — happy path against countries.json; CSV header literal; positional-arg rejection; D-07 empty result; invalid --format
  - TestRenderText / TestRenderJSON / TestRenderCSV / TestRenderCountries (cmd/ohcli/format_test.go) — pure-transform tests for the four renderers, in-memory []Holiday / []Country literals (no fixtures), 14 t.Run subtests total
  - reorderArgs / hasByte helpers in cmd/ohcli/main.go — flags-first argv reorderer so stdlib flag.Parse handles `ohcli public PL 2025 --json` regardless of token order (Rule 1 deviation; see Deviations section)
affects: [05-04, 05-05, 05-06, 05-07]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "httptest.NewServer per scenario + t.Setenv(OPENHOLIDAYS_BASE_URL, srv.URL) for in-process HTTP injection — mirrors public_holidays_test.go convention but reaches the binary through run([]string{...}, &stdout, &stderr) instead of constructing the Client directly"
    - "Server-side query-param assertions inside the handler body — catches URL-builder regressions (countryIsoCode, validFrom, subdivisionCode, languageIsoCode) that fixture-body-only assertions would mask"
    - "t.Setenv + t.Parallel split discipline: TestRun (no t.Setenv) is t.Parallel at top; TestCmdPublic / TestCmdSchool / TestCmdCountries (every leaf uses t.Setenv) are NOT t.Parallel at top, matching the Go runtime contract that forbids the combination"
    - "Pure-transform renderer tests construct in-memory []Holiday literals — no fixtures, no HTTP — so the renderer contract is pinned independently of upstream schema drift"
    - "Flags-first argv reorderer (reorderArgs) — distinguishes bool flags (--json, --csv) from value-taking flags (--lang, --format, --region) via a per-handler boolFlags map so 'ohcli school PL 2025 --region PL-SL' parses correctly regardless of token order"

key-files:
  created:
    - cmd/ohcli/format_test.go
    - cmd/ohcli/main_test.go
    - cmd/ohcli/public_test.go
    - cmd/ohcli/school_test.go
    - cmd/ohcli/countries_test.go
    - .planning/phases/05-distribution/05-02-SUMMARY.md
  modified:
    - cmd/ohcli/main.go (reorderArgs + hasByte helpers; Rule 1 deviation)
    - cmd/ohcli/public.go (route argv through reorderArgs before fs.Parse)
    - cmd/ohcli/school.go (route argv through reorderArgs before fs.Parse)
    - cmd/ohcli/countries.go (route argv through reorderArgs before fs.Parse)

key-decisions:
  - "Top-level TestCmdPublic/TestCmdSchool/TestCmdCountries do NOT call t.Parallel() — the Go testing runtime forbids t.Setenv inside any test whose ancestors are parallel. Plan 05-02 anticipated this constraint at the leaf level but mis-specified it at the parent level; the correct fix is to drop the parent t.Parallel and rely on go test's package-level scheduling for cross-file isolation. TestRun (which uses no t.Setenv) keeps t.Parallel."
  - "Rule 1 deviation: discovered during Task 2 execution that stdlib flag.Parse halts at the first non-flag token, so `ohcli public PL 2025 --json` left --json unparsed and the handler returned exit 2 with the wrong diagnostic. Fixed by adding reorderArgs in main.go that splits argv into flags-first form before fs.Parse. Helper distinguishes bool flags (--json, --csv) from value-taking flags via a per-handler boolFlags map so --region PL-SL keeps its value paired with its flag. Bug is in Plan 05-01 production code, fix is in Plan 05-02 since Plan 05-02 is the wave that proves the documented CLI shape works."
  - "Server-side handler asserts URL path + query params (countryIsoCode, validFrom, validTo, subdivisionCode, languageIsoCode) — catches URL-builder regressions independently of the fixture body. Mirrors the analog pattern in public_holidays_test.go."
  - "Pure-transform renderer tests (TestRenderText etc.) use in-memory []Holiday and []Country literals — no fixtures — so renderer contract is pinned without coupling to upstream schema drift. Matches PATTERNS.md §format_test.go guidance."

patterns-established:
  - "CLI integration-test recipe: httptest.NewServer + t.Cleanup(srv.Close) + t.Setenv(OPENHOLIDAYS_BASE_URL, srv.URL) + run([]string{argv...}, &stdout, &stderr) — drives the binary end-to-end through the same dispatch path os.Args follows in production"
  - "Exit-code assertion shape: require.Equal(t, want, code, 'stderr=%q', stderr.String()) — fails noisily with the captured stderr so test failures pinpoint the unexpected error message"
  - "D-07 empty-result assertion: assert.Empty(stdout) AND assert.Contains(stderr, 'ohcli: no <thing> found ...') — proves the two-part contract (empty stdout for clean pipes; informative stderr) in one subtest"
  - "Pure-transform renderer test: build the smallest []Holiday slice that exercises the field under test, render into &bytes.Buffer, assert against String() — no fixtures, no HTTP, fast and independent of upstream"

requirements-completed: [CLI-04]

# Metrics
duration: 25min
tasks-completed: 2
files-created: 5
files-modified: 4
test-functions: 8
test-subtests: 36
commits:
  - hash: b881759
    type: test
    msg: "test(05-02): add cmd/ohcli/format_test.go renderer tests"
  - hash: 91e5d09
    type: fix
    msg: "fix(05-02): reorder argv before flag.Parse to support positionals-first CLI invocation"
  - hash: 031a171
    type: test
    msg: "test(05-02): add cmd/ohcli httptest-driven subcommand tests"
completed: 2026-05-28
---

# Phase 5 Plan 2: cmd/ohcli Test Suite Summary

**Behavioral proof that the ohcli binary works as specified.** Adds the test suite that pins every subcommand (public, school, countries, version, help) against `httptest.NewServer`, every output mode (text/json/csv), every exit code (0/1/2), and the D-05 / D-06 / D-07 / D-08 / CLI-02 contracts. CLI-04 (`go install ./cmd/ohcli` ships a working binary) now has behavioral coverage in addition to the build gate Plan 05-01 already satisfied.

## What Was Built

### Renderer tests (Task 1, format_test.go — 305 lines)

Four `TestRenderXxx` functions, one per renderer prod function, mirroring `holiday_test.go`:

- **TestRenderText** — 3 t.Run subtests: happy path with 2 holidays produces header + 2 data rows (assert column literals: DATE / END / NAME / NATIONWIDE / TYPE); empty slice produces header only (exactly 1 line); `--lang` preference selects the correct localized name (`pl` → "Wigilia", not "Christmas Eve").
- **TestRenderJSON** — 3 t.Run subtests: round-trip via `json.Unmarshal` back to `[]Holiday` preserves StartDate / Name / Type losslessly; two-space indent applied (`"\n  "` substring assertion); empty slice still emits valid JSON `[]`.
- **TestRenderCSV** — 4 t.Run subtests: header row literal exactly `start_date,end_date,name,nationwide,type,subdivision_codes`; embedded comma in name is RFC-4180 quoted (round-trips through `csv.Reader`); subdivision codes joined with `;`; empty slice emits header only.
- **TestRenderCountries** — 4 t.Run subtests: json/csv/text dispatcher branches all green; CSV header literal `iso_code,name,official_languages` with `;`-joined OfficialLanguages; invalid format returns a typed error that echoes the offending value.

All renderer tests use in-memory `[]Holiday` / `[]Country` literals — no fixtures, no HTTP — so the renderer contract is pinned independently of upstream schema drift. Top-level `t.Parallel()` on every TestRenderXxx and every leaf t.Run.

### CLI subcommand tests (Task 2, four files — 588 lines)

**main_test.go — TestRun** (4 t.Run subtests + 3 nested help-spelling cases):

- no-args → exit 2 + stderr contains "usage:" + empty stdout
- unknown command "frobnicate" → exit 2 + stderr "ohcli: unknown command" (D-05 prefix) echoing the bad command
- version → exit 0 + stdout == `ohcliVersion()+"\n"` + empty stderr
- help / `--help` / `-h` (table-driven inner subtests) → exit 0 + stdout contains "usage:" and lists every subcommand (public, school, countries)

**public_test.go — TestCmdPublic** (10 t.Run subtests):

- text output: load PL 2025 fixture, server-side assertions on `/PublicHolidays` path + `countryIsoCode=PL` + `validFrom=2025-01-01` + `validTo=2025-12-31`, assert stdout contains DATE header and "2025-01-01"
- `--json`: same handler; `json.Unmarshal(stdout)` returns `[]Holiday` with exactly 14 entries
- `--csv`: first stdout line equals `start_date,end_date,name,nationwide,type,subdivision_codes`
- empty result (server writes `[]`): exit 0, empty stdout (D-07 pipe contract), stderr contains "ohcli: no public holidays found for PL 2025"
- usage errors (missing year, "abc" year, 9999 year above 2100 bound, `--format=ical`): all exit 2 with D-05 prefix
- runtime error (server 500): exit 1 with D-05 prefix, empty stdout (no partial render)
- `--lang pl`: server asserts `languageIsoCode=pl` lowercase on the wire

**school_test.go — TestCmdSchool** (7 t.Run subtests):

- `--region PL-SL`: server asserts `subdivisionCode=PL-SL` query param (CLI-02 wire contract), text output renders the school holidays fixture
- empty result `--region PL-XX`: server asserts `subdivisionCode=PL-XX`, stderr contains the exact D-07 "Specifics" wording `ohcli: no school holidays found for PL 2025 (region PL-XX)` with the parenthesized region suffix
- no `--region`: server asserts `subdivisionCode` query param is absent (empty string)
- empty result without `--region`: stderr contains the non-parenthesized form (no "(region" substring)
- usage errors and runtime error: same shape as TestCmdPublic

**countries_test.go — TestCmdCountries** (5 t.Run subtests):

- text happy path: server asserts `/Countries` path, stdout contains "ISO_CODE" header and "PL" row
- `--format=csv`: first stdout line equals `iso_code,name,official_languages`
- positional arg "extra": exit 2 + stderr contains "ohcli: countries takes no positional args"
- empty result (`[]`): exit 0, empty stdout, stderr contains "ohcli: no countries found"
- `--format=ical`: exit 2 + stderr contains "ohcli: invalid format"

## Verification

```
$ go test -race -count=1 ./cmd/ohcli/
ok  	github.com/egeek-tech/go-openholidays/cmd/ohcli	1.034s

$ go vet ./cmd/ohcli/
(no output — clean)

$ go test -race -count=1 ./...
ok  	github.com/egeek-tech/go-openholidays	1.672s
ok  	github.com/egeek-tech/go-openholidays/cmd/ohcli	1.039s
```

Total: 8 TestXxx functions, 36 t.Run subtests, all green under `-race`.

D-08 enforcement check:

```
$ grep -F openholidaysapi.org cmd/ohcli/*_test.go
(no matches — zero live API calls in unit tests)
```

Exit-code coverage (all three D-06 codes asserted):

```
$ grep -hoE 'Equal\(t, [012]' cmd/ohcli/*_test.go | sort -u
Equal(t, 0
Equal(t, 1
Equal(t, 2
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] cmd/ohcli flag parsing failed on positionals-before-flags argv**

- **Found during:** Task 2 (first run of `go test -race ./cmd/ohcli/`)
- **Issue:** Plan 05-01 production code called `fs.Parse(args)` directly with argv from the run dispatcher. Stdlib `flag.Parse` halts at the first non-flag token, so the documented CLI shape `ohcli public PL 2025 --json` (positionals before flags) left `--json` in the positional set: `fs.NArg() == 3` and the handler returned exit 2 with `ohcli: public requires <country> and <year>`. Confirmed with a minimal `go run` reproducer (`fs.Args() = [PL 2025 --json]` when positionals come first).
- **Fix:** Added `reorderArgs(args, boolFlags)` and a small `hasByte` helper to `cmd/ohcli/main.go`. The helper splits argv into flags-first form before `fs.Parse` runs, preserving relative order within each group. `boolFlags` is a per-handler `map[string]struct{}{"json":{}, "csv":{}}` so the helper can distinguish `--json PL` (bool flag + positional) from `--region PL` (string flag with value). The three subcommand handlers route argv through `reorderArgs` before `fs.Parse`.
- **Files modified:** `cmd/ohcli/main.go` (+78 lines for reorderArgs + hasByte + godoc), `cmd/ohcli/public.go` (+5 lines), `cmd/ohcli/school.go` (+5 lines), `cmd/ohcli/countries.go` (+6 lines)
- **Commit:** `91e5d09`
- **Why this is Rule 1 and not Rule 4:** the fix is a localized helper inside the existing dispatcher file, not a structural change. No new architecture, no new dependencies, no API change at the run/handler boundary. The CLI shape was already documented in `cmd/ohcli/main.go`'s `usage` const and in PLAN.md acceptance criteria — the fix simply makes the production code match the contract everything else already assumed.

**2. [Rule 1 — Plan-text correction] Outer TestCmdXxx must NOT call t.Parallel**

- **Found during:** Task 2 (first run of `go test -race ./cmd/ohcli/` after writing the leaf t.Setenv pattern)
- **Issue:** PLAN.md Task 2 action body said "Top `t.Parallel()`" on TestCmdPublic / TestCmdSchool / TestCmdCountries, then later cautioned that leaf t.Run subtests using t.Setenv must NOT call t.Parallel. Go's testing runtime enforces a stricter rule: `t.Setenv` panics if ANY ancestor in the test tree has called `t.Parallel()`, including the immediate parent. The first run produced `panic: testing: test using t.Setenv... can not use t.Parallel`.
- **Fix:** Removed the top-level `t.Parallel()` call from TestCmdPublic, TestCmdSchool, and TestCmdCountries. TestRun (which uses no t.Setenv) keeps `t.Parallel`. Cross-file isolation is provided by `go test`'s package-level scheduling. Added a NOTE comment block on each TestCmdXxx explaining the constraint so the next maintainer doesn't re-introduce the bug.
- **Files modified:** `cmd/ohcli/public_test.go`, `cmd/ohcli/school_test.go`, `cmd/ohcli/countries_test.go` (each: drop one `t.Parallel()` call, add NOTE in godoc)
- **Commit:** `031a171` (incorporated into the same commit as the rest of Task 2's tests)

### Auth gates

None — all tests are local-only via `httptest.NewServer`.

## Known Stubs

None. The renderer prod code (Plan 05-01) and the subcommand handlers (Plan 05-01) are all fully wired against the library; the test suite proves it.

## Threat Flags

None. The plan's `<threat_model>` register covered T-05-05 (parallel t.Setenv collisions — mitigated by removing top-level t.Parallel from TestCmdXxx), T-05-06 (live API calls — D-08 enforced via grep), T-05-07 (flaky tests — t.Cleanup(srv.Close) on every server), T-05-SC (no package-manager installs — none performed). No new threat surface introduced.

## Self-Check: PASSED

- [x] `cmd/ohcli/format_test.go` exists — `[ -f cmd/ohcli/format_test.go ]` true
- [x] `cmd/ohcli/main_test.go` exists
- [x] `cmd/ohcli/public_test.go` exists
- [x] `cmd/ohcli/school_test.go` exists
- [x] `cmd/ohcli/countries_test.go` exists
- [x] Commit `b881759` (Task 1 renderer tests) exists in `git log --all`
- [x] Commit `91e5d09` (Rule 1 fix) exists in `git log --all`
- [x] Commit `031a171` (Task 2 CLI tests) exists in `git log --all`
- [x] `go test -race -count=1 ./cmd/ohcli/` exits 0
- [x] `go vet ./cmd/ohcli/` exits 0
- [x] `go test -race -count=1 ./...` (full suite) exits 0
- [x] Zero live API references in tests (D-08 grep returns no matches)
