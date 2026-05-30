---
phase: quick-260530-vtw
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - integration_test.go
  - types.go
  - .planning/phases/05-distribution/05-04-PLAN.md
  - .planning/phases/05-distribution/05-04-SUMMARY.md
  - .planning/phases/05-distribution/05-RESEARCH.md
  - .planning/phases/05-distribution/05-PATTERNS.md
  - .planning/ROADMAP.md
autonomous: true
requirements: [TEST-08]
must_haves:
  truths:
    - "All six exported endpoint/helper surfaces (PublicHolidays, SchoolHolidays, Countries, Languages, Subdivisions, Client.IsInRegion) are exercised by the live integration suite."
    - "A reintroduction of the 260530-dvc language-casing bug (upstream returns English instead of requested localization) FAILS at least one integration assertion — via exact localized-name pins (Layer 2) AND raw-slice hasLang membership (Layer 3), never via NameFor(lang) != \"\" or require.NotEmpty(NameFor(lang))."
    - "The PL ferie-zimowe-per-województwo core value is asserted live: a non-nationwide school holiday whose Subdivisions carry a specific PL-XX code in a specific date window, with Client.IsInRegion / Holiday.IsInRegion returning true for that województwo."
    - "Client.IsInRegion's hierarchical tree walk (the behavior it adds over Holiday.IsInRegion) is exercised live in the TRUE direction: a synthetic Holiday carries only a PARENT subdivision code, and Client.IsInRegion returns true for a DESCENDANT child code — forcing the /Subdivisions fetch + buildParentIndex + upward walk, NOT the flat fast-path. A negative case (unrelated code → tree walk returns false) is also exercised."
    - "Default `go test ./...` (no -tags=integration, no OPENHOLIDAYS_LIVE) stays hermetic and green — issues zero live HTTP calls."
    - "One gated live run `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...` is GREEN."
    - "types.go SubdivisionRef.Code comment no longer falsely claims PL-SL is Śląskie; it states OpenHolidays Code is its own scheme (not ISO 3166-2)."
  artifacts:
    - path: "integration_test.go"
      provides: "hasLang helper + 7 TestIntegration_<Endpoint> functions covering PL+DE+error paths with three-layer assertions; TestIntegration_ClientIsInRegion drives the hierarchical tree walk true via a descendant code; each touched/new function carries a fresh // audit:ok 2026-05-30 mark"
      contains: "func hasLang("
    - path: "types.go"
      provides: "Corrected SubdivisionRef.Code doc comment (comment-only; audit:ok mark on the type's methods unaffected)"
      contains: "NOT ISO 3166-2"
  key_links:
    - from: "integration_test.go assertions"
      to: "hasLang(entries, lang)"
      via: "Layer-3 anti-fallback membership check on raw []LocalizedText"
      pattern: "hasLang\\("
    - from: "TestIntegration_SchoolHolidays ferie-zimowe case"
      to: "Holiday.IsInRegion / Client.IsInRegion"
      via: "assert a specific PL-XX województwo is covered in its staggered date window"
      pattern: "IsInRegion\\("
    - from: "TestIntegration_PublicHolidays DE regional case"
      to: "Holiday.Subdivisions + Nationwide==false + IsInRegion(\"DE-BY\")"
      via: "Heilige Drei Könige regional assertion"
      pattern: "DE-BY"
    - from: "TestIntegration_ClientIsInRegion descendant case"
      to: "Client.IsInRegion(ctx, h, childCode) hierarchical tree walk"
      via: "synthetic Holiday holds only the PARENT code; query a DESCENDANT child code to force /Subdivisions fetch + buildParentIndex + upward walk (not the flat fast-path)"
      pattern: "c\\.Subdivisions\\(.*CountryIsoCode: \"DE\""
---

<objective>
Translate the approved spec (`docs/superpowers/specs/2026-05-30-integration-test-coverage-design.md`) into a working live-integration suite that exercises every exported endpoint/helper with assertions that FAIL when localization or regional behavior is silently broken. This closes the bug class behind the 2026-05-30 `languageIsoCode`-lowercase incident (quick task 260530-dvc) that a count-only green test missed.

This is a TEST-ONLY coverage task. The ONLY production-source change is a one-line doc-comment correction in `types.go` (comment-only ⇒ Rule 5 audit marks stay). No production logic changes.

Purpose: a count-only integration suite gave false confidence — the lowercase-lang bug returned the right NUMBER of holidays with English names and the suite stayed green. The new suite layers three assertion types per the spec §3 so content regressions surface on the nightly run.

Output: extended `integration_test.go` (rename two tests, add `hasLang` + five new endpoint/helper tests), a corrected `types.go` comment, re-pointed planning-doc `-run` references, and fresh Rule 5 `audit:ok` marks on every touched/new test function.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@docs/superpowers/specs/2026-05-30-integration-test-coverage-design.md
@./CLAUDE.md
@integration_test.go

<spec_authority>
The spec at `docs/superpowers/specs/2026-05-30-integration-test-coverage-design.md` is PRIMARY and AUTHORITATIVE. Do NOT re-derive its decisions. In particular, §6a "Verified live values" were probed live on 2026-05-30 and MUST be used VERBATIM as inline pins — do NOT guess, re-derive, or substitute different values. The §3 three-layer assertion principle is non-negotiable.
</spec_authority>

<verified_values>
USE THESE EXACT VALUES (spec §6a, probed live 2026-05-30). Each inline pin in a test MUST carry a short comment citing the 2026-05-30 probe.

- Languages: 30 entries; includes PL/DE/EN (uppercase IsoCode).
- Countries: 36 entries; with lang=DE → Germany="Deutschland", Poland="Polen"; PL.OfficialLanguages=["PL"]. Language-filtered Country.Name is a single-language array, so hasLang membership is exact.
- PublicHolidays DE 2025 (lang=DE): 21 entries; New Year="Neujahr"; regional case = "Heilige Drei Könige" → Subdivisions {DE-ST, DE-BW, DE-BY}, Nationwide=false, so IsInRegion("DE-BY")==true.
- PublicHolidays PL 2025-01-01 (lang=PL): New Year="Nowy Rok" (raw Name languages=["PL"]).
- PublicHolidays PL 2025 full year: 14 entries (existing canary — keep).
- SchoolHolidays PL 2025: 7 periods (existing canary — keep). 4 ferie-zimowe cohorts, all Nationwide=false, Groups EMPTY, distinguished by date window + subdivision set:
    - 2025-01-20..02-02: PL-SL, PL-LU, PL-WP, PL-MA, PL-KP
    - 2025-01-27..02-09: PL-WN, PL-PK
    - 2025-02-03..02-16: PL-ZP, PL-DS, PL-MZ, PL-OP
    - 2025-02-17..03-02: PL-SK, PL-PD, PL-LB, PL-PM, PL-LD
  (PL-SL falls in the first window. Mirror the pre-validated hermetic TestClient_SchoolHolidays_IsInRegion_FerieZimowe.)
- Subdivisions PL: 16 województwa; NON-ISO Code scheme — PL-SK=Śląskie, PL-SL=Świętokrzyskie (ISO 3166-2 swaps these); Category(pl)="województwo"; codes match ^PL-.
- Subdivisions DE: 16 Bundesländer (DE-BY=Bayern, DE-BW=Baden-Württemberg, DE-ST=Sachsen-Anhalt, …); codes match ^DE-. The live DE tree carries Subdivision.Children (the hermetic client_isinregion_test.go relies on this via findFirstWithChildren) — used by TestIntegration_ClientIsInRegion to source a genuine parent/child descent for the tree-walk-true case.
</verified_values>

<type_contracts>
Verified from source — executor uses these directly, no codebase exploration needed.

Request structs (all fields exported, plain strings / Date):
- PublicHolidaysRequest{ CountryIsoCode, ValidFrom Date, ValidTo Date, LanguageIsoCode, SubdivisionCode }
- SchoolHolidaysRequest{ CountryIsoCode, ValidFrom Date, ValidTo Date, LanguageIsoCode, SubdivisionCode } (same shape as PublicHolidaysRequest)
- CountriesRequest{ LanguageIsoCode }
- LanguagesRequest{ LanguageIsoCode }
- SubdivisionsRequest{ CountryIsoCode, LanguageIsoCode }

Client methods (all `(ctx, req)` → `([]T, error)`, except IsInRegion):
- (c *Client) PublicHolidays(ctx, PublicHolidaysRequest) ([]Holiday, error)
- (c *Client) SchoolHolidays(ctx, SchoolHolidaysRequest) ([]Holiday, error)
- (c *Client) Countries(ctx, CountriesRequest) ([]Country, error)
- (c *Client) Languages(ctx, LanguagesRequest) ([]Language, error)
- (c *Client) Subdivisions(ctx, SubdivisionsRequest) ([]Subdivision, error)
- (c *Client) IsInRegion(ctx, h Holiday, code string) (bool, error)  // hierarchical; issues HTTP only when no fast-path fires

Response types (the localized-text fields hasLang/NameFor read):
- LocalizedText{ Language string `json:"language"`, Text string `json:"text"` }  // Language is the per-entry code
- Holiday{ ID, StartDate Date, EndDate Date, Type HolidayType, Name []LocalizedText, Nationwide bool, RegionalScope, TemporalScope, Subdivisions []SubdivisionRef, Groups []GroupRef, ... }
- SubdivisionRef{ Code string, ShortName string }
- Country{ IsoCode string, Name []LocalizedText, OfficialLanguages []string }
- Language{ IsoCode string, Name []LocalizedText }
- Subdivision{ Code string, ShortName string, Name []LocalizedText, Category []LocalizedText, OfficialLanguages []string, IsoCode string, Children []Subdivision, Groups []GroupRef }

Value-helper methods (all case-insensitive via strings.EqualFold):
- Holiday.NameFor(lang) / Country.NameFor(lang) / Language.NameFor(lang) / Subdivision.NameFor(lang) → all delegate to pickLocalized, which FALLS BACK to entries[0] on a language miss (THIS IS THE TRAP — see hard rule below).
- Holiday.IsInRegion(code) bool — flat match: Nationwide→true; empty code on non-nationwide→false; else strings.EqualFold over h.Subdivisions[].Code.
- Category has NO NameFor accessor (it is a plain []LocalizedText field on Subdivision). To read a localized Category value, iterate sk.Category for an entry with strings.EqualFold(entry.Language, "PL") and read its .Text — do NOT call a Category accessor that does not exist.

Client.IsInRegion fast-path vs tree-walk (verified from client_isinregion.go):
- Fast paths issue NO HTTP and do NOT exercise the tree walk: (1) h.Nationwide→true; (2) empty code→false; (3) FLAT strings.EqualFold match against h.Subdivisions[].Code→true; (4) empty h.Subdivisions (non-nationwide)→false.
- Hierarchical tree walk (the behavior Client.IsInRegion adds over Holiday.IsInRegion) fires ONLY when no fast-path matches: it derives the country from h.Subdivisions[0].Code prefix, GETs /Subdivisions, calls buildParentIndex over Subdivision.Children, and walks child→parent upward looking for a match against h.Subdivisions. Therefore: to exercise the tree walk in the TRUE direction, the queried code must be a DESCENDANT of a code in h.Subdivisions — NOT a code already present in h.Subdivisions (which would hit fast-path 3).

Error model (errors.go): validateCountry / validateLanguage are SHAPE-ONLY (exactly 2 ASCII letters, canonicalized uppercase). A two-letter code like "ZZ" PASSES client-side validation and reaches the upstream; a one-letter code like "Z" FAILS client-side and never hits the network. Sentinels: ErrInvalidCountry, ErrInvalidLanguage, ErrInvalidDateRange, ErrDateRangeTooLarge, ErrEmptyResponse, ErrResponseTooLarge, ErrMalformedResponse. *APIError carries StatusCode + Message; match via errors.As(err, &apiErr) or errors.Is(err, &APIError{StatusCode: N}).
</type_contracts>

<hard_rules>
These are NON-NEGOTIABLE. Encode them in the tests:

1. NEVER assert localization via `NameFor(lang) != ""` or `require.NotEmpty(h.NameFor(lang))` (nor `assert.NotEmpty(... NameFor ...)`). pickLocalized falls back to entries[0], so BOTH of those forms PASS under exactly the 260530-dvc bug. Use BOTH: (a) an exact localized-name PIN (`require.Equal(t, "Nowy Rok", h.NameFor("PL"))`) AND (b) `hasLang` raw-slice membership. The existing `integration_test.go` line ~106 already uses the correct pin form (`require.Equal(t, "Nowy Rok", hs[0].NameFor("PL"))`) — preserve that pattern; do NOT regress it to NotEmpty.

2. hasLang MUST be case-insensitive (strings.EqualFold) AND MUST NOT fall back to entries[0]. It returns true only when some entry's Language genuinely matches the requested code. This is the Layer-3 anti-fallback guard.

3. Three assertion layers per case (spec §3), picking those that apply:
   - Layer 1 (canary): exact counts — drift fails the nightly by design; failure message must say so and point at the §6a probe / golden fixture.
   - Layer 2 (pin): exact localized name for a stable holiday/country/language/subdivision.
   - Layer 3 (hasLang): raw-slice membership for the requested language.

4. testdata: do NOT add testdata/*_de_*.json and do NOT touch update_fixtures_test.go. Assert §6a values INLINE (spec §6 supersedes any fixture talk). Committed fixtures are a hermetic-test concern only; hermetic DE tests are out of scope.

5. Gating (every integration test): file stays under `//go:build integration` AND each test starts with `if os.Getenv("OPENHOLIDAYS_LIVE") != "1" { t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test") }`. NO t.Parallel anywhere (gentle on the volunteer-run free upstream). Use `t.Context()` + `context.WithTimeout` per the existing pattern in integration_test.go (NewClient(WithTimeout(15*time.Second)); t.Cleanup(close + cancel)).

6. Pipeline UNCHANGED: do NOT edit `.github/workflows/integration.yml` — tagged tests auto-join the nightly run.

7. Rule 1 (English): all identifiers, comments, names, messages in English. Non-English literals appear ONLY as expected upstream values (e.g. "Nowy Rok", "Śląskie", "Neujahr", "Deutschland", "województwo") — the sanctioned testdata-style exception.

8. Rule 3 (testify + structure): import testify require + assert (test-only); `require` for preconditions (NoError, non-empty precondition before indexing), `assert` for verifications. Exactly one `TestIntegration_<Endpoint>` per endpoint surface; every case inside a `t.Run(name, ...)` subtest — no top-level assertions in the outer body.

9. Rule 5 (audit:ok): every test function you ADD or whose logic/assertions you CHANGE (including the two renamed functions) gets a fresh `// audit:ok 2026-05-30` line placed ABOVE the godoc, separated from it by a blank line. The two renamed functions are being re-audited regardless (their assertions change), so re-stamp them. The types.go change is comment-only ⇒ it does NOT touch any function body ⇒ existing audit:ok marks in types.go STAY untouched.

10. Git hygiene: NEVER `git add -A` / `git add .`. Stage explicit paths only. Do NOT stage these untracked files: ohcli, GSD-PROJECT-BRIEF.md, .planning/phases/01-foundation/01-UAT.md, .planning/phases/04-resilience/04-PATTERNS.md, .planning/audit/RESUME-AFTER-COMPACT.md, docs/superpowers/specs/2026-05-29-release-pipeline-fix-design.md. Do NOT hand-edit CHANGELOG.md. Commit types: test additions → `test(integration):`; spec/comment fix + planning-ref re-point → `chore` or `refactor` (NEVER feat/fix/perf/deps/docs — those cut a release here). Footer EVERY commit with: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. We are already on branch `test/integration-coverage` — do NOT create another branch.
</hard_rules>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Correct types.go comment + add hasLang helper + re-point planning-doc -run references</name>
  <files>types.go, integration_test.go, .planning/phases/05-distribution/05-04-PLAN.md, .planning/phases/05-distribution/05-04-SUMMARY.md, .planning/phases/05-distribution/05-RESEARCH.md, .planning/phases/05-distribution/05-PATTERNS.md, .planning/ROADMAP.md</files>
  <action>
Three independent, low-risk prep edits in one task (no test logic yet — that is Tasks 2 and 3):

(a) types.go — fix the misleading SubdivisionRef.Code comment. The current comment on the Code field (around line 94) says the code is `"PL-SL" for Śląskie`, but the live API maps PL-SL → Świętokrzyskie (verified 2026-05-30 probe, spec §6a). Reword the Code field doc comment to state that the OpenHolidays `Code` is its OWN scheme and is NOT ISO 3166-2 — the ISO 3166-2 value, if any, lives in the separate Subdivision.IsoCode field. Keep the example concrete but accurate (e.g. note PL-SL resolves to Świętokrzyskie under the live API, not Śląskie). Use neutral wording such as "the OpenHolidays subdivision code (its own scheme, NOT ISO 3166-2; any ISO 3166-2 value lives in Subdivision.IsoCode)". This is a COMMENT-ONLY change — do NOT alter any field name, type, tag, or function body. Per Rule 5, comment-only edits do NOT invalidate audit:ok marks, so leave every `// audit:ok` line in types.go exactly as-is. Keep the wording English and accurate.

(b) integration_test.go — add the unexported `hasLang` test helper near the top of the file (after imports, before the first test function). Signature and contract per spec §3:
   - `func hasLang(entries []LocalizedText, lang string) bool`
   - Returns true iff some entry in `entries` has `strings.EqualFold(entry.Language, lang)`. It MUST NOT fall back to entries[0]. Add a doc comment explaining it is the anti-fallback Layer-3 guard for the 260530-dvc language-casing bug class, contrasting it explicitly with NameFor/pickLocalized (which DO fall back). Add `"strings"` to the import block (testify require already imported; assert will be added in later tasks). Give hasLang a `// audit:ok 2026-05-30` mark above its doc comment (it is new logic being introduced now).

(c) Re-point planning-doc `go test -run` / function-name references from the OLD names to the NEW names, since Task 2 renames the two functions. Update these files where they reference the runnable test names (NOT historical SUMMARY prose describing what was shipped at the time — use judgment: a `-run` command or a "this test exists" assertion that will be executed/greped should be re-pointed; a dated historical record of a past run may stay). Concretely, search and update live `-run TestIntegration_PublicHolidays_PL_2025` / `_SchoolHolidays_PL_2025` and grep-pattern references in: 05-04-PLAN.md, 05-04-SUMMARY.md, 05-RESEARCH.md, 05-PATTERNS.md, ROADMAP.md. New names: `TestIntegration_PublicHolidays` and `TestIntegration_SchoolHolidays`. Run `grep -rn "TestIntegration_PublicHolidays_PL_2025\|TestIntegration_SchoolHolidays_PL_2025" .planning/` after editing and confirm only intentional historical mentions remain (ideally zero in the files listed above).

Do NOT commit yet — the build/vet gate runs in Task 4 after all test logic lands. (Edits here are doc/comment/helper only and do not break the build: hasLang compiles standalone.)
  </action>
  <verify>
    <automated>test "$(grep -c 'func hasLang(' integration_test.go)" -eq 1 && go build -tags=integration ./... && ! grep -rn 'TestIntegration_PublicHolidays_PL_2025\|TestIntegration_SchoolHolidays_PL_2025' .planning/phases/05-distribution/05-04-PLAN.md .planning/phases/05-distribution/05-04-SUMMARY.md .planning/phases/05-distribution/05-RESEARCH.md .planning/phases/05-distribution/05-PATTERNS.md .planning/ROADMAP.md</automated>
  </verify>
  <done>hasLang exists in integration_test.go (case-insensitive, no entries[0] fallback, audit:ok stamped); types.go Code comment no longer claims PL-SL=Śląskie and states Code is not ISO 3166-2 (no function body or audit:ok line touched); planning-doc executable `-run`/grep references point to the renamed tests across all five planning files (05-04-PLAN.md, 05-04-SUMMARY.md, 05-RESEARCH.md, 05-PATTERNS.md, ROADMAP.md); `go build -tags=integration ./...` succeeds.</done>
</task>

<task type="auto">
  <name>Task 2: Extend & rename the two existing tests (PublicHolidays + SchoolHolidays) with DE, regional, and ferie-zimowe-per-województwo cases</name>
  <files>integration_test.go</files>
  <action>
Rename and extend the two existing functions in integration_test.go. Preserve the existing double-gate, no-parallel, t.Context()+WithTimeout client setup pattern verbatim. Each new case is a `t.Run` subtest; `require` for preconditions (NoError, non-empty before indexing), `assert` for verifications. Add `"github.com/stretchr/testify/assert"` to imports. Apply all three assertion layers (spec §3) and the hard rules — especially: NEVER `NameFor(lang) != ""` and NEVER `require.NotEmpty(NameFor(lang))`; pair every localized pin with `hasLang`.

RENAME 1: `TestIntegration_PublicHolidays_PL_2025` → `TestIntegration_PublicHolidays`. Keep its existing two subtests (the 14-count PL canary and the existing "Nowy Rok" pin — that pin is already correct, do NOT weaken it). ADD subtests:
   - "DE 2025 has 21 public holidays (count canary)": PublicHolidays for CountryIsoCode "DE", LanguageIsoCode "DE", full-year 2025 window. `require.Len(t, hs, 21, ...)` with a drift-canary failure message citing the 2026-05-30 probe.
   - "DE 2025 New Year is Neujahr (localization pin + hasLang)": from the DE/lang=DE result, find the New Year holiday (StartDate 2025-01-01) — `require` it is found — then `assert.Equal(t, "Neujahr", h.NameFor("DE"))` AND `assert.True(t, hasLang(h.Name, "DE"))`. (Under the 260530-dvc bug the pin would read "New Year's Day" and hasLang("DE") would be false — both fail.)
   - "DE Heilige Drei Könige is regional (Nationwide=false + Subdivisions + IsInRegion)": from the DE/lang=DE result, find the holiday whose German name is "Heilige Drei Könige" (StartDate 2025-01-06) — `require` found — then `assert.False(t, h.Nationwide)`, `assert.NotEmpty(t, h.Subdivisions)`, and `assert.True(t, h.IsInRegion("DE-BY"))` (subdivisions are {DE-ST, DE-BW, DE-BY} per §6a). Add a sanity `assert.False(t, h.IsInRegion("DE-HH"))` to prove the flat match is selective (DE-HH/Hamburg is not in the set).
   Update the function's godoc to describe the broadened PL+DE coverage; place a FRESH `// audit:ok 2026-05-30` above the godoc (its assertions changed → re-audit).

RENAME 2: `TestIntegration_SchoolHolidays_PL_2025` → `TestIntegration_SchoolHolidays`. Keep the existing 7-count PL canary subtest. ADD subtests:
   - "PL 2025 ferie zimowe per województwo (core value)": SchoolHolidays for PL, full-year 2025. From the 7 periods, isolate the ferie-zimowe cohorts (Type == HolidayTypeSchool, Nationwide == false). Assert the headline differentiator mirroring the hermetic TestClient_SchoolHolidays_IsInRegion_FerieZimowe: find the cohort whose window is 2025-01-20..02-02 (StartDate 2025-01-20) — `require` found — and `assert` it is non-nationwide, its Groups slice is EMPTY (per §6a — cohorts are distinguished by Subdivisions + date window, NOT group codes), and `assert.True(t, found.IsInRegion("PL-SL"))` (PL-SL is in that first window per §6a). Add a negative: `assert.False(t, found.IsInRegion("PL-SK"))` (PL-SK is in the LAST window 2025-02-17..03-02, not the first) to prove the window→województwo mapping is real and not a blanket true. Use Holiday.IsInRegion (flat) here — the codes are top-level in Holiday.Subdivisions.
   - "PL school holidays localized (hasLang)": SchoolHolidays for PL with LanguageIsoCode "PL". `require` non-empty; pick a stable period and `assert.True(t, hasLang(period.Name, "PL"))`. (The endpoint lacks rich localization today; hasLang is the robust assertion. Do NOT pin an exact Polish school-holiday name unless §6a gives one — it does not, so rely on hasLang here per Gold Rule 2: do not guess a name.)
   - "DE school holidays non-empty with DE localization": SchoolHolidays for DE, full-year 2025, LanguageIsoCode "DE". `require.NoError`; `assert.NotEmpty(t, hs)`; for the first period `assert.True(t, hasLang(p.Name, "DE"))`. (No DE school-holiday count is pinned in §6a — do NOT invent one. Assert non-emptiness + DE localization only.)
   Update the godoc for broadened coverage; place a FRESH `// audit:ok 2026-05-30` above it.

Locating-by-name pattern: iterate the returned slice and match on StartDate (via the Date type's accessors / Equal against NewDate(...)) AND/OR NameFor against the expected localized string, capturing the matched Holiday into a local var guarded by `require.True(t, found, ...)` before asserting on it. Do NOT assume slice ordering — search, don't index by position (except the existing hs[0] PL New Year case, which the prior author validated against the single-day 2025-01-01 window query that returns exactly that holiday).
  </action>
  <verify>
    <automated>go build -tags=integration ./... && go vet -tags=integration ./... && grep -Eq '^func TestIntegration_PublicHolidays\(t \*testing\.T\)' integration_test.go && grep -Eq '^func TestIntegration_SchoolHolidays\(t \*testing\.T\)' integration_test.go && ! grep -E 'NameFor\([^)]*\) *!= *""|NotEmpty[^)]*NameFor' integration_test.go && OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=2m -run 'TestIntegration_PublicHolidays$|TestIntegration_SchoolHolidays$' ./...</automated>
  </verify>
  <done>Both functions renamed (no `_PL_2025` suffix) and carry fresh `// audit:ok 2026-05-30` marks; PublicHolidays covers PL count + PL/DE localization pins + DE regional IsInRegion; SchoolHolidays covers PL count + ferie-zimowe-per-województwo core value (PL-SL true in first window, PL-SK false) + PL/DE localization via hasLang; neither the `NameFor(...) != ""` nor the `NotEmpty(...NameFor...)` anti-pattern is present; the gated live run for these two tests is GREEN.</done>
</task>

<task type="auto">
  <name>Task 3: Add the five new endpoint/helper tests (Countries, Languages, Subdivisions, ClientIsInRegion, ErrorPaths)</name>
  <files>integration_test.go</files>
  <action>
Add five new `TestIntegration_<Endpoint>` functions to integration_test.go, each one endpoint/helper surface, each following the SAME gate + no-parallel + client-setup + t.Run pattern as Task 2, each with a FRESH `// audit:ok 2026-05-30` mark above its godoc. Apply the three layers and hard rules throughout (pin + hasLang for every localized assertion; never `NameFor(lang) != ""` and never `require.NotEmpty(NameFor(lang))`). Use §6a verified values inline with a probe-citing comment.

1. `TestIntegration_Countries` — cases:
   - "default lists PL and DE by uppercase IsoCode + 36 count canary": Countries(CountriesRequest{}); `require.Len(t, cs, 36, ...)` drift canary; `assert.True` that the slice contains a Country with IsoCode "PL" and one with "DE" (case-sensitive uppercase). Assert the located PL country's OfficialLanguages contains "PL" (`assert.Contains(t, pl.OfficialLanguages, "PL")`, per §6a PL.OfficialLanguages=["PL"]).
   - "lang=DE localizes country names (pin + hasLang)": Countries(CountriesRequest{LanguageIsoCode: "DE"}); find Germany (IsoCode "DE") and Poland (IsoCode "PL"); `assert.Equal(t, "Deutschland", germany.NameFor("DE"))`, `assert.Equal(t, "Polen", poland.NameFor("DE"))`, and `assert.True(t, hasLang(germany.Name, "DE"))`. (§6a: language-filtered Country.Name is a single-language array → hasLang exact.)

2. `TestIntegration_Languages` — cases:
   - "default lists PL, DE, EN by uppercase IsoCode + 30 count canary": Languages(LanguagesRequest{}); `require.Len(t, ls, 30, ...)` canary; `assert.True` the slice contains IsoCode "PL", "DE", and "EN".
   - "lang=PL localizes language names (hasLang)": Languages(LanguagesRequest{LanguageIsoCode: "PL"}); pick a stable language entry (e.g. the one with IsoCode "PL" or "DE") and `assert.True(t, hasLang(entry.Name, "PL"))`. Do NOT pin an exact localized language name — §6a does not give one, and Gold Rule 2 forbids guessing. hasLang is the robust assertion here.

3. `TestIntegration_Subdivisions` — cases:
   - "PL has 16 województwa, codes ^PL- (count canary)": Subdivisions(SubdivisionsRequest{CountryIsoCode: "PL"}); `require.Len(t, subs, 16, ...)` canary; `assert` every subdivision Code has prefix "PL-".
   - "PL lang=PL localizes name + category (pins + hasLang)": Subdivisions(SubdivisionsRequest{CountryIsoCode: "PL", LanguageIsoCode: "PL"}); find the subdivision with Code "PL-SK" and `assert.Equal(t, "Śląskie", sk.NameFor("PL"))`; find Code "PL-SL" and `assert.Equal(t, "Świętokrzyskie", sl.NameFor("PL"))` — INCLUDE an inline comment that OpenHolidays Code is NOT ISO 3166-2 (ISO swaps PL-SL/PL-SK), citing the 2026-05-30 probe. Add `assert.True(t, hasLang(sk.Name, "PL"))`. Category is a `[]LocalizedText` field with NO NameFor accessor: to pin its value, iterate sk.Category for the entry with strings.EqualFold(entry.Language, "PL") and `assert.Equal(t, "województwo", entry.Text)`; also `assert.True(t, hasLang(sk.Category, "PL"))` for membership. Do NOT call a nonexistent Category accessor.
   - "DE has 16 Bundesländer, codes ^DE- (count canary)": Subdivisions(SubdivisionsRequest{CountryIsoCode: "DE"}); `require.Len(t, subs, 16, ...)`; `assert` every Code has prefix "DE-"; spot-check that Code "DE-BY" exists.

4. `TestIntegration_ClientIsInRegion` — exercises the ONE hidden-I/O Phase-3 helper live, per spec §4. The WHOLE reason `Client.IsInRegion` exists (vs `Holiday.IsInRegion`) is the hierarchical tree walk: it fetches `/Subdivisions`, builds the parent index, and walks child→parent. A code that is ALREADY in `h.Subdivisions[]` (e.g. DE-BY, PL-SL directly) triggers the flat fast-path (`client_isinregion.go` lines 87–91) and returns true with ZERO HTTP — so it does NOT exercise the tree walk. The positive case MUST therefore drive a TRUE result THROUGH the tree walk by querying a DESCENDANT code that is not itself in `h.Subdivisions`. The hermetic `client_isinregion_test.go` proves live DE data carries `Subdivision.Children` (via its `findFirstWithChildren` helper), so a genuine child→parent descent is available live. Cases:
   - "descendant code returns true THROUGH the live tree walk (the reason Client.IsInRegion exists)": Build the scenario from live data, mirroring the hermetic test's discover-at-runtime approach so it stays resilient to fixture/upstream refreshes.
       1. Fetch the DE subdivision tree live: `tree, err := c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: "DE"})`; `require.NoError`; `require.NotEmpty(t, tree)`.
       2. Locate a parent subdivision that has Children and take one `Children[i].Code` — a code that is NOT top-level (NOT directly in the holiday's Subdivisions). Reuse the hermetic test's pattern: iterate `tree` for the first entry with `len(s.Children) > 0`, capture `parentCode := s.Code` and `childCode := s.Children[0].Code`, guarded by `require.True(t, found, "live DE subdivision tree must carry at least one entry with non-empty Children — required to exercise the hierarchical walk (matches the hermetic findFirstWithChildren assumption; 2026-05-30 probe)")`.
       3. Build a synthetic Holiday value INLINE with ONLY the PARENT code in Subdivisions (constructing a value type in test code is fine — this is NOT production logic): `h := Holiday{Nationwide: false, Subdivisions: []SubdivisionRef{{Code: parentCode}}}`.
       4. Call `ok, err := c.IsInRegion(ctx, h, childCode)`; `require.NoError`; `assert.True(t, ok, ...)`. Because `childCode` is only a DESCENDANT (not in `h.Subdivisions`), no fast-path fires — this forces the `/Subdivisions` fetch + `buildParentIndex` + upward walk to return true. Add an inline comment naming this as the tree-walk-true path and noting the live DE data carries Children (verified 2026-05-30 probe + hermetic findFirstWithChildren).
   - "unrelated code returns false (tree walk finds no ancestor)": with the SAME `h` (only `parentCode` in Subdivisions), call `c.IsInRegion(ctx, h, "ZZ-XX-NEVER")` (a code absent from the DE tree) → `require.NoError`, `assert.False`. With no flat match the method fetches `/Subdivisions`, builds the parent index, walks the tree, finds no ancestor, and returns false — exercising the tree walk in the FALSE direction too. (Gold Rule 2: if the executor finds the live DE tree carries NO entry with non-empty Children on the day it runs, the `require.True(t, found, ...)` precondition above surfaces that explicitly — do NOT fabricate a synthetic child code the upstream tree would not contain; stop and report instead.)

5. `TestIntegration_ErrorPaths` — live error/validation behavior for a request that PASSES client-side validation but is rejected/empty UPSTREAM (spec §4, §2). Add `"errors"` to imports. Cases:
   - "unknown but well-formed country": use CountryIsoCode "ZZ" (passes the shape-only validateCountry → reaches upstream). Call PublicHolidays for ZZ, full-year 2025. The exact upstream mapping is NOT pinned in §6a, so PROBE it at implementation time (run the gated test once and observe): the upstream either returns a 4xx → `*APIError` (assert via `errors.As(err, &apiErr)` and that apiErr.StatusCode is the observed 4xx) OR a 2xx empty result (assert `require.NoError` + `assert.Empty(t, hs)`). Encode whichever the live API actually returns, with an inline comment recording the observed status/behavior and the implementation-date probe. Do NOT guess — Gold Rule 2: assert the behavior you observed, not an assumed one.
   - "malformed country rejected client-side (no network)": use CountryIsoCode "Z" (one letter → fails isTwoASCIILetters). Call PublicHolidays; `require.Error`; `assert.True(t, errors.Is(err, ErrInvalidCountry))`. This pins the documented typed-error mapping from errors.go and proves the client-side gate fires before the network. (Deterministic from validate.go — does not depend on §6a values.)

All five functions: English-only, testify require/assert, t.Run per case, no t.Parallel, double-gated, fresh audit:ok marks.
  </action>
  <verify>
    <automated>go build -tags=integration ./... && go vet -tags=integration ./... && for n in Countries Languages Subdivisions ClientIsInRegion ErrorPaths; do grep -Eq "^func TestIntegration_${n}\(t \*testing\.T\)" integration_test.go || { echo "MISSING TestIntegration_${n}"; exit 1; }; done && ! grep -E 'NameFor\([^)]*\) *!= *""|NotEmpty[^)]*NameFor' integration_test.go && grep -q 'c\.Subdivisions(.*CountryIsoCode: "DE"' integration_test.go && OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=3m -run 'TestIntegration_Countries$|TestIntegration_Languages$|TestIntegration_Subdivisions$|TestIntegration_ClientIsInRegion$|TestIntegration_ErrorPaths$' ./...</automated>
  </verify>
  <done>Five new TestIntegration_<Endpoint> functions exist, each audit:ok-stamped, covering: Countries (PL/DE present + DE-localized pins Deutschland/Polen + hasLang + OfficialLanguages), Languages (PL/DE/EN present + hasLang), Subdivisions (PL 16 + DE 16 count canaries, PL-SK=Śląskie / PL-SL=Świętokrzyskie pins with the non-ISO comment + Category=województwo + ^PL-/^DE- prefixes), ClientIsInRegion (POSITIVE case drives a TRUE result THROUGH the hierarchical tree walk via a descendant child code on a synthetic parent-only Holiday — NOT the flat fast-path — by fetching the live DE tree and reusing the findFirstWithChildren pattern, PLUS a negative case where an unrelated code makes the tree walk return false), ErrorPaths (observed upstream behavior for "ZZ" + deterministic ErrInvalidCountry for "Z"); neither the `NameFor(...) != ""` nor `NotEmpty(...NameFor...)` anti-pattern present; the gated live run for all five is GREEN.</done>
</task>

<task type="auto">
  <name>Task 4: Full verification gate, Rule 5 audit-mark audit, and explicit-path commits</name>
  <files>integration_test.go, types.go</files>
  <action>
Final quality gate + commit. No new logic — verify the whole suite, confirm Rule 5 compliance, then commit with explicit paths and the required footer.

1. Hermetic safety: run `go test ./...` (NO -tags, NO env var) and confirm it stays GREEN and issues zero live calls (the integration file is compile-excluded without the tag).
2. Build/vet across the integration tag: `go build -tags=integration ./...` and `go vet -tags=integration ./...` clean.
3. Formatting: `gofmt -l .` returns EMPTY (no files need formatting). If anything is listed, `gofmt -w` it.
4. Lint: `golangci-lint run` clean. Also run with the integration tag if the project config supports build-tag linting: `golangci-lint run --build-tags=integration` — resolve any new findings in the integration file (e.g. unused helpers, error-check lints). If golangci-lint is not installed locally, record that and rely on go vet + gofmt; note it in the SUMMARY so CI is the backstop.
5. ONE consolidated gated live run, GREEN: `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...`. This is the authoritative success gate (outbound HTTPS to openholidaysapi.org verified available 2026-05-30). If runtime approaches the 5m timeout, note it (the spec §7 allows bumping -timeout, but do NOT touch the workflow).
6. Renamed-test -run references resolve: confirm `go test -tags=integration -run 'TestIntegration_PublicHolidays$' ./...` and `...SchoolHolidays$` select real tests (exit 0; SKIP without the env var is acceptable and expected).
7. Rule 5 audit-mark audit on integration_test.go: every function whose body/assertions changed or was added in Tasks 1–3 (hasLang + all 7 TestIntegration_* functions) carries exactly one `// audit:ok 2026-05-30` line placed ABOVE its godoc with a blank line separating mark from godoc. Verify no stale mark sits on an unchanged-but-actually-changed function, and that types.go marks are untouched (its change was comment-only).
8. Commit in two logical commits, EXPLICIT PATHS ONLY (never `git add -A`/`.`), each with the footer `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`:
   - Commit A (chore/refactor — the source-comment fix + planning-doc re-point; NOT docs/fix): stage `types.go` and the touched planning docs (`.planning/phases/05-distribution/05-04-PLAN.md`, `.planning/phases/05-distribution/05-04-SUMMARY.md`, `.planning/phases/05-distribution/05-RESEARCH.md`, `.planning/phases/05-distribution/05-PATTERNS.md`, `.planning/ROADMAP.md` — only those actually modified). Message e.g. `chore: correct SubdivisionRef.Code doc (not ISO 3166-2) and re-point integration test -run refs`.
   - Commit B (test): stage ONLY `integration_test.go`. Message e.g. `test(integration): cover all endpoints + helpers with three-layer localization/regional assertions`.
   Before each commit run `git status --porcelain` and confirm none of the forbidden untracked paths (ohcli, GSD-PROJECT-BRIEF.md, .planning/phases/01-foundation/01-UAT.md, .planning/phases/04-resilience/04-PATTERNS.md, .planning/audit/RESUME-AFTER-COMPACT.md, docs/superpowers/specs/2026-05-29-release-pipeline-fix-design.md) are staged. Do NOT hand-edit CHANGELOG.md. Stay on branch test/integration-coverage.
  </action>
  <verify>
    <automated>go test ./... && go build -tags=integration ./... && go vet -tags=integration ./... && test -z "$(gofmt -l .)" && test "$(grep -c '// audit:ok 2026-05-30' integration_test.go)" -ge 8 && ! grep -E 'NameFor\([^)]*\) *!= *""|NotEmpty[^)]*NameFor' integration_test.go && OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...</automated>
  </verify>
  <done>`go test ./...` green and hermetic (zero live calls); `go build -tags=integration ./...`, `go vet -tags=integration ./...`, and `gofmt -l .` all clean; golangci-lint clean (or CI-backstopped note recorded); integration_test.go carries >=8 fresh `// audit:ok 2026-05-30` marks (hasLang + 7 TestIntegration_* functions), each above its godoc; neither the `NameFor(...) != ""` nor `NotEmpty(...NameFor...)` anti-pattern is present; renamed-test `-run` references resolve to real tests; the consolidated `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...` run is GREEN; two commits landed on branch test/integration-coverage with explicit paths and the required Co-Authored-By footer, no forbidden untracked path staged, CHANGELOG.md untouched.</done>
</task>

</tasks>

<verification>
Phase-level gates (all must pass before the task is complete):

- `go test ./...` — GREEN and hermetic (no -tags, no OPENHOLIDAYS_LIVE → integration file compile-excluded, zero live HTTP).
- `go build -tags=integration ./...` — succeeds.
- `go vet -tags=integration ./...` — clean.
- `gofmt -l .` — empty output.
- `golangci-lint run` — clean (CI is the backstop if not installed locally).
- `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...` — GREEN (the authoritative live gate; outbound HTTPS to openholidaysapi.org verified available 2026-05-30).
- Anti-pattern guard: `grep -E 'NameFor\([^)]*\) *!= *""|NotEmpty[^)]*NameFor' integration_test.go` finds NOTHING (no fallback-masking localization assertion anywhere — neither the `!= ""` form nor the `require.NotEmpty(NameFor(...))` form).
- Coverage guard: a `TestIntegration_<Endpoint>` exists for each of PublicHolidays, SchoolHolidays, Countries, Languages, Subdivisions, ClientIsInRegion, plus ErrorPaths.
- Tree-walk guard: `TestIntegration_ClientIsInRegion` fetches the live DE tree (`grep -q 'c\.Subdivisions(.*CountryIsoCode: "DE"' integration_test.go`) and drives its positive case through a DESCENDANT child code on a parent-only synthetic Holiday — the flat fast-path is NOT the true-direction assertion.
- Rule 5 guard: `grep -c "// audit:ok 2026-05-30" integration_test.go` >= 8 (hasLang + 7 test functions).
- Pipeline untouched: `.github/workflows/integration.yml` not in the diff.
- No DE fixtures added: `testdata/*_de_*.json` does not exist and `update_fixtures_test.go` is not in the diff.
</verification>

<success_criteria>
- All six exported endpoint/helper surfaces are exercised live with three-layer assertions (count canary + localized pin + hasLang membership), plus a live error path.
- Reintroducing the 260530-dvc language-casing bug FAILS at least one assertion via exact name pins AND hasLang — never silently passes through a NameFor fallback (neither the `!= ""` nor the `NotEmpty(NameFor(...))` form appears).
- The PL ferie-zimowe-per-województwo core value is proven live (PL-SL covered in its 2025-01-20..02-02 window; PL-SK NOT covered there).
- The DE regional public-holiday case (Heilige Drei Könige, Nationwide=false, IsInRegion("DE-BY")==true) is asserted.
- Client.IsInRegion's hierarchical tree walk is exercised live in the TRUE direction (descendant child code on a synthetic parent-only Holiday forces the /Subdivisions fetch + buildParentIndex + upward walk to return true) AND in the false direction (unrelated code → false).
- Default `go test ./...` stays hermetic and green; the one gated live run is green.
- types.go SubdivisionRef.Code comment corrected (comment-only; no audit:ok invalidation in types.go).
- Rule 5 re-stamping complete on all touched/new test functions; Gold Rules 1/3 honored; commits use chore/refactor + test types only with the required footer, explicit paths, no forbidden files staged, CHANGELOG.md untouched, branch test/integration-coverage preserved.
</success_criteria>

<output>
Create `.planning/quick/260530-vtw-integration-test-coverage/260530-vtw-SUMMARY.md` when done, recording: which §6a values were pinned, the observed live behavior for the "ZZ" error path (status code / empty), the parent/child DE codes used to drive the ClientIsInRegion tree walk, whether golangci-lint ran locally, the measured live-run duration, and the two commit SHAs.
</output>
