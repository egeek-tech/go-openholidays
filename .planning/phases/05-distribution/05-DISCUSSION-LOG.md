# Phase 5: Distribution - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-28
**Phase:** 05-distribution
**Areas discussed:** Module path & owner, CLI UX details, Coverage badge service, Release pipeline scope

---

## Module path & owner (REL-04)

### Q1 — Confirm the module path for v0.1.0?

| Option | Description | Selected |
|--------|-------------|----------|
| Keep `github.com/egeek-tech/go-openholidays` | Current `go.mod` path; egeek-tech as owning GitHub org. | ✓ |
| Move to a different org/path | Pre-tag relocation; locks for life of the module. | |

**User's choice:** Keep `github.com/egeek-tech/go-openholidays`.

### Q2 — Is the GitHub repo public and visible?

| Option | Description | Selected |
|--------|-------------|----------|
| Already public | Repo exists and is accessible. `pkg.go.dev` indexing Just Works. | ✓ |
| Will be public before tagging | Add a pre-tag visibility checklist item. | |
| Staying private at v0.1.0 | Plan ships internally; REL-01 can't be verified externally. | |

**User's choice:** Already public.

**Notes:** Module identity is fully locked. Pre-tag runbook still includes a "verify visibility" step out of caution.

---

## CLI UX details

### Q1 — `--json` output flag?

| Option | Description | Selected |
|--------|-------------|----------|
| Text table only | Simplest; users pipe to `jq` if they need JSON. | |
| Text default + `--json` | Both formats from v0.1.0. ~30 LOC + 2 tests. | |
| Text + `--json` + `--csv` | All three formats. ~80 LOC total. | ✓ |

**User's choice:** Text + `--json` + `--csv`.

**Notes:** CSV implies spreadsheet-import use case. Planner uses stdlib `encoding/csv` (RFC 4180); header row mandatory; no UTF-8 BOM by default.

### Q2 — `--lang` default and fallback?

| Option | Description | Selected |
|--------|-------------|----------|
| Default `"en"` + first-available fallback | Matches `Holiday.NameFor("xx")` Phase 3 semantics. | ✓ |
| Default to country-native + first-available | `PL` → `pl`, `DE` → `de`, derived from `<country>`. | |
| Default to env `LANG`/`LC_ALL` + `en` fallback | Env-driven lang; locale parsing. | |

**User's choice:** Default `"en"` + first-available fallback.

### Q3 — Error & exit code conventions?

| Option | Description | Selected |
|--------|-------------|----------|
| Plain stderr + 2-tier exit (0/1) | Simplest. Any error → 1. | |
| Plain stderr + 3-tier exit (0/1/2) | POSIX: 0 success, 1 runtime, 2 usage. ~5 LOC extra. | ✓ |
| Color when TTY + 3-tier exit | Same + ANSI color via `isatty`-equivalent. | |

**User's choice:** Plain stderr + 3-tier exit (0/1/2).

### Q4 — Empty-result rendering and test strategy?

| Option | Description | Selected |
|--------|-------------|----------|
| Stderr note + exit 0; tests use `httptest` | "no <thing> found for <args>" to stderr, exit 0. | ✓ |
| Stderr note + exit 1 to signal "empty" | Treats empty as error-ish for scripts. | |
| Silent success + tests use `httptest` | No output on empty; unfriendly for humans. | |

**User's choice:** Stderr note + exit 0; tests use `httptest`.

**Notes:** Stdout stays empty on empty results so pipes don't break. Live API is integration-test-only.

---

## Coverage badge service

### Q1 — Codecov, Coveralls, or self-hosted?

| Option | Description | Selected |
|--------|-------------|----------|
| Codecov (Recommended) | Most common; token-free for public repos via GitHub App. | ✓ |
| Coveralls | Older but actively maintained; same token-free flow. | |
| Self-hosted shields.io | No third-party service; static badge from coverage.txt. | |

**User's choice:** Codecov.

---

## Release pipeline scope

### Q1 — Release artifacts beyond binaries (multi-select)

| Option | Description | Selected |
|--------|-------------|----------|
| SHA256 checksums file (Recommended) | `goreleaser` default; `checksums.txt`. | ✓ |
| Source tarballs | `*.src.tar.gz` alongside binaries. | |
| SLSA provenance / cosign signatures | `cosign` keyless + SLSA attestations; ~50 lines yaml. | |
| Homebrew tap | sibling `homebrew-tap` repo; ongoing maintenance. | |

**User's choice:** SHA256 checksums + (freeform follow-up: "to sign can you use GitHub attestations").

**Notes:** User asked about GitHub's native artifact attestations as an alternative to cosign. Confirmed yes — `actions/attest-build-provenance@v1` is the modern path: free for public repos, signed by Sigstore/Fulcio, no key management, SLSA Level 3, verifiable with `gh attestation verify`.

### Q2 — Confirm signing approach?

| Option | Description | Selected |
|--------|-------------|----------|
| GitHub Artifact Attestations (Recommended) | `actions/attest-build-provenance@v1` in `release.yml`. | ✓ |
| GitHub attestations + cosign keyless | Belt-and-suspenders; both signing paths. | |
| Skip signing for v0.1.0, add in v0.2 | Just checksums; defer signing. | |

**User's choice:** GitHub Artifact Attestations.

### Q3 — CHANGELOG.md maintenance approach?

| Option | Description | Selected |
|--------|-------------|----------|
| Hand-curated keep-a-changelog (Recommended) | DOC-05 spec literal; full editorial control. | |
| Auto-generate from conventional commits | goreleaser auto-emits release notes from commits. | ✓ |
| Both — generate then hand-edit | Draft from goreleaser, author edits before tag. | |

**User's choice:** Auto-generate from conventional commits.

**Notes:** Existing commit history already follows conventional-commits (`fix(NN): ...`, `docs(NN): ...`, `feat(NN): ...`, `test(NN): ...`, `chore(NN): ...`, `refactor(NN): ...`) — convention is de facto in place. DOC-05 ("keep-a-changelog format") is satisfied via the goreleaser-generated GitHub Release notes; a top-level `CHANGELOG.md` becomes optional and, if kept, points to `https://github.com/egeek-tech/go-openholidays/releases`.

---

## Claude's Discretion

- README badge set + ordering (CI, Codecov, Go Report Card, godoc, license) — editorial.
- `docs/design.md` shape (ASCII vs Mermaid vs prose) — recommend ASCII for grep-ability.
- `CONTRIBUTING.md` depth — recommend minimal v0.1.0 (dev loop + test tiers); templates in v0.2.
- `Example_*` strategy — recommend doctest (`// Output:`) where deterministic + `// Demo:` style for live-API-dependent examples; one per public method floor.
- Fuzz seed corpus — recommend hybrid: existing `testdata/*.json` as seed + 2-3 adversarial seeds per fuzz target in `testdata/fuzz/FuzzXxx/`.
- Whether to enforce conventional commits via a `commitlint` workflow or just a CONTRIBUTING.md note — recommend the lightweight CONTRIBUTING note for v0.1.0.

## Deferred Ideas

- Homebrew tap (v0.2.x)
- Cosign signatures alongside GitHub attestations (v0.2.x if downstream demand surfaces)
- Source tarballs from goreleaser
- `--bom` flag for CSV output
- `--format=ical` (already deferred to M4 in PROJECT.md)
- Issue/PR templates in `.github/ISSUE_TEMPLATE/` (v0.2.x)
- DCO / sign-off enforcement
- Code of Conduct file (v0.2.x)
- Vanity import path
- `--lang` env-var fallback (LANG / LC_ALL)
