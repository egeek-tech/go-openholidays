# Test-Suite Audit Report — 2026-05-30

**Branch:** `chore/test-audit` (base `master`) · **Scope:** all `*_test.go` files in the `openholidays` package and `cmd/ohcli` — Test, Example, and Fuzz functions (Benchmarks are not markable).

**Summary:** 32 test files · 130 test functions audited · 119 stamped `audit:ok` (11 not stamped) · 11 findings (1 high, 4 med, 6 low).

Every marked function was cross-checked against authoritative production code (per CLAUDE.md: code wins over docs) and exercised live (`go test`, several under `-race`); the false-pass headline check passed for all 119 marked funcs. The 11 withheld functions split into one false-pass-class defect (a vacuous canonicalization assertion in `TestCmdPublic`), several Gold Rule 3 structural deviations (duplicate `TestXxx` per prod function, top-level assertions in the outer body), and two near-tautological/vacuous-guard cases. **No withheld test asserts an outright wrong expected value** — see `## False-pass risks` for the important distinction.

## Findings

| Function | File:Line | Severity | Issue | Suggested fix |
|---|---|---|---|---|
| `TestCmdPublic` | cmd/ohcli/public_test.go:53 | **high** | The subtest "--lang reaches the wire as uppercase canonical form" (lines 215-232) is vacuous: it feeds an already-uppercase `--lang PL` and asserts the wire param `languageIsoCode=="PL"`. Since `strings.ToUpper("PL")=="PL"`, the assertion stays green whether `validateLanguage` canonicalizes correctly, does nothing, or passes the input through verbatim — so it does **not** exercise the uppercase canonicalization it claims to prove (validate.go `validateLanguage` → `strings.ToUpper` line 66; sent in public_holidays.go line 135). This is the language-code wire-value class. One unsound subtest withholds the whole function. The other 10 subtests are sound (URL path/params, exit codes, the 14-entry JSON count, the exact CSV header, D-07 empty-result, 500→exit1, CR-02 `ErrInvalidCountry`→exit2). | Change the input to a lowercase code, e.g. run `--lang pl` and keep the assertion `==\"PL\"`. With a lowercase input the assertion fails if the `ToUpper` canonicalization is dropped, so it actually proves the transform. Then re-audit and stamp. |
| `TestWithCache_NonPositiveTTLDisables` | options_test.go:479 | med | Gold Rule 3 ratio violation: a **second** `TestXxx` for the single prod function `WithCache` (`TestWithCache` already exists). Content is also fully redundant — both subtests (zero-ttl disables, negative-ttl disables) are byte-for-byte duplicates of subtests already in `TestWithCache`, asserting the same `c.cache==nil`. The doc comment self-describes the duplication as intentional, but it still breaks one-test-per-prod-func. Logic is correct (per D-80: `ttl<=0` leaves `cfg.cache` nil) — a convention/structure finding, not a false-pass. | Delete `TestWithCache_NonPositiveTTLDisables`; its two cases are already covered verbatim inside `TestWithCache`. If RESIL-07/D-80 needs a documentation anchor, rename the existing `TestWithCache` subtests to reference RESIL-07 rather than spawning a second top-level `TestXxx`. |
| `TestComputeBackoff_HonorsRetryAfter` | retry_test.go:314 | med | Gold Rule 3 ratio violation: a **second** `TestXxx` for the single prod function `computeBackoff` (`TestComputeBackoff` is the first). Logic is correct (no false-pass): both subtests verify Retry-After promotion (5s wins over ≤100ms jitter; capped to `maxWait=2s`) against the `min(retryAfter, cfg.maxWait)` branch in retry.go:297-299. | Fold the two Retry-After-promotion subtests into `TestComputeBackoff` as additional `t.Run` cases, deleting this separate `TestXxx`, so `computeBackoff` has exactly one test function. |
| `TestRetry_NeverRetriesCtxErrors` | retry_test.go:571 | med | Gold Rule 3 ratio violation + redundancy: a **second** `TestXxx` for `shouldRetry`. Its two subtests assert `shouldRetry(nil, context.Canceled)==false` and `shouldRetry(nil, context.DeadlineExceeded)==false`, already covered verbatim as cases in `TestShouldRetry` (retry_test.go:111-112). Correct values, but duplicates an existing test of the same prod function. | Delete this function; its two assertions are already in `TestShouldRetry`. If the D-75 intent must be highlighted, keep it as named cases inside `TestShouldRetry` rather than a second top-level `TestXxx`. |
| `TestCacheHitContextKey_OnHit` | transport_cache_test.go:296 | med | The "miss response carries no `CacheHitContextKey`" subtest (~line 299) is vacuous: the assertion `assert.Nil(t, resp.Request.Context().Value(CacheHitContextKey))` is gated behind `if resp.Request != nil`, but the miss path returns the next handler's response unchanged and `newTestCacheTransport`'s next handler never sets `Response.Request` — so `resp.Request` is **always** nil and the assertion body never executes. The negative contract (miss must NOT carry the key) is never actually verified; the subtest stays green even if the miss path erroneously injected the key. Sibling "hit" subtest is sound, but a finding in any case withholds the whole function. Verified empirically. | Make the miss assertion unconditional and observable: either (a) have the next handler set `resp.Request = req` (so `resp.Request` is non-nil on miss) and drop the `if resp.Request != nil` guard; or (b) capture the `*http.Request` the next handler received and assert `req.Context().Value(CacheHitContextKey) == nil`. Option (a) is the smaller change. |
| `TestCacheInterface_Conformance` | cache_test.go:40 | low | Smoke/near-tautological. The only runtime assertion is `assert.NotNil(t, c)` where `c` is a `Cache` var assigned from `nc`, already `require.NotNil`-checked two lines earlier; a non-nil concrete pointer assigned to an interface var is always non-nil, so the assertion can never fail. The actual conformance proof is the compile-time `var _ Cache = (*MemoryCache)(nil)` (line 34), which a test cannot exercise at runtime. No false-pass risk, but it asserts no observable behavior of the `Cache` contract. | Either drop the runtime test and rely on the compile-time `var _ Cache` assertion (CI fails to build if conformance breaks), or exercise the contract through the interface var: `c.Put(...)`, `c.Get(...)`, `c.Close()` and assert dispatch — proving the three methods are reachable through `Cache`, not just that a pointer is non-nil. |
| `TestClient_ConcurrentAccess` | client_test.go:258 | low | Gold Rule 3 structural deviation: the outer Test body carries a top-level `require.NoError` (fixture load, ~line 261) **and** runs the entire 50-goroutine parallel dispatch + `wg.Wait()` before the lone `t.Run`; the rule forbids top-level assertions in the outer body (a `require` is an assertion). Content is sound (decodes testdata/countries.json, asserts `results[0]==results[i]` + `NoError`; race detector + payload-equality both load-bearing). No false-pass. Withheld solely on the outer-body-assertion rule. | Move the fixture-read `require.NoError`, the goroutine dispatch, and `wg.Wait()` **inside** the single `t.Run` closure (sibling countries_test.go loads its fixture inside each `t.Run`), leaving zero assertions in the outer Test body. |
| `TestClient_ConcurrentRetry_RaceClean` | client_test.go:327 | low | Same Gold Rule 3 deviation: top-level `require.NoError` on the fixture read (~lines 327-328) plus the full N-goroutine dispatch + `wg.Wait()` sit in the outer Test body before the single `t.Run`. Content is correct and meaningful (503 is retryable; the first-N-fail server guarantees each goroutine hits one 503 then 200; race detector validates the `c.randMu` serialization of `c.rand.Int64N` — the CR-01 fix). No false-pass. Withheld only on the outer-body-assertion rule. | Relocate the fixture-load `require.NoError`, goroutine spawn loop, and `wg.Wait()` into the lone `t.Run` body so the outer `TestXxx` contains only setup (server, client) and no assertions. |
| `TestClient_PublicHolidays` | public_holidays_test.go:43 | low | Subtest "optional LanguageIsoCode is canonicalized to uppercase on the wire" (line 339) feeds an already-uppercase `LanguageIsoCode:"PL"` and asserts the wire value `=="PL"` (line 345). Because `ToUpper("PL")=="PL"`, the assertion stays green even if the code did nothing or used `ToLower`-on-an-upper-input — it does not exercise the canonicalization its name/comment claim. Not a wrong-value false-pass (`"PL"` is correct), but it under-pins its own contract. All 12 other subtests are correct (14-holiday count + Wigilia 2025-12-24 vs fixture; the `ErrInvalid*` / `ErrDateRangeTooLarge` / `ErrMalformedResponse` / ctx-cancel paths). | In the line-339 subtest change the input to lowercase (`LanguageIsoCode:"pl"`) while keeping the wire assertion `=="PL"`. A passthrough or `ToLower` regression would then send `"pl"` and fail, truly pinning `strings.ToUpper`. Once amended, the whole function earns the mark. |
| `TestRetry_DeterministicClock` | retry_test.go:595 | low | Criterion-(d) tautological assertion: `assert.GreaterOrEqual(elapsed, time.Duration(0))` (line 629) can never fail — `fakeClock.Sleep` only ever advances forward (clock_test.go:64-66), so `elapsed` summed from non-negative jitter is structurally ≥0. The comment even labels it "a sanity check". The meaningful primary assertions (`hits==4`; `elapsed<=700ms` full-jitter ceiling vs the 100/200/400ms schedule) are correct. | Remove the ≥0 assertion (or replace with a meaningful lower bound, e.g. `elapsed > 0` to prove sleeps actually ran). Then the test earns the mark. |
| `TestRetry_NotARoundTripper` | retry_test.go:638 | low | Gold Rule 3(b) violation: a top-level `require.NoError(t, err)` sits in the outer Test body (line 642, after `findRepoRoot()`) outside any `t.Run`. The structural-audit logic (assert no `transport_retry.go`; assert no `type retryTransport` in any production `.go`) is otherwise sound and meaningful. | Move the `findRepoRoot()` call and its `require` inside each subtest, or compute `repoRoot` once with `t.Fatal`-free handling and perform the require check within the subtests, so the outer body holds no assertions. |

## False-pass risks

A **false-pass** in the strict sense — a test asserting a *wrong* expected value that nonetheless stays green (the bug class that let the `validateLanguage` lowercase issue ship) — was **not found** in any audited test function. Every expected value in every marked and every withheld test was cross-checked against authoritative production code and matches the correct behavior.

There is, however, one related defect worth surfacing: the **vacuous-canonicalization** class, where the asserted value is *correct* but the test cannot fail because the input is degenerate:

- **`TestCmdPublic` (high)** and **`TestClient_PublicHolidays` (low)** both feed an already-uppercase `"PL"` into the `languageIsoCode` wire-canonicalization subtest and assert `=="PL"`. The asserted value is the correct wire value, so this is *not* a wrong-value false-pass; but because `ToUpper("PL")=="PL"`, the subtests stay green even if `validateLanguage`'s `strings.ToUpper` were deleted or replaced with a passthrough. They therefore fail to guard the very canonicalization that previously regressed. The fix in both cases is to feed a **lowercase** input (`"pl"`) and keep the `=="PL"` assertion, making the transform observable. `TestCmdPublic` is rated high because it is the CLI-level wire guard and its claim ("reaches the wire as uppercase canonical form") is directly contradicted by what it actually exercises.

All other endpoint/wire tests that assert `languageIsoCode=="PL"`/`"EN"` (in countries_test.go, languages_test.go, subdivisions_test.go, integration_test.go, school_holidays_test.go) were verified to encode the **correct** uppercase wire value and were stamped; they do not repeat the shipped lowercase bug.

## Gold Rule 3 compliance

**Ratio (one `TestXxx` per exported production function).** Four deviations surfaced, all duplicate `TestXxx` for a single prod function:

- `TestWithCache_NonPositiveTTLDisables` (med) — second test for `WithCache`; fully redundant with `TestWithCache`.
- `TestComputeBackoff_HonorsRetryAfter` (med) — second test for `computeBackoff`.
- `TestRetry_NeverRetriesCtxErrors` (med) — second test for `shouldRetry`; redundant with `TestShouldRetry`.

These three are the only true ratio violations. Several other apparent surpluses were verified **not** to be violations and were stamped:

- **Sanctioned by a Key Decision:** `school_holidays_test.go` carries two `TestXxx` for `Client.SchoolHolidays` (`TestClient_SchoolHolidays` + `TestClient_SchoolHolidays_IsInRegion_FerieZimowe`) — explicitly permitted by CL-14 in PROJECT.md (verified present). `validate_test.go`'s `TestValidators_NoSensitiveData` is the documented cross-cutting ERR-04/CL-17 no-leak exception.
- **Unexported prod targets:** Rule 3's 1:1 binds *exported* functions. Files testing unexported methods (`transport_hook_test.go` ×4, `transport_cache_test.go` ×4-5, `transport_logging_test.go`, `request_test.go`, `retry_test.go` helpers, `cmd/ohcli/format_test.go` — all `package main` lowercase renderers) document deliberate splits of one method's many contracts across several named `TestXxx`; not penalized.
- **Coverage extensions / meta-tests:** `cache_test.go`'s sweeper/TTL tests, `internal_test.go`'s `TestNoInitOrGlobalState`, the `Test*_Concurrent*` and `TestRetry_*` behavioral tests cover unexported behavior or cross-cutting invariants with no exported counterpart — extensions, not duplicates.
- **Lifecycle grouping:** `cache_test.go`'s `TestMemoryCache_GetPut` groups `Get`+`Put` (documented header exception); neither has a duplicate test.

**`t.Run` / table-driven / outer-body assertions.** Three deviations, all the same shape — a top-level assertion in the outer Test body outside any `t.Run`:

- `TestClient_ConcurrentAccess` (low) and `TestClient_ConcurrentRetry_RaceClean` (low) — fixture-load `require` + the full goroutine dispatch + `wg.Wait()` run in the outer body before the lone `t.Run`.
- `TestRetry_NotARoundTripper` (low) — top-level `require.NoError` after `findRepoRoot()`.

Non-table layouts (per-subtest distinct `httptest` setup rather than a shared table) were observed across the endpoint and CLI tests and judged acceptable — those cases do not share uniform setup that table-driving would simplify, consistent with sibling files. `testify` discipline (`require` for preconditions, `assert` for verifications) was correct in every audited file. Example* and Fuzz* funcs are structurally exempt from the testify/`t.Run` sub-clauses (no `*testing.T`); their 1:1-per-symbol mapping was satisfied.

## Per-file results

| File | Funcs | Marked | Findings |
|---|---|---|---|
| bench_test.go | 0 | 0 | 0 (benchmarks only — not markable) |
| cache_test.go | 6 | 5 | 1 (low) |
| client_isinregion_test.go | 3 | 3 | 0 |
| client_test.go | 14 | 12 | 2 (low) |
| clock_test.go | 1 | 1 | 0 |
| cmd/ohcli/countries_test.go | 1 | 1 | 0 |
| cmd/ohcli/format_test.go | 4 | 4 | 0 |
| cmd/ohcli/main_test.go | 1 | 1 | 0 |
| cmd/ohcli/public_test.go | 1 | 0 | 1 (high) |
| cmd/ohcli/school_test.go | 1 | 1 | 0 |
| countries_test.go | 1 | 1 | 0 |
| date_test.go | 11 | 11 | 0 |
| errors_test.go | 5 | 5 | 0 |
| example_test.go | 17 | 17 | 0 |
| fuzz_test.go | 2 | 2 | 0 |
| holiday_test.go | 4 | 4 | 0 |
| integration_test.go | 2 | 2 | 0 |
| internal_test.go | 1 | 1 | 0 |
| languages_test.go | 1 | 1 | 0 |
| options_test.go | 12 | 11 | 1 (med) |
| public_holidays_test.go | 2 | 1 | 1 (low) |
| request_test.go | 1 | 1 | 0 |
| retry_test.go | 11 | 7 | 4 (2 med, 2 low) |
| school_holidays_test.go | 2 | 2 | 0 |
| subdivisions_test.go | 1 | 1 | 0 |
| transport_cache_test.go | 5 | 4 | 1 (med) |
| transport_header_test.go | 1 | 1 | 0 |
| transport_hook_test.go | 4 | 4 | 0 |
| transport_logging_test.go | 1 | 1 | 0 |
| types_test.go | 9 | 9 | 0 |
| update_fixtures_test.go | 1 | 1 | 0 |
| validate_test.go | 4 | 4 | 0 |
| **Total** | **130** | **119** | **11 (1 high, 4 med, 6 low)** |

## Notes

Special cases and intentional structures recorded during the audit:

- **Benchmarks (bench_test.go).** Contains only `Benchmark*` funcs (`BenchmarkClient_PublicHolidays` cold path, `BenchmarkClient_Countries` cached path). Per the Benchmark rule none are markable; `funcsAudited=0`. Both were verified compiling and passing (`-benchtime=2x`). Both designs were checked against `isCacheablePath` (transport_cache.go:93-99): `/PublicHolidays` is genuinely non-cacheable so the cold-path claim holds, and `/Countries` is cacheable so the cached-path warm-up correctly hits `cacheTransport`. Benchmarks are exempt from the one-Test-per-prod-function rule.

- **Build-tagged / double-gated files.** `integration_test.go` (`//go:build integration` + `OPENHOLIDAYS_LIVE=1`) hits live upstream for drift detection; its doc-stated golden counts (14 public, 7 school) match the fixtures. `update_fixtures_test.go` (`//go:build integration` + `OPENHOLIDAYS_LIVE=1`) is the fixture-regeneration tool — a genuine byte-equality drift test in default mode; verified its `json.Indent(body,"","    ")+'\n'` normalization matches the committed on-disk fixture format, and its uppercase `languageIsoCode=PL/EN` captures match production's `strings.ToUpper` wire form (the inline comment documents a prior EN→PL sync bug). Both are tools/live-drift tests, not 1:1 unit tests of an exported function — the ratio rule is N/A.

- **Fuzz targets (fuzz_test.go, date_test.go).** `FuzzParseLocalizedText`, `FuzzUnmarshalHoliday`, and `FuzzDateUnmarshal` assert only panic-freedom (return values intentionally discarded) — the canonical fuzz idiom, so no expected-value/false-pass risk applies. One low non-blocking divergence noted: `FuzzUnmarshalHoliday`'s in-file `f.Add` seeds (`{}`, bare object) differ from the slice-wrapped on-disk corpus (`[{}]`, `[{...}]`); for a panic-freedom fuzzer this broadens coverage rather than causing a false-pass — not a defect.

- **Examples (example_test.go).** 17 `Example*` funcs in the external `openholidays_test` package; 8 are deterministic with `// Output:` blocks (all verified to encode the correct value, including genuine `pickLocalized` fallback paths), 9 are intentionally compile-only because they issue live HTTP — this is the file's own documented convention (Pitfall 7), not a smoke-test gap.

- **Meta-invariant test (internal_test.go).** `TestNoInitOrGlobalState` (CLIENT-10) AST-walks production files asserting no `func init()` and only allowlisted package vars. Verified by mutation (planted `init()` + stray var → test failed with precise messages, then passed after removal). The 10-entry allowlist matches real production vars exactly. No prod counterpart, so the 1:1 rule is N/A.

- **Intentional multi-`TestXxx`-per-method groupings (stamped, not findings).** `transport_hook_test.go` (4 tests for one unexported `RoundTrip`), `transport_cache_test.go` (per-decision-ID split), and `cmd/ohcli/format_test.go` (4 tests over `package main` lowercase renderers) all document deliberate per-concern splits in their file headers and were stamped, since Rule 3's 1:1 binds exported functions.

- **Coverage gaps noted but not mark-blocking.** `transport_hook_test.go` never exercises the `resp.Request` (D-88 cache-hit) branch — the asserted value is correct (weak, not wrong), so no mark was withheld; a follow-up to set a distinct `resp.Request` and assert it is suggested. `school_holidays_test.go`'s malformed-JSON subtest is negative-space only (asserts no sentinel matches) — meaningful as the contract under test, but a positive decode-error-shape assertion would strengthen it.

## Resolution — same branch, follow-up commit (2026-05-30)

All 11 findings are fixed in this branch. The suite now carries **127 `audit:ok 2026-05-30` marks** (the 119 originally-sound + 8 newly-sound), with **3 redundant tests deleted** after confirming their cases live verbatim in the canonical test. Coverage was preserved on every deletion. Gates after the fix: `go test -race ./...` green · `golangci-lint run ./...` 0 issues · `gofmt` clean · `go vet -tags=integration` / `-tags=fuzz` clean.

| Finding | Resolution |
|---|---|
| `TestCmdPublic` (high) | Subtest input `--lang PL` → `pl` (keeps the `=="PL"` wire assertion), so it now fails if `validateLanguage`'s `ToUpper` were dropped. **Stamped.** |
| `TestClient_PublicHolidays` (low) | Same canonicalization fix: `LanguageIsoCode:"PL"` → `"pl"`. **Stamped.** |
| `TestWithCache_NonPositiveTTLDisables` (med) | **Deleted** — its zero/negative-ttl cases already lived in `TestWithCache`; the RESIL-07/D-80 anchor was carried onto those subtests. |
| `TestComputeBackoff_HonorsRetryAfter` (med) | **Folded** into `TestComputeBackoff` as `t.Run` cases (its mark re-cycled per Rule 5); standalone func deleted. |
| `TestRetry_NeverRetriesCtxErrors` (med) | **Deleted** — the two ctx-error cases already exist in `TestShouldRetry` (`context.Canceled`/`DeadlineExceeded` → `want:false`). |
| `TestCacheHitContextKey_OnHit` (med) | Miss subtest now captures the forwarded request and asserts unconditionally that it carries no `CacheHitContextKey` (was gated behind an always-nil `resp.Request`). **Stamped.** |
| `TestCacheInterface_Conformance` (low) | Now exercises `Put`/`Get`/`Close` through the `Cache` interface var (round-trip + `Close`), not just `NotNil`. **Stamped.** |
| `TestClient_ConcurrentAccess` (low) | Outer-body `require` + goroutine dispatch + `wg.Wait()` moved inside the `t.Run`. **Stamped.** |
| `TestClient_ConcurrentRetry_RaceClean` (low) | Same outer-body relocation. **Stamped.** |
| `TestRetry_DeterministicClock` (low) | Tautological `elapsed >= 0` strengthened to `elapsed > 0` (proves the inter-attempt sleeps advanced the fake clock). **Stamped.** |
| `TestRetry_NotARoundTripper` (low) | `findRepoRoot()` + its `require` moved into each subtest (no outer-body assertion). **Stamped.** |

Net: 130 audited functions → **127** (3 redundant deleted), all 127 stamped; **0 open findings**.
