# Live Integration Test Coverage — Design

- **Date:** 2026-05-30
- **Status:** Draft (awaiting review)
- **Topic:** Make the nightly live-API integration suite cover every endpoint and
  every localization/regional case, with assertions that cannot pass while the
  feature is silently broken.
- **Scope choice (confirmed with user):** PL + Germany + error paths
  (~22–28 nightly live calls). Not the maximal language × edge-date × pagination
  matrix.

## 1. Motivation

On 2026-05-30 a real functional bug shipped behind a green test: `validateLanguage`
lowercased the `languageIsoCode` before sending it, and the case-sensitive upstream
silently returned English-only names instead of the requested localization. The
integration suite did not catch it for two reasons:

1. **The assertions were counts.** `require.Len(hs, 14)` / `require.Len(hs, 7)` are
   blind to *content*: the lowercase-`lang` bug returned the right *number* of
   holidays with the wrong (English) *names*.
2. **`NameFor` masks the failure.** `pickLocalized` (types.go:285) falls back to
   `entries[0]` on a language miss. So the obvious "fix" — `require.NotEmpty(h.NameFor("PL"))`
   — would *also* have passed under the bug, because it returned the English
   `entries[0]`.

Separately, the suite covered only two of the six exported endpoint/helper surfaces
live (`PublicHolidays`, `SchoolHolidays`), both PL-only, and never asserted the
project's headline differentiator — **regional school-break granularity (Polish
*ferie* per województwo)** — at the subdivision level.

## 2. Goals / Non-goals

**Goals**

- Exercise all six exported endpoint/helper surfaces live: `PublicHolidays`,
  `SchoolHolidays`, `Countries`, `Languages`, `Subdivisions`, `Client.IsInRegion`.
- Cover PL and a second country (DE) across the holiday/localization/subdivision
  endpoints, to catch country-specific upstream drift.
- Cover live error/validation behavior (a request that passes client-side
  validation but is rejected/empty upstream).
- Prove the core value: *ferie zimowe* staggered cohorts mapped to specific
  województwa, via `Subdivisions` + `Client.IsInRegion`.
- Make every localized case assert in a way that **fails** when localization is
  silently broken (the layered principle in §3).

**Non-goals**

- The maximal matrix (language × leap-day/year-boundary edge dates × pagination ×
  both countries). Explicitly out of scope per the chosen breadth.
- Changing the CI pipeline (see §7 — no change needed).
- Byte-for-byte golden comparison against committed fixtures in the live tests
  (too brittle live: IDs, ordering, additive upstream changes). Live tests assert
  scalars + semantics; byte-fixtures remain the hermetic-test concern.

## 3. Design principle — three assertion layers

Every case is built from these layers, picking those that apply:

- **Layer 1 — Drift canary (exact count).** Keep `PL public = 14`, `PL school = 7`;
  add pinned DE holiday counts and subdivision counts. This is the suite's stated
  purpose (surface upstream drift). It is brittle *by design*: on legitimate
  upstream drift the nightly fails, a human investigates, and the pinned
  count / golden fixture is updated. Documented in each test's failure message.
- **Layer 2 — Semantic pin (exact localized value).** Pin a stable, well-known
  holiday's localized name to its exact expected value (e.g. PL New Year →
  `NameFor("PL") == "Nowy Rok"`). Under the lang bug this returns the English
  fallback and **fails** — this is the assertion that would have caught 260530-dvc.
- **Layer 3 — Anti-fallback invariant (raw-slice membership).** Assert that the
  *raw* localized-text slice (`Holiday.Name`, `Country.Name`, `Language.Name`,
  `Subdivision.Name`) contains an entry whose `Language` equals the requested code
  (case-insensitive) — bypassing the lossy `NameFor` fallback. This catches the bug
  class *generically* for every localized endpoint without needing each item's
  exact name.

**Hard rule:** never assert localization via `NameFor(lang) != ""`. The
`entries[0]` fallback makes that pass under exactly the bug we are guarding against.

A small unexported test helper encodes Layer 3:

```go
// hasLang reports whether entries contains a localized-text entry whose
// Language matches lang case-insensitively. Unlike NameFor, it does NOT fall
// back to entries[0] — it is the anti-fallback guard for the language-casing
// bug class (quick task 260530-dvc).
func hasLang(entries []LocalizedText, lang string) bool { ... }
```

## 4. Coverage matrix

One `TestIntegration_<Endpoint>` per endpoint; each case is a `t.Run` subtest.
"canary" = Layer 1, "pin" = Layer 2, "hasLang" = Layer 3.

| Test function | Cases (`t.Run`) | Key assertions |
|---|---|---|
| `TestIntegration_PublicHolidays` | PL 2025; PL `lang=PL`; DE 2025 `lang=DE`; DE regional | PL count canary (14); DE count canary (pinned); New Year name pin (`Nowy Rok` / `Neujahr`) + `hasLang`; **regional**: a non-nationwide DE holiday → `Nationwide==false`, `Subdivisions` non-empty, `Holiday.IsInRegion("DE-…")` true |
| `TestIntegration_SchoolHolidays` | PL 2025; PL `lang=PL`; **PL *ferie zimowe* per województwo**; DE (by subdivision) | PL count canary (7); name pin + `hasLang` (localization the endpoint lacks today); **core value**: *ferie* entries non-nationwide, carry `Subdivisions` (`PL-XX`) across 4 staggered date windows (live `Groups` is **empty** — cohorts are distinguished by `Subdivisions` + date window, not group codes); a specific województwo appears in a specific window — mirroring the pre-validated hermetic `TestClient_SchoolHolidays_IsInRegion_FerieZimowe`; DE school holidays non-empty + DE names |
| `TestIntegration_Countries` | default; `lang=DE` | list contains `PL` & `DE` by uppercase `IsoCode`; `PL.OfficialLanguages` ⊇ `"PL"`; `Country.NameFor` pin (e.g. Germany → `Deutschland`) + `hasLang("DE")` |
| `TestIntegration_Languages` | default; `lang=PL` | list contains `PL`, `DE`, `EN` by uppercase `IsoCode`; `Language.NameFor` pin + `hasLang("PL")` |
| `TestIntegration_Subdivisions` | PL; PL `lang=PL`; DE | PL count canary (16 województwa); codes match `^PL-`; `Category` localized (pl: `województwo`); `PL-SK`.`NameFor("PL") == "Śląskie"` and `PL-SL` → `Świętokrzyskie` + `hasLang` (⚠ OpenHolidays `Code` ≠ ISO 3166-2: ISO assigns PL-SL=Śląskie; the live API swaps PL-SL/PL-SK); DE count canary (16 Bundesländer), codes `^DE-` |
| `TestIntegration_ClientIsInRegion` | hierarchical child match; negative | a holiday whose `Subdivisions` carry a parent code (`PL-SL`) → `Client.IsInRegion(ctx, h, childCode)` walks `/Subdivisions` and returns `true` for a descendant, `false` for an unrelated code (exercises the one hidden-I/O Phase-3 helper live, including the tree walk) |
| `TestIntegration_ErrorPaths` | unknown country; upstream non-2xx | a request that passes client-side validation but is rejected/empty upstream → assert the documented typed-error / empty-result mapping from errors.go |

## 5. Test structure & conventions

- **One `TestIntegration_<Endpoint>` per endpoint surface**, cases as `t.Run`
  subtests (Rule 3 `t.Run`-per-case; `require` for preconditions, `assert` for
  verifications). The hermetic `TestClient_*` tests already satisfy Rule 3's
  one-test-per-exported-function mandate; the integration suite groups by endpoint.
- **Rename the two existing tests** `TestIntegration_PublicHolidays_PL_2025` →
  `TestIntegration_PublicHolidays` and `…_SchoolHolidays_PL_2025` →
  `TestIntegration_SchoolHolidays` (they now also cover DE). Adding DE/regional
  subtests modifies these functions regardless, so their `audit:ok` marks are
  re-audited either way; the rename is essentially free.
  - **Alternative (additive):** keep the `_PL_2025` names and add DE cases in
    separate functions, leaving the existing two untouched. Lower naming churn, no
    planning-ref re-point, but splits PL/DE for one endpoint across two functions.
    Chosen approach is consolidate-and-rename; this alternative is the fallback if
    review prefers minimal churn.
- **Gating unchanged:** every test stays under `//go:build integration` and the
  `OPENHOLIDAYS_LIVE != "1"` → `t.Skip` runtime guard (the double gate).
- **Serialized, gentle:** no `t.Parallel()` in the integration suite — calls stay
  serial against the volunteer-run free upstream (existing convention; the workflow
  comment calls the run "light, off-peak").
- **Context:** `t.Context()` + `context.WithTimeout` per existing pattern.
- **English only** (Rule 1). Non-English literals appear only as expected upstream
  values (e.g. `"Nowy Rok"`, `"Śląskie"`), which is the sanctioned `testdata`-style
  exception.
- **Rule 5:** re-audit + re-stamp the two modified/renamed functions on completion;
  stamp the new test functions once their assertions are reviewed correct.

## 6. Value pinning — no separate DE fixtures (Rule 2)

Exact DE values are **not guessed**: they were probed live on 2026-05-30 and are
pinned in §6a. The integration tests are self-contained drift canaries that assert
those values **inline** (with a comment citing the probe). We do **not** add
`testdata/*_de_*.json` fixtures or touch `update_fixtures_test.go` — committed
fixtures matter only for hermetic tests, and hermetic DE tests are out of scope.

The illustrative values in §4 were probed live on 2026-05-30 and are pinned in
§6a; the fixture-capture step formalizes them as committed golden files.

## 6a. Verified live values (probed 2026-05-30 — authoritative over §4 illustratives)

Pinned from a read-only live probe. Counts are drift canaries (update the fixture
on legitimate upstream drift):

- **Languages:** 30 entries; includes `PL`/`DE`/`EN` (uppercase `IsoCode`).
- **Countries:** 36 entries; with `lang=DE`: Germany → `Deutschland`, Poland →
  `Polen`; `PL.OfficialLanguages` = `["PL"]`. Filtering by language returns a
  single-language `Name` array, so `hasLang` membership is exact.
- **PublicHolidays DE 2025 (`lang=DE`):** 21 entries; New Year → `Neujahr`;
  **regional** case = `Heilige Drei Könige` → `Subdivisions {DE-ST, DE-BW, DE-BY}`,
  `Nationwide == false`, so `IsInRegion("DE-BY") == true`.
- **PublicHolidays PL 2025-01-01 (`lang=PL`):** `Nowy Rok` (raw `Name` languages =
  `["PL"]`).
- **SchoolHolidays PL 2025:** 7 periods; **4 *ferie zimowe* cohorts** (all
  `Nationwide == false`, `Groups` empty), distinguished by date window + subdivision
  set:
  - 2025-01-20…02-02: PL-SL, PL-LU, PL-WP, PL-MA, PL-KP
  - 2025-01-27…02-09: PL-WN, PL-PK
  - 2025-02-03…02-16: PL-ZP, PL-DS, PL-MZ, PL-OP
  - 2025-02-17…03-02: PL-SK, PL-PD, PL-LB, PL-PM, PL-LD
- **Subdivisions PL:** 16 województwa; **non-ISO `Code` scheme** — `PL-SK` = Śląskie,
  `PL-SL` = Świętokrzyskie (ISO 3166-2 assigns these the other way around);
  `Category` (pl) = `województwo`; `Groups` empty.
- **Subdivisions DE:** 16 Bundesländer; standard codes (DE-BY = Bayern,
  DE-BW = Baden-Württemberg, DE-ST = Sachsen-Anhalt, …).

## 7. CI pipeline

No change required. `.github/workflows/integration.yml` runs
`go test -tags=integration -count=1 -timeout=5m ./...` with `OPENHOLIDAYS_LIVE=1`,
nightly at 03:00 UTC. New `//go:build integration` tests join automatically. ~25
serial sub-second/low-second calls fit comfortably inside the 5-minute timeout; if
measured runtime approaches the limit during implementation, bump `-timeout`.

## 8. Implementation outline (detailed plan follows in the planning step)

1. Add the `hasLang` test helper.
2. Use the §6a pinned 2025 values inline (no DE fixture files).
3. Extend & rename `TestIntegration_PublicHolidays` / `_SchoolHolidays`; add the
   *ferie*-per-województwo core-value case.
4. Add `TestIntegration_Countries`, `_Languages`, `_Subdivisions`,
   `_ClientIsInRegion`, `_ErrorPaths`.
5. Re-point any planning-doc `go test -run` refs to the renamed tests; re-audit and
   stamp all touched/new functions (Rule 5).
6. Verify: `go build -tags=integration ./...`, `go vet -tags=integration ./...`,
   `gofmt`; then one gated live run `OPENHOLIDAYS_LIVE=1 go test -tags=integration
   -count=1 -timeout=5m ./...` green.

## 9. Open items to resolve at review

- Consolidate-and-rename vs additive (§5) — default is consolidate.
- DE regional public-holiday case — **resolved** by the 2026-05-30 probe: use
  `Heilige Drei Könige` (`Subdivisions {DE-ST, DE-BW, DE-BY}`) and assert
  `IsInRegion("DE-BY") == true` + `Nationwide == false`.
- **Related finding (outside this task's core scope):** `types.go:94` documents
  `Code` as `"PL-SL" for Śląskie`, but the live API maps `PL-SL` → Świętokrzyskie
  (OpenHolidays `Code` is its own scheme, not ISO 3166-2; the ISO value, if any,
  lives in the separate `Subdivision.IsoCode` field). The comment is misleading.
  Decide whether to correct it here or as a separate doc follow-up.
