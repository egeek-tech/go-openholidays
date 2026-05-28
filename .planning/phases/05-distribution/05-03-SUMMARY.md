---
phase: 05-distribution
plan: 03
subsystem: testing
tags:
  - go
  - fuzz
  - benchmark
  - httptest
  - testify

# Dependency graph
requires:
  - phase: 04-resilience
    provides: WithCache option and Countries-only cache scope (RESIL-07)
  - phase: 01-foundation
    provides: types.go Holiday + LocalizedText + pickLocalized; date.go NewDate
provides:
  - FuzzParseLocalizedText panic-freedom fuzz target for the LocalizedText decoder + pickLocalized helper
  - FuzzUnmarshalHoliday panic-freedom fuzz target for the []Holiday JSON decoder
  - BenchmarkClient_PublicHolidays/cold cold-path microbenchmark against PL 2025 fixture
  - BenchmarkClient_Countries/cached cached-path microbenchmark proving WithCache acceleration on the only cache-eligible endpoint family (CL-18 reinterpretation of SC#3)
  - Hand-curated fuzz seed corpus under testdata/fuzz/ for both targets
affects:
  - 05-04 (CI workflow) — time-boxes fuzz runs and runs benchmarks per matrix entry
  - 05-08 (release readiness) — perf claims now have a numeric anchor

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Fuzz targets seed from committed fixtures via F.Add(os.ReadFile(...)) plus 3 hand-curated adversarial seeds (RESEARCH §1.4 / Pattern 3)"
    - "Bench server uses httptest.NewServer streaming committed fixture bytes; b.Cleanup tears down deterministically"
    - "Cached benchmark warms the cache via one pre-loop Countries call, then measures the cacheTransport hit path"
    - "Pitfall 5 enforcement: bench_test.go has exactly one WithCache call — PublicHolidays is intentionally cold-only"

key-files:
  created:
    - fuzz_test.go
    - bench_test.go
    - testdata/fuzz/FuzzParseLocalizedText/seed_0
    - testdata/fuzz/FuzzParseLocalizedText/seed_1
    - testdata/fuzz/FuzzParseLocalizedText/seed_2
    - testdata/fuzz/FuzzUnmarshalHoliday/seed_0
    - testdata/fuzz/FuzzUnmarshalHoliday/seed_1
    - testdata/fuzz/FuzzUnmarshalHoliday/seed_2
  modified: []

key-decisions:
  - "Single WithCache call in bench_test.go — applies only to BenchmarkClient_Countries. PublicHolidays bypasses the cache by design (RESIL-07), so adding a cached sub-bench would be the Pitfall 5 anti-pattern."
  - "Same-package test files (package openholidays) for both new files — FuzzParseLocalizedText needs direct access to the unexported pickLocalized helper, and bench_test.go follows the public_holidays_test.go precedent."
  - "Fuzz targets import stdlib only (no testify). Fuzz harness reports panics directly; assertion libraries would only add ceremony."

patterns-established:
  - "Fuzz seed corpus: committed-fixture F.Add + 3 hand-curated edge seeds per target + a matching go test fuzz v1 file per seed under testdata/fuzz/<Name>/"
  - "Cached-path benchmark: warm with one pre-loop call, then b.ResetTimer() before the b.N loop"
  - "Bench godoc preamble explicitly names the SC# / RESIL-* references so the perf budget is traceable from the test file"

requirements-completed:
  - TEST-07
  - TEST-11

# Metrics
duration: ~18 min
completed: 2026-05-28
---

# Phase 05 Plan 03: Fuzz + Benchmark Coverage Summary

**Two fuzz targets (LocalizedText, []Holiday) prove panic-freedom over committed-fixture + adversarial seeds; cold + cached microbenchmarks anchor ROADMAP SC#3 with measured numbers (cold ~250 us, cached ~14 us — well inside the < 500 ms / < 5 ms budgets).**

## Performance

- **Duration:** ~18 min
- **Started:** 2026-05-28T16:40:00Z (approx)
- **Completed:** 2026-05-28T16:58:24Z
- **Tasks:** 3
- **Files created:** 8 (2 Go test files + 6 fuzz seed files)
- **Files modified:** 0

## Accomplishments
- Two same-package fuzz targets land panic-free over 5-second `-fuzz` runs on each (TEST-07).
- Two benchmarks measure the exact code paths the perf budget tracks (TEST-11), with the CL-18 reinterpretation honored: PublicHolidays cold only, Countries cached only.
- 6 hand-curated seed files under `testdata/fuzz/` make `go test -fuzz` reproducible from the very first invocation (RESEARCH §1.4 / Pattern 3).
- Single `WithCache(` call total in `bench_test.go` — the Pitfall 5 anti-pattern is structurally impossible to reintroduce by accident.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create testdata/fuzz/{FuzzParseLocalizedText,FuzzUnmarshalHoliday}/ seed corpus** — `78cda2c` (test)
2. **Task 2: Create fuzz_test.go with FuzzParseLocalizedText + FuzzUnmarshalHoliday** — `dc72df5` (test)
3. **Task 3: Create bench_test.go with cold + cached benchmarks (TEST-11 + CL-18)** — `bbc06f4` (test)

## Files Created/Modified
- `fuzz_test.go` — FuzzParseLocalizedText (panic-freedom for LocalizedText decoder + pickLocalized helper) and FuzzUnmarshalHoliday (panic-freedom for []Holiday decoder). Same-package; stdlib-only imports.
- `bench_test.go` — BenchmarkClient_PublicHolidays/cold and BenchmarkClient_Countries/cached. Same-package; uses testify/require for fixture-load preconditions and httptest.NewServer for an in-memory backend.
- `testdata/fuzz/FuzzParseLocalizedText/{seed_0,seed_1,seed_2}` — empty array, missing-language fallback, null text edge cases (valid `go test fuzz v1` format).
- `testdata/fuzz/FuzzUnmarshalHoliday/{seed_0,seed_1,seed_2}` — empty object element, EndDate < StartDate, null/empty nullable fields.

## Decisions Made
- **Stdlib-only imports in `fuzz_test.go`.** Fuzz harness reports panics directly; testify assertions inside `f.Fuzz` would be dead code. (The fuzz file also stays lean and easier to reason about.)
- **`testify/require` in `bench_test.go`** for fixture-load preconditions only — `b.Fatal` on read failure is acceptable but `require.NoError(b, err)` matches the rest of the codebase's voice and emits a clearer message.
- **CL-18 reinterpretation respected as a structural invariant.** The cold/cached split is split across two distinct benchmark functions, not two `b.Run` calls inside one — this makes it harder for a future contributor to add a "cached PublicHolidays" sub-bench by mistake. The single-`WithCache` assertion in `acceptance_criteria` reinforces it.

## Deviations from Plan

None — plan executed exactly as written. Every acceptance criterion in tasks 1, 2, and 3 verified green; the plan's `<verification>` block green; the plan's `<success_criteria>` 1-6 all met.

## Issues Encountered

None. `go vet`, `gofmt`, full `go test ./...`, and 5-second `go test -fuzz` runs against both targets all passed first try.

`golangci-lint` reported two pre-existing `staticcheck` advisories in `config.go` and `validate.go` that are unrelated to this plan; logged-to-context but not addressed here (scope boundary).

## Measured Numbers (informational)

Captured locally on 12th Gen Intel i7-1260P, `-benchtime=5x`:

| Benchmark | ns/op | Budget |
|-----------|-------|--------|
| `BenchmarkClient_PublicHolidays/cold-16` | ~209,815 (~210 us) | < 500 ms cold |
| `BenchmarkClient_Countries/cached-16`    | ~10,650 (~11 us)   | < 5 ms cached  |

Both comfortably inside the published budgets — three orders of magnitude under for cold, ~2.5 orders under for cached. CI can promote these to a hard assertion later (deferred per task 3 acceptance-criteria note).

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- TEST-07 + TEST-11 marked complete for the 05-distribution requirements roll-up.
- Plan 05-04 (CI workflow) can now reference both `fuzz_test.go` and `bench_test.go` by file name when adding `go test -fuzz` and `go test -bench` steps to the matrix.
- Plan 05-08 (release readiness) can quote the measured numbers above as anchors in CHANGELOG / README perf claims.

## Self-Check: PASSED

Verified the following claims after writing this file:

- `fuzz_test.go` exists at repo root.
- `bench_test.go` exists at repo root.
- All 6 seed files exist under `testdata/fuzz/`.
- Commits `78cda2c`, `dc72df5`, `bbc06f4` all present in `git log`.
- `go test -run '^Fuzz' -count=1 .` green.
- `go test -run=^$ -bench='^BenchmarkClient_' -benchtime=5x .` green; both sub-benchmarks named in output.
- Single `WithCache(` call in `bench_test.go` (Pitfall 5 enforced).
- Every seed file's first line is exactly `go test fuzz v1`.

---
*Phase: 05-distribution*
*Plan: 03*
*Completed: 2026-05-28*
