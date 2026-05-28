---
phase: 05-distribution
plan: 04
subsystem: testing
tags: [go, integration-test, live-api, build-tag, env-gate, openholidays]

# Dependency graph
requires:
  - phase: 03-endpoints-helpers
    provides: "Phase 3 golden fixtures (testdata/public_holidays_pl_2025.json, testdata/school_holidays_pl_2025.json) — define the 14 public holidays / 7 school-holiday periods asserted against the live API."
  - phase: 02-transport
    provides: "Client.PublicHolidays / Client.SchoolHolidays / NewClient / WithTimeout / Close — the public surface the integration test drives."
  - phase: 01-foundation
    provides: "Date type with NewDate constructor + PublicHolidaysRequest / SchoolHolidaysRequest structs."
provides:
  - "integration_test.go — double-gated (//go:build integration + OPENHOLIDAYS_LIVE=1) nightly drift-detection tests for the live OpenHolidays API."
  - "TestIntegration_PublicHolidays_PL_2025 — asserts PL 2025 has 14 public holidays."
  - "TestIntegration_SchoolHolidays_PL_2025 — asserts PL 2025 has 7 school-holiday periods."
affects: [phase-05-plan-06-nightly-ci, release-v0.1.0]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "double-gate pattern (compile-time build tag + runtime env-var) mirrored from update_fixtures_test.go for live-API exercises"
    - "context.WithTimeout(context.Background()) instead of testing.T.Context() to preserve Go 1.23 CI matrix leg (Pitfall 1)"

key-files:
  created:
    - "integration_test.go — live-API integration tests for TEST-08"
  modified: []

key-decisions:
  - "Use literal os.Getenv(\"OPENHOLIDAYS_LIVE\") in t.Skip gate instead of a named const, matching the plan's <action> body literally and the acceptance-criteria grep contract. update_fixtures_test.go uses a const (updateFixturesGuardEnv); the deliberate divergence here is anchored on the plan's <action> spec and is consistent with the grep-based acceptance checks."
  - "Two distinct top-level TestXxx functions (one per exported endpoint method under test) follow Gold Rule 3 — one TestXxx per exported production function (Client.PublicHolidays, Client.SchoolHolidays) — and not the single-TestUpdateFixtures pattern used in update_fixtures_test.go (which is a fixture-refresh utility, not endpoint coverage)."

patterns-established:
  - "Live-API drift-detection tests live in build-tagged files at repo root and assert tight golden-fixture counts (require.Len) so schema or holiday-list drift surfaces as a single-line CI failure."
  - "Per-test context cap (30s) layered under per-Client request cap (15s) — outer ctx wraps every t.Run; client-side timeout bounds each individual HTTP call."
  - "No t.Parallel() on live-API tests — serialize against the free public OpenHolidays upstream to avoid stress on the volunteer-run service."

requirements-completed: [TEST-08]

# Metrics
duration: 3min
completed: 2026-05-28
---

# Phase 05 Plan 04: Nightly Live-API Integration Tests (TEST-08) Summary

**Double-gated integration tests (build tag + env var) covering live PL 2025 public + school holidays against Phase 3 golden-fixture counts.**

## Performance

- **Duration:** ~3 min (149s)
- **Started:** 2026-05-28T16:55:11Z
- **Completed:** 2026-05-28T16:57:40Z
- **Tasks:** 1
- **Files modified:** 1 (created)

## Accomplishments

- Created `integration_test.go` with two `TestIntegration_*` functions covering `Client.PublicHolidays` and `Client.SchoolHolidays` against the live OpenHolidays API.
- Asserted Phase 3 golden truths: 14 public holidays and 7 school-holiday periods for PL 2025.
- Double-gated: `//go:build integration` excludes the file at compile time during default `go test ./...`; `OPENHOLIDAYS_LIVE=1` env-var check at the top of each test triggers a runtime `t.Skip` when unset. Both gates must be satisfied to issue any HTTP request to the live upstream.
- Pitfall 1 enforced — used `context.WithTimeout(context.Background(), 30*time.Second)` instead of the Go 1.24 testing context helper to keep the Go 1.23 CI matrix leg compiling.
- All verification gates pass: `go vet -tags=integration ./.` clean, `go build -tags=integration ./.` clean, `go test -tags=integration ./.` SKIPs cleanly without the env var, and default `go test ./.` excludes the file (`[no tests to run]`).

## Task Commits

1. **Task 1: Create integration_test.go with PL 2025 public + school assertions** — `2909a74` (test)

## Files Created/Modified

- `integration_test.go` — Build-tagged + env-gated live-API integration test file. Contains two `TestIntegration_*` functions, each driving the public client through one endpoint and asserting the Phase 3 golden-fixture count.

## Decisions Made

- **Literal `os.Getenv("OPENHOLIDAYS_LIVE")` over a named const.** The plan's `<action>` body specifies the literal form and the acceptance-criteria grep checks the literal token. `update_fixtures_test.go` uses a named const (`updateFixturesGuardEnv`) for its own gate, but the plan deliberately calls for the literal here. Keeping the env-var name visible at the t.Skip call site also improves grep-ability when an engineer scans for live-API gate points.
- **Two TestXxx functions, not one.** Gold Rule 3 mandates one TestXxx per exported production function. The integration file drives two exported methods (`Client.PublicHolidays`, `Client.SchoolHolidays`), so it ships two TestIntegration_* functions. (The `update_fixtures_test.go` analog has one because it is a fixture-refresh utility, not an endpoint-coverage test.)
- **No `t.Parallel()` on either test.** Live-API tests serialize against the public free OpenHolidays upstream to avoid any chance of overlapping requests stressing the volunteer-run service. The plan explicitly calls this out.
- **30s outer context cap + 15s per-call timeout.** Outer `context.WithTimeout(context.Background(), 30*time.Second)` bounds the entire test scope; `WithTimeout(15 * time.Second)` on the client bounds each individual HTTP call. This layering matches the per-test pattern in `update_fixtures_test.go` (per-call 30s, outer 5min — adjusted here to the narrower 30s outer because each integration test issues a single request, not a batch of six).

## Deviations from Plan

None - plan executed exactly as written.

Initial draft of the file declared a `integrationGuardEnv` named const (mirroring `update_fixtures_test.go`'s `updateFixturesGuardEnv`) and used it in both `os.Getenv` calls. The acceptance-criteria grep `grep -F 'os.Getenv("OPENHOLIDAYS_LIVE")'` would not have matched. The plan's `<action>` body specifies the literal form, so the file was adjusted before commit to use the literal string and to drop two doc-comment occurrences of the bare token `t.Context()` (which the `grep -F 't.Context()'` acceptance check requires to return nothing). Both adjustments are alignments to the plan's explicit grep contracts, not behavioral changes.

## Issues Encountered

None.

## User Setup Required

None — no external service configuration required. The nightly integration workflow (Plan 06) supplies both gates (`-tags=integration` and `OPENHOLIDAYS_LIVE=1`) via the gated environment.

## Verification Results

All `<acceptance_criteria>` and `<verification>` items from the plan pass:

- `[OK]` File `integration_test.go` exists at repo root.
- `[OK]` First non-blank line is exactly `//go:build integration`.
- `[OK]` `package openholidays` follows the build tag with the standard blank-line + file-scoped doc comment block in between (same shape as `update_fixtures_test.go`).
- `[OK]` `grep -E '^func TestIntegration_PublicHolidays_PL_2025\(t \*testing\.T\)' integration_test.go` matches 1.
- `[OK]` `grep -E '^func TestIntegration_SchoolHolidays_PL_2025\(t \*testing\.T\)' integration_test.go` matches 1.
- `[OK]` `grep -F 'os.Getenv("OPENHOLIDAYS_LIVE")' integration_test.go` matches 2.
- `[OK]` `grep -F 't.Skip('` matches 2.
- `[OK]` `grep -F 'context.WithTimeout(context.Background()'` matches 2.
- `[OK]` `grep -F 't.Context()'` returns 0 matches (Pitfall 1 enforced).
- `[OK]` `grep -F 'require.Len(t, hs, 14'` matches 1.
- `[OK]` `grep -F 'require.Len(t, hs, 7'` matches 1.
- `[OK]` `grep -F 'WithTimeout(15 * time.Second)'` matches 2.
- `[OK]` `go vet -tags=integration ./.` exits 0.
- `[OK]` `go build -tags=integration ./.` exits 0.
- `[OK]` Default `go test -run TestIntegration_ ./.` reports `[no tests to run]` (file excluded by build tag).
- `[OK]` `go test -tags=integration -v -run TestIntegration_ ./.` without `OPENHOLIDAYS_LIVE` outputs `--- SKIP: TestIntegration_PublicHolidays_PL_2025` and `--- SKIP: TestIntegration_SchoolHolidays_PL_2025` and exits 0.
- `[OK]` Full default `go test ./.` passes (no regressions).

## Next Phase Readiness

- Integration-test scaffolding ready for Plan 06's nightly `integration.yml` GitHub Actions workflow to wire up. That workflow supplies both gates (`-tags=integration` and `OPENHOLIDAYS_LIVE=1` env var) and runs the canonical command `go test -tags=integration -count=1 -timeout=5m ./...`.
- No blockers. The integration tests will exercise the live API only via the nightly job; default CI legs (Go 1.23 / 1.24 / stable) continue to exclude them via the missing build tag.

## Self-Check: PASSED

- `[FOUND]` `integration_test.go` at worktree root.
- `[FOUND]` `.planning/phases/05-distribution/05-04-SUMMARY.md` (this file).
- `[FOUND]` Commit `2909a74` in `git log`.

---
*Phase: 05-distribution*
*Plan: 04*
*Completed: 2026-05-28*
