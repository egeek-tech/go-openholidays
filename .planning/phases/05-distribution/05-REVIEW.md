---
phase: 05-distribution
reviewed: 2026-05-28T00:00:00Z
depth: standard
files_reviewed: 51
files_reviewed_list:
  - .github/CODEOWNERS
  - .github/dependabot.yml
  - .github/workflows/ci.yml
  - .github/workflows/integration.yml
  - .github/workflows/release.yml
  - .golangci.yml
  - .goreleaser.yaml
  - CHANGELOG.md
  - CONTRIBUTING.md
  - README.md
  - bench_test.go
  - cache_test.go
  - client.go
  - client_isinregion_test.go
  - client_test.go
  - clock_test.go
  - cmd/ohcli/countries.go
  - cmd/ohcli/countries_test.go
  - cmd/ohcli/format.go
  - cmd/ohcli/format_test.go
  - cmd/ohcli/main.go
  - cmd/ohcli/main_test.go
  - cmd/ohcli/public.go
  - cmd/ohcli/public_test.go
  - cmd/ohcli/school.go
  - cmd/ohcli/school_test.go
  - cmd/ohcli/version.go
  - config.go
  - countries_test.go
  - date_test.go
  - doc.go
  - docs/design.md
  - docs/release-runbook.md
  - errors.go
  - errors_test.go
  - example_test.go
  - fuzz_test.go
  - holiday_test.go
  - integration_test.go
  - languages_test.go
  - public_holidays_test.go
  - request.go
  - request_test.go
  - retry_test.go
  - school_holidays_test.go
  - subdivisions_test.go
  - transport_cache_test.go
  - transport_hook_test.go
  - types_test.go
  - validate.go
  - validate_test.go
findings:
  critical: 3
  warning: 12
  info: 6
  total: 21
status: issues_found
---

# Phase 5: Code Review Report

**Reviewed:** 2026-05-28
**Depth:** standard
**Files Reviewed:** 51
**Status:** issues_found

## Summary

Adversarial review surfaces a mix of pre-release blockers (README quickstart
is not compilable as written, broad action-version drift between
documentation and the actual YAML, exit-code policy gap in the CLI handler)
plus several correctness hazards in the new argv-reordering helper that the
dispatcher pipes through every subcommand. The release workflow itself is
well-locked (no `pull_request_target`, no `${{ github.event.* }}`
interpolation, explicit `id-token: write` only where needed), and the
goreleaser config is internally consistent — but the documentation
substrate that surrounds both is materially out of sync with the
actually-shipped files, and that drift will compound at the v0.1.0 tag
moment when the runbook diverges from reality.

Findings are classified per the standard CR/WR/IN scheme:
- **CR** must be fixed before tagging v0.1.0;
- **WR** should be fixed before tagging or explicitly deferred with a Key
  Decision entry;
- **IN** items are advisory.

No security vulnerabilities were detected. The workflow audit cleared (no
unsafe `${{ github.event.* }}` interpolation, no `pull_request_target`
trigger, no untrusted-input-to-shell patterns, `id-token: write` scoped
per-job, attestation chain integrity unbroken).

## Critical Issues

### CR-01: README Quickstart code block fails to compile as written (missing imports + missing main)

**File:** `README.md:19-34`
**Issue:** The README "Quickstart" code block uses `context.WithTimeout`,
`context.Background`, `time.Second`, and `fmt.Println` / `fmt.Printf` but
contains no `import` block and no `package main` / `func main()` skeleton.
It is presented as if it were a paste-and-run program. Consumers landing
from pkg.go.dev will copy this exact block, paste it into `main.go`, get
unresolved-identifier errors from `go build`, and assume the library is
broken.

Plan DOC-01 explicitly required the quickstart be runnable;
`example_test.go::Example_quickstart` is runnable because the
`openholidays_test` package supplies imports — that import substrate is
invisible to a copy-paster reading the README. The two MUST stay
byte-for-byte identical in their executable substance, but the README
fragment is missing the wrapping context that makes the substance
executable.

**Fix:** Wrap the README block in a complete `package main` + `import`
skeleton so the quickstart compiles unmodified:

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/egeek-tech/go-openholidays"
)

func main() {
    c := openholidays.NewClient()
    defer func() { _ = c.Close() }()
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    hs, err := c.PublicHolidays(ctx, openholidays.PublicHolidaysRequest{
        CountryIsoCode: "PL",
        ValidFrom:      openholidays.NewDate(2025, time.January, 1),
        ValidTo:        openholidays.NewDate(2025, time.December, 31),
    })
    if err != nil {
        fmt.Println("error:", err)
        return
    }
    fmt.Printf("got %d Polish public holidays\n", len(hs))
}
```

Then add a `go test -run Example_quickstart` check to the release-runbook
section 1 pre-tag checklist so the two stay synchronized at every release.

### CR-02: `ohcli` exit-code policy violates D-06 for library validation errors (returns exit 1 where exit 2 is required)

**File:** `cmd/ohcli/public.go:90-94`, `cmd/ohcli/school.go:86-90`
**Issue:** The handlers treat every error returned by `Client.PublicHolidays`
/ `Client.SchoolHolidays` as exit code 1 (runtime error per D-06). But the
library returns wrapped client-side validation errors —
`ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrInvalidDateRange`,
`ErrDateRangeTooLarge` — which are semantically "usage errors" (D-06 maps
usage errors to exit 2). A caller invoking `ohcli public XX 2025` with an
invalid country code gets exit 1, masking the fact that the input was
malformed and breaking the documented 3-tier POSIX exit-code contract.

The CLI already enforces partial input validation client-side (year bounds
check at `public.go:66`) and returns exit 2 — but anything that flows past
the CLI's local checks into the library's `validateCountry` /
`validateLanguage` / `validateDateRange` gates loses the distinction.

`05-RESEARCH.md §"User Constraints" D-06` is explicit: "validation rejection"
belongs to the runtime-error bucket, NOT the usage-error bucket. Reading
that literally, the implementation is correct — but the literal reading
contradicts every reasonable downstream consumer's expectation. The
research note may itself be in error; either way the issue surfaces as
"library validation errors flow through exit 1," which is at minimum
documentation-unfriendly to shell-script integrators.

**Fix:** Add a sentinel-error switch after each library call:
```go
if err != nil {
    switch {
    case errors.Is(err, openholidays.ErrInvalidCountry),
         errors.Is(err, openholidays.ErrInvalidLanguage),
         errors.Is(err, openholidays.ErrInvalidDateRange),
         errors.Is(err, openholidays.ErrDateRangeTooLarge):
        fmt.Fprintf(stderr, "ohcli: %v\n", err)
        return 2
    default:
        fmt.Fprintf(stderr, "ohcli: %v\n", err)
        return 1
    }
}
```
Apply identically to `cmd/ohcli/school.go:86-90` and to
`cmd/ohcli/countries.go:70-73`. If D-06's "validation rejection = exit 1"
wording is in fact load-bearing, then alternatively a Key Decision entry
should call out the literal wording so future maintainers don't try to
"fix" the perceived bug. The current code does neither, which is the
defect.

### CR-03: Action-version pins drift from documentation comments and Key Decision CL-18 — every workflow comment block describes a different version from what `uses:` actually references

**File:** `.github/workflows/release.yml:34-35,52,56,61`, `.github/workflows/ci.yml:14-15,47,49,78,90,91,94,103,104`, `.github/workflows/integration.yml:24,39,41`, `.github/dependabot.yml:7-9`, `.planning/PROJECT.md` CL-18, `.planning/phases/05-distribution/05-05-SUMMARY.md:182-183`
**Issue:** Multiple sources of truth disagree on which action versions
the project pins. Concrete drift across the surface:

| Action | Comment / Key Decision says | Actual `uses:` | Provenance |
|---|---|---|---|
| `actions/checkout` | `@v4` | `@v6` | Dependabot PR #4, commit `9b3f7fd` |
| `actions/setup-go` | `@v5` | `@v6` | Dependabot PR #1, commit `16242eb` |
| `goreleaser/goreleaser-action` | `@v6` | `@v7` | Dependabot PR #3, commit `e022478` |
| `codecov/codecov-action` | `@v5` | `@v6` | Dependabot PR #2, commit `2bad5a6` |
| `golangci/golangci-lint-action` | `@v7` | `@v7` | Open Dependabot bump `877de12` to `@v9` not yet merged |
| `actions/attest-build-provenance` | `@v4` | `@v4` | Consistent |
| `golang/govulncheck-action` | `@v1` | `@v1` | Consistent |

The drift means:
1. A release-troubleshooter following CL-18 or the Source-current notes in
   `05-RESEARCH.md` to reason about a CI failure will read docs that
   describe behaviors not necessarily present in the actually-pinned major
   version (e.g. `codecov-action@v5`'s OIDC contract may differ from
   `@v6`'s).
2. `setup-go@v6` and `checkout@v6` are MAJOR-version bumps whose changelogs
   often introduce breaking changes (default cache behavior, action.yml
   key renames). The workflow has not been re-validated against the new
   majors — Dependabot bumps were merged without an accompanying CI run on
   the merge commit.
3. The `dependabot.yml` comment block (line 7-9) lists the SAME stale
   versions, so Dependabot's tracking targets are inconsistent with the
   actual file state — though Dependabot reads the YAML, not the comments.

**Fix:** Choose one of:
1. **Pin to documented versions and revert the Dependabot bumps**: revert
   `9b3f7fd` / `16242eb` / `2bad5a6` / `e022478` and adopt a policy of
   reviewing Dependabot bumps before merging.
2. **Accept the bumps and update every documentation source**: update CL-18
   in PROJECT.md, the workflow header comments in all three workflow
   files, the `dependabot.yml` comment block, the runbook references, and
   the 05-05-SUMMARY references to match the actually-pinned versions.
   Then verify each bumped action's behavior against the workflow's
   expectations (specifically: does `codecov-action@v6` still support
   `use_oidc: true` the same way as `@v5`? Does `setup-go@v6` need
   explicit cache config? Does `goreleaser-action@v7` accept the same
   `args: release --clean` shape?).

Path 2 is recommended for v0.1.0 — Dependabot will keep re-proposing
bumps, and the docs-vs-pin drift is the larger long-term cost than
re-validating once.

## Warnings

### WR-01: `.goreleaser.yaml` `mod_timestamp` shape unverified against goreleaser v2's accepted template variables — release-time failure would happen AFTER the tag has been pushed

**File:** `.goreleaser.yaml:48`
**Issue:** The config declares:
```yaml
mod_timestamp: "{{ .CommitTimestamp }}"
```
Planning artifacts (`05-RESEARCH.md`, `05-06-PLAN.md`, `05-06-SUMMARY.md`)
all use this exact form, so the planner believed it documented and
correct. Whether `.CommitTimestamp` IS a documented goreleaser v2 template
variable that resolves to a unix-epoch integer in the form goreleaser
expects for `mod_timestamp` was not independently verified during this
review (Gold Rule 2 — verify or ask). The runbook
(`docs/release-runbook.md:42-43`) lists `goreleaser check` as "optional"
in section 1.

Because this is the first time the config will be exercised, any template
mismatch will manifest only at the v0.1.0 tag-push moment, AFTER the tag
is permanently in the public reflog. Rollback then requires bumping to
v0.1.1.

**Fix:** Promote the local `goreleaser check` and `goreleaser release
--snapshot --clean --skip=publish` rehearsal commands to MANDATORY in
section 1 of the runbook (currently "(Optional)"). Run them as part of the
pre-tag checklist on any machine with goreleaser v2 installed. The
runbook section 2 (dry-run rehearsal with an `-rc.1` tag) is already
"strongly recommended"; for v0.1.0 specifically, treat it as required, not
recommended. If `.CommitTimestamp` resolves to an unexpected shape, the
local check will surface it before the public tag is pushed.

### WR-02: `defer c.Close()` ignores the error return in every user-visible Example — codifies a pattern downstream callers will copy when Close evolves to return real errors

**File:** `example_test.go:34,54,76,98,118,136,158,180`, `README.md:21`
**Issue:** `Client.Close` returns `error` (signature at `client.go:128`).
Every Example in `example_test.go` uses bare `defer c.Close()`. The
README quickstart inherits the same pattern. Currently `Close` always
returns nil so the practical defect surface is small — but the example
codifies a pattern downstream consumers will copy at scale.

Note the asymmetry: the CLI handlers (`cmd/ohcli/public.go:82`,
`cmd/ohcli/school.go:77`, `cmd/ohcli/countries.go:67`) ALREADY use the
explicit-discard form `defer func() { _ = c.Close() }()`. So the library's
own code knows the right pattern; the user-facing documentation uses the
wrong one.

`errcheck` is in the mandated linter set (`.golangci.yml:11`) and SHOULD
flag bare `defer c.Close()`. If `golangci-lint run` is currently passing,
then either errcheck has been configured to ignore Examples (it has not)
or there is a bug in the lint pass.

**Fix:** Change every `defer c.Close()` in `example_test.go` and in
`README.md` to `defer func() { _ = c.Close() }()`. Then re-run
`golangci-lint run` to confirm the lint pass still exits 0.

### WR-03: `reorderArgs` mishandles negative-value flag arguments (`--offset -5`) — the godoc presents the "doesn't start with -" rule as a correctness invariant rather than a known limitation

**File:** `cmd/ohcli/main.go:136-174`
**Issue:** Line 164:
```go
if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] != '-' {
    flags = append(flags, a, args[i+1])
    i++
    continue
}
```
This treats any token starting with `-` as a flag, never as a value. A
future addition of a non-bool numeric flag like `--offset` would break
when the caller writes `ohcli ... --offset -5` — the `-5` would be
recognized as a flag rather than the value of `--offset`. The current CLI
surface (`--lang`, `--format`, `--region`) never takes a value that
legitimately starts with `-`, so the helper is correct TODAY, but the
godoc reads as if the "doesn't start with -" rule is universal.

This pre-1.0 limitation needs explicit documentation so a future
contributor adding `--year-offset` or a file-path flag doesn't get bit by
it.

**Fix:** Document the limitation explicitly in the godoc on `reorderArgs`.
If future flags need negative values, change the recognition to consume
the next arg regardless of leading `-` for known-non-bool flag names: the
`boolFlags` map already exists; add a `stringFlags` mirror.

### WR-04: `reorderArgs` treats `--` (double-dash) as a positional, losing the POSIX end-of-flags semantic

**File:** `cmd/ohcli/main.go:142-145`
**Issue:** The godoc says:
> Bare "-" and "--" are positional per stdlib flag convention

That is half-true. The stdlib `flag` package treats `--` as a
**terminator**: every token AFTER `--` is a positional, regardless of
whether it starts with `-`. `reorderArgs` collapses `--` into a positional
without ever interpreting its terminator semantics — so a caller writing
`ohcli public PL 2025 -- --not-a-flag` will see `--not-a-flag` re-routed
into the flag slot by `reorderArgs`, then `flag.Parse` will complain about
`--not-a-flag` being unrecognized.

For the current CLI surface this is harmless — no positional value
legitimately begins with `--` — but it diverges from documented stdlib
behavior and demonstrates the helper is not a drop-in equivalent of stdlib
parsing.

**Fix:** Implement the terminator: when `--` is encountered, flush
remaining tokens to `positionals` without further inspection. The change
is one early-exit branch at the top of the loop:

```go
if a == "--" {
    // POSIX end-of-flags: everything after this is positional, period.
    positionals = append(positionals, args[i+1:]...)
    break
}
if a == "-" || len(a) < 2 || a[0] != '-' {
    positionals = append(positionals, a)
    continue
}
```

### WR-05: `cmdPublic` / `cmdSchool` year-parsing accepts surprising inputs (leading zeros, `+` sign, etc.) — `strconv.Atoi` is too permissive for what the help-text presents as `<year>`

**File:** `cmd/ohcli/public.go:65-69`, `cmd/ohcli/school.go:60-64`
**Issue:** `strconv.Atoi("2025")` works. `strconv.Atoi("02025")` returns
`2025` (Atoi accepts leading zeros without complaint). `strconv.Atoi("+2025")`
returns `2025`. The "validate" branch then bounds-checks against
`1900..2100`, so the surprising-but-equivalent forms pass. This is mostly
harmless, but the help/usage text shows `<year>` and the user is
reasonably entitled to expect strict parsing.

**Fix:** Require exactly 4 ASCII digits before calling Atoi. Add a
manual length-and-charset check or a regex precondition:

```go
y := fs.Arg(1)
if len(y) != 4 || y[0] < '0' || y[0] > '9' || /* … */ {
    fmt.Fprintf(stderr, "ohcli: invalid year %q (want 4 ASCII digits)\n", y)
    return 2
}
year, _ := strconv.Atoi(y)
```

### WR-06: `docs/design.md` invariant paragraph names the rejected chain order in a way that risks misleading future readers

**File:** `docs/design.md:46`
**Issue:** The RESIL-05 invariant paragraph reads:
> Invariant (RESIL-05 / STATE.md Key Decisions): **retry lives in the
> endpoint layer, NOT as a RoundTripper.** Placing retry inside the chain
> — e.g. an order like `retry → cache → hook → logging → header → base`
> — would double-retry when callers supply their own retrying
> `*http.Client` via `WithHTTPClient`. The shipped chain therefore lifts
> retry out of the RoundTripper layer entirely and runs it inside
> `request.go::doJSONGet` against the composed `*http.Client`.

This paragraph is structurally correct (the rejected order is named to
satisfy a literal grep) but a reader who skims the chain diagram in the
section above (line 31-36: `hook → cache → logging → header → base`) and
then sees a DIFFERENT chain string a few lines later will reasonably
wonder which one is real. The qualifier "an order like" is doing a lot of
work but is easy to miss.

**Fix:** Reword to make the contrast explicit:
> The retry layer is **deliberately excluded** from the chain. A
> hypothetical chain that included retry — for example
> `retry → cache → hook → logging → header → base` — would double-retry
> when callers supply their own retrying `*http.Client` via
> `WithHTTPClient`. The shipped chain (see diagram above) therefore lifts
> retry out of the RoundTripper layer entirely and runs it inside
> `request.go::doJSONGet` against the composed `*http.Client`.

The word "hypothetical" + the back-reference to the diagram makes the
contrast unambiguous.

### WR-07: `CHANGELOG.md` is a stub pointer file but DOC-05's requirement is for an actual changelog — pre-tag links into it land on a stub with no v0.1.0 information

**File:** `CHANGELOG.md` (all 6 lines), `docs/release-runbook.md:212-216`
**Issue:** `CHANGELOG.md` contains nothing but a pointer to the GitHub
Releases page. Plan 05-RESEARCH.md D-12 documents that "if a top-level
CHANGELOG.md is still desired, it's a thin pointer file" — and that is
exactly what shipped. But the first published release (v0.1.0) won't have
a GitHub Release page until the tag is pushed and goreleaser runs.

Before that moment, every pre-tag link that points at "see CHANGELOG.md"
or "see the GitHub Releases page" lands on either a stub or a 404.
Downstream consumers (pkg.go.dev's default changelog inference, Dependabot
PR bodies, contributor questions like "what changed in v0.1.0?") will
reach this stub before the tag flow runs.

**Fix:** Add a single v0.1.0 entry to the CHANGELOG before tagging:
```markdown
## v0.1.0 (2026-05-DD)

Initial public release. See
https://github.com/egeek-tech/go-openholidays/releases/tag/v0.1.0
for the full conventional-commit-derived changelog.
```
~5 lines, resolves the pre-tag question without changing the
"goreleaser-generates-the-real-thing" policy.

### WR-08: `newClient` does not trim whitespace from `OPENHOLIDAYS_BASE_URL` — a test that accidentally injects trailing whitespace gets opaque URL parsing errors

**File:** `cmd/ohcli/main.go:102-104`
**Issue:** The check `if u := os.Getenv("OPENHOLIDAYS_BASE_URL"); u != ""`
filters out the empty-string case but accepts
`OPENHOLIDAYS_BASE_URL=" "` (single space) or `"\n"` (newline). The
library's `WithBaseURL` will store the whitespace and the first HTTP
request will fail with a URL parsing error opaque to a test author who
set the env var via `t.Setenv` accidentally with trailing whitespace.

**Fix:**
```go
if u := strings.TrimSpace(os.Getenv("OPENHOLIDAYS_BASE_URL")); u != "" {
    opts = append(opts, openholidays.WithBaseURL(u))
}
```

### WR-09: `client.go` `newClientRand` fallback comment claims "fill all 32 bytes of seed so ChaCha8 has full state diversity" — the implementation produces 32 bytes via two FNV-128a rounds with rotated inputs, which is correct but the "diversity" claim is overstated

**File:** `client.go:200-207`
**Issue:** The comment at line 187 says "fill all 32 bytes of seed so
ChaCha8 has full state diversity." The implementation uses `fnv.New128a`
(16-byte output) twice with rotated inputs (`tb||pb` then `pb||tb`). The
two halves are derived from the same two inputs (timestamp + pid) in
different orders. The avalanche effect of FNV-128a is moderate; a 64-bit
cluster of the seed may still correlate across the two halves.

For ChaCha8 jitter generation this is non-cryptographic and acceptable,
but the comment overpromises. More importantly, the path is so cold
(crypto/rand.Read on linux requires kernel-level catastrophic failure)
that spending engineering on a stronger fallback is misallocated. The
comment should be honest.

**Fix:** Weaken the wording of the comment block at `client.go:187-190` to
state truthfully: "fills all 32 bytes of seed from two FNV-128a rounds
with rotated inputs (timestamp + pid). FNV-128a is non-cryptographic but
adequate for jitter when crypto/rand is unavailable, which on any healthy
system means never."

### WR-10: `ohcliVersion` strips no `v` prefix from `info.Main.Version` — goreleaser-built and `go build` binaries emit different `ohcli version` output, even though W-05 was the fix specifically to prevent this divergence

**File:** `cmd/ohcli/version.go:33-39`, `.goreleaser.yaml:47`, `version.go`
**Issue:** The W-05 fix in `.goreleaser.yaml:47` uses `{{ trimPrefix "v"
.Version }}` to inject `0.1.0` (without `v`) into the binary's `Version`
constant — so the User-Agent header matches the `go build` form. But
`ohcliVersion` itself prefers `runtime/debug.ReadBuildInfo().Main.Version`
when that field is non-empty and not the `"(devel)"` sentinel. For a
goreleaser-built binary, `Main.Version` is `"v0.1.0"` (WITH the `v` —
this is the module version from the Go module proxy / build cache).

So:
- `ohcli version` from a goreleaser-built binary prints `v0.1.0` (with v).
- `ohcli version` from `go build .` falls through to the constant and
  prints `0.1.0` (no v).

The two outputs disagree on the leading `v`. The W-05 fix was specifically
to prevent this divergence — but it only addressed the User-Agent header,
not the `ohcli version` stdout.

**Fix:** Strip leading `v` from `info.Main.Version` in `ohcliVersion`:
```go
func ohcliVersion() string {
    info, ok := debug.ReadBuildInfo()
    if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
        return strings.TrimPrefix(info.Main.Version, "v")
    }
    return openholidays.Version
}
```
Add a test in `main_test.go::TestRun` that exercises both code paths and
asserts the leading character is not `v`.

### WR-11: `Client.Close` returns nil even when `cache.Close()` returns an error — the test surface does not cover the error-from-cache path and no log trace is emitted

**File:** `client.go:128-135`
**Issue:** The contract is fine, but the documented "errors are
intentionally swallowed" behavior is unobservable to a caller. If
`cache.Close()` returns an error (a third-party Cache implementation
backed by Redis, say), the user has no signal — not even a slog Debug
event. The test surface in `client_test.go` doesn't appear to exercise
this path. The library's own `MemoryCache.Close()` likely returns nil
always, so the path is effectively unreachable in v0.1.0; but CL-15
exports the `Cache` interface for third-party implementations, and the
silent-swallow contract gives those implementations no path to surface
shutdown errors.

**Fix:** Add a slog Debug emit inside the closeOnce when `cache.Close()`
returns a non-nil error:
```go
c.closeOnce.Do(func() {
    if c.cache != nil {
        if err := c.cache.Close(); err != nil {
            c.cfg.logger.Debug("cache.Close returned error", "error", err)
            // intentionally swallowed per documented contract
        }
    }
})
```
(May require keeping a logger reference on Client; alternatively, just
add a TODO comment for v0.2.0 to revisit the silent-swallow.)

### WR-12: `format.go::renderCSV` and `renderCountriesCSV` check `cw.Write` returns inside the loop but `encoding/csv` buffers writes — these checks are effectively dead, but lint-required by errcheck

**File:** `cmd/ohcli/format.go:100-123,176-192`
**Issue:** `csv.Writer.Write` for *encoding/csv* buffers internally and
returns nil unless the underlying `io.Writer` returned an error inline
(which a `bytes.Buffer` will never do; a `*os.File` will only on certain
filesystem failures). The Write check at line 102-104 (header) and the
Write check inside the per-row loop at line 110-117 effectively never
fire — errors genuinely surface at `cw.Flush()` + `cw.Error()`.

The current behavior is correct (errors DO eventually surface), but the
appearance of multiple error-check sites where most are dead is
confusing. errcheck (in the lint set) requires the checks, so they cannot
simply be dropped without a `//nolint` annotation.

**Fix:** Either (a) accept the present shape and add a brief comment
above the first Write check noting that errcheck requires the check
despite the Write buffering, or (b) refactor to a single helper that
batches writes and returns the Flush error. Recommend (a) — option (b) is
over-engineering for the line count.

## Info

### IN-01: `cmd/ohcli/main.go::hasByte` is a hand-rolled wrapper around `strings.IndexByte` — pulling stdlib for one function is canonical Go, not a footgun

**File:** `cmd/ohcli/main.go:178-185`
**Issue:** `hasByte` is implemented as a loop:
```go
func hasByte(s string, b byte) bool {
    for i := range len(s) {
        if s[i] == b {
            return true
        }
    }
    return false
}
```
This is equivalent to `strings.IndexByte(s, b) >= 0`. The comment claims
"Inlined here to keep reorderArgs free of strings/bytes imports in the
dispatcher file" — but `cmd/ohcli/format.go` already imports `strings`,
and the dispatcher file does not need to "keep imports out" by
architectural convention. The intrange-style loop is a Go 1.22+ idiom
that a future contributor will wonder about.

**Fix:** Replace `hasByte` with `strings.IndexByte(a, '=') >= 0` inline at
line 148 and delete the helper. Net diff: -10 lines, +1 import.

### IN-02: `cmd/ohcli/format.go::renderCSV` joins multi-subdivision codes with `;` but documents `,` as the human-readable separator for `OFFICIAL_LANGUAGES` in `renderCountriesText` — the joiner-character choice is inconsistent across renderers and output formats

**File:** `cmd/ohcli/format.go:158,185,116`
**Issue:**
- `renderCountriesText` joins `OfficialLanguages` with `,` (line 158)
- `renderCountriesCSV` joins with `;` (line 185)
- `renderCSV` joins subdivision codes with `;` (line 116)
- `renderText` doesn't show subdivisions

The CSV separator-vs-joiner inconsistency is correct (avoid embedding the
field separator in a field) but the text mode using `,` while CSV uses
`;` will surprise a user piping `text | grep ','`. The text mode rules
the operator-visible diagnostic — the CSV mode rules the
machine-consumable export. They are correct for their respective
contexts.

**Fix:** No action needed; the inconsistency is intentional and correct.
Optionally add a top-level comment to `format.go` explaining the
joiner-character policy in one sentence.

### IN-03: `docs/release-runbook.md` Section 4 verification table embeds shell commands with `\|` escapes — pasting from a markdown-table cell will produce shell-incorrect commands

**File:** `docs/release-runbook.md:125`
**Issue:** The table cell contains:
```
`gh release view v0.1.0 --json assets \| jq '.assets \| length'`
```
The `\|` is markdown-table escape syntax. GitHub's markdown renderer
strips the backslash; a copy-paste from the rendered table will produce a
working command. But a reader viewing the raw markdown source (e.g. via
`less` in a terminal) will see the literal `\|` and may copy that
verbatim into a shell, where it will fail.

**Fix:** Provide the command in a separate fenced code block above or
below the table cell. Or use HTML entity `&#124;` instead of `\|`.

### IN-04: `errors.go::APIError.Is` requires a pointer receiver — a caller writing `errors.Is(err, openholidays.APIError{StatusCode: 404})` (value, not pointer) will hit the default `==` fallback, which panics on the unexported `[]byte` field via `reflect.DeepEqual`

**File:** `errors.go:127-136`, `example_test.go` (no example calls Is)
**Issue:** `*APIError` has the `Is` method on a pointer receiver. A
caller writing `errors.Is(err, openholidays.APIError{StatusCode: 404})`
(value, not pointer) will bypass the Is method (it's not on the value
receiver) and fall through to `errors.Is`'s default behavior, which
ultimately reaches a `==` check. `APIError` has a `[]byte` Body field,
making the struct non-comparable; the operation will panic at runtime.

The godoc on `APIError.Is` does not warn against value-receiver usage.

**Fix:** Add a single sentence to the godoc on `APIError.Is`:
> Callers MUST pass a pointer (`&APIError{...}`), never a value. APIError
> contains a `[]byte` field which is not comparable; `errors.Is` against
> a value (not a pointer) will panic at runtime via reflect.DeepEqual.

### IN-05: `docs/design.md` "Strict Decoding" section duplicates the "cache stores raw bytes" claim from the "Cache Architecture" section — same content in two places risks future drift

**File:** `docs/design.md:57,91`
**Issue:** The Cache Architecture section says:
> The cache transport caches only the raw bytes of the response body.
> Decoding (including strict-mode unknown-field detection) runs on every
> read.

The Strict Decoding section says:
> Because the cache transport stores raw bytes (see above), strict mode
> applies to cached entries on every read — a cache-hit decode failure
> surfaces exactly as a cache-miss decode failure would.

These are the same claim. The second one references the first ("see
above") so the drift risk is small — but a future doc editor changing one
without the other will produce divergent versions.

**Fix:** Drop the redundant "Decoding (including strict-mode
unknown-field detection) runs on every read" sentence from the Cache
Architecture section and rely solely on the Strict Decoding section's
canonical statement.

### IN-06: `dependabot.yml` comment block lists the stale (pre-bump) action versions — Dependabot does not read the comments but the human reader will

**File:** `.github/dependabot.yml:7-9`
**Issue:** The comment block at lines 7-9 lists action versions like
`actions/checkout@v4, actions/setup-go@v5, ...` — every one of which has
been bumped since (see CR-03). Dependabot reads the YAML, not the
comments, so its behavior is unaffected. But a human reader checking
"what versions does this repo target?" will be misled by the
documentation portion.

**Fix:** Update the comment block at `.github/dependabot.yml:1-23` to
match the post-bump action versions. Alternatively, delete the
version-list line entirely — the actual versions live in the workflow
YAMLs, not in the dependabot config, so listing them here was always a
drift risk.

---

_Reviewed: 2026-05-28_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
