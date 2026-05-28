# Release Runbook — go-openholidays

Single source of truth for cutting a tagged release of `go-openholidays`. Every step below
is anchored to the workflow, config, or source file it depends on; follow them in order.
This document is consumed by the human release operator — every section maps to one or
more Phase 5 acceptance criteria (REL-01..04, DOC-07, CI-04).

Pipeline overview:

```
local pre-tag checks           →  `git push origin v0.1.0`           →  release.yml fires
                                                                          │
                                                                          ├─ goreleaser (.goreleaser.yaml) builds 6 binaries + checksums.txt
                                                                          └─ actions/attest-build-provenance@v4 signs ./dist/checksums.txt
                                                                                                  │
post-tag verification          ←  manual reads of pkg.go.dev, Go Report Card, gh attestation verify
```

---

## 1. Pre-tag checklist

Run every command from the repo root on the `main` branch. None of these steps mutate the
remote — they are local readiness gates only.

- [ ] On `main`, fully synced with origin (`git status` clean, `git pull` is a no-op).
- [ ] CI is green on the most recent `main` commit (visit the Actions tab; the `CI`
      workflow matrix legs all pass on Go 1.23.x, 1.24.x, and `stable`).
- [ ] `version.go` `Version` value matches the intended tag minus the `v` prefix
      (e.g., for tag `v0.1.0` the source has `var Version = "0.1.0"`). The
      `.goreleaser.yaml` `ldflags` use `{{ trimPrefix "v" .Version }}` so the release
      binary's `User-Agent` matches the dev binary's exactly — W-05 invariant.
- [ ] `go.mod` first line is `module github.com/egeek-tech/go-openholidays`. This
      satisfies REL-04 (module path owner — confirmed 2026-05-28).
- [ ] `go test -race -coverprofile=coverage.out ./...` exits 0 locally; coverage ≥ 85 %.
      The exact gate CI enforces on the `stable` matrix leg.
- [ ] `golangci-lint run` exits 0 locally (config: `.golangci.yml`, v2 schema,
      27 enabled linters).
- [ ] `govulncheck ./...` exits 0 locally.
- [ ] `go test -run Example .` exits 0 (TEST-09 — every `// Output:` block in
      `example_test.go` still matches).
- [ ] (Optional) `goreleaser check` exits 0 if `goreleaser` is installed locally
      (`goreleaser check` validates `.goreleaser.yaml` without building anything).

---

## 2. Dry-run rehearsal (strongly recommended for v0.1.0)

The very first release is the riskiest — `release.yml` and `.goreleaser.yaml` have only
been syntactically validated up to this point. Push a release-candidate tag from a side
branch to exercise the pipeline end-to-end before the real tag goes out.

1. Create a side branch from `main` and push an `-rc.1` tag:

   ```bash
   git checkout -b release-rehearsal
   git tag v0.1.0-rc.1
   git push origin v0.1.0-rc.1
   ```

2. Watch `release.yml` in the Actions tab. ETA ~3-5 min (cross-compile + attestation).

3. When the workflow is green, confirm the GitHub Release for `v0.1.0-rc.1` carries:

   - 6 binary archives (`ohcli_v0.1.0-rc.1_linux_amd64.tar.gz`, `_linux_arm64.tar.gz`,
     `_darwin_amd64.tar.gz`, `_darwin_arm64.tar.gz`, `_windows_amd64.zip`,
     `_windows_arm64.zip`), and
   - 1 `checksums.txt` (SHA-256 manifest).

4. Verify the attestation on at least one binary:

   ```bash
   gh attestation verify ohcli_v0.1.0-rc.1_linux_amd64.tar.gz \
       --repo egeek-tech/go-openholidays
   ```

   Exit code must be 0; the output must say the binary is verified against the workflow
   that built it.

5. Clean up the rehearsal:

   ```bash
   git tag -d v0.1.0-rc.1
   git push origin :refs/tags/v0.1.0-rc.1
   ```

   Delete the rc Release on GitHub if it was not auto-marked prerelease (the
   `prerelease: auto` flag in `.goreleaser.yaml` should mark anything matching
   `-rc.*` as prerelease automatically, but verify in the UI).

Skip this section only for subsequent stable releases (v0.1.1, v0.2.0, …) where the
release pipeline has been exercised successfully at least once.

---

## 3. Tag push (the actual release)

This is the only step that publishes anything to consumers. Do it once you are fully
satisfied with Sections 1 and 2.

```bash
# Annotated tag (recommended — carries metadata; goreleaser accepts either form).
git tag -a v0.1.0 -m "v0.1.0 — initial release"

# Or non-annotated (also works):
# git tag v0.1.0

git push origin v0.1.0
```

The push to `refs/tags/v0.1.0` is the trigger configured in `.github/workflows/release.yml`
(`on: push: tags: ['v*']`). Watch the workflow in the Actions tab. ETA ~3-5 min.

---

## 4. Post-tag verification matrix

When `release.yml` is green, run every check in this table. Each row maps to a
Phase 5 requirement that `.planning/phases/05-distribution/05-VALIDATION.md` flagged as
manual-only because the underlying surface is an external service or a CLI that CI
cannot reliably poll.

| Check | Command / URL | Expected | When |
|-------|---------------|----------|------|
| REL-03: GitHub Release artifacts present | `gh release view v0.1.0 --json assets \| jq '.assets \| length'` | `7` (6 binary archives + 1 `checksums.txt`); asset names match `ohcli_v0.1.0_*` | Immediately after `release.yml` green |
| REL-03: Attestation verifiable from CLI | Download one binary, then `gh attestation verify ohcli_v0.1.0_linux_amd64.tar.gz --repo egeek-tech/go-openholidays` | Exit 0; output reports binary "verified" against this repo's workflow | Immediately after `release.yml` green |
| REL-01: `pkg.go.dev` renders the package | Visit `https://pkg.go.dev/github.com/egeek-tech/go-openholidays@v0.1.0` | HTTP 200; no "no documentation" banner; every exported symbol renders; `Example_*` functions attached to their target symbols | Within 30 min of tag push (proxy lag — see Section 5) |
| REL-02: Go Report Card grade A | Visit `https://goreportcard.com/report/github.com/egeek-tech/go-openholidays`; for badge only: `curl -fsS https://goreportcard.com/badge/github.com/egeek-tech/go-openholidays` | Grade A; green badge | Within 24 h of tag (first scan can be slow) |
| DOC-07: pkg.go.dev godoc audit | On the pkg.go.dev page above, spot-check at least three exported symbols (e.g. `NewClient`, `Holiday.NameFor`, `HolidayType.IsKnown`) | Each section renders; no "Documentation: no" warnings; opening godoc line starts with the symbol name | Within 30 min of tag push |

---

## 5. pkg.go.dev index trigger (mitigation for Pitfall 6)

The Go module proxy fetches new tagged versions lazily. After a tag push, `pkg.go.dev`
may report "no module found" for up to 30 min until the proxy is poked. The repository
is already public (D-02 / 05-CONTEXT.md), so this is the only manual nudge required —
no visibility flip step exists for v0.1.0.

If the page returns "no module found" 5 min after the tag push, force the proxy fetch:

```bash
curl -fsS https://pkg.go.dev/github.com/egeek-tech/go-openholidays@v0.1.0 -o /dev/null
# Repeat with @latest if needed:
curl -fsS https://pkg.go.dev/github.com/egeek-tech/go-openholidays@latest -o /dev/null
```

If after 30 min the page still does not render, escalate — the most common root cause
is a `go.mod` first-line mismatch with the actual module-import path users would use
(verified clean in Section 1's bullet point).

Reference: 05-RESEARCH.md §"Common Pitfalls → Pitfall 6" — pkg.go.dev 30-min index lag.

---

## 6. Rollback

`release.yml` is idempotent on re-run: a transient failure (network blip, proxy
hiccup, GitHub API 5xx) can be recovered by hitting "Re-run workflow" in the
Actions tab. The goreleaser step uses `--clean`, which removes `./dist/` before
rebuilding.

If the tagged release contains binaries you cannot ship (wrong version pin,
attestation regression, accidental inclusion of a file, …), the only safe
recovery is to delete the tag and re-issue:

```bash
# Delete the GitHub Release (drops uploaded assets):
gh release delete v0.1.0 --yes

# Delete the tag remotely:
git push origin :refs/tags/v0.1.0

# Delete locally:
git tag -d v0.1.0

# Fix the issue on main, then either re-tag the same name (after fixing
# whatever caused the bad release) or bump:
git tag -a v0.1.1 -m "v0.1.1 — fixes broken v0.1.0"
git push origin v0.1.1
```

Note that the Go module proxy caches versions — if you reuse `v0.1.0`, callers
who already fetched the broken module via `go get` may see the old artifact
until their local module cache evicts. SemVer best practice in this situation
is to bump to `v0.1.1` rather than reusing `v0.1.0`. The reuse path exists only
as a last-resort for the first 5-10 minutes after a botched first push, before
any consumer has fetched.

---

## 7. Post-release housekeeping

After the human verifier in Section 4 has confirmed all rows pass:

- Update `version.go` to the next pre-release / development version:

  ```go
  // version.go
  var Version = "0.2.0-dev"
  ```

  Open a PR titled `chore: bump version to 0.2.0-dev`.

- Update `STATE.md` milestone status to `completed` for the v0.1.0 milestone.

- File a post-release retrospective per `.planning/RETROSPECTIVE.md` cadence if
  the file exists; otherwise add a brief note to `STATE.md` under Session
  Continuity describing what worked and what bit (especially anything about the
  release pipeline that surprised you on the first real tag).

- The `CHANGELOG.md` file is intentionally a one-line pointer to the GitHub
  Releases page — `goreleaser` auto-generates per-release notes from
  Conventional-Commit prefixes (`feat:`, `fix:`, `perf:`, …) per the
  `changelog.groups` block in `.goreleaser.yaml`. No manual `CHANGELOG.md` edit
  is required.

---

## 8. References

- `.github/workflows/release.yml` — the workflow this runbook triggers (`on: push: tags: ['v*']`).
- `.github/workflows/integration.yml` — nightly live-API integration suite (independent of releases; mentioned here only for completeness).
- `.goreleaser.yaml` — drives the 6-binary build matrix, archive layout, and `checksums.txt`.
- `version.go` — single source of truth for the `Version` literal; W-05 invariant pinned via `trimPrefix "v" .Version` in `.goreleaser.yaml` ldflags.
- `go.mod` first line — `module github.com/egeek-tech/go-openholidays` (REL-04).
- `.planning/phases/05-distribution/05-VALIDATION.md` §"Manual-Only Verifications" — canonical list of post-tag checks for REL-01..03 and DOC-07.
- `.planning/phases/05-distribution/05-RESEARCH.md` §"Common Pitfalls → Pitfall 6" — pkg.go.dev 30-min index lag mitigation.
- `.planning/PROJECT.md` Key Decisions CL-18 — Source-current action-version pins exercised by this pipeline.
