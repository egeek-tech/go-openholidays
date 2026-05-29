---
status: complete
phase: 05-distribution
source:
  - .planning/phases/05-distribution/05-01-SUMMARY.md
  - .planning/phases/05-distribution/05-02-SUMMARY.md
  - .planning/phases/05-distribution/05-03-SUMMARY.md
  - .planning/phases/05-distribution/05-04-SUMMARY.md
  - .planning/phases/05-distribution/05-05-SUMMARY.md
  - .planning/phases/05-distribution/05-06-SUMMARY.md
  - .planning/phases/05-distribution/05-07-SUMMARY.md
  - .planning/phases/05-distribution/05-08-SUMMARY.md
started: 2026-05-29T18:49:03Z
updated: 2026-05-29T18:59:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Install and run ohcli
expected: |
  `go install github.com/egeek-tech/go-openholidays/cmd/ohcli@latest`
  completes without errors. The resulting `ohcli` binary is on PATH.
  Running it with no args prints a usage block to stderr and exits
  with code 2.
result: pass

### 2. ohcli public PL 2025 (text)
expected: |
  `ohcli public PL 2025` prints a text-formatted table with header
  columns DATE, END, NAME, NATIONWIDE, TYPE. Body contains exactly
  14 rows — the canonical Polish 2025 public holidays. Exit code 0.
result: pass

### 3. ohcli public PL 2025 --json
expected: |
  `ohcli public PL 2025 --json` prints a valid JSON array (parsable
  by `jq`). The array has exactly 14 elements. Each element has
  StartDate, Name, Type fields. Exit code 0.
result: pass

### 4. ohcli school PL 2025 --region PL-SL
expected: |
  `ohcli school PL 2025 --region PL-SL` prints school holiday rows
  for Silesia (PL-SL). The output renders the same column header as
  public-holidays and contains non-zero rows. Exit code 0.
result: pass

### 5. ohcli countries
expected: |
  `ohcli countries` prints a list of countries with ISO-2 codes and
  English names. The list includes PL (Poland), DE (Germany), and
  at least 25 others. Exit code 0.
result: pass

### 6. Exit codes follow D-06 / D-07 policy
expected: |
  • `ohcli` (no args)      → exit 2, stderr contains "usage:"
  • `ohcli frobnicate`     → exit 2, stderr begins with "ohcli: "
  • `ohcli public PL 2025` → exit 0 (success path)
  No partial output on error paths.
result: pass

### 7. Library test suite passes with -race
expected: |
  `go test -race ./...` (from repo root) prints a final "ok" line
  for every package and exits 0. No race detector warnings. No
  flaky failures across 3 consecutive runs.
result: pass

### 8. Latest GitHub Release ships binaries + attestation
expected: |
  The latest published release at
  https://github.com/egeek-tech/go-openholidays/releases lists 7
  assets: 6 ohcli binaries (linux/darwin/windows × amd64/arm64)
  and checksums.txt. `gh attestation verify --owner egeek-tech`
  against any of those binaries succeeds.

  NOTE: known broken at time of UAT creation — published v0.2.3
  has 0 assets; v0.2.4/v0.3.0/v0.4.0 are drafts. Fix queued in PR #29.
result: issue
reported: |
  User ran `gh attestation verify /home/rtkocz/go/bin/ohcli --owner egeek-tech`
  and got `HTTP 404: Not Found` on the attestations-API lookup.
  Caveat: the binary tested was a local `go install` build, not a
  release-uploaded binary; locally-built binaries don't carry
  attestations regardless. The underlying Phase 5 gap is that no
  release has ever uploaded binaries to attest in the first place
  (v0.2.0..v0.2.3 published with 0 assets; v0.2.4..v0.4.0 are drafts).
severity: blocker

### 9. pkg.go.dev page renders
expected: |
  https://pkg.go.dev/github.com/egeek-tech/go-openholidays loads
  and displays the full public API — Client, NewClient, the five
  endpoint methods (Countries, Languages, Subdivisions,
  PublicHolidays, SchoolHolidays), the option constructors
  (WithBaseURL/WithHTTPClient/...), Holiday and its helpers,
  Date, the error sentinels, and the eight worked Examples from
  example_test.go.
result: pass

### 10. README Quickstart compiles unmodified
expected: |
  Copy the 14-line Quickstart code block from README.md verbatim
  into a fresh Go file under any module that imports
  `github.com/egeek-tech/go-openholidays`. `go build` succeeds
  without edits. `go run` against the live API returns the
  expected 14 Polish 2025 public holidays.
result: pass

## Summary

total: 10
passed: 9
issues: 1
pending: 0
skipped: 0
blocked: 0

## Gaps

- truth: "Latest GitHub Release ships 6 ohcli binaries + checksums.txt; attestation verifiable via `gh attestation verify --owner egeek-tech`"
  status: failed
  reason: |
    Released GitHub Releases v0.2.0..v0.2.3 publish with zero assets
    (release-please-action created the release with draft:false, GitHub
    auto-marked it immutable, goreleaser's POST /releases/:id/assets
    returned 422 — run 26632385497 log). v0.2.4/v0.3.0/v0.4.0 are stuck
    as drafts because the previous fix attempt (PR #19) set draft:true
    without the partner `force-tag-creation: true`, so the underlying
    git tag is never pushed and Phase 2 checkout fails. With no
    binaries uploaded, attest-build-provenance has nothing to sign.
  severity: blocker
  test: 8
  artifacts:
    - .github/workflows/release-please.yml
    - .goreleaser.yaml
    - release-please-config.json
  missing:
    - "force-tag-creation: true in release-please-config.json"
    - "use_existing_draft: true and mode: replace under .goreleaser.yaml release:"
    - "workflow_dispatch recovery path in release-please.yml for stuck drafts"
  fix_status: in_flight
  fix_pr: 29
