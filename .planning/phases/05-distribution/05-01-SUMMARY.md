---
phase: 05-distribution
plan: 01
subsystem: cli
tags: [cli, go, stdlib-flag, tabwriter, csv, json, ohcli]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: openholidays.Version constant, Date, Holiday, Country types
  - phase: 02-transport
    provides: NewClient, WithUserAgent, WithBaseURL, WithTimeout options
  - phase: 03-endpoints
    provides: Client.PublicHolidays, Client.SchoolHolidays, Client.Countries with (ctx, Request) shape
provides:
  - cmd/ohcli demo binary with public/school/countries/version/help subcommand dispatch
  - Three stdlib output renderers (text/tabwriter, JSON/encoding-json, CSV/encoding-csv)
  - OPENHOLIDAYS_BASE_URL env-var test seam for Plan 02 httptest-driven integration tests
  - Pitfall-8-compliant version resolution chain (debug.ReadBuildInfo → openholidays.Version fallback)
affects: [05-02, 05-03, 05-05, 05-06, 05-07]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Stdlib flag subcommand dispatch (each subcommand owns its own flag.FlagSet)"
    - "io.Writer injection for stdout/stderr so integration tests can wire *bytes.Buffer"
    - "Functional-options pattern reuse from library (WithUserAgent, WithBaseURL, WithTimeout)"
    - "Per-format renderer dispatcher (render → renderText/renderJSON/renderCSV)"
    - "Parallel renderCountries for Country payloads (CountriesRequest returns []Country, not []Holiday)"

key-files:
  created:
    - cmd/ohcli/main.go
    - cmd/ohcli/version.go
    - cmd/ohcli/format.go
    - cmd/ohcli/public.go
    - cmd/ohcli/school.go
    - cmd/ohcli/countries.go
  modified: []

key-decisions:
  - "Task 1 main.go ships placeholder cmdPublic/cmdSchool/cmdCountries stubs so the build-and-vet acceptance gate passes before Task 3 lands the real handlers; Task 3 deletes them"
  - "renderCountries lives in format.go alongside the Holiday renderers (parallel structure: same dispatcher pattern, snake_case CSV header iso_code,name,official_languages, semicolon-joined official_languages)"
  - "Subdivisions/OfficialLanguages CSV columns join with ';' so each value stays a single field and round-trips through every spreadsheet importer without quoting"
  - "School subcommand empty-result message branches on whether --region was provided so the operator-facing diagnostic identifies the exact filter chain that returned zero rows"
  - "ohcliVersion implementation per Pitfall 8 — debug.ReadBuildInfo wins when Main.Version is non-empty AND not '(devel)'; library Version is the fallback. On modern Go toolchains a local build yields a pseudo-version like v0.0.0-<ts>-<sha>+dirty rather than the '(devel)' the plan's acceptance text assumed — implementation follows the canonical Pitfall 8 logic; behavior is correct"

patterns-established:
  - "Subcommand handler signature: func cmdXxx(ctx context.Context, args []string, stdout, stderr io.Writer) int"
  - "Exit-code policy: 0 success (incl. empty result per D-07), 1 runtime error, 2 usage error"
  - "Empty-result handling: emit 'ohcli: no <thing> found ...' on stderr, return 0 (D-07)"
  - "Stderr error prefix: every CLI-emitted diagnostic starts with the literal 'ohcli: ' (D-05)"
  - "--json / --csv shortcut flags override --format ('--json' wins if both shortcuts are set, per documented switch ordering)"
  - "Year bounds enforced client-side: 1900..2100 inclusive; out-of-range → exit 2 (T-05-01 mitigation)"

requirements-completed: [CLI-01, CLI-02, CLI-03]

# Metrics
duration: 7min
completed: 2026-05-28
---

# Phase 05 Plan 01: ohcli CLI core (subcommand dispatch + renderers + test seam) Summary

**Stdlib-only ohcli demo CLI with public/school/countries/version subcommand dispatch, three output renderers (text/tabwriter, JSON, CSV), OPENHOLIDAYS_BASE_URL test seam for Plan 02, and Pitfall-8-compliant version resolution.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-05-28T16:53:34Z
- **Completed:** 2026-05-28T17:00:14Z
- **Tasks:** 3
- **Files created:** 6 (622 LOC total)
- **Files modified:** 0

## Accomplishments

- `cmd/ohcli/` binary compiles cleanly via `go build`, installs via `go install`, vets clean (CLI-01/02/03 compile gate green).
- Subcommand dispatch (`public`, `school`, `countries`, `version`, `help`, default) routes to per-subcommand handlers each with their own `flag.FlagSet` (D-03 stdlib-only).
- Exit-code policy enforced: 0 on success or empty result (D-07), 1 on runtime error, 2 on usage error (D-06). Stderr error prefix `ohcli: ` applied uniformly (D-05).
- `OPENHOLIDAYS_BASE_URL` env var resolved through `newClient()` and forwarded to `openholidays.WithBaseURL` — the seam Plan 02 will use to point the binary at an `httptest.NewServer`.
- Three Holiday renderers (text/tabwriter, JSON/encoding-json with two-space indent, CSV/encoding-csv with snake_case header + no BOM) plus the parallel `renderCountries` set for the `countries` subcommand.
- `ohcli version` resolves through Pitfall 8 chain — `debug.ReadBuildInfo().Main.Version` first, fall back to `openholidays.Version` when empty or `(devel)`.
- Zero non-stdlib runtime dependencies in `cmd/ohcli` dep tree (CLI-03): `go list -deps ./cmd/ohcli` shows only stdlib + `github.com/egeek-tech/go-openholidays`.

## Task Commits

Each task was committed atomically:

1. **Task 1: cmd/ohcli/main.go + version.go (entrypoint, dispatch, version resolution)** - `bc55e46` (feat)
2. **Task 2: cmd/ohcli/format.go (text/json/csv Holiday renderers)** - `8bb9ec8` (feat)
3. **Task 3: cmd/ohcli/{public,school,countries}.go + renderCountries (subcommand handlers with OPENHOLIDAYS_BASE_URL seam)** - `ae4ecd2` (feat)

## Files Created/Modified

- `cmd/ohcli/main.go` — Package main entrypoint; `run(args, stdout, stderr)` testable dispatcher; `newClient()` helper with OPENHOLIDAYS_BASE_URL seam; const `usage`.
- `cmd/ohcli/version.go` — `ohcliVersion()` Pitfall-8 resolution chain (debug.ReadBuildInfo → openholidays.Version fallback).
- `cmd/ohcli/format.go` — `render(w, hs, lang, format)` dispatcher + `renderText/renderJSON/renderCSV` for Holiday + parallel `renderCountries` set for Country payloads.
- `cmd/ohcli/public.go` — `cmdPublic(ctx, args, stdout, stderr) int` handler. Full calendar-year `PublicHolidaysRequest`, empty-result message + exit 0 per D-07.
- `cmd/ohcli/school.go` — `cmdSchool` sibling of cmdPublic adding `--region` flag → `SchoolHolidaysRequest.SubdivisionCode` (CLI-02). Empty-result wording branches on whether --region was set.
- `cmd/ohcli/countries.go` — `cmdCountries` rejects positional args; routes through `renderCountries`.

## Decisions Made

- **Task 1 stub handlers in main.go to satisfy the build-and-vet gate:** Task 1's `<files>` only lists `main.go` + `version.go`, but its acceptance criteria require `go build ./cmd/ohcli` to exit 0. main.go's switch dispatches to cmdPublic/cmdSchool/cmdCountries — which Task 3 creates. Bridged by adding three stub handlers in main.go (returning exit 1 with "not yet implemented" on stderr) plus a `var _ = newClient` reference shim. Task 3 deletes both when the real handlers land in dedicated files. The alternative (Task 1 creates empty placeholder files for the three handlers) would violate Task 1's `<files>` declaration; the chosen approach keeps Task 1's file footprint exactly as the plan specifies.
- **renderCountries lives in format.go (not countries.go):** Task 3 recommended extending format.go and the plan's acceptance criteria explicitly check for `^func renderCountries\(` in `cmd/ohcli/format.go`. Keeps all per-format dispatchers in one file.
- **Semicolon delimiter inside CSV cells:** Subdivisions list (Holiday CSV) and OfficialLanguages list (Country CSV) both join with `';'` rather than `,` so the value stays a single field and round-trips through every spreadsheet importer without requiring CSV quoting. Downstream consumers split on `;` to recover the original list.
- **School empty-result message branches on --region:** The two wordings ("no school holidays found for <country> <year>" vs. "no school holidays found for <country> <year> (region <CC-RR>)") match CONTEXT.md "Specifics" so an operator skimming logs can identify the exact filter chain that returned zero rows.
- **`defer func() { _ = c.Close() }()` instead of `defer c.Close()`:** errcheck (part of the .golangci.yml lint set per PROJECT.md "Style") flags unchecked error returns. Wrapping the Close in an explicit discard matches the project-wide idiom and keeps the lint set green when Plan 05 ships its linter run.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Task 1 acceptance gate required `go build ./cmd/ohcli` to succeed before Task 3's handlers existed**

- **Found during:** Task 1 (Create cmd/ohcli/main.go + cmd/ohcli/version.go)
- **Issue:** main.go's subcommand dispatch references `cmdPublic` / `cmdSchool` / `cmdCountries`, which Task 3 creates in dedicated files. Without those symbols defined somewhere, Task 1's `go build ./cmd/ohcli` and `go vet ./cmd/ohcli` acceptance gates fail with `undefined: cmdPublic` etc.
- **Fix:** Added three placeholder handler stubs to `cmd/ohcli/main.go` for Task 1 (each prints "ohcli: <name> subcommand not yet implemented" to stderr and returns exit 1) plus a `var _ = newClient` reachability shim so the helper does not trip the unused-symbol lint. Task 3 deletes all four lines and the cmd*-stub-block comment when the real handlers land.
- **Files modified:** cmd/ohcli/main.go (Task 1 commit adds the stubs; Task 3 commit removes them)
- **Verification:** `go build ./cmd/ohcli && go vet ./cmd/ohcli` exit 0 after both Task 1 and Task 3.
- **Committed in:** `bc55e46` (Task 1 — adds stubs) and `ae4ecd2` (Task 3 — removes stubs)

**2. [Note - Plan accuracy] Task 1 acceptance text expected `ohcli version` to print `0.1.0` on a local-build binary; modern Go toolchains print a pseudo-version instead**

- **Found during:** Task 1 functional verification (`./ohcli version`)
- **Issue:** The plan's Task 1 acceptance text says: *"`./ohcli version` ... prints `0.1.0\n` to stdout ... debug.ReadBuildInfo will report `(devel)` for local build, falls back to library Version."* On Go 1.23+ a `go build ./cmd/ohcli` inside a module's working tree no longer reports `(devel)` — it reports a pseudo-version (`v0.0.0-<ts>-<sha>+dirty`) populated from the git state. The Pitfall 8 fallback logic the plan specifies is therefore implemented exactly as documented, but the literal output the acceptance text predicts is not what the toolchain produces.
- **Fix:** None required — the implementation follows the Pitfall 8 spec verbatim. `go run ./cmd/ohcli version` (which gets `(devel)` from ReadBuildInfo) still falls back to `openholidays.Version` and prints `0.1.0` as the plan intended. The pseudo-version output for `go build` artifacts is correct behavior on modern Go and is what end-users running `go install github.com/egeek-tech/go-openholidays/cmd/ohcli@v0.1.0` will see (their install will be tagged `v0.1.0`, not `(devel)`). Documented here for the verifier's audit trail.
- **Files modified:** None
- **Verification:** `go run ./cmd/ohcli version` → `0.1.0\n` (library Version fallback path); installed/tagged binaries will show the tag.
- **Committed in:** N/A (no code change)

---

**Total deviations:** 1 auto-fixed (Rule 3 — blocking) + 1 plan-text inaccuracy noted for the verifier
**Impact on plan:** No scope creep. The Rule 3 fix is structural (Task 1 cannot independently satisfy its build gate without bridge stubs that Task 3 removes); the plan-text inaccuracy is a real-world toolchain-behavior mismatch the planner did not anticipate but the implementation is correct per the authoritative Pitfall 8 logic.

## Issues Encountered

- `go install ./cmd/ohcli` deposited an `ohcli` binary at `$GOBIN`; `go build ./cmd/ohcli` left an `ohcli` artifact in the working tree during functional verification. Cleaned up before the Task 3 commit so the worktree stays free of untracked binaries. A repo-level `.gitignore` adding `ohcli` and `/cmd/ohcli/ohcli` is recommended for future plans but out of scope here (no `.gitignore` exists in the repo today; adding one would be a cross-cutting change not in this plan's brief).

## Known Stubs

None. The Task 1 placeholder handlers were removed in Task 3; every subcommand is wired to its real handler.

## Self-Check: PASSED

Verified:
- `cmd/ohcli/main.go` exists (FOUND)
- `cmd/ohcli/version.go` exists (FOUND)
- `cmd/ohcli/format.go` exists (FOUND)
- `cmd/ohcli/public.go` exists (FOUND)
- `cmd/ohcli/school.go` exists (FOUND)
- `cmd/ohcli/countries.go` exists (FOUND)
- Commit `bc55e46` in git log (FOUND — Task 1)
- Commit `8bb9ec8` in git log (FOUND — Task 2)
- Commit `ae4ecd2` in git log (FOUND — Task 3)
- `go build ./cmd/ohcli` exits 0 (verified)
- `go install ./cmd/ohcli` exits 0 (verified)
- `go vet ./cmd/ohcli` exits 0 (verified)
- `go list -deps ./cmd/ohcli` contains no third-party non-library packages (verified)

## Next Phase Readiness

- **Plan 05-02 (CLI integration tests) ready:** All seams in place — `run(args, stdout, stderr)` accepts `io.Writer`, `newClient()` honors `OPENHOLIDAYS_BASE_URL`, each subcommand handler is testable in isolation with a captured `*bytes.Buffer`. Plan 02 can drive `httptest.NewServer` fixtures end-to-end without re-implementing the CLI.
- **Plan 05-05 (goreleaser) ready:** `version.go` is ldflags-friendly — goreleaser-built binaries will populate `debug.ReadBuildInfo().Main.Version` and the Pitfall-8 chain will surface the tag.
- **No blockers** for downstream plans.

---
*Phase: 05-distribution*
*Plan: 01*
*Completed: 2026-05-28*
