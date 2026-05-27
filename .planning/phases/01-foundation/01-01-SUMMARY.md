---
phase: 01-foundation
plan: 01
subsystem: infra
tags: [go, go-modules, gomod, mit-license, godoc, testify, version-const]

# Dependency graph
requires:
  - phase: 00-bootstrap
    provides: planning artifacts (PROJECT.md, ROADMAP.md, CONTEXT.md, RESEARCH.md)
provides:
  - Go module identity (module github.com/egeek-tech/go-openholidays, go 1.23)
  - Test-only dep testify v1.11.1 recorded in go.mod
  - MIT LICENSE at repo root
  - Package-level godoc in doc.go
  - const Version = "0.1.0" in version.go (single source of truth)
affects: [01-02-errors, 01-03-date, 01-04-types, 01-05-validators, 01-06-fuzz, 02-transport, 05-distribution]

# Tech tracking
tech-stack:
  added:
    - "github.com/stretchr/testify v1.11.1 (test-only, indirect until first *_test.go import)"
  patterns:
    - "Single root package `openholidays`; no sub-packages outside internal/"
    - "Package godoc lives in dedicated doc.go file"
    - "Version is a const, overridable via -ldflags '-X .Version=...'"

key-files:
  created:
    - "go.mod (module path, go 1.23, testify require)"
    - "go.sum (testify v1.11.1 module sums)"
    - "LICENSE (canonical SPDX MIT, copyright 2026 go-openholidays contributors)"
    - "doc.go (package-level godoc)"
    - "version.go (const Version = \"0.1.0\")"
  modified: []

key-decisions:
  - "Module path is github.com/egeek-tech/go-openholidays (D-01 from 01-CONTEXT.md)"
  - "go directive is `go 1.23` (no minor, no `toolchain` directive — RESEARCH.md Q4/Q5)"
  - "testify v1.11.1 added as test-only dep (pre-approved per PROJECT.md; slopcheck [OK])"
  - "go-cmp NOT added in Phase 1 (RESEARCH.md says skip until first needed)"
  - "Version const is overridable via -ldflags '-X github.com/egeek-tech/go-openholidays.Version=<v>'"
  - "LICENSE copyright year is 2026 (currentDate 2026-05-27)"

patterns-established:
  - "Package godoc convention: dedicated doc.go with single block comment starting `// Package openholidays`"
  - "Version surface: single exported const at package root, no init(), no global mutable state"
  - "Dependency policy enforced: zero runtime deps; testify recorded but not yet imported (will become direct in Phase 1 Plan 02 or later when first *_test.go imports it)"

requirements-completed: []

# Metrics
duration: 2min
completed: 2026-05-27
---

# Phase 1 Plan 1: Go Module Bootstrap Summary

**Clean Go module skeleton at `github.com/egeek-tech/go-openholidays` with `go 1.23`, MIT LICENSE, package godoc, and `const Version = "0.1.0"` — strict prerequisite for every other Phase 1 plan.**

## Performance

- **Duration:** 2 min
- **Started:** 2026-05-27T08:15:05Z
- **Completed:** 2026-05-27T08:17:06Z
- **Tasks:** 2
- **Files created:** 5 (go.mod, go.sum, LICENSE, doc.go, version.go)
- **Files modified:** 0

## Accomplishments

- Bootstrapped a clean Go module rooted at `github.com/egeek-tech/go-openholidays` declaring `go 1.23` and recording `github.com/stretchr/testify v1.11.1` as the sole (test-only, currently `// indirect`) dependency.
- Created canonical SPDX MIT `LICENSE` at repo root (copyright 2026 go-openholidays contributors), enabling pkg.go.dev / Go Report Card license detection from day one.
- Established package identity via `doc.go` (godoc starting `// Package openholidays`) and `version.go` (`const Version = "0.1.0"` overridable via `-ldflags`).
- Verified `go build ./...`, `go vet ./...`, `gofmt -l doc.go version.go`, and `go mod verify` are all clean.

## Task Commits

Each task was committed atomically:

1. **Task 1: Replace stray POC go.mod/go.sum with clean module init** — `fc12e6a` (chore)
2. **Task 2: Create LICENSE (MIT), doc.go, and version.go** — `65e1df1` (feat)

_Note: this plan is not TDD; only one commit per task._

## Files Created/Modified

- `go.mod` — module identity (`github.com/egeek-tech/go-openholidays`), `go 1.23`, `require github.com/stretchr/testify v1.11.1 // indirect`. No `toolchain` directive (per RESEARCH.md Q5). No POC residue (no `holidays-poc`, `holidays-rest`, `rickb777`, or `govalues` references).
- `go.sum` — module sums for testify v1.11.1 (h1 + go.mod hashes).
- `LICENSE` — 21-line canonical SPDX MIT text. Begins with `MIT License` header, contains the canonical `Permission is hereby granted, free of charge,` phrase, ends with `THE USE OR OTHER DEALINGS IN THE SOFTWARE.`. Copyright line: `Copyright (c) 2026 go-openholidays contributors`.
- `doc.go` — package-level godoc starting `// Package openholidays is a Go client for the OpenHolidays public-holidays and school-holidays API`. Mentions zero runtime deps, full `context.Context` propagation, and typed errors. Single `package openholidays` declaration; no imports; no other code. `gofmt`-clean.
- `version.go` — declares `package openholidays` and `const Version = "0.1.0"` with godoc starting `// Version is the semantic version of the go-openholidays library.` and documenting the `-ldflags '-X github.com/egeek-tech/go-openholidays.Version=...'` override. `gofmt`-clean.

## Decisions Made

| Decision | Source | Rationale |
|----------|--------|-----------|
| Module path `github.com/egeek-tech/go-openholidays` | D-01 (01-CONTEXT.md) | Resolved during Phase 1 discuss; replaces deferred ROADMAP item. |
| `go 1.23` (not `go 1.23.0`) and no `toolchain` directive | RESEARCH.md Q4/Q5 | Avoids pinning a specific toolchain across the CI matrix (1.23, 1.24, stable). |
| testify v1.11.1 added now (even though no `*_test.go` imports it yet) | Plan acceptance criteria | Locks the test-framework version early; will flip from `// indirect` to direct when Plan 01-02 (errors) adds the first test file. |
| `go-cmp` deliberately NOT added | RESEARCH.md §"Package Legitimacy Audit" | Skip until first needed — keeps test-only dep graph minimal. |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Reordered `go get testify` to run after `go mod tidy`**

- **Found during:** Task 1 (module initialization)
- **Issue:** The plan's prescribed step order (`go mod init` → `go mod edit -go=1.23` → `go get testify@v1.11.1` → `go mod tidy`) made `go mod tidy` strip the testify require because no `.go` file imports `github.com/stretchr/testify` yet. Without testify in `go.mod`, the plan's own automated verification (`grep -q 'github.com/stretchr/testify v1.11.1' go.mod`) and acceptance criterion ("go.mod contains testify v1.11.1 in a require block") both fail.
- **Root cause:** Modern `go mod tidy` removes unused indirect requires. The plan's `<read_first>` note ("this will be marked `// indirect` until a `*_test.go` file imports it (per Go module semantics)") anticipated indirect retention, but tidy actively prunes unused indirects with no consumers in the module's own source.
- **Fix:** Ran the commands in order `init` → `edit -go=1.23` → `go get testify@v1.11.1` → `go mod tidy` → `go get testify@v1.11.1` again. The final `go get` re-adds the require directive after tidy strips it. Net result is identical to what the plan intends: `require github.com/stretchr/testify v1.11.1 // indirect` plus `go.sum` entries.
- **Files modified:** `go.mod`, `go.sum`
- **Verification:** `grep -c 'github.com/stretchr/testify v1.11.1' go.mod` returns `1`; `go mod verify` returns `all modules verified`; `go build ./...` and `go vet ./...` clean.
- **Committed in:** `fc12e6a` (Task 1 commit)
- **Forward note:** When Phase 1 Plan 02 (errors) adds the first `*_test.go` importing testify, `go mod tidy` will convert the `// indirect` marker to direct. No further action needed here.

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Procedural only — final on-disk state matches plan acceptance criteria exactly. No scope creep, no API surface change.

## Issues Encountered

None — the deviation above is the only surprise, and it was an order-of-operations issue with Go's tidy behavior, not a true blocker.

## User Setup Required

None — no external service configuration, no environment variables, no credentials required. Library has zero runtime deps and no network calls in this plan.

## Next Phase Readiness

- **Module is buildable, vet-clean, gofmt-clean** — every Phase 1 plan that follows (`01-02-errors`, `01-03-date`, `01-04-types`, `01-05-validators`, `01-06-fuzz`) can declare `package openholidays` and import `github.com/stretchr/testify/...` in `*_test.go` without further setup.
- **`Version` const is the single source of truth** — Phase 2's HTTP client will reference `openholidays.Version` for the `User-Agent: go-openholidays/<version>` header, and Phase 5's `cmd/ohcli` will reference it for `--version` output.
- **MIT LICENSE detection** — pkg.go.dev and Go Report Card will both correctly classify the repo as MIT from `v0.1.0` onward.

## Self-Check: PASSED

Verified before final return:

- `[ -f go.mod ]` → FOUND (`module github.com/egeek-tech/go-openholidays`, `go 1.23`, testify require)
- `[ -f go.sum ]` → FOUND (testify v1.11.1 sums)
- `[ -f LICENSE ]` → FOUND (21 lines, canonical SPDX MIT text)
- `[ -f doc.go ]` → FOUND (package-level godoc)
- `[ -f version.go ]` → FOUND (`const Version = "0.1.0"`)
- `git log --oneline | grep fc12e6a` → FOUND (Task 1)
- `git log --oneline | grep 65e1df1` → FOUND (Task 2)
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `gofmt -l doc.go version.go` → empty output
- `go mod verify` → `all modules verified`
- `go test ./...` → `[no test files]` (expected; no production .go logic yet)

---
*Phase: 01-foundation*
*Plan: 01*
*Completed: 2026-05-27*
