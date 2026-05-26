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

## Conventions evolve

These conventions are the floor, not the ceiling. New conventions discovered during implementation (idioms, lint rules, fixture patterns, naming schemes) belong in this file. Update freely and commit.

---
*Last updated: 2026-05-27 after gold project rules were added.*
