# Function Audit Report — 2026-05-30

**Branch:** `chore/function-audit` (base `master`) · **Scope:** all `.go` production functions in the `openholidays` package and `cmd/ohcli`.

**Summary:** 95 functions audited · 85 marked `audit:ok` (10 not stamped) · 12 findings (1 high, 1 med, 10 low) · overall line coverage **92.0%** (`go tool cover -func`).

The single high-severity finding (the `validateLanguage` lowercase bug) is **fixed in-flight via PR #32** (`fix(validate): canonicalize language ISO code to uppercase (was silently returning English)`, OPEN). It is the root cause behind the language-code cross-references recorded against every endpoint that delegates to it (`Countries`, `Languages`, `PublicHolidays`, `SchoolHolidays`, `Subdivisions`) and is consolidated to a single row below. Endpoint methods that merely delegate to the validator were stamped; `validateLanguage` itself and `Languages` (whose own doc comment encodes the wrong wire format) were not.

## Findings

| Function | File:Line | Severity | Issue | Suggested fix |
|---|---|---|---|---|
| `validateLanguage` | validate.go:63 | **high** | Canonicalizes the ISO 639-1 language code to **lowercase** via `strings.ToLower(code)`, but the OpenHolidays API expects the `languageIsoCode` query parameter **uppercase** on the wire (sibling `validateCountry` correctly uses `strings.ToUpper`). A non-empty `LanguageIsoCode` filter is therefore sent in the wrong case and silently degrades the result (returns English/wrong localization). Both the code and its doc comment encode the wrong intent. The defect surfaces through every endpoint that delegates to it (`Countries`, `Languages`, `PublicHolidays`, `SchoolHolidays`, `Subdivisions`). **Fixed in-flight via PR #32** — not fixed in this branch, not stamped. | Change to `strings.ToUpper(code)`; update the validator doc comment, the `LanguageIsoCode` field docs in `public_holidays.go`/`school_holidays.go`/`countries.go`/`languages.go`/`subdivisions.go` ("…lowercase…" → "…uppercase…"), and invert the lowercase success expectations in `TestValidateLanguage` and the per-endpoint wire-assertion tests. Tracked in **PR #32**. |
| `cmdPublic` | cmd/ohcli/public.go:71 | **med** | Year-range validation bounds `1900` and `2100` are inlined magic literals **and** duplicated verbatim at `cmd/ohcli/school.go:61`. Flaggable on two counts (a tunable inlined, and a magic literal duplicated across sites). Not stamped. Not fixed here: the clean fix is a single shared const referenced by both files, but this audit may only edit `public.go`; adding the const in `public.go` alone would create a *new* divergence rather than deduplicating. | In a cross-file PR, add package-level `const minYear = 1900` / `const maxYear = 2100` (or a single block) in `main.go`, then replace the inlined `year < 1900 || year > 2100` checks in both `public.go:71` and `school.go:61`. Re-audit and stamp afterward. |
| `cmdSchool` | cmd/ohcli/school.go:61 | low | Same year-bound tunables `1900`/`2100` inlined and duplicated with `public.go:71`. Not stamped. | Same shared-const extraction as the `cmdPublic` finding; defer to the cross-file PR. |
| `defaultConfig` | config.go:118 | low | The 15 s default timeout is inlined as `15 * time.Second` rather than a named const. Value is correct (matches PROJECT.md / D-28 / CLIENT-06) and appears only once, but it is a tunable expressed as a literal. Not stamped. | Extract `const defaultTimeout = 15 * time.Second` (mirroring the existing `defaultBaseURL` const) and reference it. Mechanical and behavior-preserving; left as a finding per "when in doubt, do not fix". |
| `validateDateRange` | validate.go:98 | low | The 3-calendar-year window limit is inlined as the magic literal `-3` in `to.AddDate(-3, 0, 0)`. A max-range tunable inlined rather than named. Logic verified correct (backward-from-`to` anchoring handles the leap-day boundary). Appears at one site only. Not stamped. | Introduce `const maxDateRangeYears = 3` and use `to.AddDate(-maxDateRangeYears, 0, 0)`. Note the value is also embedded in the error prose ("spans more than 3 years") and the doc comment, so a const will not fully centralize it without templating the message — hence recorded as a finding rather than applied. |
| `Languages` | languages.go:82 | low | Method logic is correct (delegates canonicalization, propagates ctx, wraps errors), but the doc comments assert an inaccurate wire format: `LanguagesRequest.LanguageIsoCode` (≈ line 29-30) and the `Languages` doc (≈ line 47-48) say the code is "lowercase on the wire" / "canonicalized to lowercase". This encodes the same wrong assumption PR #32 corrects. Because the doc is currently inaccurate about behavior, the function fails the accurate-doc criterion and was **not** stamped. | After PR #32 merges, update both doc comments to "uppercase", then re-audit and stamp. Not applied here — coupled to #32's in-flight behavior change; editing now risks a merge conflict. |
| `PublicHolidays` | public_holidays.go:78 | low | Doc comment attributes the per-request timeout wrapping to this method ("wraps ctx via `context.WithTimeout(ctx, d)` before dispatching"), but the method delegates to `doJSONGet` (request.go:64-68), which applies the timeout when `c.timeout > 0`. End-to-end behavior a caller sees is accurate, so this is a doc-attribution imprecision, not a behavioral bug. **Stamped** (Gold Rule 5 exempts doc-only edits). | Optional doc-only reword: "when the Client was constructed with `WithTimeout(d)` and `d > 0`, the request is dispatched under `context.WithTimeout(ctx, d)`". No code change; mark remains valid. |
| `renderText` | cmd/ohcli/format.go:81 | low | The `tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)` configuration tuple is duplicated verbatim with `renderCountriesText` (line 175); `padding=2` is a display tunable inlined. Documented inline (mitigates severity) but a future tweak requires editing two sites. Not stamped. | Extract a shared `func newTabWriter(w io.Writer) *tabwriter.Writer` and call it from both renderers. Left as a finding (adds a helper, beyond the strictly-mechanical const-extraction bar); do in a follow-up and re-audit both. |
| `renderCountriesText` | cmd/ohcli/format.go:175 | low | Same duplicated `tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)` tuple as `renderText` (`padding=2` inlined). Not stamped. | Share a single `newTabWriter(w)` helper (see `renderText`). Re-audit and stamp both once deduplicated. |
| `newClient` | cmd/ohcli/main.go:101 | low | The 15 s per-request timeout is inlined as `WithTimeout(15 * time.Second)` and silently duplicates the library's own default (`config.go:118`). The doc claims it "matches the library's documented default" — true today, but with no compile-time link it can diverge silently. Not stamped. | Either drop the explicit `WithTimeout` (the library applies the same 15 s default) or extract a named const in `cmd/ohcli`. Not applied — "drop it" vs "name it" is an intent question, not a mechanical fix. |
| `cmdCountries` | cmd/ohcli/countries.go:33 | low | Stale doc comment on exit-code behavior: it states library and render errors "return exit code 1", but the code returns `libErrExitCode(err)` (line 73), which yields **exit 2** for validation sentinels (`ErrInvalidCountry`/`ErrInvalidLanguage`/`ErrInvalidDateRange`/`ErrDateRangeTooLarge`) per CR-02 and 1 otherwise. Reachable here: a malformed `--lang` flows into `CountriesRequest.LanguageIsoCode`, validated client-side, returning an `ErrInvalidLanguage`-wrapping error → exit 2. Code behavior is the intended one; the doc is wrong. Not stamped. | Reword the doc to describe the CR-02 exit-2 vs exit-1 split (validation sentinels → 2, other library errors and render errors → 1; all emit `ohcli: %v` on stderr). Then re-audit and stamp. |
| `ohcliVersion` | cmd/ohcli/version.go:6 | low | Package doc (line 6) and func doc (line 26) call `openholidays.Version` a "constant", but it is a mutable package `var` (`var Version = "0.5.1" // x-release-please-version`). Documentation-accuracy nit only — behavior is identical and the resolution logic is correct, so **stamped** `audit:ok`. | Doc-only PR: reword the two "constant" occurrences to "version variable" / "package-level `Version` variable". Pure prose change. |

## Missing tests

Exported production functions lacking a dedicated `TestXxx` per Gold Rule 3 (aggregated from the per-file `test_gaps`). Strict 1:1 applies to **exported** functions only; unexported helpers are listed under "Missing coverage" instead.

- **`MemoryCache.Close`** (cache.go) — no `TestMemoryCache_Close`; covered under `TestMemoryCache_CloseIdempotent` (test file documents this as an intentional grouping).
- **`MemoryCache.Get` / `MemoryCache.Put`** (cache.go) — share `TestMemoryCache_GetPut`; the test file documents the grouping as an intentional exception. No standalone `TestMemoryCache_Put`.

All other exported functions have a dedicated `TestXxx`. Notable *unexported* functions exercised only transitively (Rule 3 does not mandate a dedicated test, but isolation would localize regressions): `newClientRand`, `defaultConfig`/`composeHTTPClient`/`buildTransport`, `truncateForError`/`sanitizeForError`/`toUTCMidnight`, `parseAPIMessage`/`buildAPIError`, `statusOf`/`bytesIn`, `cacheKey`, `pickLocalized`, `isTwoASCIILetters`, and the `cmd/ohcli` helpers `reorderArgs`/`hasByte`/`libErrExitCode`/`newClient`/`ohcliVersion` and the `renderCountries*` renderers.

## Missing coverage

Functions below 85% line coverage (`go tool cover -func=/tmp/audit-cover.out`), ranked ascending. The 0.0% entries are predominantly options setters and value methods exercised indirectly or not at all by unit tests; the named non-zero entries are the highest-value gaps.

| Function | File:Line | Coverage |
|---|---|---|
| `newMemoryCacheWithClock` | cache.go:142 | 0.0% |
| `MemoryCache.Put` | cache.go:200 | 0.0% |
| `sweepLoop` | cache.go:234 | 0.0% |
| `MemoryCache.Close` | cache.go:280 | 0.0% |
| `Date.After` | date.go:151 | 0.0% |
| `Date.DaysUntil` | date.go:175 | 0.0% |
| `toUTCMidnight` | date.go:194 | 0.0% |
| `sanitizeForError` | date.go:236 | 0.0% |
| `Holiday.Days` | holiday.go:97 | 0.0% |
| `Country.NameFor` | types.go:209 | 0.0% |
| `Language.NameFor` | types.go:228 | 0.0% |
| `Subdivision.NameFor` | types.go:270 | 0.0% |
| `pickLocalized` | types.go:285 | 0.0% |
| `WithUserAgent` | options.go:94 | 0.0% |
| `WithLogger` | options.go:111 | 0.0% |
| `WithTimeout` | options.go:136 | 0.0% |
| `WithStrictDecoding` | options.go:164 | 0.0% |
| `WithMaxRetryWait` | options.go:291 | 0.0% |
| `WithCache` | options.go:338 | 0.0% |
| `WithCacheBackend` | options.go:365 | 0.0% |
| `WithRequestHook` | options.go:437 | 0.0% |
| `statusOf` | transport.go:136 | 0.0% |
| `bytesIn` | transport.go:158 | 0.0% |
| `cacheKey` | transport_cache.go:108 | 0.0% |
| `main` | cmd/ohcli/main.go:52 | 0.0% |
| `hasByte` | cmd/ohcli/main.go:186 | 0.0% |
| `libErrExitCode` | cmd/ohcli/main.go:203 | 0.0% |
| `renderCountriesCSV` | cmd/ohcli/format.go:202 | 0.0% |
| `newClientRand` | client.go:188 | 7.7% |
| `startSweeper` | cache.go:215 | 50.0% |
| `ohcliVersion` | cmd/ohcli/version.go:35 | 75.0% |
| `cmdCountries` | cmd/ohcli/countries.go:35 | 78.8% |
| `renderCountriesText` | cmd/ohcli/format.go:175 | 80.0% |
| `cmdSchool` | cmd/ohcli/school.go:39 | 82.9% |
| `renderJSON` | cmd/ohcli/format.go:102 | 83.3% |
| `renderCountriesJSON` (note) | — | n/a |
| `parseAPIMessage` | request.go:377 | 83.3% |

Notes on the named gaps:
- **`newClientRand` (7.7%)** — the `crypto/rand`-failure fallback (FNV-128a mix of timestamp+pid) is unreachable in modern Go (`crypto/rand.Read` crashes rather than returning an error) and untestable without a seam. Treat as dead/uncovered defensive code, not a missing assertion.
- **`startSweeper` (50%)** / **`MemoryCache.Put`, `sweepLoop`, `Close`, `newMemoryCacheWithClock` (0%)** — the active-sweeper eviction path (advance a fake clock, assert the goroutine deleted the expired key), the idle-`Close` branch (sweeper never started → `time.After(closeSweeperWait)` arm), and the `ttl<=0` useless-cache contract are all unexercised. TTL expiry is verified only via the lazy-on-read `Get` path.
- **`cmdCountries` (78.8%)** — the library-validation-error path returning **exit 2** via `libErrExitCode` (e.g. malformed `--lang` → `ErrInvalidLanguage`) is uncovered; this is the exact branch that contradicts its stale doc comment.
- **`parseAPIMessage` (83.3%)** — the third-priority `error` branch and the unparseable-body → `""` branch are untested.

## Missing integration tests

Live-API behaviors lacking a guard:

- **Language-name case on the wire** — that a valid `LanguageIsoCode` reaches upstream in the case the API actually honors (the gap that let the lowercase bug ship). **Covered by PR #32's new integration test**, which asserts the corrected uppercase contract end-to-end. No additional guard needed here once #32 lands.
- **Active-sweeper eviction against a live/aged cache** — the memory-bounding goroutine's core purpose (deleting expired keys at the map level) is verified only via lazy-on-read expiry, never by observing the sweeper itself evict.
- **`SchoolHolidays` optional-param omission** — no guard that `groupCode` / `subdivisionCode` are *omitted* (not sent empty) when the request fields are blank; the omit-when-empty branch is only smoke-covered via the happy path.
- **`doJSONGet` retry-loop wiring** — retry exhaustion (`retry exhausted (N attempts)`), `Retry-After` honoring, `computeBackoff` sleep via `c.sleepFunc`, loop-top ctx-cancel between attempts, and mid-sleep ctx-cancel (WR-01) are unit-tested only as isolated helpers; the loop integration inside `doJSONGet` has no end-to-end guard. WR-02 (non-nil resp **and** non-nil err) and WR-06 (retry-exhausted on a retryable status) are likewise unguarded.

## Per-file results

| File | Funcs | Marked ok | Findings |
|---|---|---|---|
| cache.go | 8 | 8 | 0 (1 cross-ref recorded against validate.go) |
| client.go | 4 | 4 | 0 |
| client_isinregion.go | 3 | 3 | 0 |
| config.go | 3 | 2 | 1 (low) |
| countries.go | 1 | 1 | 0 (1 cross-ref to validate.go) |
| date.go | 13 | 13 | 0 |
| doc.go | 0 | 0 | 0 |
| errors.go | 2 | 2 | 0 |
| holiday.go | 4 | 4 | 0 |
| languages.go | 1 | 0 | 2 (1 high cross-ref + 1 low) |
| options.go | 11 | 11 | 0 |
| public_holidays.go | 1 | 1 | 1 low (+ 1 high cross-ref) |
| request.go | 4 | 4 | 0 |
| retry.go | 3 | 3 | 0 |
| school_holidays.go | 1 | 1 | 0 (1 high cross-ref) |
| subdivisions.go | 1 | 1 | 0 (1 high cross-ref) |
| transport.go | 4 | 4 | 0 (1 high cross-ref) |
| transport_cache.go | 3 | 3 | 0 |
| transport_hook.go | 1 | 1 | 0 |
| types.go | 5 | 5 | 0 |
| validate.go | 4 | 2 | 2 (1 high, 1 low) |
| version.go | 0 | 0 | 0 |
| cmd/ohcli/countries.go | 1 | 0 | 1 (low) |
| cmd/ohcli/format.go | 8 | 6 | 2 (low) |
| cmd/ohcli/main.go | 6 | 5 | 1 (low) |
| cmd/ohcli/public.go | 1 | 0 | 1 (med) |
| cmd/ohcli/school.go | 1 | 0 | 1 (low) |
| cmd/ohcli/version.go | 1 | 1 | 1 (low) |
| **Total** | **95** | **85** | **12 distinct (1 high, 1 med, 10 low)** |

The high-severity `validateLanguage` defect is counted once (rooted in validate.go); the identical cross-references logged against `cache.go`, `countries.go`, `languages.go`, `public_holidays.go`, `school_holidays.go`, `subdivisions.go`, and `transport.go` are the same bug surfacing through delegating callers, not separate defects.

## Trivial fixes applied

Behavior-preserving, mechanical fixes applied in-branch (verified by build + existing assertions):

- **cache.go** — extracted the magic divisor `4` in `startSweeper` (`m.ttl / 4`) into a named, documented `const sweeperIntervalDivisor = 4`.
- **cache.go** — extracted the inlined `time.Millisecond` `Close`-wait cap into a named, documented `const closeSweeperWait = time.Millisecond`.
- **cmd/ohcli/format.go** — extracted the duplicated two-space JSON indent literal `"  "` (used by `renderJSON` and `renderCountriesJSON`) into `const jsonIndent = "  "`; rewired both `SetIndent` call sites (`TestRenderJSON` asserts the exact `"\n  "` indent bytes).
- **cmd/ohcli/format.go** — extracted the duplicated semicolon list-separator `";"` (used by `renderCSV` and `renderCountriesCSV`) into `const csvListSep = ";"`; rewired both `strings.Join` call sites (`TestRenderCSV` asserts `"PL-SL;PL-WP"`, `TestRenderCountries` asserts `"pl;de"`).
