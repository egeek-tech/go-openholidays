---
phase: 03-endpoints-helpers
reviewed: 2026-05-28T00:00:00Z
depth: standard
files_reviewed: 26
files_reviewed_list:
  - client_isinregion.go
  - client_isinregion_test.go
  - client_test.go
  - countries.go
  - countries_test.go
  - errors.go
  - holiday.go
  - holiday_test.go
  - internal_test.go
  - languages.go
  - languages_test.go
  - public_holidays.go
  - public_holidays_test.go
  - request.go
  - request_test.go
  - school_holidays.go
  - school_holidays_test.go
  - subdivisions.go
  - subdivisions_test.go
  - testdata/countries.json
  - testdata/languages.json
  - testdata/public_holidays_pl_2025.json
  - testdata/school_holidays_pl_2025.json
  - testdata/subdivisions_de.json
  - testdata/subdivisions_pl.json
  - update_fixtures_test.go
findings:
  critical: 0
  warning: 0
  info: 0
  total: 0
status: clean
---

# Phase 3: Code Review Report (re-review after fix pass)

**Reviewed:** 2026-05-28T00:00:00Z
**Depth:** standard
**Files Reviewed:** 26
**Status:** clean

## Summary

Re-review of phase 03 (`endpoints-helpers`) after the `--fix --all` pass landed the 11 commits in `101f289..b38386b`. Every prior finding (0 Critical + 6 Warning + 5 Info from the 2026-05-28 review) has been verified to hold. No new defects were introduced by the fix pass. No issues previously missed were surfaced under the standard-depth adversarial sweep.

Phase 03 scope as reviewed:
- Five endpoint methods (`Countries`, `Languages`, `Subdivisions`, `PublicHolidays`, `SchoolHolidays`) plus the `Client.IsInRegion` hierarchical helper
- Four pure `Holiday` value helpers (`NameFor`, `IsInRegion`, `Days`, `Range`)
- The `doJSONGet[T any]` generic HTTP-and-decode pipeline and the `validateHolidays` post-decode invariant guard
- The six committed `testdata/*.json` fixtures (real upstream OpenHolidays bytes; non-English strings admitted under CONVENTIONS.md Rule 1 testdata exception)
- The `update_fixtures_test.go` integration-tagged fixture-refresh harness

## Re-verification of prior fixes (all 11 hold)

| ID    | Fix                                                                   | Verified at                                                  | Status |
|-------|-----------------------------------------------------------------------|--------------------------------------------------------------|--------|
| WR-01 | 5-node cycle subtest exercising `len(parentIdx)+1` cap upper-bound    | `client_isinregion_test.go:275-333`                          | HOLDS  |
| WR-02 | Deterministic `drainCountingTransport` body counter replaces `runtime.NumGoroutine` | `countries_test.go:33-63, 237-299`                | HOLDS  |
| WR-03 | `Holiday.Days()` clamps to 0 when `EndDate.Before(StartDate)`         | `holiday.go:91-96`; test at `holiday_test.go:142-155`        | HOLDS  |
| WR-04 | IsInRegion 5xx subtest asserts `apiErr.Path` and `apiErr.Message`     | `client_isinregion_test.go:218-221`                          | HOLDS  |
| WR-05 | `retry exhausted (N attempts)` prefix carries the path                | `request.go:147` + `request.go:169`                          | HOLDS  |
| WR-06 | `Client.IsInRegion(ctx, h, "")` returns `(true, nil)` when `h.Nationwide` | `client_isinregion.go:78-80`; matching `holiday.go:61-63`; tests at `client_isinregion_test.go:93-105` and `holiday_test.go:61-71` | HOLDS  |
| IN-01 | `doJSONGet` godoc references `validateHolidays` in present tense      | `request.go:55-58`                                           | HOLDS  |
| IN-02 | Dead `isTwoASCIIUppers` / `isTwoASCIILowers` removed                  | `validate.go` (only `isTwoASCIILetters` remains, lines 110-121) | HOLDS  |
| IN-03 | Subdivisions ctx-cancel ceiling tightened to 150 ms                   | `subdivisions_test.go:265-273`                               | HOLDS  |
| IN-04 | `Subdivisions` documents trust-the-upstream model                     | `subdivisions.go:89-96`                                      | HOLDS  |
| IN-05 | `tmp.Close` deferred alongside `os.Remove` in fixture-refresh harness | `update_fixtures_test.go:256-267`                            | HOLDS  |

## Re-verification methodology

- **Cycle defense (WR-01):** Traced `buildParentIndex` on the 5-node nested-cycle fixture by hand. Result `parentIdx` = `{DE-B:DE-A, DE-C:DE-B, DE-D:DE-C, DE-E:DE-D, DE-A:DE-E}` (5 entries). With cap `len(parentIdx)+1 = 6` and the upward walk starting at `DE-A` against `h.Subdivisions=[{DE-X}]`, iterations 0..5 traverse the cycle once fully (5 hops) plus one extra hop, then return `(false, nil)`. A regression that tightened the cap to `len(parentIdx)-1 = 4` would terminate after 4 iterations without completing the cycle traversal — still returning `(false, nil)` on the 2-node test but distinguishable on the 5-node test. The cap upper-bound is meaningfully exercised.
- **Body-counter (WR-02):** `drainCountingTransport.RoundTrip` increments `openCount` after every successful `RoundTrip`, then wraps `resp.Body` with `countingBody` whose `Close()` decrements via `CompareAndSwap` (so double-close is idempotent). `assert.Equal(int32(0), countingRT.openCount.Load())` is the deterministic post-condition. No process-global sample, no `time.Sleep`. Replays correctly under `t.Parallel()`.
- **WR-03 contract change:** `Holiday.Days()` returning 0 when `EndDate < StartDate` is a deliberate pre-1.0 behavior change. The godoc explicitly states the clamp; callers branching on `h.Days() > N` get a defined non-negative value. Confirmed `validateHolidays` still rejects this shape on the endpoint path, so the clamp only fires on hand-built `Holiday` values (e.g. those constructed in unit tests).
- **WR-04 sister-coverage:** Asserting `apiErr.Path == "/Subdivisions"` (the inner-call path) and `apiErr.Message == "Internal Server Error"` in the IsInRegion 5xx subtest brings it into parity with the five sibling endpoint 5xx tests. Without these the IsInRegion test would only assert `StatusCode`, allowing a regression that lost title-fallback or rewrote the inner path to slip past.
- **WR-05 carry-prefix:** Both the `httpErr != nil` branch (`request.go:147`) and the retry-exhausted-`*APIError` branch (`request.go:169`) now use `"openholidays: GET %s: retry exhausted (%d attempts): %w"`. Path-routing via `strings.Contains(err.Error(), path)` is uniform regardless of whether the failure is transport-level or HTTP-status-level.
- **WR-06 contract change:** Both `Holiday.IsInRegion` (no-HTTP path) and `Client.IsInRegion` (HTTP-capable path) now have the `Nationwide` check FIRST, before the empty-code defensive guard. Symmetric — a nationwide holiday matches every region by definition. Matching tests assert the no-HTTP path on both.
- **IN-02 dead-code removal:** Confirmed `validate.go` only has the single `isTwoASCIILetters` helper. The previously-dead `isTwoASCIIUppers` / `isTwoASCIILowers` are gone. Pattern matrix: the W-01 Unicode-fold defense is intact (`validateCountry` and `validateLanguage` both call `isTwoASCIILetters` on the original bytes BEFORE `strings.ToUpper`/`ToLower`).
- **IN-03 tightened ceiling:** `assert.LessOrEqual(t, elapsed, 150*time.Millisecond, ...)` matches the documented budget of `20 ms cancel-delay + 100 ms target + 30 ms scheduler slack`. The CLIENT-09 ≤ 100 ms contract has a regression detector. Sibling tests (`countries_test.go` at 500 ms, `languages_test.go` at 1 s) intentionally absorb broader CI flake on their respective subtests.
- **IN-04 documentation:** The trust model is now spelled out in the `Subdivisions` godoc; downstream `Client.IsInRegion` callers can read the upstream-trust contract without git-archaeology.
- **IN-05 defer hygiene:** The fixture-refresh harness now `defer tmp.Close(); defer os.Remove(tmp.Name())` (single deferred closure, executes both). The doubled `Close` (once in the deferred path, once after `tmp.Write`) is harmless — `tmp.Close` on an already-closed file returns an error that the closure ignores. Remove on the original temp name is a no-op after `os.Rename` moves the entry. File-handle leak between `CreateTemp` and explicit close on an early require-abort is closed.

## Adversarial scan (no findings)

Every endpoint method's input-validation chain, query-builder, and post-decode pass was traced. Highlights of the negative findings (things checked and found clean):

- **Path-traversal / injection in URL builder:** `req.URL.RawQuery = q.Encode()` uses `url.Values.Encode` which RFC-3986 percent-encodes every byte. No user input flows into `req.URL.Path` directly — endpoints set the constant path literal (e.g. `/Countries`) at construction time. `SubdivisionCode` and `GroupCode` go through `q.Set(...)` so a value like `"PL-SL; DROP TABLE"` would be percent-encoded safely (validated by `request_test.go:136-157` which captures the encoded `RawQuery`).
- **Cycle defense correctness:** The 2-node and 5-node cycle tests both terminate within their 2-second deadlines. `buildParentIndex` itself does NOT bound recursion on `Subdivision.Children` (a cyclic Children pointer at the JSON level would loop), but the godoc on `buildParentIndex` explicitly documents this as out-of-scope; the parent-index-level cycle (the realistic threat) is bounded by `Client.IsInRegion`'s `len(parentIdx)+1` cap.
- **Date wraparound:** `validateDateRange` correctly anchors the 3-year window at `to.AddDate(-3, 0, 0)` (backward from `to`) rather than `from.AddDate(3, 0, 1)` (forward from `from`) to avoid the leap-day asymmetry documented in Pitfall 3. Unit-tested at validate_test.go.
- **`validateHolidays` ID echo:** Uses `%q` formatting, so a hostile upstream returning `Holiday.ID == "../etc/passwd"` produces a quoted-and-escaped diagnostic with no log-injection risk.
- **`json.Decoder.More()` (CR-01 fix from phase 02):** Verified present at `request.go:207`. The boundary-truncation gate correctly ignores RFC 8259 whitespace between top-level values, so a small body + trailing whitespace in a separate HTTP chunk no longer produces a false `ErrResponseTooLarge`. Regression-locked by `countries_test.go:301-335`.
- **`*APIError.Body` cap (4 KiB):** `buildAPIError` reads via `io.LimitReader(resp.Body, apiErrorBodyCap)` so a hostile multi-MB error envelope cannot inflate APIError.Body beyond 4 KiB. Test at `countries_test.go:186-204` and `request_test.go:229-246`.
- **`io.LimitReader` on retried-response drains:** The mid-loop drain at `request.go:125` uses `io.LimitReader(resp.Body, maxResponseBytes+1)` so an infinite-stream retry response cannot block the drain indefinitely (T-02-12).
- **`req.Clone(ctx)` per attempt (WR-07 from prior phase):** Present at `request.go:108`. Each retry attempt gets a fresh `*http.Request` so a future endpoint that adds a request body cannot accidentally send empty bodies on retries. Currently preventive (all five endpoints are nil-body GETs).
- **Nil-ctx defensive guard:** Tested at `request_test.go:85-96` and at every endpoint's *_test.go (`countries_test.go:222-235` shows the canonical pattern). Returns `"openholidays: nil context"` before any HTTP dispatch.
- **Gold Rule 3 compliance:** Every exported phase-03 production function has exactly one `TestXxx`. The `TestClient_SchoolHolidays_IsInRegion_FerieZimowe` exception is documented at `school_holidays_test.go:342-361` as the CL-14 narrow exception (SC2-COMBINED gap closure; explicitly scoped to this test only).
- **`%w` error wrapping:** Every sentinel-bearing return wraps via `fmt.Errorf("...: %w", Err...)`. `errors.Is(err, ErrXxx)` therefore holds through the endpoint method's caller-facing wrap on every error path.
- **Empty response handling:** `ErrEmptyResponse` is wrapped via `%w` from `request.go:181`. Empty `LanguageIsoCode` correctly omits the query parameter (`request_test.go:159-180`).
- **Concurrent use:** `TestClient_ConcurrentAccess` (50 parallel `Countries` calls) and `TestClient_Close` "concurrent close is race-safe (100 goroutines)" cover the CLIENT-07 / CLIENT-08 concurrency contracts.
- **Test-only deps are confined:** `github.com/stretchr/testify/assert`, `.../require` and (transitively) `go-cmp` only appear in `*_test.go` imports. The CLIENT-10 AST audit at `internal_test.go:124-239` is the mechanical guard.

## Carry-overs (out of phase 03 scope, noted for tracking)

Three phase-04 findings still touch `request.go` (which is in phase 03 scope). They are NOT phase 03 issues — they are carried in the open phase 04 review:

- **phase 04 WR-01:** `resp.Body` leak on the final-attempt CheckRedirect edge. When `c.http.Do(attemptReq)` returns both a non-nil `resp` AND a non-nil `httpErr` (the documented `net/http.Client.Do` behavior when `CheckRedirect` rejects a redirect), and `shouldRetry(resp, err)` returns false, the loop breaks and the `httpErr != nil` branch at `request.go:138-149` returns BEFORE the drain-and-close defer at `request.go:151` is registered. The non-nil `resp.Body` is never closed.
- **phase 04 WR-02:** "retry exhausted (N attempts)" is reported even when only attempt 0 ran. The condition at `request.go:146` (`if maxAttempts > 1`) triggers whenever retry is enabled, regardless of whether `shouldRetry` returned false on the first attempt. A non-retryable transport error on attempt 0 produces a misleading `"retry exhausted (5 attempts)"` message. (The mirror condition at `request.go:168` is correctly guarded by `&& shouldRetry(resp, nil)`.)
- **phase 04 WR-07:** `req.Clone(ctx)` per attempt — already implemented at `request.go:108` (verified above).

These three findings remain open in the phase 04 review; this re-review does not duplicate them as phase 03 findings.

---

_Reviewed: 2026-05-28T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
