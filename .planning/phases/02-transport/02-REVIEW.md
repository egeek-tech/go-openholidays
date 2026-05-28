---
phase: 02-transport
reviewed: 2026-05-28T00:00:00Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - client.go
  - client_test.go
  - config.go
  - countries.go
  - countries_test.go
  - errors.go
  - errors_test.go
  - internal_test.go
  - options.go
  - options_test.go
  - testdata/countries.json
  - transport.go
  - transport_header_test.go
  - transport_logging_test.go
  - validate.go
  - validate_test.go
findings:
  critical: 0
  warning: 0
  info: 0
  total: 0
status: clean
---

# Phase 02: Code Review Report (Round 4 / Third Re-Review)

**Reviewed:** 2026-05-28T00:00:00Z
**Depth:** standard
**Files Reviewed:** 16
**Status:** clean

## Summary

Third re-review of Phase 02 (transport / client / errors / validate / Countries endpoint) after the round-3 fix pass landed in commits `5b1c66c..805fdf3`. All four round-3 findings (WR-01 dead userAgent/logger fields, IN-01 fnv.Write errcheck, IN-02 dead Client.closed, IN-03 vacuous require.NotNil) are mechanically confirmed in place. A fresh adversarial sweep across the 16 in-scope files surfaced no remaining bugs, no security vulnerabilities, and no quality defects worth flagging.

The code is ready to ship.

## Round 3 Fix Verification

### WR-01 — Client.userAgent and Client.logger removed

- `client.go:58-69` — `Client` struct contains no `userAgent` field and no `logger` field. The fields documented in the godoc block as "removed by the WR-01 (re-review) follow-up" are indeed absent. Confirmed.
- `client.go` imports — no `log/slog` import. The struct field that previously required it is gone; `slog.Default()` is referenced from `config.go::defaultConfig` instead. Confirmed.
- `options_test.go:131-156` — `headerTransportFromChain(t, c)` and `loggingTransportFromChain(t, c)` helpers correctly walk the default chain `*loggingTransport -> *headerTransport` to inspect transport-side state.
- `options_test.go::TestWithUserAgent` (lines 109-129) — both subtests now assert via `headerTransportFromChain(t, cli).userAgent`. Confirmed.
- `options_test.go::TestWithLogger` (lines 166-193) — both subtests now assert via `loggingTransportFromChain(t, cli).logger`. Confirmed.
- `client_test.go::TestNewClient::defaults applied when no Option supplied` (lines 67-90) — type-asserts the chain `*loggingTransport -> *headerTransport` and asserts both `lt.logger != nil` and `ht.userAgent == "go-openholidays/"+Version`. Confirmed.

### IN-01 — fnv.Write errcheck idiom

- `client.go:188-194` — all four `Write` calls on `h1`/`h2` use the `_, _ = h.Write(...)` discard idiom:

```go
_, _ = h1.Write(tb[:])
_, _ = h1.Write(pb[:])
...
_, _ = h2.Write(pb[:])
_, _ = h2.Write(tb[:])
```

- Comment at `client.go:183-186` documents why the explicit discard is the project convention (errcheck flags unchecked returns regardless of the `hash.Hash` "never errors" guarantee). Confirmed.

### IN-02 — Client.closed atomic.Bool removed

- `client.go:58-69` — `Client` struct has no `closed atomic.Bool` field. Confirmed.
- `client.go` imports — no `sync/atomic` import. Only `sync` is imported (for `sync.Once`). Confirmed.
- `client.go:127-134` — `Close()` is now a clean six-liner: `c.closeOnce.Do(func() { if c.cache != nil { _ = c.cache.Close() } }); return nil`. No flag-set, no atomic operation. Confirmed.
- Grep for `c.closed` / `client.closed` across the repo — zero matches. Confirmed.
- `client_test.go` continues to import `sync/atomic` but only for unrelated helpers (`countingBody.closed`, `countriesServer.hits`, `hookCount`, `lastIsCacheHit`). The atomic import is load-bearing for these test fixtures and is not a remnant of the removed field.

### IN-03 — Vacuous require.NotNil removed

- `errors_test.go:41` — the load-bearing `require.NotNil(t, s.err, "sentinel must be non-nil")` inside `TestSentinelErrors` is preserved. Confirmed.
- `errors_test.go:89` — line content is `wrapped := fmt.Errorf("context %q: %w", "ZZZ", s.err)` — not a `require.NotNil`. The vacuous assertion that previously sat near this line is gone.
- `errors_test.go:151` — line content is `assert.Equal(t, c.want, c.err.Error())` — not a `require.NotNil`.
- `errors_test.go:222` — line content is the assertion-message fragment `"errors.Is(base{status=%d}, target=%T{...}) mismatch",` — not a `require.NotNil`.
- `errors_test.go:253` — line content is `assert.Equal(t, "msg", got.Message, "Message preserved")` — the load-bearing `require.True(t, errors.As(wrapped, &got), ...)` lives at line 247 instead (the actually-load-bearing assertion). Confirmed.
- The four vacuous `require.NotNil` calls in `errors_test.go` are gone, the one load-bearing assertion at `TestSentinelErrors:41` is preserved.

## Fresh Adversarial Scan

The round-3 fix pass introduced no new defects. Specifically:

- **`client.go::newClientRand`** — fallback path is sound. Two FNV-128a hashes are seeded with `(nanoTimestamp, pid)` and `(pid, nanoTimestamp)` respectively, producing 16-byte digests each that fill the 32-byte ChaCha8 seed. The `uint64(time.Now().UnixNano())` and `uint64(os.Getpid())` conversions are intentional bit reinterpretations for hashing (sign loss has no semantic effect on a hash input).
- **`client.go::Close`** — `sync.Once` correctly guards `c.cache.Close()`. The nil-check on `c.cache` is correct (default Client has `c.cache == nil`).
- **`client.go::ctxSleep`** — `select { case <-ctx.Done(): ... case <-t.C: ... }` is the canonical interruptible-sleep pattern. The `if d <= 0 { return nil }` early-out avoids arming the timer for zero/negative durations.
- **`config.go::buildTransport`** — chain composition order matches the documented D-89 chain. Inside-out construction (innermost first) is the correct Go decorator idiom.
- **`config.go::composeHTTPClient`** — shallow-copy via `cp := *cfg.httpClient` is correct; only `Transport` is overwritten, preserving caller `Timeout`, `Jar`, `CheckRedirect` (asserted by `options_test.go::TestWithHTTPClient/preserves CheckRedirect across shallow copy`).
- **`countries.go::Countries`** — validate-language-first, then dispatch to `doJSONGet`. The order means a malformed `LanguageIsoCode` short-circuits before any HTTP, including the nil-ctx guard inside `doJSONGet` — consistent with the test at `countries_test.go:378-387` and is the documented contract.
- **`errors.go::APIError.Is`** — correctly handles `target.StatusCode == 0` as wildcard; covered exhaustively by `TestAPIError_Is` table cases.
- **`internal_test.go::allowedVars`** — closed allowlist with 10 entries. `defaultBaseURL` (added in IN-02 round 2) is a `const`, not a `var`, so it correctly falls outside the audit's `ast.GenDecl.Tok == token.VAR` filter.
- **`options.go::WithBaseURL`** — empty-string and all-slash inputs both fall back to the default. Trailing-slash trimming via `strings.TrimRight(u, "/")` is correct.
- **`options.go::WithRetry`** — `maxWait <= 0` guard ensures a prior `WithMaxRetryWait(positive)` is not clobbered by `WithRetry` (asserted by `options_test.go::TestWithRetry::WithMaxRetryWait followed by WithRetry`).
- **`transport.go::headerTransport.RoundTrip`** — unconditional `req.Clone(req.Context())` honors the http.RoundTripper "no caller mutation" contract; preserves caller-supplied Accept/User-Agent (covered by tests).
- **`transport.go::loggingTransport.RoundTrip`** — emits exactly one Debug record per round trip; never reads the response body (asserted by `TestLoggingTransport_RoundTrip::does not read resp.Body`).
- **`validate.go::isTwoASCIILetters`** — byte-arithmetic guard against Unicode case-fold bypasses; W-01 regression cases in `validate_test.go` confirm the post-fix behavior.

No bugs, no security issues, no quality defects worth flagging across any of the 16 in-scope files.

---

_Reviewed: 2026-05-28T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
