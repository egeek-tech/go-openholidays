---
phase: 03-endpoints-helpers
plan: 10
subsystem: helpers
tags: [gap-closure, WR-01, holiday, range, iter.Seq, utc-midnight, testify, gold-rule-3]

requires:
  - phase: 03-endpoints-helpers
    plan: 06
    provides: "Holiday.Range() iter.Seq[Date] iterator with per-step NewDate rebuild — the FIRST iteration was the WR-01 drift point closed here"
  - phase: 01-foundation
    provides: "NewDate(year, month, day) UTC-midnight constructor in date.go (delegated to by the fix)"
provides:
  - "Holiday.Range() with consistent first-iteration UTC-midnight normalization — the godoc contract ('every yielded Date is rebuilt via NewDate') now matches the implementation on every iteration including the first"
  - "TestHoliday_Range non-UTC regression subtest gating the WR-01 fix against future drift"
affects: ["downstream consumers of Holiday.Range (none in v0.x — closure is defense-in-depth, endpoint-returned Holidays were already UTC-midnight via validateHolidays)"]

tech-stack:
  added: []
  patterns:
    - "First-iteration normalization symmetric with subsequent iterations — d := NewDate(h.StartDate.Year(), h.StartDate.Month(), h.StartDate.Day()) at the loop entry, identical shape to next := d.AddDate(0,0,1); d = NewDate(next.Year(), next.Month(), next.Day()) at the loop tail"
    - "TDD regression coverage via hand-built non-UTC Holiday — proves the contract without depending on endpoint-layer validateHolidays canonicalization"

key-files:
  created: []
  modified:
    - "holiday.go — Range() first-iteration loop initializer now goes through NewDate (one-line change inside the existing closure)"
    - "holiday_test.go — TestHoliday_Range gains a 6th subtest 'non-UTC StartDate yields UTC-midnight first Date (WR-01 regression)' as the last subtest in the function"

key-decisions:
  - "Defense-in-depth fix only — production callers were unaffected because validateHolidays canonicalizes endpoint-returned Start/End to UTC-midnight; the godoc IS the contract and the implementation now satisfies it on every iteration"
  - "TDD discipline preserved despite plan ordering Task 1 (fix) before Task 2 (test): RED verified locally by temporarily reverting Task 1 in-memory and confirming the new subtest fails with 'first yielded Date must be UTC-midnight regardless of StartDate location, got CET' before restoring the fix and committing the subtest"
  - "Gold Rule 3 invariant preserved: TestHoliday_Range remains the ONLY TestXxx for Range — the new case is a t.Run subtest, not a new TestXxx; grep -cE 'func TestHoliday_Range\\(' = 1"

patterns-established:
  - "WR-class warnings from VERIFICATION reports closed by a single-purpose plan: one fix + one regression test that would have caught the drift"
  - "Hand-built Holiday with time.FixedZone(...) StartDate as the canonical non-UTC fixture for Holiday.Range invariant tests (cheap, deterministic, no testdata dependency)"

requirements-completed: [HELP-04]

duration: 6min
completed: 2026-05-27
---

# Phase 03 Plan 10: WR-01 Holiday.Range First-Yield UTC Normalization Gap Closure

**Single-line fix to `Holiday.Range()` so the FIRST yielded `Date` is rebuilt through `NewDate` like every subsequent yield, matching the godoc contract — plus a non-UTC regression subtest that would have caught the drift.**

## Performance

- **Duration:** ~6 min (including TDD RED-cycle verification)
- **Started:** 2026-05-27T20:05:23Z (plan execution begin)
- **Completed:** 2026-05-27T20:11:11Z
- **Tasks:** 2
- **Files created:** 0
- **Files modified:** 2 (`holiday.go`, `holiday_test.go`)
- **Commits:** 2 (`2584162` fix, `848f6c3` test)

## Accomplishments

### Task 1 — `holiday.go` first-iteration normalization (commit `2584162`)

The one-line diff inside `func (h Holiday) Range() iter.Seq[Date]`:

```
-		d := h.StartDate
+		d := NewDate(h.StartDate.Year(), h.StartDate.Month(), h.StartDate.Day())
```

Before the fix, the loop entered with `d` carrying the receiver's `StartDate.Time` *verbatim* — including any non-UTC location a hand-built Holiday might have introduced via a struct literal. The first `yield(d)` therefore returned that non-UTC Date, contradicting the godoc paragraph six lines above:

> "Every yielded Date is rebuilt via NewDate(year, month, day), so each yielded value is at UTC midnight regardless of the receiver's internal time.Time location."

After the fix, the first yield is rebuilt through `NewDate` identically to the loop-tail step (`next := d.AddDate(0, 0, 1); d = NewDate(next.Year(), next.Month(), next.Day())`). The implementation now satisfies the contract on every iteration.

Endpoint-returned Holidays were unaffected — `validateHolidays` in `request.go` already canonicalizes `Start/EndDate` to UTC midnight before any consumer can call `Range()`. The defect was a defense-in-depth gap for hand-built `Holiday{}` values, not a production-callers bug.

The godoc comment itself was already correct; only the implementation drifted. The fix preserves the comment verbatim.

### Task 2 — `holiday_test.go` non-UTC regression subtest (commit `848f6c3`)

A new subtest inside the existing `TestHoliday_Range` function (Gold Rule 3 — one `TestXxx` per exported production function preserved; the new case is a 6th `t.Run` subtest, not a new `TestXxx`):

```go
t.Run("non-UTC StartDate yields UTC-midnight first Date (WR-01 regression)", func(t *testing.T) {
    t.Parallel()
    cet := time.FixedZone("CET", 3600)
    h := Holiday{
        StartDate: Date{Time: time.Date(2025, time.January, 18, 0, 0, 0, 0, cet)},
        EndDate:   NewDate(2025, time.January, 18),
    }
    var dates []Date
    for d := range h.Range() {
        dates = append(dates, d)
    }
    require.Len(t, dates, 1, "single-day span must yield exactly one Date")
    first := dates[0]
    assert.Equal(t, time.UTC, first.Location(),
        "first yielded Date must be UTC-midnight regardless of StartDate location, got %s", first.Location())
    assert.Equal(t, 0, first.Hour())
    assert.Equal(t, 0, first.Minute())
    assert.Equal(t, 0, first.Second())
    assert.Equal(t, 0, first.Nanosecond())
    assert.Equal(t, 2025, first.Year())
    assert.Equal(t, time.January, first.Month())
    assert.Equal(t, 18, first.Day())
})
```

The fixture is a single-day span (`StartDate == EndDate` calendar day) so the iterator yields exactly one `Date` — and that one `Date` is the FIRST iteration, which is precisely what WR-01 was about. Assertions cover:

1. **Location invariant** — `first.Location()` must equal `time.UTC` (the WR-01 trigger).
2. **Time-of-day invariant** — `Hour/Minute/Second/Nanosecond` must all be 0.
3. **Calendar fidelity** — the year/month/day must match the configured input (no off-by-one from the rebuild).
4. **Iteration count** — `require.Len(t, dates, 1)` as the precondition before unpacking.

Testify imports (`assert`, `require`) and `time` were already in `holiday_test.go` — no new imports needed.

### TDD RED Verification (local-only — not committed)

Plan ordering put the fix in Task 1 and the test in Task 2, so committing in plan order shows GREEN immediately. To prove the new subtest is genuine regression coverage rather than a tautology, the fix was temporarily reverted in-memory after Task 2's edit (with the new subtest in place), then `go test -race -count=1 -run 'TestHoliday_Range/non-UTC' ./...` was run. The subtest FAILED with:

```
Messages:   first yielded Date must be UTC-midnight regardless of StartDate location, got CET
--- FAIL: TestHoliday_Range/non-UTC_StartDate_yields_UTC-midnight_first_Date_(WR-01_regression)
```

The fix was then re-applied (back to the committed state) and the suite verified clean. This RED-cycle is documented here rather than committed because committing a known-broken `holiday.go` in the middle of the plan would have left the wave in a bisect-hostile state.

## Verification Gates (all PASS)

| Gate | Command | Result |
|------|---------|--------|
| Task 1 — fix line present | `grep -cE 'd := NewDate\(h\.StartDate\.Year\(\), h\.StartDate\.Month\(\), h\.StartDate\.Day\(\)\)' holiday.go` | 1 |
| Task 1 — old line gone | `grep -cE '^\s*d := h\.StartDate\s*$' holiday.go` | 0 |
| Task 2 — new subtest name | `grep -cE 'non-UTC StartDate yields UTC-midnight first Date' holiday_test.go` | 1 |
| Task 2 — CET fixture | `grep -cF 'time.FixedZone("CET", 3600)' holiday_test.go` | 1 |
| Task 2 — Gold Rule 3 invariant | `grep -cE 'func TestHoliday_Range\(' holiday_test.go` | 1 |
| Suite — TestHoliday_Range under race | `go test -race -count=1 -run TestHoliday_Range ./...` | `ok ... 1.013s` |
| Suite — full package under race | `go test -race -count=1 ./...` | `ok ... 1.915s` |
| Static — vet clean | `go vet ./...` | exit 0 |
| Static — build clean | `go build ./...` | exit 0 |
| Diff scope | `git diff --stat 44a379ac6dc768b3135e070eeacec5cc2b439a8f HEAD` | `holiday.go \| 2 +-`, `holiday_test.go \| 29 +++++++++++++++++++++++++++++` (no other files) |

All 6 TestHoliday_Range subtests pass under `-race -count=1`:

- `14-day_ferie_zimowe_yields_14_Dates_inclusive` (PASS, pre-existing)
- `single-day_yields_exactly_one_Date` (PASS, pre-existing)
- `EndDate_before_StartDate_yields_zero` (PASS, pre-existing)
- `early_break_stops_iteration` (PASS, pre-existing)
- `every_yielded_Date_is_UTC_midnight` (PASS, pre-existing)
- `non-UTC_StartDate_yields_UTC-midnight_first_Date_(WR-01_regression)` (PASS, NEW)

## Files

**Modified (2):**

- `holiday.go` (132 lines, +1/−1) — line 119 first-iteration loop initializer rebuilt through `NewDate`. Godoc comment block at lines 87–113 unchanged.
- `holiday_test.go` (242 lines, +29/−0) — new `t.Run` subtest appended at the end of `TestHoliday_Range`. No other test function touched.

**Commits (2):**

- `2584162 fix(03-10): normalize Holiday.Range first-iteration yield through NewDate (WR-01)` — 1 file changed, 1 insertion, 1 deletion
- `848f6c3 test(03-10): add TestHoliday_Range non-UTC StartDate regression subtest (WR-01)` — 1 file changed, 29 insertions

## Deviations from Plan

None — plan executed exactly as written. The plan acknowledged that Task 1 lands the fix before Task 2 adds the regression test (and that the existing 5 subtests pass either way), which is the order I followed. TDD RED-cycle verification was performed locally to confirm the new subtest is genuine regression coverage; the result is documented above but not committed (committing the reverted intermediate state would have polluted bisect history).

## Gap Closure

**WR-01-RANGE-FIRST-YIELD** (severity: warning, source: `.planning/phases/03-endpoints-helpers/03-VERIFICATION.md` frontmatter; `03-REVIEW.md` WR-01) — **CLOSED**.

Expected-after-fix from the verification frontmatter:

> "Add a TestHoliday_Range subtest with a non-UTC-midnight StartDate that asserts the first yielded Date is UTC-midnight."

Delivered: subtest `non-UTC StartDate yields UTC-midnight first Date (WR-01 regression)` constructs a non-UTC `StartDate` via `time.FixedZone("CET", 3600)` and asserts `first.Location() == time.UTC` (plus zero Hour/Minute/Second/Nanosecond and calendar-field fidelity).

## Self-Check: PASSED

- `holiday.go` present at modified path: FOUND
- `holiday_test.go` present at modified path: FOUND
- Commit `2584162` exists: FOUND (`git log --oneline --all | grep 2584162`)
- Commit `848f6c3` exists: FOUND (`git log --oneline --all | grep 848f6c3`)
- `.planning/phases/03-endpoints-helpers/03-10-SUMMARY.md` created: FOUND (this file)
- All success criteria from PLAN.md `<success_criteria>` block: PASS

## Known Stubs

None. No placeholder values, empty data flows, or TODO/FIXME markers introduced.

## Threat Flags

None. The change is a normalization-correctness fix on a pure-value helper method — no new network surface, auth path, file access, or schema change at any trust boundary.
