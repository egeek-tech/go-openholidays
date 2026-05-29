# egeek-tech-release-bot

A GitHub App that lets Release Please cut release PRs **with real CI cascading
through them** — instead of the silently-broken state the default
`GITHUB_TOKEN` produces.

## TL;DR

Release Please opens a Release PR on every push to `master` that contains
release-worthy commits. Branch protection requires CI checks (lint, tests
matrix, govulncheck) to pass before that PR can be merged. With the default
`GITHUB_TOKEN`, GitHub blocks any downstream workflow run that would have
been triggered by that PR's creation — the anti-loop guard. Result: the
Release PR sits **forever** in "Some checks haven't completed yet" because
the checks were never even allowed to start. This bot exists to be the
**actor** that opens the Release PR, so CI runs on it like a normal PR.

## What it does

| Action | Why |
|--------|-----|
| Opens / updates Release PRs on `master` | Replaces `GITHUB_TOKEN` as the PR author so CI cascades |
| Bumps `version.go` (via `// x-release-please-version` marker) | Single source of truth for the library `Version` const |
| Bumps `.release-please-manifest.json` | Release Please bookkeeping |
| Updates `CHANGELOG.md` from Conventional Commits | Generated per `release-please-config.json` `changelog-sections` |
| Cuts the annotated `vX.Y.Z` tag + drafts the GitHub Release on merge | Goreleaser + attest-build-provenance then attach signed binaries via the consolidated `release-please.yml` workflow |

## What it does NOT do

- Does not push to `master` directly. All changes flow through a PR that
  the human owner reviews and merges.
- Does not bypass branch protection — it acts within it.
- Does not edit production source code outside the bump markers.
- Does not have access to any repo outside its explicit installation list.

## How it works

```
push to master  ─►  release-please.yml fires  ─►  release-please-action
                                                        │
                                                        │  uses token minted by
                                                        │  actions/create-github-app-token
                                                        ▼
                                          Bot identity opens Release PR
                                                        │
                                                        ▼
                                  CI cascades correctly (lint, tests, govulncheck)
                                                        │
                          owner reviews diff, sees ✅, merges Release PR
                                                        │
                                                        ▼
                                  release-please.yml fires again
                                                        │
                          Phase 1: tag vX.Y.Z + draft GitHub Release
                          Phase 2: goreleaser builds 6 binaries + checksums.txt
                          Phase 3: attest-build-provenance signs checksums.txt
                          Phase 4: `gh release edit --draft=false` publishes
```

The token minted by `actions/create-github-app-token` is scoped to **the
repo running the workflow** and expires after one hour. The bot's
private key never enters CI runtime — it stays in the org secret
`RELEASE_PLEASE_APP_PRIVATE_KEY` and is exchanged for a short-lived token
inside the workflow.

## Required permissions (App-side)

| Permission | Why |
|--------|-----|
| **Contents** — Read and write | Push branch updates, tag the release, edit `version.go` / `CHANGELOG.md` |
| **Pull requests** — Read and write | Open / update / label the Release PR |
| **Workflows** — Read and write | Required so the App can author a PR that touches `.github/workflows/*` files (Release Please occasionally needs to bump action versions there) |

No other permissions are granted. The App does not have access to
`Issues`, `Secrets`, `Members`, `Discussions`, or anything organization-wide.

## Installed on

| Repository | Since |
|------------|-------|
| `egeek-tech/go-openholidays` | (initial install) |

Add new repos via the Configure page:
https://github.com/organizations/egeek-tech/settings/installations

## Operator runbook

### Rotate the private key

1. Open the App settings page → **Private keys** → **Generate a private key**.
2. Replace the value of the `RELEASE_PLEASE_APP_PRIVATE_KEY` org secret with
   the contents of the new `.pem` file.
3. On the App settings page, delete the **old** private key entry so a
   leaked copy of the old PEM stops working.

The org secret is consumed by the workflow at run-time only; no cached
copies exist anywhere else.

### Audit recent activity

Visit the App settings page → **Advanced** → **Recent Deliveries**. Each
delivery is a payload the App sent or received, with full request/response
context.

For a higher-signal view, query the audit log for actions performed by
the App identity:
https://github.com/organizations/egeek-tech/settings/audit-log

### Suspend the bot

To temporarily stop Release Please from acting (e.g. during a freeze):
go to the org Installations page → `egeek-tech-release-bot` → **Suspend**.
The workflow will still run on push to `master`, but every API call the
App tries to make returns 403. The CI matrix still runs.

To resume: same page → **Unsuspend**.

### Uninstall the bot

Org Installations page → **Uninstall**. After uninstalling, Release Please
falls back to the default `GITHUB_TOKEN` (which means cascading-CI breaks
again). Reinstall via the App's public install link.

## Why not just bypass branch protection?

The repo owner is the sole reviewer + merger by design — bypass is the
escape hatch for "the rare edge case I personally want to override".
Bypassing on **every release** would erode that signal and make the
audit log noisy. The bot lets the same branch-protection contract hold
for human-authored and bot-authored PRs without distinction.

## Why not a Personal Access Token (PAT)?

A PAT is bound to one human user's identity. The token has at least the
scope of that user — typically far broader than three repo-scoped
permissions. PATs also expire and require manual rotation; the App's
key can be rotated without touching the workflow file. The App identity
also shows up cleanly in audit logs as a separate principal.

## Source

This page lives at
[`docs/release-bot.md`](https://github.com/egeek-tech/go-openholidays/blob/master/docs/release-bot.md)
in the `go-openholidays` repository. Treat it as the contract for what
the bot is allowed to do — if you change the App's permissions or
install scope, update this page in the same PR.
