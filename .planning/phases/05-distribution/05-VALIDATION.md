---
phase: 5
slug: distribution
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-28
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Source: `.planning/phases/05-distribution/05-RESEARCH.md` §"Validation Architecture".

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (stdlib) + `github.com/stretchr/testify` (assert + require) |
| **Config file** | `.golangci.yml` (to be created in Wave 1), `.goreleaser.yaml` (Wave 3) |
| **Quick run command** | `go test -race ./...` |
| **Full suite command** | `go test -race -cover ./... && golangci-lint run && govulncheck ./...` |
| **Integration command** | `OPENHOLIDAYS_LIVE=1 go test -tags=integration ./...` |
| **Fuzz command** | `go test -fuzz=Fuzz<Name> -fuzztime=60s ./...` |
| **Bench command** | `go test -run=^$ -bench=. -benchmem ./...` |
| **Estimated runtime (quick)** | ~5 seconds |
| **Estimated runtime (full)** | ~30 seconds (excluding integration + fuzz) |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./...` plus any task-specific `go test -run <Pattern>`
- **After every plan wave:** Run `go test -race -cover ./... && golangci-lint run`
- **Before `/gsd:verify-work`:** Full suite must be green (race + coverage ≥ 85% + lint + govulncheck)
- **Max feedback latency:** ~30 seconds for full suite (excluding integration + fuzz which are time-boxed in CI)

---

## Per-Task Verification Map

Plans are not yet generated — this table is populated by `/gsd:plan-phase` once `*-PLAN.md` files exist. The planner must derive one row per task and back each with an `<automated>` command. The 27 Phase 5 requirements drive this table; each row's `Requirement` column references a REQ-ID from §"Validation Architecture" in `05-RESEARCH.md`.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

**Reference: Requirement → Validation method** (from `05-RESEARCH.md` §"Validation Architecture"):

| REQ-ID | Verification |
|--------|--------------|
| CLI-01 | `go build ./cmd/ohcli` succeeds; binary printed to `./ohcli` |
| CLI-02 | Per-subcommand `TestRun_*` tests in `cmd/ohcli/*_test.go` cover argv shapes + exit codes (0/1/2) |
| CLI-03 | `grep -rE 'import\s+\("([^"]+)"' cmd/ohcli/` shows zero non-stdlib imports beyond the library's own module path |
| CLI-04 | Unit tests assert exact stdout/stderr for text + `--json` + `--csv`; empty-result behavior asserts stderr line + empty stdout + exit 0 |
| TEST-07 | `FuzzParseLocalizedText` + `FuzzUnmarshalHoliday` exist; `go test -fuzz=Fuzz<Name> -fuzztime=10s` in CI; seed corpus in `testdata/fuzz/Fuzz<Name>/` |
| TEST-08 | `integration_test.go` with `//go:build integration` tag + `OPENHOLIDAYS_LIVE=1` env gate; nightly workflow exercises it |
| TEST-09 | `example_test.go` contains `Example_<Symbol>` per exported method; `go test -run Example` passes; `// Output:` blocks match |
| TEST-10 | CI gate: `go test -race -cover -coverprofile=cover.out`; threshold check (≥ 85%) enforced via `go tool cover` + awk gate **or** Codecov status check |
| TEST-11 | `Benchmark_PublicHolidays_PL_2025` (cold path) + `Benchmark_Countries_Cached` (cached path, per cached-budget reinterpretation in research §3.4); both run under `go test -run=^$ -bench=.` |
| CI-01 | `.github/workflows/ci.yml` exists with `strategy.matrix.go: [1.23.x, 1.24.x, stable]` + `os: ubuntu-latest`; passes on a PR |
| CI-02 | Same workflow runs `go vet ./...` and `go build ./...` |
| CI-03 | Same workflow runs `go test -race -cover ./...` and uploads `cover.out` to Codecov via `codecov/codecov-action@v5` |
| CI-04 | Same workflow runs `golangci-lint run` via `golangci/golangci-lint-action@v6` |
| CI-05 | Same workflow runs `govulncheck ./...` via `golang/govulncheck-action@v1` |
| CI-06 | Same workflow runs `go mod tidy && git diff --exit-code go.mod go.sum` |
| CI-07 | README contains Codecov badge that resolves; Codecov status check appears on PRs |
| DOC-01 | `README.md` exists; `≤ 20-line` quickstart compiles as `Example_quickstart` in `example_test.go` |
| DOC-02 | `doc.go` exists with package overview godoc |
| DOC-03 | `example_test.go` contains one `Example_<Symbol>` per exported method (planner enumerates count) |
| DOC-04 | `docs/design.md` exists with ASCII transport-chain diagram + cache layer overview |
| DOC-05 | `CHANGELOG.md` exists (one-line pointer to GitHub Releases per CONTEXT.md D-12); goreleaser changelog block configured to render conventional-commit groups |
| DOC-06 | `CONTRIBUTING.md` exists with dev-loop instructions + conventional-commit policy note |
| DOC-07 | `grep -L "^// $(basename <symbol>)"` over exported symbols returns no hits — every exported symbol's godoc starts with its own name; pkg.go.dev renders OK after tag push |
| REL-01 | `.goreleaser.yaml` exists with `version: 2` + 6-binary matrix; `goreleaser check` exits 0 |
| REL-02 | `.github/workflows/release.yml` exists with `on: push: tags: ['v*']`; `id-token: write` permission set for attestation step |
| REL-03 | `actions/attest-build-provenance@v4` step present in release.yml after goreleaser; `gh attestation verify <binary> --repo egeek-tech/go-openholidays` succeeds on the released artifact |
| REL-04 | `go.mod` line 1 is `module github.com/egeek-tech/go-openholidays`; `git ls-remote https://github.com/egeek-tech/go-openholidays.git` resolves; `pkg.go.dev/github.com/egeek-tech/go-openholidays` renders after tag |

---

## Wave 0 Requirements

Wave 0 (infrastructure before any tests can be written):

- [ ] `.golangci.yml` — finalized from `.golangci.yml_backup` with cross-project debris stripped (see RESEARCH §2.5)
- [ ] `testdata/fuzz/FuzzParseLocalizedText/` directory created with hand-curated seed corpus (per RESEARCH §1.4)
- [ ] `testdata/fuzz/FuzzUnmarshalHoliday/` directory created with hand-curated seed corpus
- [ ] `example_test.go` skeleton with one stub `Example_*` per exported method (compile-checked even if `// Output:` is filled in later)
- [ ] `cmd/ohcli/` directory created with `main.go`, `format.go`, and per-subcommand files

*All Wave 0 items map to Wave 1 plans — no separate Wave 0 plan is required; the first plan in each Wave is the Wave 0 prerequisite for downstream plans.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `pkg.go.dev` indexes module after tag push | REL-04 / DOC-07 | External service has ~5-30 min lag after tag push; can't poll in CI without flakiness | After `v0.1.0` tag push, wait 30 min, `curl -fsS https://pkg.go.dev/github.com/egeek-tech/go-openholidays` returns 200; README badge for godoc resolves |
| `goreleaser` produces 6 cross-platform binaries on tag | REL-01 / REL-02 | Release workflow runs on tag — verified post-release, not pre-merge | After release.yml runs on `v0.1.0`: `gh release view v0.1.0 --json assets` lists 6 binary archives + 1 `checksums.txt` |
| GitHub Artifact Attestation verifiable from CLI | REL-03 | Verification requires `gh` CLI + the released binary; not a `go test` artifact | `gh attestation verify ohcli_v0.1.0_linux_amd64.tar.gz --repo egeek-tech/go-openholidays` exits 0 |
| Go Report Card grade A | DOC-07 | External service; one-shot scan post-tag | After `v0.1.0` push: `https://goreportcard.com/report/github.com/egeek-tech/go-openholidays` shows grade A |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (`.golangci.yml`, fuzz seed corpus directories, `example_test.go` skeleton, `cmd/ohcli/` scaffold)
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s for the quick suite
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
