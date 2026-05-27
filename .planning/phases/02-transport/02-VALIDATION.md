---
phase: 2
slug: transport
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-27
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Source: `02-RESEARCH.md` §Validation Architecture (canonical) — copy locked here for plan-checker / executor consumption.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` stdlib + `github.com/stretchr/testify` v1.11.1 (Gold Rule 3) |
| **Config file** | None — `go test` reads CLI flags only |
| **Quick run command** | `go test -race ./...` |
| **Full suite command** | `go test -race -cover ./...` |
| **Estimated runtime** | ~2 s (small package, all in-process httptest) |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./...`
- **After every plan wave:** Run `go test -race -cover ./...`
- **Before `/gsd:verify-work`:** `go test -race -cover ./... && go vet ./... && go build ./...` all green
- **Max feedback latency:** 5 s (quick run); 10 s (full suite + vet + build)

---

## Per-Task Verification Map

> Authoritative test-to-requirement mapping. Every requirement in Phase 2 has at least one automated test command. The `Plan` / `Wave` / `Task ID` columns are filled in by the planner during `/gsd:plan-phase`.

| Req ID | Behavior | Test Type | Automated Command | File Exists | Status |
|--------|----------|-----------|-------------------|-------------|--------|
| CLIENT-01 | `NewClient` returns usable client, never errors | unit | `go test -run TestNewClient ./...` | ❌ Wave 0 | ⬜ pending |
| CLIENT-02 | `WithHTTPClient` shallow-copies | unit | `go test -run TestWithHTTPClient ./...` | ❌ Wave 0 | ⬜ pending |
| CLIENT-03 | `WithBaseURL` overrides default URL | unit | `go test -run TestWithBaseURL ./...` | ❌ Wave 0 | ⬜ pending |
| CLIENT-04 | `WithUserAgent` overrides default UA | unit | `go test -run TestWithUserAgent ./...` | ❌ Wave 0 | ⬜ pending |
| CLIENT-05 | `WithLogger` injects `*slog.Logger` | unit | `go test -run TestWithLogger ./...` | ❌ Wave 0 | ⬜ pending |
| CLIENT-06 | `WithTimeout` sets per-request timeout (default 15 s) | unit | `go test -run TestWithTimeout ./...` | ❌ Wave 0 | ⬜ pending |
| CLIENT-07 | Goroutine-safe under `-race` | unit (concurrency) | `go test -race -run TestClient_ConcurrentAccess ./...` | ❌ Wave 0 | ⬜ pending |
| CLIENT-08 | `Close()` idempotent stub returning `nil` | unit | `go test -run TestClient_Close ./...` | ❌ Wave 0 | ⬜ pending |
| CLIENT-09 | Ctx cancellation observed within ≤ 100 ms | unit (timing) | `go test -run TestClient_ContextCancel ./...` | ❌ Wave 0 | ⬜ pending |
| ENDPT-01 | `Countries(ctx)` returns typed `[]Country` | unit (httptest) | `go test -run TestClient_Countries ./...` | ❌ Wave 0 | ⬜ pending |
| TRANS-01 | `Accept: application/json` + `User-Agent: ^go-openholidays/` headers sent | unit | `go test -run TestHeaderTransport_RoundTrip ./...` | ❌ Wave 0 | ⬜ pending |
| TRANS-02 | 10 MiB cap returns `ErrResponseTooLarge` | unit (httptest streaming 12 MiB) | `go test -run TestClient_Countries/oversize ./...` | ❌ Wave 0 | ⬜ pending |
| TRANS-03 | Response body drained + closed on every code path | unit (goroutine-delta) | `go test -run TestClient_Countries/oversize ./...` | ❌ Wave 0 | ⬜ pending |
| TRANS-04 | Each RoundTripper unit-tested in isolation | unit | `go test -run "TestHeaderTransport\|TestLoggingTransport" ./...` | ❌ Wave 0 | ⬜ pending |
| OBS-01 | Debug-level logging only; body never logged | unit (captured slog) | `go test -run TestLoggingTransport_RoundTrip ./...` | ❌ Wave 0 | ⬜ pending |
| OBS-02 | OBS-02 fields (`method, path, status, duration_ms, attempt, bytes_in`) set on every record | unit (captured slog) | `go test -run TestLoggingTransport_RoundTrip ./...` | ❌ Wave 0 | ⬜ pending |
| TEST-04 | N parallel `Countries` calls under `-race` | unit (concurrency) | `go test -race -run TestClient_ConcurrentAccess ./...` | ❌ Wave 0 | ⬜ pending |
| W-01 (fix) | Unicode case-fold bypass rejected (`KK`, `İ`, `ı`, `ſ`) | unit (regression) | `go test -run "TestValidateCountry\|TestValidateLanguage" ./...` | ⚠️ partial | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Files that must exist with at least skeleton tests **before any production-code task runs** in later waves:

- [ ] `client.go` + `client_test.go` — `TestNewClient`, `TestClient_Close`, `TestClient_ConcurrentAccess`, `TestClient_ContextCancel`
- [ ] `options.go` + `options_test.go` — one `TestXxx` per `WithX` (5 functions = 5 tests)
- [ ] `config.go` (no public symbols — covered transitively via `client_test.go` and `countries_test.go`)
- [ ] `transport.go` + `transport_header_test.go` + `transport_logging_test.go`
- [ ] `countries.go` + `countries_test.go` — `TestClient_Countries` with happy + 4xx + 5xx + ctx-cancel + oversize subtests
- [ ] `testdata/countries.json` — 2-country PL+DE fixture (captured 2026-05-27)
- [ ] `errors.go` extension — append `ErrResponseTooLarge` to existing `var (...)` block + `TestErrResponseTooLarge_sentinel` regression test in `errors_test.go`
- [ ] `internal_test.go` modification — add `"ErrResponseTooLarge"` to `allowedVars`
- [ ] `validate.go` + `validate_test.go` — W-01 fix: reorder ASCII shape check; add 4 regression cases (`KK`, `İ`-prefix, `ı`-prefix, `ſ`-prefix)

*No framework install needed — `go test` is stdlib; `testify` is already in `go.mod`.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Re-capture `testdata/countries.json` from live upstream | ENDPT-01 fixture freshness | Live network not available inside `go test`; capture by hand if Phase 2 lands > 7 days after research | `curl -sSf https://openholidaysapi.org/Countries \| jq '[.[] \| select(.isoCode=="PL" or .isoCode=="DE")]' > testdata/countries.json` |

*All other phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all `❌ Wave 0` references
- [ ] No watch-mode flags (no `-test.v` interactive watchers; CI runs are one-shot)
- [ ] Feedback latency < 5 s (quick) / < 10 s (full)
- [ ] `nyquist_compliant: true` set in frontmatter after planner pass

**Approval:** pending
