---
phase: 3
slug: endpoints-helpers
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-27
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `github.com/stretchr/testify` v1.11.1 (already in `go.mod`) |
| **Config file** | None (Go `testing` requires no config; `httptest.NewServer` per-test) |
| **Quick run command** | `go test ./... -count=1` |
| **Full suite command** | `go test -race -count=1 ./...` |
| **Integration command** | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 ./...` |
| **Fixture refresh command** | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -update -run TestUpdateFixtures ./...` |
| **Estimated runtime** | ~10 seconds full race (Phase 1 AST audit + Phase 2 transport + new Phase 3 tests) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -run TestClient_<EndpointJustWritten>|TestHoliday_<HelperJustWritten> -count=1` (~1–2 seconds)
- **After every plan wave:** Run `go test -race -count=1 ./...` (~10 seconds)
- **Before `/gsd:verify-work`:** Full suite green AND integration build compiles (`go vet -tags=integration ./...`)
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 3-01-01 | 01 | 0 | (foundation) | T-3-DoS-OverSize | `io.LimitReader` 10 MiB cap preserved in `doJSONGet[T]` | unit | `go test -run TestDoJSONGet -count=1 ./...` | ❌ W0 (Plan 1) | ⬜ pending |
| 3-01-02 | 01 | 0 | (foundation) | T-3-Tampering-MalformedHoliday | `ErrResponseTooLarge` boundary gate preserved | unit | `go test -run TestDoJSONGet -count=1 ./...` | ❌ W0 (Plan 1) | ⬜ pending |
| 3-01-03 | 01 | 0 | (Countries retrofit) | — | `Countries(ctx, CountriesRequest{})` returns same data as Phase 2 `Countries(ctx)` | unit | `go test -run TestClient_Countries -count=1 ./...` | ✅ (refactor) | ⬜ pending |
| 3-02-01 | 02 | 1 | ENDPT-02 | T-3-InputVal-Lang | Optional `LanguageIsoCode` validated only when non-empty | unit | `go test -run TestClient_Languages -count=1 ./...` | ❌ W0 (Plan 2) | ⬜ pending |
| 3-02-02 | 02 | 1 | TEST-01 | — | 4 error paths in table (network fail / 4xx / 5xx / malformed JSON / ctx cancel) | unit | `go test -run TestClient_Languages -count=1 ./...` | ❌ W0 (Plan 2) | ⬜ pending |
| 3-03-01 | 03 | 1 | ENDPT-03 | T-3-InputVal-Country | Required `CountryIsoCode` validated; returns 16 PL województwa from fixture | unit | `go test -run TestClient_Subdivisions -count=1 ./...` | ❌ W0 (Plan 3) | ⬜ pending |
| 3-03-02 | 03 | 1 | TEST-01 | — | 4 error paths | unit | `go test -run TestClient_Subdivisions -count=1 ./...` | ❌ W0 (Plan 3) | ⬜ pending |
| 3-04-01 | 04 | 1 | ENDPT-04 | T-3-InputVal-DateRange | Required country + date range validated; returns 14 PL 2025 holidays incl. Wigilia 2025-12-24 | unit | `go test -run TestClient_PublicHolidays -count=1 ./...` | ❌ W0 (Plan 4) | ⬜ pending |
| 3-04-02 | 04 | 1 | TEST-01 | T-3-Tampering-MalformedHoliday | `validateHolidays` rejects zero/inverted dates with `ErrMalformedResponse` | unit | `go test -run TestValidateHolidays -count=1 ./...` | ❌ W0 (Plan 4) | ⬜ pending |
| 3-04-03 | 04 | 1 | (CL-12) | T-3-Tampering-MalformedHoliday | `ErrMalformedResponse` added to `allowedVars` in `internal_test.go` | unit | `go test -run TestNoInitOrGlobalState -count=1 ./...` | ❌ (extend allowlist) | ⬜ pending |
| 3-05-01 | 05 | 1 | ENDPT-05 | T-3-InputVal-DateRange | `GroupCode` passes through; 7 PL 2025 school periods from fixture | unit | `go test -run TestClient_SchoolHolidays -count=1 ./...` | ❌ W0 (Plan 5) | ⬜ pending |
| 3-05-02 | 05 | 1 | TEST-01 | — | 4 error paths | unit | `go test -run TestClient_SchoolHolidays -count=1 ./...` | ❌ W0 (Plan 5) | ⬜ pending |
| 3-06-01 | 06 | 1 | HELP-01 | — | `Holiday.NameFor("pl")` returns Polish; `"xx"` falls back to first entry | unit | `go test -run TestHoliday_NameFor -count=1 ./...` | ❌ W0 (Plan 6) | ⬜ pending |
| 3-06-02 | 06 | 1 | HELP-02 | — | `Holiday.IsInRegion("PL-SL")` true for Śląskie ferie cohort; false for cohort B | unit | `go test -run TestHoliday_IsInRegion -count=1 ./...` | ❌ W0 (Plan 6) | ⬜ pending |
| 3-06-03 | 06 | 1 | HELP-03 | — | `Holiday.Days()` returns 14 for 14-day ferie zimowe (DST-safe via UTC midnight) | unit | `go test -run TestHoliday_Days -count=1 ./...` | ❌ W0 (Plan 6) | ⬜ pending |
| 3-06-04 | 06 | 1 | HELP-04 | — | `Holiday.Range()` yields 14 `Date` values inclusive (CL-11 deviation from ROADMAP `iter.Seq[time.Time]`) | unit | `go test -run TestHoliday_Range -count=1 ./...` | ❌ W0 (Plan 6) | ⬜ pending |
| 3-07-01 | 07 | 2 | HELP-02 (extended) | T-3-DoS-CycleInChildren | `Client.IsInRegion` walks DE-BY/Augsburg tree; cycle defense bounds iterations | unit | `go test -run TestClient_IsInRegion -count=1 ./...` | ❌ W0 (Plan 7) | ⬜ pending |
| 3-08-01 | 08 | 3 | TEST-03 | T-3-Tampering-FixtureClobber | `-update` flag double-gated by `//go:build integration` AND `OPENHOLIDAYS_LIVE=1`; sanity-check guards against empty/non-200 overwrite | integration | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -update -run TestUpdateFixtures ./...` | ❌ W0 (Plan 8) | ⬜ pending |
| 3-08-02 | 08 | 3 | TEST-02 | — | No unit test makes live HTTP calls (grep gate) | grep | `! grep -rl 'http\.Get\|http\.DefaultClient\.Do' *_test.go` | ✅ (Phase 2 invariant) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `request.go` + `request_test.go` — `doJSONGet[T any]`, `validateHolidays`, moved constants/helpers (Plan 1)
- [ ] `countries.go` refactor — call `doJSONGet[[]Country]`; signature retrofit to `Countries(ctx, CountriesRequest)` (Plan 1)
- [ ] `languages.go` + `languages_test.go` — covers ENDPT-02 (Plan 2)
- [ ] `subdivisions.go` + `subdivisions_test.go` — covers ENDPT-03 (Plan 3)
- [ ] `public_holidays.go` + `public_holidays_test.go` — covers ENDPT-04 + TEST-01 (Plan 4)
- [ ] `school_holidays.go` + `school_holidays_test.go` — covers ENDPT-05 (Plan 5)
- [ ] `holiday.go` + `holiday_test.go` — covers HELP-01..04 (Plan 6)
- [ ] `client_isinregion.go` + `client_isinregion_test.go` (or appended to `client.go`/`client_test.go`) — covers extended HELP-02 (Plan 7)
- [ ] `update_fixtures_test.go` (build-tagged `//go:build integration`) — covers TEST-03 (Plan 8)
- [ ] `testdata/languages.json`, `subdivisions_pl.json`, `subdivisions_de.json`, `public_holidays_pl_2025.json`, `school_holidays_pl_2025.json` — captured live during Phase 3
- [ ] Re-capture `testdata/countries.json` after Countries retrofit (Plan 1)
- [ ] `internal_test.go` `allowedVars` extended to include `ErrMalformedResponse` (Plan 4)
- [ ] Framework install: **none required** (testify already in `go.mod`)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live upstream contract regression check | TEST-03 / drift detection | Requires live network; cannot run in offline CI | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 ./...` (without `-update`) — fixture diff surfaces silent upstream shape changes |
| Fixture pretty-print stability | (test infra hygiene) | gofmt-style stable diff requires `json.Indent` round-trip; only enforced when `-update` is run | After running `-update`, `git diff testdata/` should show only intended deltas |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (request.go, validateHolidays, ErrMalformedResponse allowlist, 5 fixtures + re-captured countries.json, 8 endpoint/helper source+test files)
- [ ] No watch-mode flags (Go `testing` is one-shot per invocation)
- [ ] Feedback latency < 10s (full race suite)
- [ ] `nyquist_compliant: true` set in frontmatter after planner verifies every plan task maps to a row above

**Approval:** pending
