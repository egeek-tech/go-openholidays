---
phase: 04-resilience
plan: 01
subsystem: testing-infrastructure
tags: [test-helper, fake-clock, deterministic-time, race-safety, D-94, D-95]
requires:
  - Phase 1 baseline (package openholidays at repo root)
  - testify v1.11.1 (already in go.mod, no new dep)
provides:
  - fakeClock test helper (Now/Advance/Sleep) for downstream Phase 4 tests
  - Sleep signature matching Client.sleepFunc (to be added Plan 02)
affects:
  - none (test-only file, excluded from go build by Go toolchain)
tech-stack:
  added: []
  patterns:
    - test-helper-as-same-package-file (mirrors transport_header_test.go::roundTripperFunc)
    - sync.Mutex-guarded fake clock per D-95
    - testify/assert with t.Parallel() etiquette per options_test.go
key-files:
  created:
    - clock_test.go
  modified: []
decisions:
  - "D-95 implemented verbatim: type fakeClock struct { mu sync.Mutex; now time.Time } with Now/Advance/Sleep methods; package openholidays (same-package) per D-94 so downstream tests can assign fakeClock.Sleep to Client.sleepFunc without a conversion."
  - "Omitted github.com/stretchr/testify/require import — no preconditions in the test (no NewClient, no constructor that can fail). Plan explicitly permits this: 'require may be unused — only add it if any precondition check needs it; otherwise omit to keep go vet clean'."
metrics:
  duration_minutes: 6
  tasks_completed: 1
  files_created: 1
  files_modified: 0
  completed_date: 2026-05-27
---

# Phase 04 Plan 01: Deterministic fakeClock Test Helper — Summary

Landed a same-package `_test.go` helper (`clock_test.go`) that exposes a race-safe, deterministic `fakeClock` (Now/Advance/Sleep) implementing D-95 verbatim, with a self-verifying `TestFakeClock_RaceFree` proving the three documented properties under `go test -race`.

## What Was Built

### Files Added

| File | Lines | Purpose |
|------|-------|---------|
| `clock_test.go` | 148 | `fakeClock` helper + `newFakeClock` constructor + `TestFakeClock_RaceFree` self-verifying test. `package openholidays` (same-package) so downstream Phase 4 tests inject `fc.Now` into `Client.nowFunc` and `fc.Sleep` into `Client.sleepFunc` (added Plan 02 per D-94) without conversion. |

### API Surface (test-only)

```go
type fakeClock struct { mu sync.Mutex; now time.Time }                  // D-95
func newFakeClock(t time.Time) *fakeClock
func (f *fakeClock) Now() time.Time
func (f *fakeClock) Advance(d time.Duration)
func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error   // matches Client.sleepFunc signature (D-94)
```

`Sleep` semantics (D-95 verbatim): returns `ctx.Err()` immediately if `ctx` is already cancelled (without advancing); otherwise advances the clock by `d` and returns `nil`. Synchronous — never sleeps on the wall clock.

### Test Cases Added

`TestFakeClock_RaceFree` (3 subtests, all `t.Parallel()`):

1. **`Now and Advance are race-free under 100 goroutines`** — launches 100 writer goroutines each calling `fc.Advance(time.Millisecond)` 100 times concurrently with 100 reader goroutines each calling `fc.Now()` 100 times. After `sync.WaitGroup.Wait()`, asserts `fc.Now() == start.Add(10*time.Second)` — proves all 10,000 Advance calls observed (no lost updates) and the read path is `-race`-clean.
2. **`Sleep returns ctx.Err() on cancelled ctx without advancing the clock`** — cancels ctx, calls `fc.Sleep(ctx, time.Hour)`, asserts `errors.Is(err, context.Canceled)` and `fc.Now() == start` (unchanged).
3. **`Sleep advances the clock by d and returns nil on live ctx`** — `fc.Sleep(context.Background(), 5*time.Second)`, asserts no error and `fc.Now() == start.Add(5s)`.

## Verification Results

| Check | Command | Result |
|-------|---------|--------|
| Race-detector smoke test | `go test -race -run TestFakeClock_RaceFree -count=1 -v ./...` | **PASS** (4 of 4: parent + 3 subtests; 1.024s wall) |
| Full regression suite | `go test -race -count=1 ./...` | **PASS** (1.822s wall) |
| CLIENT-10 invariant | `go test -race -run TestNoInitOrGlobalState -count=1 ./...` | **PASS** (test files exempt by design, allowlist unchanged) |
| Static analysis | `go vet ./...` | clean |
| Production build | `go build ./...` | clean (clock_test.go excluded from build by Go toolchain) |
| Format | `gofmt -l clock_test.go` | clean (no output) |
| No init() | `grep -n "func init" clock_test.go` | no match |

Verbatim race-test output:

```text
=== RUN   TestFakeClock_RaceFree
=== PAUSE TestFakeClock_RaceFree
=== CONT  TestFakeClock_RaceFree
=== RUN   TestFakeClock_RaceFree/Now_and_Advance_are_race-free_under_100_goroutines
=== PAUSE TestFakeClock_RaceFree/Now_and_Advance_are_race-free_under_100_goroutines
=== RUN   TestFakeClock_RaceFree/Sleep_returns_ctx.Err()_on_cancelled_ctx_without_advancing_the_clock
=== PAUSE TestFakeClock_RaceFree/Sleep_returns_ctx.Err()_on_cancelled_ctx_without_advancing_the_clock
=== RUN   TestFakeClock_RaceFree/Sleep_advances_the_clock_by_d_and_returns_nil_on_live_ctx
=== PAUSE TestFakeClock_RaceFree/Sleep_advances_the_clock_by_d_and_returns_nil_on_live_ctx
=== CONT  TestFakeClock_RaceFree/Now_and_Advance_are_race-free_under_100_goroutines
=== CONT  TestFakeClock_RaceFree/Sleep_returns_ctx.Err()_on_cancelled_ctx_without_advancing_the_clock
=== CONT  TestFakeClock_RaceFree/Sleep_advances_the_clock_by_d_and_returns_nil_on_live_ctx
--- PASS: TestFakeClock_RaceFree (0.00s)
    --- PASS: TestFakeClock_RaceFree/Sleep_returns_ctx.Err()_on_cancelled_ctx_without_advancing_the_clock (0.00s)
    --- PASS: TestFakeClock_RaceFree/Sleep_advances_the_clock_by_d_and_returns_nil_on_live_ctx (0.00s)
    --- PASS: TestFakeClock_RaceFree/Now_and_Advance_are_race-free_under_100_goroutines (0.01s)
PASS
ok  	github.com/egeek-tech/go-openholidays	1.024s
```

## Commits

| Task | Commit | Message |
|------|--------|---------|
| 1 | `131a9c1` | `test(04-01): add fakeClock deterministic time/sleep helper` |

## Success Criteria — Verification Trace

- [x] `clock_test.go` exists at repo root (`/data/git/private/holidays/clock_test.go` via worktree path).
- [x] `fakeClock` exposes exactly the three D-94/D-95 methods: `Now() time.Time`, `Advance(d time.Duration)`, `Sleep(ctx context.Context, d time.Duration) error`.
- [x] `Sleep` signature is `func(context.Context, time.Duration) error` — type-identical to the `Client.sleepFunc` field Plan 02 will add, so future assignment will compile without a conversion.
- [x] `go test -race -run TestFakeClock_RaceFree -count=1 ./...` exits 0.
- [x] Race detector exits 0 for the broader regression suite (`go test -race ./...`).
- [x] Plan-level threat T-04-01 (Tampering, fakeClock state under concurrent mutation) mitigated by `sync.Mutex` and exercised by the 100×100 race-free subtest.

## Deviations from Plan

**None — plan executed exactly as written.**

Two minor implementation choices were explicitly permitted by the plan's `<action>` block, so they are noted here for completeness rather than as deviations:

1. `github.com/stretchr/testify/require` was NOT imported. Plan said: *"require may be unused — only add it if any precondition check needs it; otherwise omit to keep `go vet` clean — verify by checking if the test body uses `require.X`."* The test body has no preconditions (no `NewClient` to nil-check), so `require` is omitted.
2. Sleep cancelled-ctx assertion uses `assert.ErrorIs(t, err, context.Canceled)` (not `assert.Equal`). Plan said *"assert `fc.Sleep(ctx, time.Hour)` returns `context.Canceled`"* — `ErrorIs` is the correct testify idiom for checking sentinel-error identity (per Phase 1's errors_test.go conventions) and is the strictly-stronger assertion.

## Authentication Gates

None encountered — this is a test-only file with no network IO, no secrets, no external services.

## Known Stubs

None. The fakeClock helper is fully functional; no placeholder values, no TODOs, no hardcoded empties that flow anywhere.

## Threat Flags

None. This plan ships test-only code with no production behavior change. The plan's `<threat_model>` STRIDE register already covers the only relevant threat (T-04-01, Tampering, mitigated by sync.Mutex + race test).

## Deferred Issues

None — every verification check passed on the first run. No auto-fix attempts were consumed.

## Self-Check: PASSED

- `clock_test.go` at worktree root: **FOUND** (148 lines, 4607 bytes).
- Commit `131a9c1`: **FOUND** in `git log` on `worktree-agent-ae9b0f14ea7affb94`.
- All assertions in the Verification Results table re-runnable from any clean checkout of this commit.
