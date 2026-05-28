# Security Policy

`go-openholidays` is a zero-runtime-dependency Go client library for the
public OpenHolidays API. This document describes how the project handles
security reports and which versions receive fixes.

## Supported Versions

Once `v1.0.0` ships, the project follows strict SemVer. Only the latest
minor release line receives security fixes; older lines must upgrade.

| Version | Supported                  |
| ------- | -------------------------- |
| `1.x`   | :white_check_mark: (latest minor) |
| `< 1.0` | :x: (pre-stable; superseded by 1.x once released) |

The latest minor is always the one referenced in the most recent
[GitHub Release](https://github.com/egeek-tech/go-openholidays/releases).

## Reporting a Vulnerability

**Please do not file a public GitHub issue for security reports.**

Use GitHub's Private Vulnerability Reporting:

1. Open <https://github.com/egeek-tech/go-openholidays/security/advisories/new>
2. Or: Security tab on the repository → "Report a vulnerability"

Include in your report:

- Affected version(s) — git SHA or tag.
- A clear description of the issue.
- Reproduction steps or a minimal proof-of-concept.
- Impact assessment if you have one.
- Optional: a suggested fix.

Reports submitted through Private Vulnerability Reporting create a
private advisory visible only to the reporter and the maintainer.

## What to Expect

- **Acknowledgement:** within 7 days of receipt.
- **Triage:** initial severity assessment within 14 days.
- **Fix:** timeline depends on severity and complexity; the advisory
  is updated as the work progresses.
- **Disclosure:** coordinated. A GitHub Security Advisory (and CVE,
  when warranted) is published alongside the patch release.

There is no formal SLA — this is an open-source library maintained on
best-effort basis. Critical issues are prioritized.

## Scope

In scope:

- The library code under the module root (`github.com/egeek-tech/go-openholidays`).
- The `cmd/ohcli` demo binary.
- The release pipeline (`goreleaser` + artifact attestation) when an
  issue would let an attacker publish a tampered binary.

Out of scope:

- Vulnerabilities in the upstream OpenHolidays API itself
  (<https://www.openholidaysapi.org>) — report those to the upstream
  project.
- Vulnerabilities in user code that embeds this library; the library's
  job is to return validated data and propagate errors, not to harden
  the embedding application.
- DoS via maliciously-large response bodies from the upstream — the
  client caps response size with `io.LimitReader` (10 MiB) before
  decoding. Bug reports against that cap value are welcome; reports
  that simply demonstrate the cap exists are not.
- Denial-of-service against `*_test.go` files or fuzz corpora.

## Security Practices

The repository runs the following security gates on every PR
(see `.github/workflows/ci.yml` and `.github/dependabot.yml`):

- `govulncheck` — detects use of known-vulnerable stdlib and
  third-party symbols against the active call graph.
- `golangci-lint` with `gosec`, `staticcheck`, `errcheck`, and
  `govet` enabled.
- GitHub Secret Scanning + push protection (org-level enabled).
- Dependabot updates for both `github-actions` and `gomod`
  ecosystems on a weekly cadence.
- Release binaries are signed via
  [GitHub Artifact Attestation](https://docs.github.com/en/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds)
  (`actions/attest-build-provenance`); checksums in
  `dist/checksums.txt` cover every released binary.

All third-party GitHub Actions are pinned to a 40-character commit
SHA — tag-rewrite attacks against the action supply chain are
mitigated at the workflow level.
