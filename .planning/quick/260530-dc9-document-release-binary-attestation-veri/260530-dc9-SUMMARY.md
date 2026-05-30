---
phase: quick-260530-dc9
plan: 01
subsystem: docs
tags: [slsa, attestation, sigstore, gh-cli, release, goreleaser]

# Dependency graph
requires:
  - phase: 05-distribution
    provides: "release-please.yml SLSA build-provenance attestation over the six ohcli archives"
provides:
  - "README ## Verifying release binaries section with the verified gh attestation verify recipe"
  - "Tightened release-runbook §3 REL-03 row directing verification at the ARCHIVE, not the binary/checksums"
affects: [distribution, release-runbook, consumer-facing docs]

# Tech tracking
tech-stack:
  added: []
  patterns: ["Document the verified-correct gh attestation verify recipe and the three 404 traps (binary, checksums.txt, local build)"]

key-files:
  created: []
  modified:
    - README.md
    - docs/release-runbook.md

key-decisions:
  - "Placed the new README section between ## CLI and ## Architecture so the un-attested `go install …@latest` local build contrasts directly with verifying a signed release archive"
  - "Kept the runbook REL-03 command verbatim with the literal <archive> placeholder; added the --signer-workflow hardened-verify gotcha as a one-line footnote outside the table to keep it well-formed"

patterns-established:
  - "Attestation docs always verify the ARCHIVE listed in checksums.txt, never the unpacked binary or checksums.txt itself"

requirements-completed: [DOC-RELEASE-ATTEST]

# Metrics
duration: 6min
completed: 2026-05-30
---

# Quick 260530-dc9: Document release binary attestation verification Summary

**Documents the verified `gh attestation verify` recipe for released `ohcli` archives in the README, plus tightens the release-runbook REL-03 row to target the ARCHIVE — warning against the three 404 traps (unpacked binary, `checksums.txt`, locally-built binary).**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-05-30
- **Completed:** 2026-05-30
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- New `## Verifying release binaries` README section: one-line SLSA/Sigstore intro, a working two-command `sh` block (`gh release download v0.5.0 …` + `gh attestation verify ohcli_0.5.0_linux_amd64.tar.gz …`), a `> ` warning callout covering the archive-vs-binary-vs-`checksums.txt` 404 trap and the local-build-never-attested note, and the `--signer-workflow` hardened-verify line.
- Release-runbook §3 REL-03 row rewritten: the stale "Download one binary, then" lead-in is gone; the cell now directs the operator to download and verify a release ARCHIVE while keeping the `gh attestation verify <archive> --repo egeek-tech/go-openholidays` command verbatim. Table stays a well-formed 4-column (6-field) row.
- Added a one-line REL-03 hardened-verify footnote below the table noting the signing ref is `refs/heads/master`, so pin `--signer-workflow …` rather than `--source-ref refs/tags/…`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add "Verifying release binaries" section to README.md** - `e442ad6` (docs)
2. **Task 2: Tighten the REL-03 attestation row in release-runbook §3** - `09811ca` (docs)

**Plan metadata:** committed by the orchestrator (SUMMARY.md + STATE.md not committed here).

## Files Created/Modified
- `README.md` - Added `## Verifying release binaries` section between `## CLI` and `## Architecture`.
- `docs/release-runbook.md` - Rewrote the §3 REL-03 attestation Command/URL cell; added a one-line hardened-verify footnote below the table.

## Decisions Made
- Section placement: between `## CLI` and `## Architecture`, per the plan's `<readme_map>` rationale (contrast un-attested local `go install` build with verifying a signed release archive).
- Hardened-verify gotcha added to the runbook as a one-line footnote outside the table (rather than inside the cell) to keep the table well-formed and avoid introducing a literal `|`.

## Deviations from Plan

None - plan executed exactly as written. All command examples use the verified-correct forms from `<verified_facts>` (concrete `ohcli_0.5.0_linux_amd64.tar.gz` archive and `gh release download v0.5.0` in the README; literal `<archive>` placeholder preserved verbatim in the runbook). No code, deps, version bumps, or release/tag edits (Gold Rule 4 respected). English only (Gold Rule 1).

## Issues Encountered
None.

## Verification

- **Task 1 automated gate:** PASS — `## Verifying release binaries` heading present and ordered after `## CLI` / before `## Architecture`; contains the verified `gh release download v0.5.0 …`, `gh attestation verify ohcli_0.5.0_linux_amd64.tar.gz …`, and `HTTP 404` strings.
- **Task 2 automated gate:** PASS — REL-03 row retains `gh attestation verify <archive> --repo egeek-tech/go-openholidays`; the stale "Download one binary, then" wording is absent; the matched row has exactly 6 `awk -F'|'` fields (well-formed 4-column row).
- No `.go` files touched — `gofmt`/`go build` unaffected.

## User Setup Required
None - no external service configuration required.

## Self-Check: PASSED
- FOUND: README.md `## Verifying release binaries` section (Task 1 gate PASS)
- FOUND: docs/release-runbook.md tightened REL-03 row (Task 2 gate PASS)
- FOUND commit: `e442ad6` (Task 1)
- FOUND commit: `09811ca` (Task 2)

## Next Phase Readiness
- Consumer-facing attestation verification is now documented end-to-end (README + runbook). No blockers.

---
*Phase: quick-260530-dc9*
*Completed: 2026-05-30*
