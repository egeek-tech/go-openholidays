# Conventions

These conventions are non-negotiable. They take precedence over any contrary suggestion in research, agent recommendations, or generated plans. Any deviation requires explicit user approval logged in PROJECT.md `Key Decisions`.

## Gold Project Rules

### Rule 1 — Everything in English

All code, code comments, package documentation, godoc strings, test names, commit messages, CHANGELOG entries, README, design docs, and ADRs must be written in English. No Polish, no mixed-language identifiers, no non-ASCII identifiers.

**Why**: The library targets a global OSS audience and is intended for `pkg.go.dev`. Mixed-language sources block contributors and reviewers, and break `golint`/`revive` style expectations.

**Applies to**: Every `.go`, `.md`, `.yaml`, `.sh`, `Makefile`, commit message, and PR description in the repo. Test data fixtures in `testdata/` may contain non-English strings only when they reflect real upstream API responses (e.g., Polish holiday names like `"Wigilia Bożego Narodzenia"` come straight from OpenHolidays).

### Rule 2 — Never guess; verify or ask

If something is not known with confidence, do not write it as if it were. Either:
- Verify it (read the source file, run the command, hit the endpoint, check the upstream OpenAPI spec).
- Or stop and ask the user.

**Why**: Confidently-stated guesses produce silent bugs and erode trust. Costs one tool call to verify; costs a debug cycle to fix a wrong guess.

**Applies to**:
- API claims ("the endpoint returns X") — verify by reading the OpenAPI spec or hitting the live endpoint.
- Code claims ("function Y returns Z") — read the source.
- Test claims ("this assertion passes") — run the test.
- Behavior claims ("the retry honors `Retry-After`") — exercise it under a fake transport.

Words like *"I think"*, *"probably"*, *"should be"*, *"most likely"* in a draft response signal stop-and-verify before sending. If verification would take longer than asking, ask first.

### Rule 3 — Test conventions (testify + one-test-per-prod-function + t.Run)

Tests use `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require` as the assertion libraries. They are test-only dependencies (added under `require` but only imported by `*_test.go` files; `go mod why github.com/stretchr/testify` must show only test imports).

**Structure**:
1. **One `TestFunction` per production function**. If `holidays.go` exports `func (c *Client) PublicHolidays(...)`, the test file has exactly one `func TestClient_PublicHolidays(t *testing.T)`. No more, no fewer. Internal helpers may have their own tests when they merit independent coverage, but the public-function-to-test-function ratio stays 1:1.
2. **Each test case lives inside a `t.Run(name, func(t *testing.T) { ... })`**. No top-level assertions in the outer `TestXxx` body. This makes table-driven tests scale and lets `go test -run TestClient_PublicHolidays/cancel_mid_request` target a single case.
3. **`require` for preconditions, `assert` for verifications**. `require.NoError(t, err)` aborts the case when setup fails; `assert.Equal(t, want, got)` reports without aborting so multiple checks in a case still run.
4. **Table-driven by default** when more than 2 cases share setup. Each row in the table maps to one `t.Run` call.

**Why**:
- `testify` is the de-facto Go test framework in 2025/2026; raw `if got != want { t.Fatalf(...) }` is allowed but tedious for SDKs with rich result types.
- One-test-per-prod-function makes `go test -run TestXxx` predictable and matches IDE "go to test" navigation.
- `t.Run` per case lets CI report each case as a separate row, enables `-run` filtering, and gates failures per-case under `-failfast`.

**Constraint update**: This rule expands the test-only dependency policy in PROJECT.md. Approved test-only deps:
- `github.com/stretchr/testify` (assert + require) — primary assertion library.
- `github.com/google/go-cmp` — deep-equal diffs when testify's output is insufficient (rare).

Any further test-only dep requires user approval and a `Key Decisions` entry.

**Example layout** (illustrative, not a target file):

```go
func TestClient_PublicHolidays(t *testing.T) {
    t.Parallel()

    cases := []struct {
        name    string
        req     PublicHolidaysRequest
        server  http.HandlerFunc
        wantLen int
        wantErr error
    }{
        {name: "happy path PL 2025", req: ..., wantLen: 14},
        {name: "invalid country", req: ..., wantErr: ErrInvalidCountry},
        {name: "5xx triggers retry exhaustion", server: ..., wantErr: ...},
        {name: "ctx cancel mid request", req: ..., wantErr: context.Canceled},
        {name: "malformed JSON", server: ..., wantErr: ...},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            srv := httptest.NewServer(tc.server)
            t.Cleanup(srv.Close)

            c := NewClient(WithBaseURL(srv.URL))
            got, err := c.PublicHolidays(t.Context(), tc.req)

            if tc.wantErr != nil {
                require.ErrorIs(t, err, tc.wantErr)
                return
            }
            require.NoError(t, err)
            assert.Len(t, got, tc.wantLen)
        })
    }
}
```

### Rule 4 — Published releases and tags are immutable; fix forward only

Once a release tag (`vX.Y.Z`) has been pushed to origin, it is **frozen**. Once a GitHub Release has been transitioned out of draft to published state, it is **frozen**. The only sanctioned action on a frozen release is to read it; the only sanctioned response when something is wrong with one is to cut a new release that supersedes it.

**Forbidden**:
- `git push --force` on any tag ref under `refs/tags/v*`
- `git push origin :refs/tags/vX.Y.Z` (deleting a tag from origin)
- `gh release delete vX.Y.Z` on a release that is NOT in draft state
- Editing the body / notes / title / asset list of a published release
- "Re-issuing" a tag with different content (delete + recreate)
- Force-pushing master after a release tag has been cut from it
- Amending a `CHANGELOG.md` entry that corresponds to an already-published version (write a new entry that documents the fix instead)

**Allowed**:
- Discarding a release that is still `draft: true` — drafts are not yet reachable by consumers, so cleaning them up is a one-time pre-publication action. Example: the v0.2.4 zombie draft was deletable in the window before any consumer fetched.
- Cutting a new release that supersedes a broken one (`v0.2.5` supersedes `v0.2.4`; the broken release stays as historical record).
- Adding clarifying comments to `docs/release-runbook.md` §8 "Release history" describing what went wrong with a prior release.

**Why**:
- `go get` and the Go module proxy cache module bytes by `(module, version)`. Once any caller has fetched `vX.Y.Z`, the bytes they pulled are committed to their local cache and the upstream proxy keeps serving them indefinitely. Rewriting a tag does **not** retract those bytes — it just makes the upstream and downstream disagree.
- SLSA / sigstore attestations chain to the specific commit + tag + workflow run that produced them. Rewriting a tag breaks every attestation that referenced it; `gh attestation verify` against the rewritten artifact would fail with a transparency-log mismatch.
- Branch protection bypass logs and audit logs reference prior state by SHA. Rewriting introduces "ghost" entries the audit cannot cross-reference.
- Trust signal: "we ship and stand behind our releases" beats "we ship and silently rewrite when something looks wrong" by a wide margin for OSS consumers evaluating the library.

**How to recover from a bad release** (the only sanctioned path):
1. Do not delete or rewrite the bad tag/release. Leave it as historical record.
2. Open a `fix:` PR with the correct change.
3. Let Release Please cut the next version (e.g. `v0.2.5` supersedes a broken `v0.2.4`).
4. Document the bad release in `docs/release-runbook.md` §8 "Release history" and (if helpful) §6 "Known issues" so future operators know what happened.

**Precedents (proof this rule is load-bearing)**:
- `v0.2.0` shipped with 0 binary assets (GITHUB_TOKEN cascade gap)
- `v0.2.1` shipped with 0 binary assets (goreleaser `trimPrefix` template bug)
- `v0.2.2` shipped with 0 binary assets (immutable-release 422 on goreleaser upload)
- `v0.2.4` is a stray draft created by an aborted release run
- **None of these have been deleted or rewritten**. `v0.2.3`, `v0.2.5+` are the fix-forward responses; the broken releases remain on the Releases tab as audit trail.

### Rule 5 — `audit:ok` marks certify reviewed logic; modifying a function invalidates its mark

A production function may carry a single `// audit:ok YYYY-MM-DD` comment line directly above it, certifying that its logic was deliberately reviewed and found correct on that date (correct behavior matching its name/doc, sound error handling, no flaggable hardcoded values, follows these Gold Rules). The mark sits *above* the function and is separated from the godoc doc comment by a blank line, so it never renders on `pkg.go.dev`:

```go
// audit:ok 2026-05-30

// NameFor returns the localized holiday name ...
func (h Holiday) NameFor(lang string) string {
```

**Required**: any change to a function's code or behavior MUST delete that function's `// audit:ok` line in the same commit. The function must be re-audited before a new mark is added.

**Exempt**: pure doc-comment / typo fixes that do not change behavior.

**Why**: the mark is a *freshness* signal, not a permanent badge. Its entire value rests on the invariant that a marked function's logic is exactly what was reviewed. The moment the body changes, a retained mark falsely certifies logic that was never reviewed in its current form — worse than no mark at all.

**Enforcement**: by convention today (this rule + code review). A CI / pre-commit guard that fails when a changed function still carries its prior mark is a noted future follow-up, not yet wired.

## Style

### Em-dashes ("—", U+2014) in godoc and Markdown are deliberate

Source files use em-dashes (`—`, U+2014) for parenthetical breaks in
godoc comments and Markdown prose. This is a deliberate choice, ratified
2026-05-28 after Phase 01 review IN-02:

- Em-dashes are valid Unicode and round-trip cleanly through `gofmt`,
  `go doc`, `pkg.go.dev`, and every modern terminal.
- Replacing them with ASCII `--` or ` - ` would touch many files for
  pure-stylistic churn with no mechanical benefit.
- The Gold Rule 1 ("everything in English") concerns *identifiers and
  prose language*, not the punctuation glyphs used inside English prose.

This applies ONLY to godoc / Markdown prose. **Code itself stays ASCII**:
no non-ASCII identifiers, no non-ASCII string literals in production
code (test fixtures in `testdata/` may carry non-ASCII strings only when
they reflect real upstream API responses, per Rule 1's existing carve-out).

If a future contributor wants strict-ASCII source files, the bar is to
update this section and either land a one-shot mechanical replacement
across every file or to ship a lint rule that prevents re-introduction.

## Conventions evolve

These conventions are the floor, not the ceiling. New conventions discovered during implementation (idioms, lint rules, fixture patterns, naming schemes) belong in this file. Update freely and commit.

---
*Last updated: 2026-05-30 — Rule 5 added: `audit:ok` marks certify reviewed logic; modifying a function invalidates its mark.*
