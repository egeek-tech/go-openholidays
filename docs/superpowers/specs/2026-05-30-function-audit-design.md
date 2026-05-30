# Function Audit & `audit:ok` Certification — Design

**Date:** 2026-05-30
**Status:** Approved (brainstorming)
**Branch:** `chore/function-audit`

## Goal

Deeply audit every production function for logic correctness, certify the correct
ones with an inline `audit:ok` mark, auto-fix only trivial issues, and produce a
test/coverage gap report. Add a Gold Rule making the mark a freshness signal that
must be removed whenever a function changes.

## Scope

All **95 production functions** (47 exported, 48 unexported) across 27 `.go` files
(library root + `cmd/ohcli`). Test files and `testdata/` are out of scope as audit
*subjects* (but tests are *assessed* for the gap report). Baseline coverage at
start: **92.6%** total.

## 1. Gold Rule 5

Added to `.planning/codebase/CONVENTIONS.md` (canonical) + mirrored in `CLAUDE.md`:

> **Rule 5 — `audit:ok` marks certify reviewed logic; modifying a function invalidates its mark.**
> A production function may carry a `// audit:ok YYYY-MM-DD` line certifying its
> logic was reviewed correct on that date. Any change to that function's code or
> behavior REQUIRES deleting its `audit:ok` line in the same commit; it must be
> re-audited before re-marking. A stale mark on changed code is worse than none.
> Pure doc-comment typo fixes are exempt. Enforcement is by convention now; a
> CI/pre-commit guard is a future follow-up.

## 2. Mark format (godoc-safe)

```go
// audit:ok 2026-05-30

// NameFor returns ...
func (h Holiday) NameFor(lang string) string {
```

A blank line separates the mark from the doc comment so it never renders on
pkg.go.dev. **Proof-first:** one function is stamped and validated (`go doc` +
`golangci-lint`) before mass application.

## 3. Audit criteria (mark earned only if ALL hold)

- Logic matches the function's name + doc comment.
- No *flaggable* hardcoded value. Flaggable = a magic literal duplicated across
  sites, a tunable (timeout/limit/size) inlined instead of a named const, or a
  value diverging from an existing const. **Not** flaggable: error/log messages,
  query-param keys, API path segments, `fmt` verbs.
- Sound error handling (no swallowed errors; correct wrapping).
- `context.Context` propagated on HTTP paths.
- No obvious bugs (nil-deref, off-by-one, resource/goroutine leaks).
- Follows the Gold Rules (English, zero-dep, etc.).
- Exported funcs have an accurate doc comment.

A failing criterion ⇒ **no mark**; logged as a finding (severity + `file:line` +
suggested fix).

## 4. Deliverable — `.planning/audit/AUDIT-REPORT.md`

- Per-function result (✅ marked / ⚠️ finding) + objective coverage % per function.
- **Missing tests** — exported funcs lacking a dedicated `TestXxx` (Gold Rule 3).
- **Missing coverage** — low-% functions, ranked.
- **Missing integration tests** — live-API behaviors lacking a guard.
- Severity-ranked findings backlog for separate fix PRs.

## 5. Execution — multi-agent Workflow

1. **Baseline (main):** `go test -coverprofile` → per-function % (done: 92.6%).
2. **Fan-out (parallel, ~10–16 concurrent):** one agent per file. Each audits its
   functions, applies `audit:ok` to clean ones + any trivial fix, `gofmt`s its
   file, returns a compact structured verdict. Disjoint files ⇒ no edit conflicts;
   heavy work stays in subagent contexts.
3. **Synthesis agent:** writes `AUDIT-REPORT.md` from all verdicts; returns a
   compact summary.
4. **Verify (serial, main):** `go test -race ./...` + `golangci-lint run` + `gofmt -l .`.
5. **Checkpoint/pause:** resumable; if main context nears ~90%, checkpoint + stop.

## 6. Out of scope (YAGNI)

No substantive bug-fixing (findings → backlog), no CI enforcement of Rule 6, no
test *writing* (gap *report* only), no refactors beyond trivial const extraction.

## Notes

- This branch forks `master`, which still has the `validateLanguage` lowercase bug
  (fix in-flight as PR #32). The audit will flag it; the report cross-references #32.
- Commits: `docs:` (Rule 6 + spec), `chore(audit):` (marks), `refactor:`/`fix:`
  only for applied trivial fixes (each with passing tests).
