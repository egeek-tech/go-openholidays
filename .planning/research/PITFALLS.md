# Pitfalls Research

**Domain:** Go HTTP/JSON SDK library wrapping a public REST API (OpenHolidays)
**Researched:** 2026-05-26
**Confidence:** HIGH for Go stdlib HTTP/JSON pitfalls (Context7-grade stdlib knowledge, cross-referenced with Go issue tracker and AWS guidance). HIGH for OpenHolidays-specific gotchas verified against `openholidaysapi.org/swagger/v1/swagger.json`. MEDIUM for the 3-year window cap (asserted by project brief; not present in the OpenAPI spec — must be enforced client-side as a defensive measure, see Pitfall OH-1).

Phase numbering used below maps to the working M1 sketch:

- **Phase 1 — Types & Decoding** (domain types, `LocalizedText`, `Holiday`, `Subdivision`, custom `UnmarshalJSON`).
- **Phase 2 — Transport** (`Client`, `Option`s, `http.Client` hygiene, `User-Agent`, body limits).
- **Phase 3 — Endpoints** (the five `Countries`/`Languages`/`Subdivisions`/`PublicHolidays`/`SchoolHolidays` methods + client-side validation).
- **Phase 4 — Retry & Cache** (`WithRetry`, `WithCache`, `Retry-After`, jitter).
- **Phase 5 — Helpers & Iteration** (`Name(lang)`, `IsInRegion`, `Days`, `Range` iter.Seq).
- **Phase 6 — CLI** (`cmd/ohcli`).
- **Phase 7 — Tests & CI** (race, fuzz, integration gate, golangci-lint, govulncheck).
- **Phase 8 — Release** (`goreleaser`, `CHANGELOG`, `pkg.go.dev` examples, `v0.1.0` tag).

A pitfall mapped to "Phase 2" must be prevented by the time Phase 2 is reviewed; later phases may add depth (e.g. retry-on-429 prevention lives in Phase 4, but the *test* for it lives in Phase 7).

---

## Critical Pitfalls

### Pitfall HTTP-1: Sharing `http.DefaultClient` (inheriting global state)

**What goes wrong:**
The library reaches for `http.DefaultClient` or `http.Get` when the caller did not pass `WithHTTPClient`. `http.DefaultClient` has **no timeout** by default, has a shared `Transport` whose connection pool, proxy, and TLS config can be mutated by *any* code in the process, and a misbehaving server can hang every consumer indefinitely. Worse, if the caller has also mutated `http.DefaultClient` for their own reasons (common in test setups), our library inherits that state silently.

**Why it happens:**
`http.DefaultClient` is the path of least resistance — one line, no plumbing. Go beginners see it everywhere in tutorials and forget that a library has different lifetime semantics than a `main()`.

**How to avoid:**
- `NewClient` constructs its **own** `*http.Client` with a non-zero `Timeout` (15 s per the brief) and its **own** `*http.Transport` (so consumers' mutations don't leak in).
- `WithHTTPClient(c *http.Client)` is supported but documented as "we will use what you give us, including its timeout, transport, and cookie jar — that's your contract".
- Never call `http.Get`, `http.Post`, or `http.DefaultClient.Do` anywhere in the codebase. A grep -n 'http\.\(DefaultClient\|Get\|Post\|Head\|Do\)' check belongs in CI.

**Warning signs:**
- Any reference to `http.DefaultClient` or top-level `http.Get` in code review.
- Tests that pass in isolation but flake under `go test ./... -count=10` (shared-state symptom).
- A consumer reporting "your library hangs forever on a network blip" → timeout was not enforced.

**Phase to address:** Phase 2 (transport).

---

### Pitfall HTTP-2: Forgetting `resp.Body.Close()` on every return path

**What goes wrong:**
An early return between `c.httpClient.Do(req)` and `defer resp.Body.Close()` leaks a file descriptor, leaks the underlying TCP connection (it can't be returned to the pool), and on a long-running server eventually produces `too many open files` or socket exhaustion. The bug is invisible at unit-test scale and shows up in production at week 3.

**Why it happens:**
A `return err` slipped in between the `Do` call and the `defer`, e.g. after a status-code check. Or the early return happens *before* `Do` succeeded but *after* `resp` is non-nil — Go's `http.Client.Do` contract says: if `err == nil` then `resp.Body` is non-nil and must be closed, **even on 4xx/5xx**.

**How to avoid:**
- **Pattern:** Place `defer resp.Body.Close()` on the line immediately following the `err == nil` check on the `Do` result. No code between the `Do` and the `defer` may early-return.
- A small private helper `doRequest(ctx, req) (*http.Response, error)` whose contract is "if err is nil, caller owns Close" centralizes the rule.
- `errcheck` (already in the golangci-lint profile) catches naked `resp.Body.Close()` without checking the return — but **does not** catch missing-close. A custom `staticcheck` SA9001/SA5012 sweep does.

**Warning signs:**
- File-descriptor growth in a long-running test harness (`lsof -p $PID | wc -l` trending up).
- `dial tcp: lookup ...: too many open files` in production.
- Any function that does `c.http.Do(req)` and then has more than one `return` between that line and a deferred close.

**Phase to address:** Phase 2 (transport plumbing). Verified again in Phase 7 via a long-running integration test that issues 10 000 requests and asserts FD count stays flat.

---

### Pitfall HTTP-3: Closing without draining (`io.Copy(io.Discard, resp.Body)`)

**What goes wrong:**
Connection-pool starvation. If the response body is closed *before* it has been read to EOF (e.g. we decoded a JSON array but the server sent trailing whitespace, or we hit a `json.Decoder.Decode` error and bailed early, or we got a 4xx and called `Close` immediately), Go's HTTP/1.1 transport cannot return the TCP connection to the keep-alive pool. Each request burns a fresh TCP handshake; throughput drops 5-10×; cold latency dominates the 500 ms P95 budget.

**Why it happens:**
Decoder errors, status-code short-circuits, and "read just enough to know the shape" patterns. Developers assume `Close()` reclaims everything — historically it did not. **Recent change to be aware of:** in current Go (post-CL 737720, landing late 2024 / Go 1.24 stream), `Body.Close()` auto-drains up to 256 KiB or 50 ms, whichever first. That helps small responses but is **not** a license to skip explicit draining for larger payloads (a year of holidays per country is small, but `Subdivisions` for 30 countries with full localization could exceed 256 KiB; in any case the 50 ms cap means a slow drain still loses the connection).

**How to avoid:**
- Helper:
  ```go
  func drainAndClose(b io.ReadCloser) {
      _, _ = io.Copy(io.Discard, io.LimitReader(b, 1<<20)) // cap drain at 1 MiB
      _ = b.Close()
  }
  ```
- Call `drainAndClose(resp.Body)` from the `defer` instead of `resp.Body.Close()`. The `LimitReader` cap is belt-and-braces against a malicious server that streams forever.
- For 4xx/5xx paths, read the error body via `io.LimitReader(resp.Body, 64<<10)` into the `*APIError.Body` field *first*, then drain.

**Warning signs:**
- Benchmark shows latency rising as request count grows.
- `netstat -an | grep TIME_WAIT | wc -l` climbs.
- `Transport.IdleConnTimeout` firing constantly under load.

**Phase to address:** Phase 2 (drain helper in the transport core). Phase 7 micro-benchmark proves keep-alive reuse stays ≥ 95 %.

---

### Pitfall HTTP-4: No `io.LimitReader` cap on response bodies (OOM risk)

**What goes wrong:**
A misbehaving or compromised server streams a 10 GB response; the library calls `json.NewDecoder(resp.Body).Decode(&out)` which streams happily; allocator OOMs the consumer's process. Brief specifies a **10 MiB cap**. Without it, the library is a denial-of-service vector for any user who pointed it at a hostile `WithBaseURL`.

**Why it happens:**
`json.NewDecoder` does not bound input; `io.ReadAll` does not bound input; neither does the stdlib give you a default cap. Engineers assume the network or the OS will stop it. They will not in time.

**How to avoid:**
- Wrap the body before decoding:
  ```go
  const maxBody = 10 << 20 // 10 MiB
  body := io.LimitReader(resp.Body, maxBody+1)
  if err := json.NewDecoder(body).Decode(&out); err != nil { … }
  // After successful decode, peek one more byte; if non-EOF, body exceeded cap.
  ```
- Or use `http.MaxBytesReader(nil, resp.Body, maxBody)` — server-side helper that also works client-side and returns a typed `*MaxBytesError` on overflow.
- Expose `WithMaxResponseSize(int64)` as an Option **only** if a user requests it; the default 10 MiB covers all OpenHolidays responses (a full year of EU-wide data is < 1 MiB).

**Warning signs:**
- Any direct `io.ReadAll(resp.Body)` without a wrapping `LimitReader`.
- `json.NewDecoder(resp.Body).Decode(&out)` without a wrap.
- Fuzz test that feeds an oversize body and watches the process not OOM.

**Phase to address:** Phase 2. Fuzz/property test in Phase 7.

---

### Pitfall HTTP-5: Missing `User-Agent` (CDN rejection, opaque logs)

**What goes wrong:**
Some upstream CDNs and WAFs (Cloudflare, Imperva) classify requests with no `User-Agent` as bots and return 403 or 429. Even when accepted, operators of the upstream API cannot identify your client when they need to debug abuse or contact you. Brief mandates `go-openholidays/<version>` per call.

**Why it happens:**
`net/http` does not set `User-Agent` unless told to (it sets `Go-http-client/1.1`, which is even worse — looks like a generic scraper). Library authors forget because their local test fixture never enforces it.

**How to avoid:**
- Inject `User-Agent: go-openholidays/<version>` from a single `do()` helper. The version string lives in `internal/version.Version` and is `-ldflags`-injectable at release time.
- `WithUserAgent(string)` Option **prepends** to the library's UA, never replaces it — so the upstream always sees `go-openholidays/v0.1.0 caller-app/1.2.3` even if the consumer adds their own.
- Test: a `httptest.Server` that asserts the inbound `User-Agent` matches `^go-openholidays/`.

**Warning signs:**
- Sudden 403s from upstream in CI that flake.
- `WithUserAgent` Option that *replaces* rather than augments.
- The string `Go-http-client/1.1` appearing in upstream server logs.

**Phase to address:** Phase 2.

---

### Pitfall HTTP-6: Reusing `*http.Request` across calls

**What goes wrong:**
`http.Request` is **not safe** for reuse after `client.Do` returns. Its `Body` is single-read; its `URL.RawQuery` may be mutated by transport; reusing the same `*Request` from two goroutines races on internal fields. A naive "cache the request, swap context" optimization for retries produces nondeterministic failures under `-race`.

**Why it happens:**
Looks like a clean optimization for retry loops ("why rebuild the request?"). Looks fine in single-threaded tests. Blows up only under load.

**How to avoid:**
- **Build a fresh `*http.Request` for every attempt** in the retry loop. The cost is microseconds; the alternative is unreproducible bugs.
- If retries need to re-send the body (n/a here — all GETs have no body — but document the rule anyway), use `req.GetBody` and rebuild.
- Code review rule: a `*http.Request` value must never escape a single `Do` call.

**Warning signs:**
- Any field of type `*http.Request` on a struct.
- A retry loop that builds the request once before the loop body.

**Phase to address:** Phase 4 (retry loop).

---

### Pitfall CTX-1: Storing `ctx` in struct fields

**What goes wrong:**
A field like `type Client struct { ctx context.Context }` ties the lifetime of every request to one ambient context. The caller's per-request cancellation is silently ignored; long-running clients leak goroutines on the original context's cancel chain; tests can't pass distinct contexts per call.

**Why it happens:**
Looks like a tidy way to "thread context once". Go's own documentation explicitly says: **do not store contexts in structs**. Library authors who haven't read that line replicate the antipattern.

**How to avoid:**
- `context.Context` is **always** the first argument of every exported method. No `Client.ctx`. Period.
- A `go vet` check (`contextcheck` lint, part of `golangci-lint`) flags this; enable it in `.golangci.yml`.
- The `WithRequestHook` callback receives the ctx-bound `*http.Request`, never a separate `ctx` field.

**Warning signs:**
- Any struct field of type `context.Context`.
- A method missing `ctx context.Context` as its first parameter.
- `golangci-lint` not configured with `contextcheck`.

**Phase to address:** Phase 2 (Client design) — enforced via lint in Phase 7.

---

### Pitfall CTX-2: Not threading `ctx` into the retry loop / goroutines

**What goes wrong:**
The retry loop computes `time.Sleep(backoff)` without selecting on `ctx.Done()`. The caller cancels after 100 ms; the library sleeps 30 s before noticing; the brief's ≤ 100 ms cancellation contract is violated. Worse, if a request hook spawns a goroutine for async logging without ctx, that goroutine leaks past cancellation.

**Why it happens:**
`time.Sleep` is one line; the `select { case <-time.After: case <-ctx.Done(): }` form is three lines. Devs default to the short version.

**How to avoid:**
- **Sleep helper:**
  ```go
  func sleepCtx(ctx context.Context, d time.Duration) error {
      t := time.NewTimer(d)
      defer t.Stop()
      select {
      case <-t.C:
          return nil
      case <-ctx.Done():
          return ctx.Err()
      }
  }
  ```
- Every backoff wait goes through `sleepCtx`. Return immediately on `ctx.Err()`; do not swallow it.
- `goroutineleak` test in Phase 7 (uber-go/goleak) verifies no goroutine outlives a cancelled context.
- `RequestHook` is documented to be **synchronous**; if a consumer wants async, they spawn — and the documented pattern is `go func() { … }()` from inside their hook.

**Warning signs:**
- `time.Sleep` anywhere in the retry/backoff code.
- A retry loop without a `<-ctx.Done()` branch.
- Cancellation tests with > 100 ms tolerance.

**Phase to address:** Phase 4 (retry). Verified in Phase 7 with `goleak`.

---

### Pitfall CTX-3: Swallowing `ctx.Err()` in error returns

**What goes wrong:**
A request fails with `context.Canceled`; the library wraps it as a generic `*APIError{StatusCode: 0, Body: "request failed"}`. Callers' `errors.Is(err, context.Canceled)` returns `false`. Their cleanup logic doesn't fire. They retry against an already-cancelled context, generating noise.

**Why it happens:**
A wrapper layer that always returns `*APIError` flattens stdlib sentinels. Or `err = fmt.Errorf("openholidays: %s", err)` without `%w`.

**How to avoid:**
- Always use `%w` when wrapping: `fmt.Errorf("openholidays: GET %s: %w", path, err)`.
- Before constructing an `*APIError`, check `if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) { return err }` and propagate untouched.
- Test: `ctx, cancel := …; cancel(); err := c.PublicHolidays(ctx, …); assert errors.Is(err, context.Canceled)`.

**Warning signs:**
- `fmt.Errorf` without `%w` near HTTP error paths.
- A flat `*APIError` returned for transport-layer failures.
- No unit test for "caller cancelled mid-flight".

**Phase to address:** Phase 2 (error type design) + Phase 7 (test).

---

### Pitfall JSON-1: Strict decoding by default → upstream-add-field break

**What goes wrong:**
`json.Decoder.DisallowUnknownFields()` is set on the library's main decoder path. OpenHolidays adds a `notes` field next quarter (their OpenAPI explicitly does not promise field stability and `quality` already appears in the wild despite being absent from the spec — confirmed against `openholidaysapi.org/swagger/v1/swagger.json` 2026-05-26). Every consumer's deploy starts failing on Tuesday morning. Library author publishes a hotfix at 02:00.

**Why it happens:**
Strict decoding *looks* like defensive coding. It is — for a server validating client input. For a **client** consuming an upstream you do not control, it inverts the contract: now any non-breaking server change is a breaking client change.

**How to avoid:**
- **Default: lenient decoding** (`json.NewDecoder(body).Decode(&out)` with no `DisallowUnknownFields`). Unknown fields are silently dropped — which is what `encoding/json` does out of the box.
- **`WithStrictDecoding(true)` is an explicit opt-in** for users who want CI to catch upstream changes (useful in a contract-test environment, dangerous in production).
- Document this loudly in `doc.go`: "Lenient by default because the upstream is not version-locked. Use `WithStrictDecoding` in pre-production fuzzing only."

**Warning signs:**
- `DisallowUnknownFields()` called outside a test path or outside the strict-mode branch.
- An issue from a user saying "upgrade to v0.1.1 broke when API added field X".
- No documentation comment on `WithStrictDecoding` explaining the tradeoff.

**Phase to address:** Phase 1 (decoder design) — must be settled before any endpoint method is written.

---

### Pitfall JSON-2: Non-pointer fields for optional values → silent data loss

**What goes wrong:**
`type Holiday struct { Quality string }` for the upstream's optional `quality` field. Server omits the field → Go zero-value `""` is indistinguishable from server-sent `""`. Caller's `if h.Quality == "" { … }` logic does the wrong thing in 5 % of cases. Worse, custom marshalling round-trip cannot reproduce the original.

**Why it happens:**
Pointers in JSON structs look ugly. `*string` requires nil-checks at every read site. Engineers favor ergonomics over correctness in v0 and don't revisit.

**How to avoid:**
- **Decision rule per field**, applied at Phase 1:
  - If the field is `nullable: true` in the upstream OpenAPI **and** the zero value of the Go type carries meaning, use a pointer (`*string`, `*time.Time`).
  - If the upstream omits the field but absence is semantically the same as zero, a non-pointer is fine.
- Verified against `openholidaysapi.org/swagger/v1/swagger.json` (read 2026-05-26): the following are `nullable: true` — `comment`, `subdivisions`, `groups`. The undocumented-but-observed `quality` field should default to non-pointer `string` until we see a response where empty-string and absent have different meanings; if they do, promote to `*string`.
- For `Nationwide bool`, the upstream always sends it; non-pointer is correct.

**Warning signs:**
- A struct field corresponding to a `nullable: true` upstream field that is **not** a pointer or a slice (slices' nil-vs-empty distinction is preserved by `encoding/json`).
- A helper method like `Holiday.HasComment()` whose implementation is `len(h.Comment) > 0` for a slice — that's fine — but the same idea for a string requires a pointer.

**Phase to address:** Phase 1 (types).

---

### Pitfall JSON-3: Custom `UnmarshalJSON` that ignores `null` or empty string

**What goes wrong:**
`Date.UnmarshalJSON([]byte("null"))` calls `time.Parse("2006-01-02", "null")` and returns a confusing error. Or it returns `nil` and the zero `Date{}` propagates, making downstream code think the holiday is on January 1st of year 1. Or `[]byte("\"\"")` parses to zero-time and silently passes through.

**Why it happens:**
The minimal custom `UnmarshalJSON` is three lines; the *correct* one is a dozen. Devs ship the minimal version, ship goes green, the regression appears the first time the upstream returns an empty string for a missing observance.

**How to avoid:**
- Mandatory template for every custom unmarshaler:
  ```go
  func (d *Date) UnmarshalJSON(b []byte) error {
      // 1. null → leave d as zero value, signal via *Date pointer at caller
      if bytes.Equal(b, []byte("null")) {
          return nil
      }
      // 2. strip quotes; fail if not a JSON string
      if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
          return &json.SyntaxError{Offset: 0}
      }
      s := string(b[1 : len(b)-1])
      // 3. empty string → explicit error (do NOT silently zero)
      if s == "" {
          return fmt.Errorf("openholidays: empty date string")
      }
      // 4. parse with the exact expected layout
      t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
      if err != nil {
          return fmt.Errorf("openholidays: %q: %w", s, err)
      }
      *d = Date(t)
      return nil
  }
  ```
- Fuzz target: `FuzzDateUnmarshal(t *testing.T, b []byte)` — invariant is "never panic, never return a non-zero `Date` from an error path".
- Round-trip test: `MarshalJSON(UnmarshalJSON(x)) == x` for valid inputs.

**Warning signs:**
- A custom unmarshaler under 8 lines.
- No fuzz target for any custom unmarshaler.
- No explicit `null` handling.

**Phase to address:** Phase 1 (date parsing). Fuzzing in Phase 7.

---

### Pitfall JSON-4: `time.Time` zero value passing as a valid date

**What goes wrong:**
The library exposes `Holiday.StartDate time.Time` (or `civil.Date`). Decoder produces a zero `time.Time` for a missing date. Caller's calendar UI renders "Jan 1, year 1" as the start of a school holiday. Or scheduling logic computes a negative date range.

**Why it happens:**
Go's `time.Time{}` is a valid value (year 1, month 1, day 1, UTC). It does not panic on access. It silently propagates.

**How to avoid:**
- The decoder treats a missing or empty `startDate`/`endDate` as a **decode error**, not a zero value. (See Pitfall JSON-3.)
- Public API: every `Holiday` returned from a public method has been validated to have non-zero `StartDate` and `EndDate` with `EndDate >= StartDate`. A `validate(h *Holiday) error` pass runs after decoding and before returning.
- Test: a fixture that sets `startDate: ""` must produce an error from `PublicHolidays`, not a `Holiday` with year-1 dates.

**Warning signs:**
- Caller bug reports involving "Jan 1, year 1".
- No `validate` step in the response pipeline.
- `time.Time.IsZero()` never called anywhere in the codebase.

**Phase to address:** Phase 1 (types) + Phase 3 (endpoint validation pass).

---

### Pitfall TZ-1: `YYYY-MM-DD` ambiguity — UTC vs local TZ assumption

**What goes wrong:**
Library parses `"2025-12-25"` as `time.Parse("2006-01-02", s)` — which produces a `time.Time` in **UTC**. A user in Warsaw (UTC+1) calling `holiday.StartDate.Equal(timeNowInWarsaw.Truncate(24*time.Hour))` gets `false` for Christmas Day because of the hour offset. Or worse: a user in Auckland (UTC+13) sees Christmas Day "starting" at 11 AM on Christmas Eve local time.

**Why it happens:**
`time.Time` always carries a timezone; `YYYY-MM-DD` does not. The upstream OpenHolidays spec does not specify a timezone for its date strings — they are calendar dates, not instants. The Go developer mental model defaults to UTC.

**How to avoid:**
- **Do not expose `time.Time` for holiday dates.** Either:
  1. Use a `civil.Date`-style type (no timezone, just Y/M/D). Either define our own minimal type or accept a single test-only dependency on `cloud.google.com/go/civil` — but **brief mandates zero runtime dependencies**, so we define our own.
  2. Expose `time.Time` but normalize to `time.UTC` at midnight and document this loudly: "dates are returned in UTC at 00:00:00; do not compare to wall-clock times".
- The brief specifies a custom `UnmarshalJSON` for `YYYY-MM-DD` (line 22) — go with option 1: our own `Date` type that does not carry a timezone.
- Helper `(d Date) In(loc *time.Location) time.Time` converts on demand when callers need a `time.Time`.

**Warning signs:**
- A holiday-equality test that uses `time.Now()` instead of a frozen test clock.
- Caller bug reports about "off by one day" near month/year boundaries.
- `time.Parse` (which defaults to UTC) used without an explicit `time.ParseInLocation` and without documentation.

**Phase to address:** Phase 1 (date type design).

---

### Pitfall TZ-2: DST off-by-one in date arithmetic

**What goes wrong:**
Helper `Holiday.Days() int` is implemented as `int(h.End.Sub(h.Start).Hours() / 24)`. Across the spring DST jump (Europe loses one hour in late March), a 14-day winter ferie that crosses March 30 reports 13 days. Across the autumn jump, the same calculation reports 14.something rounded down to 14 — usually right by luck, sometimes off.

**Why it happens:**
`time.Time.Sub` returns a `Duration` in clock-seconds; `Duration / 24h` assumes every day is exactly 24 hours. DST violates that assumption.

**How to avoid:**
- For calendar arithmetic, **never subtract `time.Time`s**. Convert to a Y/M/D civil date and compute the difference in days using a calendar walk or `time.Time.YearDay` adjustment.
- Reference: Go's own `time.Time.AddDate(0, 0, n)` is calendar-correct; use it for `Range()` iteration. For `Days()`, use:
  ```go
  func (h Holiday) Days() int {
      s := time.Date(h.Start.Year(), h.Start.Month(), h.Start.Day(), 0, 0, 0, 0, time.UTC)
      e := time.Date(h.End.Year(), h.End.Month(), h.End.Day(), 0, 0, 0, 0, time.UTC)
      return int(e.Sub(s).Hours()/24) + 1
  }
  ```
  — because both endpoints are normalized to UTC midnight, no DST is involved.
- Table-driven test covering: leap year Feb 28/29, DST spring forward (Europe last Sunday of March), DST fall back (Europe last Sunday of October), Polish ferie-zimowe crossing March 30.

**Warning signs:**
- `h.End.Sub(h.Start)` used for day-counting.
- Tests that pass everywhere except in `TZ=Europe/Warsaw go test`.
- The string `time.Hour * 24` in calendar arithmetic.

**Phase to address:** Phase 5 (helpers — `Days()`, `Range()`).

---

### Pitfall TZ-3: Treating a multi-day school holiday as a single date

**What goes wrong:**
The POC's data model has `StartDate` and `EndDate` (see `openholidays/main.go:42-54`). A naive `Holiday` model with only a `Date` field collapses Polish ferie zimowe (14 days) to a single day. Or a `Range` helper returns `[]time.Time{h.Date}` when it should return 14 dates.

**Why it happens:**
The `PublicHolidays` view of life is "one date per holiday" — that's the mental model from the holidays-rest POC. `SchoolHolidays` and OpenHolidays in general are multi-day periods. Engineers extend the wrong shape.

**How to avoid:**
- The canonical `Holiday` struct has both `StartDate` and `EndDate` from day one. For single-day holidays they are equal.
- `(h Holiday) Days() int` returns 1 for single-day, N for N-day. Validated by tests against the POC's confirmed data (ferie zimowe śląskie 2025: 14 days, see `openholidays/main.go:181-182`).
- `(h Holiday) Range() iter.Seq[Date]` iterates inclusively from StartDate to EndDate.

**Warning signs:**
- A field named `Date` instead of `StartDate`/`EndDate`.
- A helper that ignores `EndDate` for school holidays.
- Tests that only verify single-day public holidays and not multi-day school holidays.

**Phase to address:** Phase 1 (types).

---

### Pitfall RETRY-1: Retrying 4xx (except 429)

**What goes wrong:**
Retry policy retries any non-2xx response. A bad query (e.g. `countryIsoCode=ZZ`) returns 400; the library hammers the endpoint 5 times before failing; the upstream sees this client as abusive; the caller's pipeline runs 5× over budget.

**Why it happens:**
The naive policy is "retry on failure". 4xx is a "failure" by HTTP status. The nuance — that 4xx means *the client is wrong, retrying won't help* — is one Stack Overflow comment away from being missed.

**How to avoid:**
- Retry-eligible status set: `{408 Request Timeout, 429 Too Many Requests, 500 Internal Server Error, 502 Bad Gateway, 503 Service Unavailable, 504 Gateway Timeout}`. **Never** retry on 400/401/403/404/405/410/422/etc.
- Also retry on `net.Error` with `Timeout() == true` and on connection-reset errors.
- The retry-eligible function lives in `internal/retry/policy.go` and is unit-tested with a status-code matrix.
- Note re: OpenHolidays: the OpenAPI spec only documents 400 and 500. 429 is not documented; we retry it anyway because *if* it happens, retrying is the right answer.

**Warning signs:**
- Retry policy that retries on `resp.StatusCode >= 400`.
- An integration test that sees 5× the expected request count for a 400 path.
- No documented retry-eligibility table.

**Phase to address:** Phase 4 (retry).

---

### Pitfall RETRY-2: Ignoring `Retry-After`

**What goes wrong:**
Server returns `429 Too Many Requests` with `Retry-After: 30`. Library backs off 1 s (its computed jitter), retries, gets 429 again, retries, 429, retries. Server escalates to a longer cooldown or an IP ban. The upstream operator (this is a free public API run by volunteers — see https://www.openholidaysapi.org/en/) reaches out asking us to back off.

**Why it happens:**
`Retry-After` is a one-line check most libraries skip. Or the parser only handles seconds and not the HTTP-date variant.

**How to avoid:**
- On retryable status codes, parse `Retry-After`:
  ```go
  func parseRetryAfter(h string, now time.Time) (time.Duration, bool) {
      if h == "" { return 0, false }
      if s, err := strconv.Atoi(h); err == nil && s >= 0 {
          return time.Duration(s) * time.Second, true
      }
      if t, err := http.ParseTime(h); err == nil { // RFC 7231 HTTP-date
          if d := t.Sub(now); d > 0 { return d, true }
      }
      return 0, false
  }
  ```
- If `Retry-After` is present, **use it** (capped at `WithMaxRetryWait`, default 60 s, to bound the worst case). Do not add jitter on top of an explicit server hint.
- If absent, fall back to exponential backoff with full jitter (see RETRY-4).
- OpenHolidays does not currently send `Retry-After` (no rate-limit headers in their OpenAPI spec, confirmed 2026-05-26). The library still handles it correctly because (a) the upstream may add it, (b) `WithBaseURL` lets consumers point at proxies that do send it.

**Warning signs:**
- Retry code that doesn't read `resp.Header.Get("Retry-After")`.
- A test fixture that sends `Retry-After: 5` and asserts the library waits ≥ 5 s.

**Phase to address:** Phase 4 (retry).

---

### Pitfall RETRY-3: Unbounded retry loop ignoring `ctx`

**What goes wrong:**
Retry loop is `for { if err := attempt(); err == nil { return } sleep(backoff()) }`. Caller's 30 s timeout never fires because `sleep` doesn't check ctx (see CTX-2). Or the loop has a max-attempt cap but no ctx check, so it retries through cancellation.

**Why it happens:**
"Retries should be bounded" is on every checklist; "retries should respect context" is not.

**How to avoid:**
- Retry loop pseudocode:
  ```go
  for attempt := 0; ; attempt++ {
      if err := ctx.Err(); err != nil { return err }
      resp, err := do(req)
      if !shouldRetry(resp, err, attempt, maxAttempts) {
          return resp, err
      }
      wait := computeBackoff(attempt, resp.Header.Get("Retry-After"))
      if err := sleepCtx(ctx, wait); err != nil { return err }
  }
  ```
- Both the ctx-check and the max-attempt cap are guards; neither alone is enough.

**Warning signs:**
- A retry loop with `for {` and no `ctx.Err()` check inside.
- `time.Sleep` instead of `sleepCtx`.

**Phase to address:** Phase 4. Verified Phase 7 with goleak + a 100 ms ctx cancellation test.

---

### Pitfall RETRY-4: Same-jitter retries → thundering herd

**What goes wrong:**
100 instances of the consumer's service all retry at exactly `1 s + 2 s + 4 s + 8 s` after an upstream blip; the upstream sees 100 concurrent requests at the 1 s mark, then again at 3 s, then 7 s, then 15 s. Self-inflicted DDoS.

**Why it happens:**
Plain exponential backoff (no jitter) is the default form in tutorials. Adding jitter is a one-line change devs forget.

**How to avoid:**
- **Full jitter** formula (AWS canonical, https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/):
  ```go
  delay := time.Duration(rand.Int63n(int64(min(cap, base * (1<<attempt)))))
  ```
- Use a per-Client `*rand.Rand` seeded from `crypto/rand` at `NewClient` time, **not** the global `math/rand` (which would be the same seed across the fleet pre-Go-1.20, and is shared-mutable state).
- Cap: 60 s. Base: 250 ms. Max attempts: 5 (configurable via `WithMaxRetries`).
- Tests verify two consecutive backoffs from a deterministic seed are within the right range but **different**.

**Warning signs:**
- `delay := base * (1 << attempt)` with no randomization.
- Use of `math/rand.Intn` instead of a per-client `*rand.Rand`.
- All retries from one Client land at the same millisecond.

**Phase to address:** Phase 4.

---

### Pitfall CACHE-1: Caching error responses

**What goes wrong:**
TTL cache stores the result of `PublicHolidays("PL", …)`. The call returned a 503 with an `*APIError`. Cache stores `(nil, *APIError{503})`. For the next 24 hours, every caller gets the stale 503 — until TTL expires — even though the upstream recovered after 30 s.

**Why it happens:**
A simple `cache.Set(key, result)` without distinguishing success vs failure looks complete. Treating error returns the same as success returns is a one-line bug.

**How to avoid:**
- **Cache only on success** (`err == nil` *and* `resp.StatusCode == 200`).
- For non-2xx, never write to cache.
- Optionally: a **negative cache** with a *much* shorter TTL (e.g. 5 s) to prevent stampedes on a hard-error condition (404 on a non-existent country code). Negative cache is opt-in via `WithNegativeCacheTTL(d)`, default off.

**Warning signs:**
- A cache write that doesn't condition on `err == nil`.
- Persistent 5xx for hours after the upstream has recovered, in production logs.

**Phase to address:** Phase 4 (cache).

---

### Pitfall CACHE-2: Cache keyed only by endpoint params, not by (baseURL, …)

**What goes wrong:**
Consumer creates `client1 := NewClient(WithBaseURL("https://prod"))` and `client2 := NewClient(WithBaseURL("https://staging"))`. They share a process. They share a global cache (oops — see CACHE-3 below). `client2.PublicHolidays(...)` returns prod data because the cache key is just `(country, lang, from, to)` and prod-data is already cached. Catastrophic in test environments where staging has different data.

**Why it happens:**
The cache key derivation looks "obvious" — it's the function's input. The base URL is not a function input; it's a Client property. Forgotten.

**How to avoid:**
- **Cache lives on the `Client`**, not globally. Two `Client`s = two caches, automatically isolated by construction.
- If a global/shared cache is ever introduced, the key includes `(baseURL, country, lang, from, to)`.
- `WithBaseURL` documented as "creates a new cache scope"; constructor explicitly resets the cache state.

**Warning signs:**
- A `var globalCache sync.Map` or package-level cache var.
- A `Client.cache` field whose key type does not include `baseURL`.
- An integration test that creates two clients pointing at two URLs and observes one's cache leaking to the other.

**Phase to address:** Phase 4 (cache).

---

### Pitfall CACHE-3: Memory leak — no TTL eviction loop

**What goes wrong:**
TTL cache implementation uses `map[string]entry{ value, expiresAt }`. Reads check `time.Now() < expiresAt` and return. **Writes never remove expired entries.** A long-running service that queries `Subdivisions("PL", "PL")`, `Subdivisions("PL", "EN")`, `Subdivisions("DE", "DE")`, … accumulates entries for every (country, lang) tuple ever requested. After a year of varied queries, the map has 195 × 14 ≈ 2730 stale entries — small, but unbounded.

**Why it happens:**
"TTL = ignore expired entries on read" feels complete. The eviction step is invisible until memory profiling shows growth.

**How to avoid:**
- Either:
  - **Lazy eviction on access** + a periodic sweeper goroutine that walks the map every minute and removes expired entries (sweeper is started on first cache write, stopped on `Client.Close()`).
  - Or a bounded-size cache (LRU + TTL — hashicorp/golang-lru is one well-tested option, but brief mandates zero deps, so we'd write a minimal version in `internal/cache`).
- `Client.Close()` is **mandatory** to add (brief does not currently mention it — flag for plan review). It stops sweepers and releases the connection pool.
- Test: insert 10 000 entries with 1 ms TTL, wait, assert map size returns to 0.

**Warning signs:**
- A cache implementation with no sweeper goroutine and no eviction-on-write.
- A `Client` with no `Close()` method (impossible to release the sweeper).
- Memory profile showing the cache map growing linearly with unique keys.

**Phase to address:** Phase 4 (cache). Flag for roadmap: `Client.Close()` should be added to the Active requirements list in PROJECT.md.

---

### Pitfall CACHE-4: Race on read-during-evict

**What goes wrong:**
Two goroutines race: one reads `cache[key]`, the eviction sweeper deletes the entry mid-read. Without synchronization this is a data race; with a single mutex it's a contention hotspot for read-heavy workloads.

**Why it happens:**
Naive `map[string]…` is not safe for concurrent use. Wrapping every access in a single `sync.Mutex` works but serializes reads.

**How to avoid:**
- `sync.RWMutex`: reads take `RLock`, writes/evictions take `Lock`. Acceptable performance for a read-heavy access pattern (most calls hit cached `Subdivisions`).
- Do **not** reach for `sync.Map` reflexively. `sync.Map` is optimized for "write once, read many times by many goroutines" — TTL caches don't match that pattern because entries are *re-written* on expiry (see CACHE-5 next).
- Tests under `go test -race` with at least 100 concurrent readers + 1 sweeper running for 1 s; must pass.

**Warning signs:**
- A `map[K]V` cache field with no mutex.
- `sync.Map` used for a TTL cache.
- No `-race` flag in CI.

**Phase to address:** Phase 4. Race-tested in Phase 7.

---

### Pitfall CACHE-5: `sync.Map` for the wrong access pattern

**What goes wrong:**
`sync.Map` is chosen because "it's the concurrent map". Performance under read-heavy load with periodic eviction is *worse* than `sync.RWMutex + map` because every write invalidates the read-only fast path. Internal duplication doubles memory under churn.

**Why it happens:**
Standard library docs say `sync.Map` is for "two patterns: (1) write-once read-many, (2) disjoint key sets". Devs read "concurrent" and pick it without reading the patterns. The Go authors literally documented this footgun.

**How to avoid:**
- Default to `sync.RWMutex + map` for this cache. Benchmark both. The brief specifies performance contracts: "< 5 ms when cached" — easily met by either, but RWMutex wins on simplicity.
- Document the choice in `internal/cache/cache.go` with a comment citing the `sync.Map` docs.

**Warning signs:**
- `sync.Map` in `internal/cache/`.
- A benchmark comparing the two not present.

**Phase to address:** Phase 4 (cache implementation).

---

### Pitfall CONC-1: `Client` not goroutine-safe but appearing to work

**What goes wrong:**
`Client` has a `lastRequestID int` counter incremented on each call. Two goroutines race on the increment; tests pass at single-thread; `-race` catches it; release happens anyway because CI runs without `-race`. Production hits the race intermittently and produces duplicate IDs.

**Why it happens:**
Mutable counters / metrics / "last error" fields creep into the Client struct as "convenience". They turn a goroutine-safe API into one that isn't.

**How to avoid:**
- **`Client` is immutable after construction.** Options set fields *during* `NewClient`; nothing else writes to those fields.
- Per-request state (e.g. attempt number, headers) lives on the stack of the calling goroutine, not on the Client.
- Counters that need to be shared use `sync/atomic` and are explicitly documented.
- **CI runs all tests with `-race`**, both unit and integration. Brief mandates this; verify the CI config in Phase 7 actually has the flag.

**Warning signs:**
- A `Client` field that is mutated outside `NewClient`.
- A non-atomic counter on the Client.
- A CI job without `-race`.

**Phase to address:** Phase 2 (Client design) + Phase 7 (CI verification).

---

### Pitfall CONC-2: Goroutine leaks in retry / hook paths

**What goes wrong:**
`RequestHook` is documented to be synchronous; consumer ignores docs and uses `go func() { … }()` inside it; that goroutine outlives the ctx and references the in-flight `*http.Response` whose body has been closed. Or the library itself spawns a goroutine for "async logging" that doesn't drain on `Close`.

**Why it happens:**
Goroutines are cheap and easy to spawn; the discipline to wait for them or signal cancellation is forgotten.

**How to avoid:**
- **The library spawns no goroutines** outside of (a) the cache eviction sweeper (lifetime bound to `Client.Close`) and (b) the test helpers (lifetime bound to test).
- `RequestHook` documented synchronous; if a consumer wants async they own the goroutine and the leak.
- `go.uber.org/goleak` runs in `TestMain` of every package test — fails the test if any goroutine outlives the test (excluding the runtime's own).

**Warning signs:**
- `go func(` anywhere in the library code besides the sweeper.
- Test packages without `goleak.VerifyTestMain` (or per-test `defer goleak.VerifyNone(t)`).

**Phase to address:** Phase 7 (test setup includes goleak).

---

### Pitfall LOG-1: Logging response bodies at Info level

**What goes wrong:**
The library logs `"GET /PublicHolidays returned: " + string(body)` at Info level. In production, every holiday query writes a multi-KB blob to operator logs. Disk fills; SIEM ingests holiday names instead of audit events; the consumer's logging budget is hosed. Low harm in this domain (no PII) but a pattern that scales to security breaches in other libraries.

**Why it happens:**
"Log everything during development, tune later" is a common pattern. The tune-later step rarely happens.

**How to avoid:**
- **Default logging level: `slog.LevelWarn`** for the library's logger. Info-level emits only "request started", "request completed (status, duration)" — no bodies, no headers.
- Response bodies log at `Debug` only. Headers other than `Content-Type`, `Content-Length`, `Retry-After` log at `Debug` only.
- The library uses `slog.Default()` unless `WithLogger(*slog.Logger)` is set. No global `log` package.
- Test: a captured slog handler asserts that at default level, no response body bytes appear in output.

**Warning signs:**
- `slog.Info(... "body", string(b))` anywhere.
- The `log` package imported (vs `log/slog`).
- A logger whose default level isn't documented.

**Phase to address:** Phase 2 (logging plumbing).

---

### Pitfall LOG-2: Using the global `log` package

**What goes wrong:**
A library that calls `log.Printf` writes to stderr and the consumer's structured logging pipeline either misses it (because it's not JSON) or ingests it as a malformed event. Test output is polluted.

**Why it happens:**
`log.Printf` is one import shorter than `slog`. Habit from older Go code.

**How to avoid:**
- Brief mandates `slog.Default()` (line 79). Never import `"log"` (only `"log/slog"`).
- A lint check: `forbidigo` in golangci-lint with the pattern `^log\.` (matching the package, not the type).

**Warning signs:**
- `import "log"`.
- Test runs with stray `2025/05/26 12:34:56` lines that don't come from the test harness.

**Phase to address:** Phase 2 (logging) — lint in Phase 7.

---

### Pitfall TEST-1: Tests that depend on network (flaky CI)

**What goes wrong:**
A unit test does `http.Get("https://openholidaysapi.org/...")` directly. CI fails when GitHub Actions has a transient network blip, when the upstream is briefly down, when GitHub's egress IP is rate-limited. PR reviewer re-runs CI three times before merging. Trust in the test suite erodes.

**Why it happens:**
"It's just a smoke test." Writing an `httptest.Server` takes 20 lines; calling the real API takes 1.

**How to avoid:**
- **Unit tests** (`go test ./...`) **never touch the network**. Period. They use `httptest.Server` with golden fixtures pinned in `testdata/`.
- **Integration tests** live behind both `//go:build integration` and `OPENHOLIDAYS_LIVE=1` env gate (brief line 32). They run nightly in CI, not on every PR.
- Golden fixtures captured from the real API during Phase 1 by a `scripts/refresh-fixtures.sh` script. Re-captured monthly to detect upstream drift.

**Warning signs:**
- A unit test importing `net/http` but not `net/http/httptest`.
- CI flake count > 5 % on the unit-test job.
- No `//go:build integration` tag anywhere.

**Phase to address:** Phase 7 (testing strategy).

---

### Pitfall TEST-2: `httptest` servers leaked across tests (port exhaustion)

**What goes wrong:**
`srv := httptest.NewServer(...)` without `defer srv.Close()`. Each test allocates a port; ports leak; long test runs (`-count=100`) exhaust the ephemeral port range; `bind: address already in use`.

**Why it happens:**
`defer` line missed in copy-paste. Test passes individually, fails in bulk.

**How to avoid:**
- Helper:
  ```go
  func newTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
      srv := httptest.NewServer(h)
      t.Cleanup(srv.Close)
      return srv
  }
  ```
- Use `t.Cleanup`, not `defer` — `t.Cleanup` runs even on `t.Fatal`/panic; `defer` in the test function does too, but `t.Cleanup` composes better when the server is built inside a helper.
- CI runs `go test -count=10 ./...` once per week as a soak job.

**Warning signs:**
- A test that calls `httptest.NewServer` without a matching cleanup.
- Bulk-test runs failing with port-bind errors.

**Phase to address:** Phase 7.

---

### Pitfall TEST-3: Time-dependent tests without a fake clock

**What goes wrong:**
Retry test waits a real 1 s; cache TTL test waits 5 s for expiry. Unit suite runtime balloons to minutes. Or, worse: the test asserts `time.Since(start) >= 1 s` and CI machines are too slow / too fast and the assertion is flaky.

**Why it happens:**
`time.Now()` and `time.After` are convenient. Plumbing a clock interface looks like over-engineering until the test suite is slow.

**How to avoid:**
- A `Clock` interface in `internal/clock`:
  ```go
  type Clock interface {
      Now() time.Time
      NewTimer(d time.Duration) Timer
  }
  ```
  Real implementation wraps `time`; fake implementation advances under test control.
- Retry, cache TTL, and any other time-dependent code take a `Clock` (defaulting to real). Unit tests inject a fake.
- Unit-suite runtime budget: ≤ 5 s. Anything longer is grounds for refactoring to use the fake clock.

**Warning signs:**
- `time.Sleep` in a test.
- Unit test taking > 100 ms.
- Test that asserts `time.Since(start) >= X` (intrinsically flaky).

**Phase to address:** Phase 4 (Clock interface designed when retry/cache are built) + Phase 7 (test refactor).

---

### Pitfall TEST-4: Race-flaky tests that pass without `-race`

**What goes wrong:**
CI runs `go test ./...`; race detector off; concurrency bugs hide. Months later, a user reports an intermittent data race in production. Reproducing is a multi-day investigation.

**Why it happens:**
`-race` makes tests slower (2-10×) and devs sometimes disable it locally. CI inherits the local config.

**How to avoid:**
- CI matrix step is `go test -race -cover ./...`. The `-race` flag is **mandatory**; no PR merges without it green.
- A pre-commit hook is welcome but not enforced (devs hate slow hooks); CI is the gate.
- Tests are explicitly designed to exercise concurrency: at least one test per stateful package spawns ≥ 8 goroutines hammering the API.

**Warning signs:**
- CI config without `-race`.
- A package with stateful types (Client, Cache) that has no concurrency test.

**Phase to address:** Phase 7.

---

### Pitfall API-1: Exporting too much in v0.x

**What goes wrong:**
v0.1 exports `Client.HttpClient`, `Client.BaseURL`, `Client.Retries int`, `Client.Cache map[string]…` so users can poke at them. v0.5, after feedback, we want to refactor `Client` internals — but the exported fields are now part of the public API. v1.0 release blocked by backwards-compat. Or we break callers in v0.6 and they complain (which is technically allowed in v0.x but feels wrong if we promise stability).

**Why it happens:**
"Just export it for now, we'll see who uses it" — and then everyone uses it.

**How to avoid:**
- **Default to unexported.** Every export gets justified in a PR.
- Configuration goes through `Option` functions (`WithHTTPClient`, `WithBaseURL`, …). The `Client` struct has zero exported fields.
- Exported types in the public API: `Client`, `Holiday`, `Subdivision`, `Country`, `Language`, `LocalizedText`, `SubdivisionRef`, `Date` (custom date type), `APIError`, the error sentinels, the `Option` type. That's it.
- Anything else lives under `internal/`.
- `pkg.go.dev` API surface listing is reviewed before tagging `v0.1.0` — every exported symbol is justified or removed.

**Warning signs:**
- A `Client` field that is exported and isn't `Options`-set.
- A type in the root package that doesn't appear in `doc.go` examples.
- An exported function whose only consumer is internal tests.

**Phase to address:** Phase 2 (Client surface) + Phase 8 (pre-release API review).

---

### Pitfall API-2: Exposing concrete types where an interface would serve

**What goes wrong:**
`WithCache(c *internal.LRUCache)` is exported. We later want to add a `WithCache(c RedisCache)` for a distributed cache; can't, because the function signature is concrete. Or `WithLogger(*slog.Logger)` is fine because `slog.Logger` is itself stable, but `WithLogger(*MyLogger)` would lock us in.

**Why it happens:**
Concrete types are easier to think about. Interfaces require designing the right method set up front.

**How to avoid:**
- **Public injection points use interfaces.** For caching: a `Cache` interface in the public API with `Get`, `Set`, `Delete` methods. Default in-memory implementation lives unexported in `internal/cache`; users can supply their own via `WithCache(Cache)`.
- For logging: `*slog.Logger` is acceptable because slog is stable stdlib.
- For HTTP: `*http.Client` is acceptable because it's stdlib and pervasive.
- For retry policy: a `RetryPolicy` interface with `ShouldRetry(*http.Response, error, attempt int) (wait time.Duration, retry bool)`.

**Warning signs:**
- An `Option` that takes an unexported concrete type.
- An interface in the public API with > 5 methods (too large; split it).

**Phase to address:** Phase 4 (cache + retry are the main injection points).

---

### Pitfall API-3: `init()` side effects, global mutable state

**What goes wrong:**
The library has an `init()` that creates a default cache, or seeds a package-level `rand` source, or registers a `metrics.Counter`. Consumers can't disable it; tests can't isolate it; two packages importing different versions of the lib conflict.

**Why it happens:**
"Convenient setup". Anti-pattern in libraries; necessary evil only in main packages.

**How to avoid:**
- Brief mandates "no `init()` side effects, no global mutable state" (line 74). Verified by code review.
- A lint check: `forbidigo` flagging `func init()` in any file outside `internal/version` (where it sets `Version` once).
- All state lives on the `Client`; constructed by `NewClient(opts ...Option) *Client`.

**Warning signs:**
- `func init()` in any file.
- A package-level `var defaultX = …` that is mutable.

**Phase to address:** Phase 2.

---

### Pitfall API-4: Returning slice/map references callers can mutate

**What goes wrong:**
`client.Subdivisions(...)` returns the same underlying slice that the cache stores. Caller does `subs[0].Name = "modified"`. Next caller reads the cache, gets the mutated value. Or, even without a cache: caller sorts the slice in place, library's internal sorted-by-startdate invariant breaks for the next call.

**Why it happens:**
Returning a slice is a pointer-like operation; the underlying array is shared. Defensive copying feels like wasted CPU.

**How to avoid:**
- Cached values are **immutable from the caller's perspective**. Either:
  - Return a defensive copy on every `Get` (simple, costs O(n) per call, n is small here).
  - Document immutability and trust callers (cheaper, brittle).
- Chosen approach: defensive copy on cache hits — the brief's < 5 ms cached-call budget easily absorbs a 16-element slice copy.
- The `name []LocalizedText` slice on `Holiday` is similarly defensive-copied on access.

**Warning signs:**
- A method that returns a field directly without copying.
- A test that mutates the returned slice and then calls the method again expecting fresh data.

**Phase to address:** Phase 4 (cache) + Phase 5 (helper accessors).

---

### Pitfall OH-1: 3-year query window cap — silent truncation if not enforced

**What goes wrong:**
Caller passes `validFrom=2020-01-01`, `validTo=2025-12-31` (6 years). The brief (line 64) and POC (`openholidays/main.go` confirms the API works for 1-year windows but doesn't document the cap) state the upstream caps at 3 years. The upstream's response either: (a) silently returns only the first 3 years, (b) returns 400 Bad Request, or (c) — confirmed possibility per the OpenAPI spec being silent on this — accepts the call but with undefined behavior. Note: **the 3-year cap is NOT in the OpenAPI spec at `openholidaysapi.org/swagger/v1/swagger.json`** (verified 2026-05-26). The cap is the brief author's observation from POC use; treat it as a defensive guard, not as a contract.

**Why it happens:**
Optimistic input passthrough. Library doesn't validate; upstream policy isn't enforced.

**How to avoid:**
- **Client-side validation** (per brief line 22): if `validTo - validFrom > 3 years`, return `ErrDateRangeTooLarge` *before* hitting the network.
- Also validate `validFrom <= validTo` and reject if not.
- Validation runs in Phase 3 in each `*Holidays` method.
- Error message includes the actual span: `"date range 2192 days exceeds 3-year max (1095 days); split into multiple calls"`.

**Warning signs:**
- An endpoint method that builds a query string without first validating the date range.
- No test for a 4-year request returning `ErrDateRangeTooLarge`.

**Phase to address:** Phase 3 (endpoint validation).

---

### Pitfall OH-2: Optional fields `comment`, `subdivisions`, `groups` missing in some responses

**What goes wrong:**
Library code does `for _, c := range h.Comment { … }` assuming the slice is present. For most holidays it's absent; the slice is nil; the `for` is a no-op (fine). But `c := h.Comment[0]` panics. Or strict-mode decoder rejects the response because `comment` is absent and we mistakenly required it.

**Why it happens:**
Confirmed against OpenAPI spec (read 2026-05-26): `comment`, `subdivisions`, and `groups` are `nullable: true`. Plus there's a `quality` field that **appears in real responses but is not in the OpenAPI spec** — that's already schema drift the library must tolerate.

**How to avoid:**
- All three documented-nullable fields are `[]LocalizedText` or `[]SubdivisionRef` with the `omitempty` JSON tag. They are nil-safe to iterate.
- Helper methods (`HasComment`, `Name(lang)`) handle nil/empty correctly — `Name(lang)` already does in the POC (`openholidays/main.go:131-141`).
- `quality` field: defensive — include in the struct as `Quality string `json:"quality,omitempty"``, default lenient decoding accepts both presence and absence.
- Golden fixture tests cover: holiday with no comment, holiday with comment in PL only, holiday with no subdivisions, holiday with `quality` field set vs absent.

**Warning signs:**
- A method that indexes into `h.Comment` or `h.Subdivisions` without a length check.
- Strict-mode decoder enabled in production paths (would reject responses with unexpected `quality` field).
- No golden fixture covering an empty-comment holiday.

**Phase to address:** Phase 1 (types) + Phase 5 (helpers).

---

### Pitfall OH-3: `name` is an array of `{language, text}`, not a `map[lang]string`

**What goes wrong:**
Naive engineer looks at the POC's `holidays-rest/sdk-go` mock (which uses `map[string]string` for `name` — see `main.go:33-34`) and assumes OpenHolidays does the same. They write `Name map[string]string `json:"name"``. Decoding silently produces an empty map (because the JSON is `[{"language":"PL","text":"..."}]`, not `{"PL":"..."}`). Tests pass against the wrong fixture. Bug in production.

**Why it happens:**
The two POCs use the same word "name" with different shapes. Engineers pattern-match on the first one they read.

**How to avoid:**
- Confirmed against OpenAPI spec (2026-05-26): `name` is `array of LocalizedText` with `{language: string, text: string}`. POC matches this (`openholidays/main.go:46`, `LocalizedText` type at line 32).
- The Go type is `Name []LocalizedText` — not a map.
- The helper `(h Holiday) Name(lang string) string` does case-insensitive (`strings.EqualFold` per POC line 133) lookup over the slice and falls back to the first entry if the requested language isn't present. POC already gets this right.
- Test fixtures match the array shape exactly. A fixture-validation test asserts the shape.

**Warning signs:**
- A field of type `map[string]string` for any `Name` or `Comment` field.
- A `Name(lang)` helper implemented as `h.Name[lang]`.
- Tests against a hand-rolled fixture that uses the map shape instead of the array shape.

**Phase to address:** Phase 1 (types) — done correctly in POC, just don't regress.

---

### Pitfall OH-4: No rate-limit headers — can't tune backoff from server hints

**What goes wrong:**
Library tries to read `X-RateLimit-Remaining` to throttle proactively. Header is never present (confirmed against OpenAPI spec 2026-05-26: no rate-limit headers documented; POC confirmed none observed). Library either silently does nothing (fine) or treats absence as "no limit, hammer freely" (not fine). Eventually the upstream operators reach out.

**Why it happens:**
Other public APIs (GitHub, OpenAI, Stripe) send rich rate-limit metadata. Devs assume OpenHolidays does too.

**How to avoid:**
- Retry strategy stays **conservative by default**: max 3 retries, base 250 ms, cap 60 s, full jitter. No reliance on server hints.
- `Retry-After` is honored *if present* (Pitfall RETRY-2) but absence is assumed.
- Document the upstream's keyless / no-rate-limit-header state in `docs/design.md` and `doc.go`: "OpenHolidays does not publish rate-limit headers. Use the cache for reference endpoints; do not poll holiday endpoints in tight loops."
- If we ever observe a 429 from the upstream, that's a signal to file an issue and discuss adding `WithRateLimit(qps int)` Option (token bucket client-side).

**Warning signs:**
- Code reading `X-RateLimit-*` headers.
- Aggressive retry config that assumes the upstream will tell us to slow down.

**Phase to address:** Phase 4 (retry default config) + Phase 8 (docs).

---

### Pitfall OSS-1: Tagging `v1.0.0` too early

**What goes wrong:**
Excited maintainer tags `v1.0.0` after the first release. Two weeks later, a user reports the `Cache` interface should have a `Has(key) bool` method. SemVer commitment forbids the addition without a `v2.0.0`. Maintainer either ships `v2.0.0` (looks unstable) or refuses the change (loses the user). Either way: bad.

**Why it happens:**
`v1.0.0` "looks done"; `v0.x.y` "looks unfinished". Pride.

**How to avoid:**
- Stay in `v0.x` until: (a) at least 3 months of production use across ≥ 2 distinct consumer projects, (b) no breaking changes in the last 2 releases, (c) every exported symbol has been justified in writing.
- Brief mandates `v0.1.0` as the M1 deliverable; honor that.
- Each `v0.x` minor allows breaking changes documented in `CHANGELOG.md`.
- The "Module path owner deferred" line in PROJECT.md (Key Decisions) is a reminder that the github org/user is also a v1-stability dependency; do not tag v1.0 until module path is permanent.

**Warning signs:**
- A `v1.0.0` tag with only one consumer.
- A `v0.5.0` with no `CHANGELOG` discipline.
- "Breaking change" issues filed against a `v1.x` release.

**Phase to address:** Phase 8 (release) — explicitly tag `v0.1.0`, not `v1.0.0`.

---

### Pitfall OSS-2: Missing `pkg.go.dev` examples (Go report card grade drops)

**What goes wrong:**
`pkg.go.dev` ranks documentation; missing `example_test.go` for public methods drops the "examples" score. Discoverability suffers. Users land on the docs page, see no runnable snippet, leave.

**Why it happens:**
Examples are tedious to write; tests cover the same code path "better"; devs skip them.

**How to avoid:**
- Brief line 36 mandates "example_test.go with one example per public method". Honor it literally.
- Each example is a runnable, passing-doc-example in the `pkg.go.dev` sense: starts `func Example…`, ends with `// Output:` comment.
- A CI step `go test -run Example ./...` enforces that examples actually run.
- README quickstart (line 36 says ≤ 20 lines) doubles as the `ExampleClient` example.

**Warning signs:**
- A public method without a corresponding `Example` function.
- An example without `// Output:` (won't run, won't appear in docs).
- `pkg.go.dev` "Examples" tab empty.

**Phase to address:** Phase 8 (docs prep).

---

### Pitfall OSS-3: `go.mod` `go 1.22` vs actual feature use

**What goes wrong:**
`go.mod` says `go 1.22`. Library uses `iter.Seq` (Go 1.23 feature) for the `Range()` helper. Consumers on Go 1.22 get a compile error on `go get`. Or the inverse: `go.mod` says `go 1.23` but no 1.23-only features are used and consumers are needlessly upgrade-forced.

**Why it happens:**
The `go` directive in `go.mod` is informative (since Go 1.21+ it actually controls language version), but devs treat it loosely.

**How to avoid:**
- Brief says "Go ≥ 1.22 minimum" (line 69). `go.mod` directive is `go 1.22`.
- `iter.Seq` is Go 1.23 — that's a problem. Either:
  - Bump the minimum to Go 1.23 (update `go.mod` and CI matrix) — recommended since 1.22 is EOL by the time v0.1.0 lands.
  - Or skip `iter.Seq` for v0.1.0 and add it in a later minor.
  - **Flag for plan review.** PROJECT.md line 26 says "Go 1.23 range-over-func"; PROJECT.md line 69 says "Go ≥ 1.22 minimum". These are inconsistent. The roadmap should pick one.
- CI matrix tests every supported minor version. If `go.mod` says `1.22`, CI must include `1.22`, `1.23`, and `stable`.

**Warning signs:**
- `go.mod` minimum lower than the lowest CI matrix entry.
- Use of any `iter.*`, `slices.*` from 1.21+, `maps.*` (1.21+), `cmp.Or` (1.22), `min`/`max` builtins (1.21+) without confirming the `go` directive.
- `go vet` warning about language version.

**Phase to address:** Phase 1 (initial `go.mod` setup) — and flag the 1.22/1.23 inconsistency in PROJECT.md for the planning agent to resolve.

---

### Pitfall OSS-4: Broken `goreleaser` config (no smoke test)

**What goes wrong:**
`.goreleaser.yml` looks right; first `v0.1.0` tag triggers CI; goreleaser fails 3 minutes in because of a path typo, a missing `LICENSE`, or a Windows build flag that doesn't exist on the version it pulls. Tag is published; release artifacts are not. Untagging and re-tagging `v0.1.0` is awkward (Git tag was published; reasonable Git etiquette is to publish `v0.1.1` instead).

**Why it happens:**
Release configs are touched once and then trusted. The first real run is the only proof.

**How to avoid:**
- `goreleaser check` step in CI on every PR — catches syntax errors.
- `goreleaser release --snapshot --clean` step on every PR — catches config errors without publishing.
- A `pre-tag-checklist.md` ritual: dry-run the release locally, verify all 6 binary targets (linux/darwin/windows × amd64/arm64), verify `CHANGELOG.md` is current, verify `go install` works after.
- Optionally: tag `v0.0.1-rc1` as a real test of the pipeline before the public `v0.1.0`.

**Warning signs:**
- A `.goreleaser.yml` that's never been dry-run.
- CI without a `goreleaser check` step.
- A first-time tag.

**Phase to address:** Phase 8 (release pipeline). Phase 7 CI includes the dry-run.

---

### Pitfall OSS-5: Missing `CHANGELOG.md` discipline

**What goes wrong:**
`v0.1.0` ships with a `CHANGELOG.md`. `v0.2.0` ships without updating it (forgotten in the rush). `v0.3.0` updates it with "added stuff and fixed stuff". By `v1.0.0`, consumers have no migration guide for breaking changes.

**Why it happens:**
Changelogs feel redundant when commit messages exist. They aren't — the audience and granularity differ.

**How to avoid:**
- Brief mandates `CHANGELOG.md` (line 37).
- Keep-a-Changelog format (https://keepachangelog.com): sections for Added / Changed / Deprecated / Removed / Fixed / Security per release.
- A CI lint that fails the build if a PR introduces an exported-API change but doesn't touch `CHANGELOG.md` (`changelog-action` or a custom script).
- For each `v0.x` minor, a "Breaking changes" subsection is mandatory (or "None" explicitly).
- Tag descriptions on GitHub releases pull from `CHANGELOG.md`, not from cherry-picked commit messages.

**Warning signs:**
- A release tagged without a changelog entry.
- "Bug fixes and improvements" as the only changelog line.
- A breaking change in `v0.x` without a `Breaking changes` callout.

**Phase to address:** Phase 8 (docs + release).

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Skip `WithMaxResponseSize` Option; hardcode 10 MiB | One fewer Option to document | Future users with adversarial endpoints can't tune; can be added later non-breakingly | **Always acceptable for v0.1.0** — 10 MiB default covers OpenHolidays response sizes by 10× |
| Skip `Client.Close()` in v0.1.0 | One fewer public method | Cache sweeper leaks goroutines if a Client is GC'd; impossible to add Close non-breakingly later (existing callers won't call it) | **Never** — add `Close()` from day one as a no-op or sweeper-stopper. Flag for PROJECT.md update. |
| Lazy-only cache eviction (no sweeper) | Simpler code; no goroutine to manage | Memory leak on long-running processes; user complaints at week 3 | **Never** — sweeper is one well-tested file in `internal/cache` |
| Skip negative cache for now | One fewer Option | Stampede on rare 404s — but OpenHolidays' free public-API nature makes 404 storms unlikely | **Acceptable for v0.1.0**; revisit if users hit it |
| Use `math/rand` global instead of per-Client `*rand.Rand` | One fewer field | Multi-Client fleets pre-Go-1.20 had identical seeds; thundering herd risk; also a global-state taint we explicitly forbid (PROJECT.md line 74) | **Never** — per-Client `*rand.Rand` is one line more |
| Skip `goleak` in test setup | Faster test scaffold | Goroutine leaks ship undetected | **Never** — goleak is one import |
| Skip golangci-lint `contextcheck`, `forbidigo` | Faster `.golangci.yml` setup | CTX-1 and OSS-3 pitfalls slip through review | **Never** — they're free once configured |
| Tag `v0.1.0` directly without `v0.0.1-rc1` smoke | One fewer tag | Release pipeline bug discovered post-publish, ugly to undo | Acceptable **if** goreleaser dry-run is in CI; risky otherwise |
| Skip integration tests in Phase 7; add in M2 | Faster M1 ship | Live-API drift goes undetected; first user reports the schema change | **Never** — integration gate (brief line 32) is mandatory |
| Skip fuzz tests for date/JSON unmarshalers | Faster M1 ship | JSON-3 / JSON-4 / TZ-1 pitfalls become production bugs | **Acceptable for v0.1.0 only if** unit-table tests cover the matrix in Pitfall JSON-3; fuzz adds defense-in-depth |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| OpenHolidays public API | Assuming HTTPS with valid cert is enough security | Validate response *shape* — schema drift is the real threat, not MITM |
| OpenHolidays `validFrom`/`validTo` | Passing dates in any format (`"2025-1-1"`, `"01/01/2025"`) | API expects strict `YYYY-MM-DD`; library formats via `time.Format("2006-01-02")` after validating ranges |
| `countryIsoCode` query param | Lowercase (`"pl"`) — API may accept or may not; the OpenAPI spec is silent | Client-side validation: require 2-letter uppercase; uppercase before sending |
| `languageIsoCode` query param | Sending unsupported language code, getting empty `name[]` arrays back | Client-side validation: validate against the list from `/Languages`; or fall back to first entry in `Name(lang)` helper (POC already does this) |
| `httptest.Server` in tests | Setting up a real TLS server (`NewTLSServer`) and not pinning the cert | OpenHolidays is plain HTTPS to a CA-signed cert; tests use plain `NewServer` and inject baseURL via `WithBaseURL` |
| `slog.Logger` consumer integration | Library logging at `Debug` going to `/dev/null` because consumer's slog handler is at `Info` — drowning useful signal | Library logs at appropriate levels (Warn / Info / Debug as designed in LOG-1); responsibility is on consumer to set the handler level |
| `goleak` and stdlib goroutines | Goleak failing because Go's HTTP transport keeps idle goroutines around | Use `goleak.IgnoreTopFunction` for known stdlib functions (`net/http.(*Transport).dialConn`), or call `transport.CloseIdleConnections()` in test cleanup |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| No connection-pool reuse (HTTP-3) | Latency rises with concurrent requests; `netstat` shows many `TIME_WAIT` | Drain body before close; reuse `*http.Client` (don't create per-call) | At ≥ 10 req/s sustained |
| Per-call `NewClient` from consumer | Cold connection on every call; defeats keep-alive | Document loudly: `Client` is shared, long-lived. Cache works only if the Client is | Immediately at any volume |
| Cache without sweeper (CACHE-3) | Memory grows linearly; eventually OOM | Sweeper goroutine + `Close()` method | After ~weeks of varied queries; sooner if call patterns generate many unique (country, lang) tuples |
| `sync.Mutex` instead of `RWMutex` for read-heavy cache (CACHE-4) | Cache lookups serialize under load | `sync.RWMutex` | At ≥ 100 goroutines doing concurrent cache reads |
| `json.Decoder` re-creating per call (negligible but real) | None visible; minor allocation noise | Acceptable — modern Go's `json.NewDecoder` is fast; pooling decoders is premature optimization | Never breaks; just an over-engineering trap to avoid |
| `time.Now()` called inside hot loops | Microbench noise; doesn't break correctness | Pass `now` into helpers that use it more than once | Never breaks; clean code matters more than the perf delta |
| Reflection in JSON decoding (stdlib `encoding/json`) | Decoding is slower than `easyjson`/`sonic` | Acceptable for OpenHolidays response sizes; the brief's < 500 ms cold budget is easy | Would break at ≥ 1000 holidays per response, which OpenHolidays never returns |
| Linear search in `Name(lang)` over `[]LocalizedText` (POC line 131) | None — slice is ≤ 14 entries (14 languages) | Linear search is fine; building a map per call costs more than it saves | Never |
| Cache miss stampede | Cold start: 100 goroutines all call `Subdivisions("PL", "PL")` simultaneously; 100 upstream requests | Single-flight (`golang.org/x/sync/singleflight`) — but that's a runtime dep we don't want. Alternative: lock the cache key during fetch so concurrent callers wait | At any concurrent cold-start scenario; lower priority since OpenHolidays is keyless and tolerant. Defer to M2 if measured. |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| `WithBaseURL("http://internal-test/")` allowed without warning | Library happily downgrades from HTTPS to HTTP; in production, a misconfig sends data over plaintext | Validate `WithBaseURL` scheme: warn (log at Warn) if not HTTPS; do not refuse outright (test environments need HTTP) |
| `WithHTTPClient(c)` where `c.Transport` skips TLS verification | Caller suppresses cert validation; library inherits it silently | Document loudly: "you are responsible for the security of your `*http.Client`". A test sanity check could verify the default transport's `TLSClientConfig.InsecureSkipVerify` is `false` but cannot enforce on user-supplied clients |
| Response body containing data logged at Info | Holiday names are not PII, but the *pattern* of logging full bodies generalizes badly | LOG-1 enforces: bodies only at Debug |
| `*APIError.Body` field exposes upstream response unfiltered | If the upstream ever sends sensitive data in error bodies (it shouldn't — public API), the library propagates it | Cap `*APIError.Body` at 64 KiB; document that it's for diagnostic display only, not for parsing |
| Logging full request URLs at Info | Query strings could contain caller-sensitive data in custom hooks | Log path + method, not the full URL with query; query logged at Debug |
| `govulncheck` not in CI | A stdlib or transitive-test-dep vuln (e.g. an httptest dependency) goes unpatched | Brief mandates `govulncheck` in CI (line 35). Verified in Phase 7. |
| `gosec` warnings ignored | False positives accumulate; real issues hide in the noise | Run `gosec` in CI; allow per-line nolint comments but require justification |
| Pinning to `master`/`main` of test deps | A test dep gets compromised; library tests run malicious code | `go.sum` pins versions; `dependabot.yml` auto-updates with review |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Inscrutable error: `openholidays: 400` | User has no idea what was wrong with their input | `*APIError{StatusCode:400, Path:"/PublicHolidays", Body:"validFrom must be ISO 8601"}` — include path and (truncated) body |
| Sentinel error overload: 20 `Err…` constants | User can't tell which to check for; `errors.Is` becomes a switch statement | Five sentinels per the brief (line 23); add more only with justification |
| Returning `time.Time` for dates | Caller has to handle timezones they don't expect (TZ-1) | Return a `Date` type with no timezone; provide `.In(loc)` for conversion |
| Cache silently returning stale data | User can't tell if a result is fresh or cached | Either (a) accept it as the cache contract, or (b) expose `Result` struct with a `FromCache bool` field. **For v0.1.0: option (a)** — the brief's < 5 ms cached budget implies invisible-cache semantics, and reference endpoints rarely go stale |
| `WithRetry(true)` boolean instead of `WithRetry(policy RetryPolicy)` | Caller can't tune attempts/wait/jitter | Take an interface (or a struct of options); `WithRetry(DefaultRetryPolicy())` is the convenient default |
| `ohcli` CLI with no `--help` discoverability | New user runs `ohcli` with no args, gets a panic or empty output | `ohcli` (no args) prints usage; every subcommand has `--help`; `ohcli --version` exists |
| `ohcli` flag conflicts (e.g. `-h` short for `--help` vs `--host`) | Caller's habit clashes with library's flag set | Stick to one short-flag convention; document in `cmd/ohcli/README.md` |
| Default `WithStrictDecoding(true)` | Caller's pipeline breaks on innocuous upstream addition (JSON-1) | Default is lenient (false); strict is opt-in |
| `Holiday.Range()` allocates a full `[]Date` (not an iterator) | Memory wastes for long ranges; not idiomatic Go 1.23 | Use `iter.Seq[Date]` (brief line 26); the Days/Range distinction matters |

## "Looks Done But Isn't" Checklist

- [ ] **`WithHTTPClient`:** Often missing — documentation that user-supplied client owns its own timeout/transport config; verify a test where a custom `*http.Client` is injected and respected.
- [ ] **`User-Agent` header:** Often missing — assert via `httptest.Server` that every request carries `User-Agent: ^go-openholidays/`.
- [ ] **Response body draining:** Often missing — benchmark shows keep-alive reuse ≥ 95 %; FD count flat after 10 000 requests.
- [ ] **`io.LimitReader` on response:** Often missing — fuzz test with a 1 GB body fixture confirms no OOM.
- [ ] **Context propagation on retry:** Often missing — `ctx, cancel := …; go cancel after 50 ms; assert error returned within 100 ms`.
- [ ] **`time.Sleep` replaced everywhere by `sleepCtx`:** Often missing — `grep -r 'time\.Sleep' .` in non-test code returns nothing.
- [ ] **Strict decoding off by default:** Often missing — test injects a fixture with an unknown field, default decoder accepts it.
- [ ] **`null` handling in custom unmarshalers:** Often missing — fuzz test feeds `"null"`, `""`, `"\"\""`, `"\"bad-date\""` and never panics.
- [ ] **Date type doesn't carry timezone:** Often missing — `reflect.TypeOf(h.StartDate).String()` is `Date`, not `time.Time`.
- [ ] **Multi-day school holidays:** Often missing — golden fixture for Polish ferie zimowe shows `Days() == 14`.
- [ ] **Retry honors `Retry-After`:** Often missing — `httptest.Server` returns 429 + `Retry-After: 2`; library waits ≥ 2 s on next attempt.
- [ ] **Retry doesn't retry 4xx (except 429):** Often missing — fixture returns 400; library returns immediately, no retry.
- [ ] **Full jitter, not equal jitter:** Often missing — two consecutive backoffs from the same seed differ.
- [ ] **Cache only stores successes:** Often missing — fixture returns 503; cache size unchanged after the call.
- [ ] **Cache TTL eviction runs:** Often missing — insert with 10 ms TTL, sleep 20 ms, fixture asserts cache size returned to 0.
- [ ] **Cache copies returned slices:** Often missing — caller mutates returned slice; next call returns unmutated data.
- [ ] **`Client.Close()` stops sweeper:** Often missing — `goleak` after `Close()` reports no leaked goroutines.
- [ ] **No `init()` in any file:** Often missing — `grep -rn 'func init' --include='*.go'` returns nothing (except `internal/version`).
- [ ] **No `http.DefaultClient` reference:** Often missing — `grep -rn 'http\.\(DefaultClient\|Get\|Post\)' --include='*.go'` returns nothing.
- [ ] **`-race` in CI:** Often missing — verify `.github/workflows/*.yml` actually has `-race`.
- [ ] **`govulncheck` in CI:** Often missing — verify CI workflow runs it.
- [ ] **`goleak` in tests:** Often missing — verify `TestMain` in each test package calls `goleak.VerifyTestMain(m)` or per-test cleanup.
- [ ] **`example_test.go` per public method:** Often missing — list of public methods × list of Example functions; every public method has at least one example with `// Output:`.
- [ ] **`CHANGELOG.md` updated:** Often missing — before tagging, verify `Unreleased` section has entries; promote to versioned section.
- [ ] **`goreleaser` dry-run green:** Often missing — local `goreleaser release --snapshot --clean` succeeds.
- [ ] **`go.mod` Go directive matches features used:** Often missing — `go vet ./...` reports no language-version warnings.
- [ ] **Golden fixtures match real responses:** Often missing — `scripts/refresh-fixtures.sh` is committed; last-refresh date is recent.
- [ ] **Integration tests gated:** Often missing — `go test ./...` without `-tags integration` skips them; `OPENHOLIDAYS_LIVE=1 go test -tags integration ./...` runs them.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| HTTP-2 (missing `Body.Close`) leaked FDs in production | MEDIUM | Hotfix `defer drainAndClose` everywhere; consumer restarts processes; cherry-pick to a patch release |
| HTTP-3 (no drain) burning CPU/network | MEDIUM | Hotfix drain helper; benchmark proves keep-alive reuse; ship patch |
| HTTP-4 (no body limit) OOM exposure | HIGH if exploited | Patch with `LimitReader` wrap; CVE disclosure; advise users to upgrade; assess whether any consumer pointed library at a hostile baseURL |
| JSON-1 (strict default) breaking on upstream field add | HIGH | Patch to lenient default; bump *minor* (`v0.x → v0.x+1`) because behavior changed; document in CHANGELOG as a *fix*, not a feature |
| TZ-1 (UTC-vs-local off-by-one) | HIGH | Add `Date` type; deprecate `time.Time` returns; ship as a new minor with migration guide. Painful because it touches the public API |
| RETRY-1 (retrying 4xx) hammering upstream | LOW | One-line fix in retry policy; ship patch within hours |
| RETRY-2 (ignoring Retry-After) | LOW | One-function fix; ship patch |
| CACHE-1 (caching errors) stale 5xx | MEDIUM | Patch to condition cache write on success; consumers must clear their cache or restart |
| CACHE-3 (no sweeper) memory leak | MEDIUM | Add sweeper + `Close()`; existing consumers must opt in to `Close()`; document the leak in the affected versions' CHANGELOG |
| OH-1 (3-year-window not validated) | LOW | Add validation; one new sentinel error; ship patch |
| OH-3 (`Name` map vs array shape) | HIGH | This is a wire-format misunderstanding; if shipped wrong, every consumer's code breaks on fix. Catch in Phase 1 |
| OSS-1 (tagged v1.0 too early) | HIGH | Cannot un-tag; either commit to maintaining v1 backward-compat forever, or accept the embarrassment of `v2.0.0` weeks later |
| OSS-3 (`go.mod` version mismatch) | LOW | Bump minimum or remove feature use; patch release |
| OSS-4 (broken goreleaser) | LOW–MEDIUM | Re-tag as `v0.1.1`; document the dry-run gap |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| HTTP-1 (DefaultClient) | Phase 2 | grep CI check + unit test injects custom client |
| HTTP-2 (Body.Close) | Phase 2 | FD-count integration test over 10 000 requests |
| HTTP-3 (drain) | Phase 2 | Keep-alive reuse benchmark ≥ 95 % |
| HTTP-4 (LimitReader) | Phase 2 | Fuzz test with oversize body |
| HTTP-5 (User-Agent) | Phase 2 | httptest asserts UA prefix |
| HTTP-6 (Request reuse) | Phase 4 | Race test on retry loop; no `*http.Request` field |
| CTX-1 (ctx in struct) | Phase 2 | `contextcheck` lint |
| CTX-2 (ctx in retry) | Phase 4 | 100 ms cancellation test |
| CTX-3 (swallowing ctx.Err) | Phase 2 + Phase 7 | `errors.Is(err, ctx.Canceled)` assertion in cancellation test |
| JSON-1 (strict default) | Phase 1 | Decoder default is lenient; documented in `doc.go` |
| JSON-2 (non-pointer optional) | Phase 1 | Field-by-field review against OpenAPI nullable list |
| JSON-3 (custom unmarshal) | Phase 1 | Fuzz target + null/empty/bad-format unit tests |
| JSON-4 (time.Time zero) | Phase 1 + Phase 3 | Validation pass after decode |
| TZ-1 (date timezone) | Phase 1 | Custom `Date` type; documented; not `time.Time` |
| TZ-2 (DST arithmetic) | Phase 5 | Table-driven test crossing DST transitions |
| TZ-3 (multi-day holidays) | Phase 1 | Both `StartDate` and `EndDate` from day one |
| RETRY-1 (retry 4xx) | Phase 4 | Status-code matrix test |
| RETRY-2 (Retry-After) | Phase 4 | Fixture-driven test asserts library waits |
| RETRY-3 (unbounded loop) | Phase 4 | `sleepCtx` helper; cancellation test |
| RETRY-4 (same jitter) | Phase 4 | Two-backoff-differ test; per-Client `*rand.Rand` |
| CACHE-1 (cache errors) | Phase 4 | 5xx fixture; cache size unchanged |
| CACHE-2 (key includes baseURL) | Phase 4 | Two-Client isolation test |
| CACHE-3 (TTL eviction) | Phase 4 | TTL expiry test; `Client.Close()` stops sweeper |
| CACHE-4 (read-during-evict) | Phase 4 | `-race` concurrent reader/sweeper test |
| CACHE-5 (sync.Map misuse) | Phase 4 | Cache implementation uses `sync.RWMutex` |
| CONC-1 (Client mutability) | Phase 2 + Phase 7 | All-tests-with-`-race`; immutable-Client invariant doc |
| CONC-2 (goroutine leaks) | Phase 7 | `goleak.VerifyTestMain` |
| LOG-1 (body logging) | Phase 2 | Captured-handler test at Info level |
| LOG-2 (global log) | Phase 2 | `forbidigo` lint on `import "log"` |
| TEST-1 (network in unit tests) | Phase 7 | Integration tag + env gate enforced |
| TEST-2 (httptest leaks) | Phase 7 | `t.Cleanup` helper; soak job |
| TEST-3 (real clock in tests) | Phase 4 + Phase 7 | Clock interface; unit-suite < 5 s |
| TEST-4 (race in CI) | Phase 7 | CI matrix runs `-race` |
| API-1 (over-export) | Phase 2 + Phase 8 | Pre-tag API surface review |
| API-2 (concrete-type lock-in) | Phase 4 | `Cache`, `RetryPolicy` are interfaces |
| API-3 (init/global state) | Phase 2 | `forbidigo` lint on `func init` |
| API-4 (returning mutable refs) | Phase 4 + Phase 5 | Defensive copy on cache hits; mutation test |
| OH-1 (3-year window) | Phase 3 | `ErrDateRangeTooLarge` test |
| OH-2 (optional fields nil) | Phase 1 + Phase 5 | Golden fixture with missing fields |
| OH-3 (`Name` shape) | Phase 1 | Schema-matched fixture; type is `[]LocalizedText` |
| OH-4 (no rate-limit headers) | Phase 4 + Phase 8 | Conservative retry defaults; docs explain |
| OSS-1 (v1 too early) | Phase 8 | First tag is `v0.1.0` |
| OSS-2 (no examples) | Phase 8 | `go test -run Example ./...` green |
| OSS-3 (go.mod mismatch) | Phase 1 | `go.mod` directive matches feature use; CI matrix matches |
| OSS-4 (broken goreleaser) | Phase 7 (dry-run in CI) + Phase 8 (release) | `goreleaser check` + snapshot in CI |
| OSS-5 (CHANGELOG discipline) | Phase 8 | CI lint requires CHANGELOG update on API change |

## Sources

- **OpenHolidays OpenAPI spec** (primary source, verified 2026-05-26): `https://openholidaysapi.org/swagger/v1/swagger.json` — confirmed: `name` is array of `{language, text}`; `comment`, `subdivisions`, `groups` are `nullable: true`; `quality` is NOT in the spec but appears in POC responses (schema drift to defend against); only 200/400/500 documented; no rate-limit headers documented; no max-window documented.
- **OpenHolidays API landing page**: `https://www.openholidaysapi.org/en/api/` — confirms public, keyless access; OpenAPI definition referenced.
- **Project POCs** (in-repo reference material):
  - `/data/git/private/holidays/main.go` — `holidays-rest/sdk-go` + `rickb777/date/v2` mock-backed POC.
  - `/data/git/private/holidays/openholidays/main.go` — live OpenHolidays POC for PL 2025 (14 public, 7 school-periods, 16 subdivisions; established the wire-shape).
- **Project brief**: `/data/git/private/holidays/.planning/PROJECT.md`.
- **Go `net/http` body-draining behavior**: `https://yngvest.github.io/blog/posts/go-pconn/` and Go issue `https://github.com/golang/go/issues/77370` — confirmed that callers must drain for keep-alive reuse, and that recent Go (post-CL 737720) auto-drains up to 256 KiB / 50 ms on `Close()` but explicit drain remains the recommended defensive pattern.
- **Go `net/http` close-vs-drain documentation issue**: `https://github.com/golang/go/issues/60240`.
- **AWS Architecture Blog — Exponential Backoff and Jitter**: `https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/` — canonical source for Full Jitter formula; cited in the Cloudflare backoff package.
- **AWS Builders' Library — Timeouts, retries and backoff with jitter**: `https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/`.
- **Go `encoding/json` DisallowUnknownFields**: `https://pkg.go.dev/encoding/json#Decoder.DisallowUnknownFields` and the original proposal `https://github.com/golang/go/issues/15314`.
- **Go FAQ on storing Context in structs**: `https://pkg.go.dev/context` — "Do not store Contexts inside a struct type; instead, pass a Context explicitly to each function that needs it."
- **`sync.Map` documented use cases**: `https://pkg.go.dev/sync#Map` — "The Map type is optimized for two common use cases…" (read-mostly with disjoint key sets; not the TTL-cache pattern).
- **`go.uber.org/goleak`**: `https://github.com/uber-go/goleak` — goroutine-leak detection in tests.
- **Keep-a-Changelog format**: `https://keepachangelog.com/en/1.1.0/`.
- **`pkg.go.dev` example documentation conventions**: `https://go.dev/blog/examples`.

---
*Pitfalls research for: Go HTTP/JSON SDK library wrapping OpenHolidays REST API*
*Researched: 2026-05-26*
