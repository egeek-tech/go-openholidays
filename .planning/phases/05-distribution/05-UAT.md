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
updated: 2026-05-29T19:00:00Z
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
result: pass
verified_against: |
  v0.5.0 — first release through the fixed pipeline (PR #29 + PR #28).
  Confirmed: 7 assets present (checksums.txt + 6 ohcli binaries
  linux/darwin/windows × amd64/arm64). `gh attestation verify
  --owner egeek-tech ohcli_0.5.0_linux_amd64.tar.gz` returns exit 0.
  Attestation predicate https://slsa.dev/provenance/v1, build URI
  https://github.com/egeek-tech/go-openholidays/.github/workflows/release-please.yml@refs/heads/master,
  signer github-hosted runner.

  Earlier failure (local `go install`-built binary returning 404) was
  expected — local builds never carry attestations; only release-
  uploaded artifacts do.

  Orphan drafts v0.2.4 / v0.3.0 / v0.4.0 remain as drafts per Gold
  Rule 4 and the earlier user decision ("leave drafts alone; let
  v0.4.0 bridge the gap"). They have no binaries but are non-blocking
  audit-trail artifacts.

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
passed: 10
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none — all tests pass]

## Resolved Gaps

- truth: "Latest GitHub Release ships 6 ohcli binaries + checksums.txt; attestation verifiable via `gh attestation verify --owner egeek-tech`"
  was_status: failed
  resolved_in: v0.5.0
  fix_pr: 29
  verification: |
    v0.5.0 published 2026-05-29T14:24:00Z with 7 assets. Attestation
    verified end-to-end against ohcli_0.5.0_linux_amd64.tar.gz, exit 0.
    Build URI chains to .github/workflows/release-please.yml on master.
