---
phase: quick-260530-dvc
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - validate_test.go
  - public_holidays_test.go
  - countries_test.go
  - languages_test.go
  - subdivisions_test.go
  - cmd/ohcli/public_test.go
  - validate.go
  - .planning/phases/01-foundation/01-CONTEXT.md
  - .planning/phases/01-foundation/01-VERIFICATION.md
autonomous: true
requirements: [VALID-04]

must_haves:
  truths:
    - "validateLanguage(\"pl\"), (\"PL\"), (\"Pl\") all return \"PL\" and nil error"
    - "validateLanguage rejects non-ASCII and wrong-length input exactly as before (W-01 shape guard intact)"
    - "Every endpoint that sends languageIsoCode puts the UPPERCASE form on the wire"
    - "ohcli public PL 2025 --lang PL returns Polish holiday names, not English"
    - "go test -race ./... is green; gofmt -l is empty; golangci-lint run is clean"
  artifacts:
    - path: "validate.go"
      provides: "validateLanguage canonicalizing to uppercase via strings.ToUpper"
      contains: "strings.ToUpper(code)"
    - path: "validate_test.go"
      provides: "Failing-first unit test asserting uppercase canonical form for validateLanguage"
      contains: "canonOK: \"PL\""
  key_links:
    - from: "validate.go validateLanguage"
      to: "OpenHolidays API wire format (uppercase ISO 639-1)"
      via: "strings.ToUpper canonicalization mirroring validateCountry"
      pattern: "strings\\.ToUpper\\(code\\)"
---

<objective>
Fix `validateLanguage` so it canonicalizes language ISO 639-1 codes to UPPERCASE
before they reach the OpenHolidays API. Today it lowercases (`strings.ToLower`),
which the case-sensitive upstream silently treats as "no/unknown language" and
returns English-only holiday names. This reverses prior decision D-21.

Purpose: A user running `ohcli public PL 2025 --lang PL` (or any consumer passing
a `LanguageIsoCode`) currently gets English names instead of the requested
language. The library's core promise — correctly-typed, correctly-localized
holiday data — is silently broken for the language filter on every endpoint.

Output: A one-character production change (`ToLower` → `ToUpper`) made test-first,
the unit test and five wire-assertion tests flipped to encode the correct
contract, the validator doc comment corrected, and decision D-21 annotated
REVERSED in the planning record.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md

<verified_root_cause>
VERIFIED LIVE this session against https://openholidaysapi.org — ground truth,
do NOT re-derive or re-verify against the network:

- `validate.go:61` `validateLanguage` returns `strings.ToLower(code)`. This is the bug.
- OpenHolidays is case-SENSITIVE and requires UPPERCASE ISO codes on the wire:
    - `languageIsoCode=PL` → `[{"language":"PL","text":"Nowy Rok"}]` (Polish, correct)
    - `languageIsoCode=pl` → `[{"language":"EN","text":"New Year's Day"}]` (silent English fallback, WRONG)
    - `DE` vs `de` shows the identical pattern.
- `validateCountry` (validate.go:28-38) already `ToUpper`s — it is the correct,
  working analog. The fix makes `validateLanguage` mirror it exactly.
- Independent in-repo corroboration: `update_fixtures_test.go:168,174,180,192`
  is the live-API fixture refresher. It sends `languageIsoCode=EN` / `=PL`
  (UPPERCASE) directly to the real upstream, bypassing `validateLanguage`
  entirely, and its comment (L186-191) documents that upstream honors the
  language param. Uppercase-on-the-wire is therefore the proven-correct form.
- All language-bearing endpoints route through `validateLanguage`
  (public_holidays.go, school_holidays.go, countries.go, languages.go,
  subdivisions.go), so this single validator fix covers every endpoint.
</verified_root_cause>

<interfaces>
<!-- The working analog the fix must mirror. Executor: copy this shape exactly. -->

From validate.go (validateCountry — CORRECT, do NOT change it):
```go
func validateCountry(code string) (string, error) {
    if !isTwoASCIILetters(code) {
        return "", fmt.Errorf("%w: %q", ErrInvalidCountry, code)
    }
    return strings.ToUpper(code), nil
}
```

From validate.go (validateLanguage — BUGGY, the only production line to change):
```go
func validateLanguage(code string) (string, error) {
    if !isTwoASCIILetters(code) {
        return "", fmt.Errorf("%w: %q", ErrInvalidLanguage, code)
    }
    return strings.ToLower(code), nil   // BUG: must become strings.ToUpper(code)
}
```

`isTwoASCIILetters(code)` is the W-01 Unicode-fold guard that runs on the
ORIGINAL bytes BEFORE canonicalization. It MUST stay exactly where it is and
unchanged — it is what rejects U+212A Kelvin sign, U+0130, etc. before ToUpper
runs. Do not move it, do not modify it.
</interfaces>

<scope_do_not>
- Do NOT change `Holiday.NameFor` / `pickLocalized` — already case-insensitive
  via `strings.EqualFold` and correct.
- Do NOT change CLI arg handling (`reorderArgs` / `public.go` / `school.go` /
  `countries.go`) — verified correct; `--lang PL` already flows to the request.
- Do NOT touch `testdata/` fixtures — real upstream responses already carry
  uppercase language codes (e.g. `{"language":"PL",...}`).
- Do NOT touch `update_fixtures_test.go` — it already sends uppercase on the
  wire and never calls `validateLanguage`. Changing it would be wrong.
- Do NOT hand-edit `CHANGELOG.md` — Release Please owns it (Gold Rule 4 /
  docs/release-runbook.md §7). The fix is conveyed via the Conventional-Commit
  `fix:` subject only.
- Do NOT add a networked / live-API test. Hermetic tests only (unit +
  httptest mock) — user chose this option.
- Do NOT change the `isTwoASCIILetters` shape guard or its position (W-01).
- Do NOT change the `ErrInvalidLanguage` sentinel, the `%w`/`%q` wrapping, or
  any reject-case behavior — only the success-path canonicalization direction.
</scope_do_not>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Write failing tests — flip validateLanguage unit test + 5 wire assertions to UPPERCASE</name>
  <files>validate_test.go, public_holidays_test.go, countries_test.go, languages_test.go, subdivisions_test.go, cmd/ohcli/public_test.go</files>
  <behavior>
    These edits MUST fail against the current (lowercasing) production code,
    proving the test now encodes the correct contract. After Task 2 they pass.

    - validateLanguage("pl") → "PL", nil   (currently returns "pl" → FAILS now)
    - validateLanguage("PL") → "PL", nil
    - validateLanguage("Pl") → "PL", nil
    - validateLanguage("en") → "EN", nil
    - validateLanguage("DE") → "DE", nil
    - All existing reject cases (empty / 1 letter / 3 letters / digit / whitespace
      / non-ASCII / symbol / the four W-01 fold cases) stay UNCHANGED and keep
      passing — they assert the shape guard and the empty-string return, neither
      of which this fix touches.
    - Every endpoint wire assertion expects the UPPERCASE languageIsoCode value.
  </behavior>
  <action>
    Make the following edits. All comments, subtest names, and assertion
    messages MUST be reworded from "lowercase"/"lowercased" to "uppercase" so the
    test corpus stops documenting the bug. English only (Gold Rule 1).

    1. validate_test.go — in `TestValidateLanguage`, rewrite the five
       `successCases` (currently around L107-113) to assert the uppercase
       canonical form, and reword each case `name`:
         - {name: "uppercase passes through", input: "PL", canonOK: "PL"}
         - {name: "lowercase canonicalizes", input: "pl", canonOK: "PL"}
         - {name: "mixed case canonicalizes", input: "Pl", canonOK: "PL"}
         - {name: "additional happy case uppercase EN", input: "EN", canonOK: "EN"}
         - {name: "additional happy case lowercase de", input: "de", canonOK: "DE"}
       Leave the entire `rejectCases` slice and the reject-loop assertions
       untouched. Update the `TestValidateLanguage` doc comment (L96-98) so it
       says "canonicalize to uppercase" instead of "canonicalize to lowercase".

    2. public_holidays_test.go — subtest at L339:
         - Rename subtest "...is canonicalized to lowercase on the wire" →
           "...is canonicalized to uppercase on the wire".
         - L345-346: assert `"PL"` (was `"pl"`); reword message to
           "...canonicalized to uppercase per validateLanguage".
         - L357 inline comment "uppercase input → lowercase wire form" →
           "uppercase input → uppercase wire form" (input stays "PL").

    3. countries_test.go — subtest at L338:
         - L342 comment "languageIsoCode=pl (lowercased canonical form...)" →
           "languageIsoCode=PL (uppercase canonical form per validateLanguage)".
         - L347-348: assert `"PL"` (was `"pl"`); reword message to
           "expected canonicalized uppercase languageIsoCode in query".

    4. languages_test.go — subtest at L81:
         - L35 doc-comment bullet "(canonicalized lowercase)" → "(canonicalized uppercase)".
         - L87-88: assert `"EN"` (was `"en"`); reword message to
           "expected canonicalized uppercase languageIsoCode in query".
         - L91: change the inline mock response body's `"isoCode":"en"` is a
           RESPONSE body (not a wire param) — leave it as-is; only the query
           assertion changes. The input on L98 stays `"EN"`.
         - L96 comment "Uppercase input → validateLanguage canonicalizes to
           lowercase" → "...canonicalizes to uppercase \"EN\"".

    5. subdivisions_test.go — subtest at L137:
         - L39 doc-comment bullet "canonicalized to lowercase on the wire (D-55)"
           → "canonicalized to uppercase on the wire (D-55)". Keep the D-55 ID:
           D-55 is the empty-string-omission decision and remains accurate; only
           the lowercase wording is wrong.
         - Rename subtest "lowercased languageIsoCode reaches the wire" →
           "uppercase languageIsoCode reaches the wire".
         - L144-145 comment "...canonicalize to lowercase before url.Values.Set
           runs" → "...canonicalize to uppercase before url.Values.Set runs"
           (caller still passes "EN").
         - L146-147: assert `"EN"` (was `"en"`); reword message to
           "languageIsoCode must be canonicalized to uppercase before reaching the wire".
           Leave the `countryIsoCode` assertion (`"PL"`) untouched.

    6. cmd/ohcli/public_test.go — subtest at L215:
         - Rename "--lang reaches the wire as lowercase canonical form" →
           "--lang reaches the wire as uppercase canonical form".
         - L220 comment "Library canonicalizes the language code to lowercase." →
           "Library canonicalizes the language code to uppercase.".
         - L221-222: assert `"PL"` (was `"pl"`); reword message to
           "--lang must reach the upstream as uppercase per validateLanguage".
         - L230: change the CLI input arg from `--lang pl` to `--lang PL` so the
           subtest exercises the realistic case and the wire expectation `"PL"`
           is unambiguous regardless of input case.

    Do NOT touch update_fixtures_test.go (already uppercase, bypasses the
    validator) or any testdata/*.json fixture.
  </action>
  <verify>
    <automated>cd /data/git/private/holidays && go build ./... && go test ./... -run 'TestValidateLanguage|TestClient_PublicHolidays|TestClient_Countries|TestClient_Languages|TestClient_Subdivisions' 2>&1 | tail -30; echo "EXPECT: TestValidateLanguage (and the language-wire subtests) FAIL because production still lowercases — this proves the tests are RED before Task 2."</automated>
  </verify>
  <done>
    All six test files compile. `go test` for the listed tests shows the
    validateLanguage success cases and the five language-wire subtests FAILING
    (expected "PL"/"EN", got "pl"/"en"). Every reject case and every
    non-language assertion still passes. No testdata or update_fixtures_test.go
    change. This RED state is the precondition for Task 2.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Apply the fix — validateLanguage ToUpper + doc comment + annotate D-21 REVERSED</name>
  <files>validate.go, .planning/phases/01-foundation/01-CONTEXT.md, .planning/phases/01-foundation/01-VERIFICATION.md</files>
  <behavior>
    With production canonicalizing to uppercase, every RED test from Task 1
    turns GREEN, and the previously-passing reject/shape tests stay GREEN.
    - validateLanguage("pl"|"PL"|"Pl") → "PL", nil
    - validateLanguage("en") → "EN", nil; ("DE") → "DE", nil
    - W-01 fold cases ("KK","İa","İİ","Ka") still rejected with ErrInvalidLanguage
    - All five endpoint wire subtests now see the uppercase code.
  </behavior>
  <action>
    1. validate.go — single production change at L61: replace
       `return strings.ToLower(code), nil` with
       `return strings.ToUpper(code), nil`. This mirrors validateCountry exactly.
       Leave the `isTwoASCIILetters(code)` shape guard above it unchanged (W-01).

    2. validate.go — rewrite the `validateLanguage` doc comment (L40-52) to
       describe uppercase canonicalization, mirroring the validateCountry comment:
         - L40-42: "...canonicalizes a language ISO 639-1 alpha-2 code to
           uppercase and verifies it is exactly 2 ASCII letters in [A-Z].
           Returns the canonical (uppercase) form, which is what the
           OpenHolidays API expects on the wire."
         - L50: "Accepts any input case (\"pl\", \"PL\", \"Pl\" all map to
           \"PL\")." — drop the trailing "per D-21" or change it to note the
           reversal; D-21's lowercase choice is being reversed by this fix.
         - Update the W-01 comment inside the function body (L54-57) that
           references "strings.ToLower canonicalizes them" → "strings.ToUpper
           canonicalizes them", keeping the U+212A / U+0130 examples accurate
           (these fold to ASCII under ToUpper too, so they must still be rejected
           by the pre-fold shape guard).

    3. .planning/phases/01-foundation/01-CONTEXT.md — annotate the D-21 entry at
       L57 as REVERSED. Append after the existing D-21 line (do not delete the
       original — preserve the audit trail, consistent with the project's
       fix-forward ethos):
         "**[REVERSED 2026-05-30, quick task 260530-dvc]** validateLanguage now
         canonicalizes to UPPERCASE via strings.ToUpper, not lowercase. Verified
         live: the OpenHolidays API is case-sensitive and lowercase
         languageIsoCode silently returns English-only names. See validate.go
         and the matching validateCountry analog."

    4. .planning/phases/01-foundation/01-VERIFICATION.md — update the D-21 row at
       L175 ("validateLanguage canonicalizes to lowercase") to note the reversal,
       e.g. append " — REVERSED to uppercase 2026-05-30 (quick 260530-dvc); API
       is case-sensitive, lowercase returned English." Keep the row; do not
       delete history.

    Do NOT edit CHANGELOG.md (Release Please owns it). Do NOT touch 01-RESEARCH.md
    or the 01-05 / 02-04 PLAN files — those are historical execution records;
    the decision-of-record (01-CONTEXT.md) and its verification
    (01-VERIFICATION.md) are the two places the reversal is annotated.
  </action>
  <verify>
    <automated>cd /data/git/private/holidays && gofmt -l validate.go && go build ./... && go test -race ./... 2>&1 | tail -20 && echo "--- gofmt (empty=clean) ---" && gofmt -l . && echo "--- golangci-lint ---" && golangci-lint run ./... && echo "--- Examples ---" && go test -run Example ./... 2>&1 | tail -5</automated>
  </verify>
  <done>
    validate.go:61 reads `return strings.ToUpper(code), nil`. The shape guard is
    unchanged. `go test -race ./...` is fully GREEN (every Task-1 RED test now
    passes; all reject/shape/W-01 tests still pass). `gofmt -l .` prints nothing.
    `golangci-lint run` reports no issues. `go test -run Example ./...` is green.
    D-21 is annotated REVERSED in 01-CONTEXT.md and 01-VERIFICATION.md with the
    original text preserved.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| caller → validateLanguage | Untrusted ISO-code string crosses here before any HTTP request |
| validateLanguage → OpenHolidays API | Canonicalized code crosses to the case-sensitive public upstream |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-dvc-01 | Tampering | validateLanguage Unicode-fold bypass (W-01) | mitigate | `isTwoASCIILetters(code)` shape-check runs on ORIGINAL bytes BEFORE `strings.ToUpper`; guard and its position kept unchanged by this fix. Reject-case tests (incl. U+212A, U+0130 fold cases) preserved verbatim. |
| T-dvc-02 | Information Disclosure | validator error messages | accept | Error wraps `ErrInvalidLanguage` with `%q` of the ORIGINAL value only (ERR-04). Unchanged by this fix; TestValidators_NoSensitiveData still guards it. |
| T-dvc-03 | Spoofing | silent-language-fallback (the bug itself) | mitigate | Canonicalizing to uppercase makes the upstream honor the requested language instead of silently substituting English — closes a data-correctness defect, not a classic security threat, but it is the core integrity issue this plan fixes. |
| T-dvc-SC | Tampering | npm/pip/cargo installs | n/a | No package installs. Zero new dependencies (Gold Rule: zero runtime deps). No legitimacy gate required. |
</threat_model>

<verification>
Phase-level (run from repo root /data/git/private/holidays):

- `go test -race ./...` — fully green (unit + httptest mocks across library and cmd/ohcli).
- `gofmt -l .` — prints nothing (clean).
- `golangci-lint run ./...` — no issues (govet, errcheck, staticcheck, gosec, revive, gocritic).
- `go test -run Example ./...` — green (doc examples still compile/run).
- Manual ground-truth already established live this session — do NOT re-hit the network.

Goal-backward check: with the fix in place, `ohcli public PL 2025 --lang PL`
sends `languageIsoCode=PL`, upstream returns Polish names, `NameFor("PL")`
(case-insensitive) finds the PL entry — user sees Polish, not English.
</verification>

<success_criteria>
- validate.go:61 canonicalizes via `strings.ToUpper(code)` (mirrors validateCountry).
- `isTwoASCIILetters` W-01 shape guard unchanged and still first in the function.
- validate_test.go `TestValidateLanguage` success cases assert uppercase canonical form; all reject cases unchanged and passing.
- Five wire-assertion subtests (public_holidays, countries, languages, subdivisions, cmd/ohcli/public) assert the uppercase languageIsoCode, with names/comments reworded off "lowercase".
- No testdata fixture and no update_fixtures_test.go change.
- D-21 annotated REVERSED in 01-CONTEXT.md and 01-VERIFICATION.md (original text preserved).
- `go test -race ./...`, `gofmt -l .`, `golangci-lint run`, `go test -run Example ./...` all clean.
- Commit subject: `fix(validate): canonicalize language ISO code to uppercase to match OpenHolidays API` (English; body notes silent-English-fallback bug and that it reverses D-21). CHANGELOG untouched.
</success_criteria>

<output>
Create `.planning/quick/260530-dvc-fix-validatelanguage-to-uppercase-langua/260530-dvc-SUMMARY.md` when done.
</output>
