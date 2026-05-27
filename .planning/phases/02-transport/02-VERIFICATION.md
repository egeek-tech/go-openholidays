---
phase: 02-transport
verified: 2026-05-27T14:58:45Z
status: passed
score: 5/5 must-haves verified
overrides_applied: 0
---

# Phase 2: Transport Verification Report

**Phase Goal:** `Client` constructed via functional options, RoundTripper chain composes header + logging, `Countries` proves the end-to-end pipeline (NewClient → chain → decode → typed return).
**Verified:** 2026-05-27T14:58:45Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth (Success Criterion) | Status | Evidence |
|---|---------------------------|--------|----------|
| 1 | `NewClient(WithBaseURL(ts.URL), WithUserAgent("test/1"))` returns usable client; `c.Countries(ctx)` against httptest.Server returns typed `[]Country`; server received `Accept: application/json` plus `User-Agent` matching `^go-openholidays/` | VERIFIED (with WARNING) | End-to-end: `countries_test.go:48-81` happy-path subtest uses `NewClient(WithBaseURL(srv.URL))` and asserts typed `[]Country` with `IsoCode == "PL"/"DE"`, `NameFor("PL") == "Polska"`, etc. Header injection unit-isolated in `transport_header_test.go:43-78` (asserts `captured.Header.Get("Accept") == "application/json"` and `captured.Header.Get("User-Agent") == "go-openholidays/"+Version`). Chain wiring at `config.go:87-96` (`buildTransport`) inserts `&headerTransport{userAgent: cfg.userAgent}` into every request path — `grep -c '&headerTransport{' config.go` = 1. Test run: `TestClient_Countries/happy_path_returns_PL+DE_from_fixture` PASS in 0.00s. WARNING: no httptest handler in `countries_test.go` calls `r.Header.Get("Accept")` to verify the server-side observation directly — header contract is proven by composition of unit-isolated assertions + chain wiring. The transitive proof is sound but the literal text of the SC requests a server-side observation. |
| 2 | `ctx, cancel := context.WithCancel(...); cancel(); c.Countries(ctx)` returns within ≤ 100 ms with `errors.Is(err, context.Canceled)` true (TestClient_ContextCancel). | VERIFIED | `client_test.go:180-214` `TestClient_ContextCancel`: server hangs 10s on `r.Context().Done()`; caller cancels at 50ms via `time.AfterFunc`; asserts `elapsed < 200*time.Millisecond` AND `errors.Is(err, context.Canceled)` true. Test ran in 0.05s — well under the 200ms ceiling. Wrap preservation verified through `fmt.Errorf("openholidays: GET /Countries: %w", err)` at `countries.go:93`. |
| 3 | `TestClient_ConcurrentAccess` runs N parallel `Countries` calls under `-race` and exits cleanly with zero data-race reports. | VERIFIED | `client_test.go:133-174` `TestClient_ConcurrentAccess`: 50 goroutines call `c.Countries(context.Background())` against a 5-20ms-delay httptest server; asserts all 50 succeed AND payload[i] == payload[0]. Test run: `go test -race` → `PASS` in 0.04s with zero race reports. Coverage: 94.1%. Client struct (`client.go:31-38`) has only `closed atomic.Bool` mutable; all other fields are immutable after `NewClient`. |
| 4 | 12 MiB body causes typed oversized-response error (10 MiB cap via `io.LimitReader`); response bodies are always drained then closed; goleak-style FD audit verifies. | VERIFIED (with WARNING) | `countries_test.go:200-263` "oversize triggers ErrResponseTooLarge with no goroutine leak (D-49)" streams 11 MiB JSON, asserts `errors.Is(err, ErrResponseTooLarge)` true, and asserts `runtime.NumGoroutine` delta ≤ +5 after 200ms settle. Test PASS in 0.67s. Defer drain at `countries.go:95-102` (`io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1)); resp.Body.Close()`) runs on every code path. Decoder uses `io.LimitedReader{R: resp.Body, N: maxResponseBytes}` at `countries.go:107`. ROADMAP SC says "12 MiB" but test uses "11 MiB" (CONTEXT.md D-49 codified 11 MiB) — both exceed the 10 MiB cap, so functionally equivalent. WARNING: WR-04 in 02-REVIEW.md flags the `runtime.NumGoroutine` audit as known-flaky compromise vs. `goleak`; CONTEXT D-49 accepts this tradeoff. |
| 5 | `Client.Close()` exists as an idempotent no-op stub callable from any goroutine; logging emits structured `slog` records at `Debug` level with `method`, `path`, `status`, `duration_ms`, `attempt`, `bytes_in` fields — never response bodies above `Debug`. | VERIFIED | `client.go:81-84` `Close() error { c.closed.Store(true); return nil }` — atomic flag flip. `client_test.go:108-125` "concurrent close is race-safe (100 goroutines)" PASS. `transport.go:108-120` `loggingTransport.RoundTrip` emits exactly one `LogAttrs(req.Context(), slog.LevelDebug, "openholidays http", ...)` call with all six OBS-02 fields. `transport_logging_test.go:57-95` asserts `rec["level"] == "DEBUG"` and all six fields. `transport_logging_test.go:161-190` asserts via `trackedReader` that `tr.reads.Load() == 0` after RoundTrip (OBS-01 invariant — body never read). `grep -c "resp.Body" transport.go` = 0 — body is genuinely never touched. |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `transport.go` | headerTransport + loggingTransport + statusOf + bytesIn | VERIFIED | 156 lines; all four symbols present; no init/no package-level vars; chain wiring contract documented in file header godoc. |
| `transport_header_test.go` | TestHeaderTransport_RoundTrip + roundTripperFunc adapter | VERIFIED | 131 lines; `type roundTripperFunc` declared (shared with logging_test); 3 subtests cover defaults-set, defaults-preserved, error-propagated; Pitfall HTTP-2 (caller req.Header untouched) asserted. |
| `transport_logging_test.go` | TestLoggingTransport_RoundTrip with OBS-01/OBS-02 contracts | VERIFIED | 191 lines; 4 subtests: all six OBS-02 fields, ContentLength=-1 forwarded as bytes_in=-1, nil-resp paths give status=-1 & bytes_in=-1, trackedReader confirms no body read (OBS-01). |
| `config.go` | clientConfig + defaultConfig + composeHTTPClient + buildTransport | VERIFIED | 96 lines; all four symbols; `cp := *cfg.httpClient` shallow copy at line 69 (D-37); chain order header→logging→underlying at lines 92-94; `http.DefaultTransport` fallback at line 90. |
| `options.go` | Option type + 5 WithX constructors | VERIFIED | 117 lines; `type Option func(*clientConfig)`; all five WithX (HTTPClient, BaseURL, UserAgent, Logger, Timeout); `strings.TrimRight(u, "/")` trim; nil-logger → slog.Default; zero-timeout stored verbatim. |
| `client.go` | Client struct + NewClient + Close | VERIFIED | 85 lines; Client struct with 6 unexported fields including `closed atomic.Bool`; NewClient applies opts and calls composeHTTPClient; Close = atomic.Store + nil. |
| `client_test.go` | TestNewClient + TestClient_Close + TestClient_ConcurrentAccess + TestClient_ContextCancel | VERIFIED | 215 lines; all 4 top-level tests present; CR-01 fix unaffected; 100-goroutine concurrent Close subtest + 50-goroutine ConcurrentAccess. |
| `options_test.go` | 5 TestWithX functions | VERIFIED | 173 lines; one TestWithX per Option per Gold Rule 3; shallow-copy isolation (Pitfall HTTP-1) + CheckRedirect preservation asserted. |
| `countries.go` | Client.Countries + buildAPIError + parseAPIMessage + maxResponseBytes const | VERIFIED | 192 lines; signature matches ENDPT-01; D-42 ordering preserved (nil-ctx → WithTimeout → NewRequestWithContext → Do → defer drain-close → status check → decode → sentinel gates). CR-01 fix applied (commit 20ccdf7): `limited.N == 0` for mid-truncation and `decoder.More()` for boundary-truncation. parseAPIMessage priority detail → title → error matches RFC 7807. |
| `countries_test.go` | TestClient_Countries with 6+ subtests + countriesFixtureCapturedAt const | VERIFIED | 302 lines; 9 subtests including CR-01 regression: happy-path PL+DE, 4xx with detail, 5xx with title, error fallback, 4 KiB body cap, empty body → ErrEmptyResponse, nil-ctx defensive, oversize → ErrResponseTooLarge with goroutine-leak audit, CR-01 trailing whitespace is NOT oversize. `countriesFixtureCapturedAt = "2026-05-27"` declared. |
| `errors.go` (MOD) | ErrResponseTooLarge sentinel appended | VERIFIED | Line 43 `ErrResponseTooLarge = errors.New("openholidays: response too large")`. Multi-line godoc documents both boundary- and mid-truncation cases. Total exported sentinels = 6 (5 Phase 1 + this one). |
| `errors_test.go` (MOD) | TestSentinelErrors + TestSentinels_ErrorsIs extended to cover ErrResponseTooLarge | VERIFIED | Lines 25-32 / 71-78 include `{"ErrResponseTooLarge", ErrResponseTooLarge}`; both tests pass for 6 sentinels with prefix + identity + wrap-recoverable checks. |
| `internal_test.go` (MOD) | allowedVars contains ErrResponseTooLarge | VERIFIED | Line 60 `"ErrResponseTooLarge": {},`. CLIENT-10 audit `TestNoInitOrGlobalState` PASS — zero unexpected vars, zero init() functions in production code. |
| `validate.go` (MOD) | isTwoASCIILetters byte-level helper; reorder before strings.ToUpper/ToLower | VERIFIED | Lines 110-121 `isTwoASCIILetters`; called at lines 34 and 58 BEFORE strings.ToUpper/ToLower; `grep -c "unicode" validate.go` = 0 (no unicode package import — byte arithmetic only per D-32). |
| `validate_test.go` (MOD) | 8 W-01 regression cases (4 per validator) | VERIFIED | Country rejectCases lines 74-77 (`ıA`, `ſA`, `ıı`, `ſſ`); Language rejectCases lines 153-156 (`KK`, `İa`, `İİ`, `Ka`). All 8 W-01 subtests pass under `-race`. Leading comment blocks document the ToUpper/ToLower fold for each. |
| `testdata/countries.json` | 2 entries PL + DE with multi-language Name | VERIFIED | `jq 'length'` = 2; isoCodes = {DE, PL}; PL entry has `name[]` with EN/PL/DE; DE entry has `name[]` with EN/DE; both have non-empty `officialLanguages`. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `headerTransport.RoundTrip` | `http.Request.Clone(req.Context())` | deep-copy before header mutation | WIRED | `transport.go:61` `reqCopy := req.Clone(req.Context())`; asserted by `transport_header_test.go:74-77` (caller's req.Header untouched). |
| `loggingTransport.RoundTrip` | `slog.Logger.LogAttrs` | single Debug record per round trip | WIRED | `transport.go:111` exact pattern `l.logger.LogAttrs(req.Context(), slog.LevelDebug, "openholidays http", ...)`. |
| `transport_header_test.go` | `transport_logging_test.go` | shared roundTripperFunc adapter | WIRED | Declared once in `transport_header_test.go:19`; `grep -c "type roundTripperFunc" transport_logging_test.go` = 0; both files compile in same package without redeclaration. |
| `NewClient` | `composeHTTPClient → buildTransport → headerTransport → loggingTransport` | options applied to clientConfig, materialized into immutable Client | WIRED | `client.go:65` `http: composeHTTPClient(cfg)`; `config.go:68-72` composeHTTPClient shallow-copies + assigns Transport; `config.go:87-96` buildTransport composes chain. End-to-end exercised by `TestClient_Countries/happy_path`. |
| `composeHTTPClient` | shallow copy of cfg.httpClient | `cp := *cfg.httpClient` (Pitfall HTTP-1) | WIRED | `config.go:69`; isolation asserted by `TestWithHTTPClient/non-nil shallow-copies` (mutating caller's `*http.Client.Timeout` post-NewClient does NOT change `c.http.Timeout`). |
| `Countries` | `ErrResponseTooLarge` sentinel | post-Decode sentinel via `decoder.More()` (CR-01 fix) + `limited.N == 0` mid-truncation | WIRED | `countries.go:123` (mid-truncation), `countries.go:136` (boundary via decoder.More); CR-01 regression test pins both fixes. |
| `Countries` | `buildAPIError → *APIError` | construct on `resp.StatusCode >= 400` | WIRED | `countries.go:103-104` `if resp.StatusCode >= 400 { return nil, buildAPIError(resp, "/Countries") }`. Three subtests cover 4xx/5xx/503 priorities. |
| `Countries` | defer drain-then-close | `io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1)); resp.Body.Close()` | WIRED | `countries.go:95-102`; bounded drain prevents infinite-stream block (T-02-12); goroutine-leak audit confirms no leak after 11 MiB response. |
| `internal_test.go allowedVars` | `ErrResponseTooLarge` | CLIENT-10 AST audit allowlist | WIRED | `internal_test.go:60`; `TestNoInitOrGlobalState` PASS. |
| `countries_test.go` | `testdata/countries.json` | `os.ReadFile(filepath.Join("testdata", "countries.json"))` | WIRED | `countries_test.go:51` (happy path) and `client_test.go:136` (concurrent access). |
| `validateCountry / validateLanguage` | `isTwoASCIILetters(code)` | ASCII shape check BEFORE strings.ToUpper/ToLower | WIRED | `validate.go:34` and `validate.go:58` — checks run on ORIGINAL bytes; 8 W-01 regression cases lock the reorder semantics. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|---------------------|--------|
| `Client.Countries` | `countries []Country` | `json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&countries)` | YES — typed `[]Country` returned from upstream JSON; fixture-replay produces 2 PL+DE entries with all 3 fields populated | FLOWING |
| `*APIError.Body` | `body []byte` | `io.ReadAll(io.LimitReader(resp.Body, apiErrorBodyCap))` in `buildAPIError` | YES — capped at 4 KiB; asserted by 4xx body truncation subtest | FLOWING |
| `*APIError.Message` | `msg string` | `parseAPIMessage(body)` from JSON envelope | YES — priority detail → title → error all three exercised by 4xx/5xx/503 subtests | FLOWING |
| `slog` record | 6-field LogAttrs | per-request data from `req.Method`, `req.URL.Path`, `resp.StatusCode`, `time.Since(start)`, hardcoded 1, `resp.ContentLength` | YES — JSON-decoded record contains all six fields with non-zero/non-nil values; nil-resp paths produce -1 sentinels (proven by 3rd logging subtest) | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Module compiles | `go build ./...` | exit 0, no output | PASS |
| Vet clean | `go vet ./...` | exit 0, no output | PASS |
| Full test suite (race + cover) | `go test -race -cover ./...` | `ok ... 1.727s coverage: 94.1% of statements` | PASS |
| Phase 2 umbrella tests | `go test -race -run "TestClient_Countries\|TestClient_ConcurrentAccess\|TestClient_ContextCancel\|TestNewClient\|TestClient_Close" -v ./...` | All subtests PASS in 1.7s | PASS |
| W-01 regression | `go test -race -run "TestValidateCountry\|TestValidateLanguage" -v ./...` | 8 W-01 subtests PASS | PASS |
| CLIENT-10 audit | `go test -race -run TestNoInitOrGlobalState ./...` | PASS — no init, no unexpected package vars | PASS |
| Fixture shape | `jq 'length' testdata/countries.json` (verified earlier) | `2` | PASS |
| Fixture isoCodes | `jq -r '.[] | .isoCode' testdata/countries.json | sort | tr '\n' ' '` | `DE PL ` | PASS |

### Probe Execution

Phase 2 does not declare probe scripts (`scripts/*/tests/probe-*.sh`); no probe directory exists in this Go library project. The behavioral spot-checks above run the project's mandated probe equivalent (`go test -race -cover ./...`).

| Probe | Command | Result | Status |
|-------|---------|--------|--------|
| (none declared) | n/a | n/a | N/A |

### Requirements Coverage

All 17 Phase 2 requirements claimed by plans cross-reference REQUIREMENTS.md:

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| CLIENT-01 | 02-02 | `NewClient(opts ...Option) *Client`; never returns an error | SATISFIED | `client.go:59-71` NewClient; no error return; `TestNewClient/defaults_applied` PASS |
| CLIENT-02 | 02-02 | `WithHTTPClient` shallow-copies | SATISFIED | `options.go:41-47` + `config.go:68-72`; `TestWithHTTPClient/non-nil_shallow-copies` PASS asserts post-mutation isolation |
| CLIENT-03 | 02-02 | `WithBaseURL` overrides default | SATISFIED | `options.go:58-65`; trailing-slash trim covered by 4-case table in `TestWithBaseURL` |
| CLIENT-04 | 02-02 | `WithUserAgent` overrides default | SATISFIED | `options.go:75-81`; empty-string no-op (D-38) covered by `TestWithUserAgent/empty_string_is_no-op` |
| CLIENT-05 | 02-02 | `WithLogger` injects slog.Logger; defaults to slog.Default | SATISFIED | `options.go:90-98`; nil-fallback covered; `grep -c slog.SetDefault *.go` = 0 (library never mutates global) |
| CLIENT-06 | 02-02 | `WithTimeout` sets per-request timeout (default 15s) | SATISFIED | `options.go:113-117` + `config.go:55` default `15*time.Second`; `TestWithTimeout` covers positive/zero/negative |
| CLIENT-07 | 02-03 | Client goroutine-safe (TestClient_ConcurrentAccess under -race) | SATISFIED | `client_test.go:133-174`; 50 parallel calls PASS under -race; coverage 94.1% |
| CLIENT-08 | 02-02 | `Close()` idempotent; safe from any goroutine | SATISFIED | `client.go:81-84` atomic flag flip; 100-goroutine concurrent subtest PASS |
| CLIENT-09 | 02-03 | ctx cancellation interrupts within ≤ 100 ms (TestClient_ContextCancel) | SATISFIED | `client_test.go:180-214`; 50ms cancel + 200ms ceiling PASS; `errors.Is(err, context.Canceled)` holds through fmt.Errorf wrap |
| ENDPT-01 | 02-03 | `Countries(ctx) ([]Country, error)` | SATISFIED | `countries.go:78-139` exact signature; happy path + 8 subtests PASS |
| TRANS-01 | 02-01 | All requests include Accept + User-Agent | SATISFIED | `transport.go:60-68` headerTransport; unit-isolated assertion + chain-wiring proof |
| TRANS-02 | 02-03 | 10 MiB cap via io.LimitReader; typed error | SATISFIED | `countries.go:42` const; `countries.go:107` LimitReader; `countries.go:122/136` ErrResponseTooLarge sentinel; oversize subtest PASS |
| TRANS-03 | 02-03 | Bodies drained then closed via defer | SATISFIED | `countries.go:95-102` defer drain-then-close on every path; goroutine-leak audit confirms |
| TRANS-04 | 02-01 | RoundTripper chain composes header + logging + each unit-tested | SATISFIED | `transport.go` two RoundTrippers; `transport_header_test.go` + `transport_logging_test.go` unit-isolate each |
| OBS-01 | 02-01 | Requests logged at Debug only; body never logged | SATISFIED | `transport.go:111` LogAttrs at slog.LevelDebug only; `grep -c "resp.Body" transport.go` = 0; trackedReader subtest confirms |
| OBS-02 | 02-01 | Structured fields: method, path, status, duration_ms, attempt, bytes_in | SATISFIED | `transport.go:111-118` exactly 6 fields; logging_test asserts each by name |
| TEST-04 | 02-03 | TestClient_ConcurrentAccess under -race | SATISFIED | Same as CLIENT-07 evidence |

Additionally claimed (in-scope per CONTEXT.md D-32 / D-34):

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| VALID-01 | 02-04 | Country code validation (2 ASCII uppercase letters) | HARDENED | W-01 fix reorders ASCII shape check before strings.ToUpper; 4 regression cases lock the fix |
| VALID-04 | 02-04 | Language code ISO 639-1 validation | HARDENED | Mirror of VALID-01; 4 regression cases (`KK`, `İa`, `İİ`, `Ka`) |

Phase 2 traceability complete — all 17 requirements have at least one mechanical test assertion AND production-code wiring.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | | TODO/FIXME/XXX/TBD/PLACEHOLDER in production code | n/a | `grep -rnE "TODO\|FIXME\|XXX\|TBD\|PLACEHOLDER" --include="*.go" .` (excluding `.planning/`, `testdata/`, `.git/`) returns ZERO results |
| `validate.go` | 128-143 | Unused helpers `isTwoASCIIUppers` / `isTwoASCIILowers` (REVIEW WR-01) | Info | Will fail `staticcheck` (`U1000: unused function`) once lint gate runs in CI (Phase 5). Documented as known compromise — comment block explicitly states "Currently unreachable from validateCountry/validateLanguage after the W-01 reorder; retained as defense-in-depth and for direct testing". |
| `options.go` | 58-65 | `WithBaseURL("/")` trims to empty string and assigns it (REVIEW WR-02) | Info | Open issue from 02-REVIEW.md; not blocking Phase 2 goal (default still kept on `WithBaseURL("")`); flagged for follow-up before v0.1.0 tag |
| `internal_test.go` | 69-75 | `skipDirs` includes `"internal"` (REVIEW WR-03) | Info | Phase 1 W-03 follow-up; explicitly out of scope per CONTEXT.md D-34. No `internal/` package exists yet so audit invariant is not currently bypassed. |
| `countries_test.go` | 200-263 | `runtime.NumGoroutine` audit instead of `go.uber.org/goleak` (REVIEW WR-04) | Info | Acknowledged compromise per CONTEXT.md D-49 (goleak requires new dep + Key Decisions entry). Currently passing; flagged as known flake candidate. |
| `client_test.go` | 128-214 | `TestClient_ConcurrentAccess` / `TestClient_ContextCancel` not bound to `Client.Countries` per Gold Rule 3 (REVIEW WR-05) | Info | CONTEXT.md D-47/D-48 mandate these test names; documented context-vs-rule tension. Tests do PASS and provide coverage; Gold Rule 3 reading is strict. |

All warnings are Info-level open code-review items documented in 02-REVIEW.md — none block goal achievement. The one BLOCKER from the review (CR-01: false-positive ErrResponseTooLarge on chunked responses) was fixed in commit 20ccdf7 and pinned by a regression subtest (`TestClient_Countries/CR-01_regression`).

### Human Verification Required

None. All five ROADMAP success criteria are mechanically verifiable and have passing automated assertions. The header-injection contract (SC #1) is verified via composition of unit-isolated `transport_header_test.go` assertions + the chain wiring + the end-to-end `TestClient_Countries/happy_path` — sound but not strictly tested via `r.Header.Get(...)` at the server side. This is a verification rigor note (recorded as Info-level WARNING), not a behavior gap.

### Gaps Summary

No gaps found that prevent goal achievement.

The phase delivers:
- Functional Option pattern (CLIENT-01..06, CLIENT-08) with documented defaults, shallow-copy isolation, idempotent atomic Close.
- RoundTripper chain (TRANS-01, TRANS-04, OBS-01, OBS-02) with header injection via `req.Clone`, single Debug-level slog record with all six OBS-02 fields, no body reads inside transports.
- End-to-end Countries endpoint (ENDPT-01, TRANS-02, TRANS-03) with nil-ctx guard, per-request `context.WithTimeout`, drain-then-close defer, RFC 7807 ProblemDetails parsing, 4 KiB APIError.Body cap, 10 MiB decode cap with `ErrResponseTooLarge` sentinel.
- Concurrency safety (CLIENT-07, CLIENT-09, TEST-04) — 50-goroutine -race verification + ≤ 200ms ctx-cancel verification + 100-goroutine Close safety.
- W-01 validator hardening (VALID-01, VALID-04) — ASCII shape check before case canonicalization closes the Unicode case-fold bypass; 8 regression cases pin the fix.
- CR-01 BLOCKER fixed in commit 20ccdf7: `decoder.More()` replaces unsound sentinel-byte read; regression subtest locks the fix.

Test suite: `go test -race -cover ./...` PASS in 1.727s with 94.1% coverage. No production code carries TODO/FIXME/XXX/TBD markers. CLIENT-10 audit (`TestNoInitOrGlobalState`) green.

Five Info-level WARNINGS recorded for future cleanup (WR-01..WR-05 from 02-REVIEW.md) — all are quality/maintainability items, none block the phase goal.

---

_Verified: 2026-05-27T14:58:45Z_
_Verifier: Claude (gsd-verifier)_
