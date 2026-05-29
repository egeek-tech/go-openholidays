<!-- GSD:project-start source:PROJECT.md -->
## Project

**go-openholidays**

`go-openholidays` is an idiomatic, dependency-light Go client library for the public OpenHolidays API (https://www.openholidaysapi.org). It exposes public holidays, school holidays, country and language metadata, and administrative subdivisions through a clean, well-tested Go-first API targeted at backend engineers building HR, scheduling, education, and leave-management applications — especially those needing **regional school-break granularity** (e.g. Polish *ferie* per województwo) that competing libraries do not cover.

**Core Value:** A single, well-tested Go client that returns both **public holidays AND school holidays per administrative subdivision** for the public OpenHolidays API, with zero runtime dependencies, full `context.Context` propagation, and typed errors. If everything else fails, this must work: `client.PublicHolidays(ctx, ...)` and `client.SchoolHolidays(ctx, ...)` returning correctly-typed, validated data for Poland 2025.

### Constraints

- **Tech stack**: Go ≥ 1.23 minimum (raised from 1.22 after research surfaced that `iter.Seq` is a Go 1.23 feature). CI matrix tests 1.23, 1.24, and `stable`. — `iter.Seq` is core to the helper API; aligning the floor avoids build tags or a separate compat shim.
- **Dependency policy**: zero runtime dependencies — no non-stdlib import in any `.go` file outside `*_test.go`. Test-only deps must be vetted and may only appear in `*_test.go` imports; pre-approved set: `github.com/stretchr/testify` (assert + require — primary assertion library per Gold Rule 3), `github.com/google/go-cmp` (deep-equal diffs when testify output is insufficient). Any additional test-only dep requires a `Key Decisions` entry. — Reduces supply-chain attack surface and keeps `go get` fast for consumers.
- **License**: MIT, single root `LICENSE`; no per-file headers required. — Standard for Go OSS libraries.
- **Style**: `gofmt`-clean; `.golangci.yml` shipped in repo; lints required: `govet`, `errcheck`, `staticcheck`, `gosec`, `revive`, `gocritic`. — Enforces code quality without bikeshedding.
- **Public surface area**: minimize. Every exported symbol must have a doc comment. Internal helpers live under `internal/`. — Stable v1.0 API later requires a disciplined v0.x surface now.
- **No `init()` side effects, no global mutable state.** — Predictability and testability.
- **HTTP semantics**: every request sends `Accept: application/json` and `User-Agent: go-openholidays/<version>`. Default timeout 15 s. `io.LimitReader` caps response body at 10 MiB. — Robustness against misbehaving servers.
- **Cancellation**: `context.Context` cancellation must interrupt in-flight HTTP within ≤ 100 ms. — Standard Go ctx-aware client contract.
- **Performance**: listing 1 year of PL public holidays must be < 500 ms cold and < 5 ms when cached. — Modest but measurable; a microbenchmark proves it.
- **Security**: no secrets in repo; `govulncheck` clean in CI; inputs validated client-side before hitting network. — OSS supply-chain hygiene.
- **Logging**: default `slog.Default()`, structured. HTTP calls logged at `Debug`. Response bodies must never be logged at `Info` or above. — Avoid accidentally exposing data in operator logs.
- **Backwards compat**: pre-1.0 (`v0.x`) — breaking changes allowed with CHANGELOG entries. From `v1.0` onward, strict SemVer. — OSS norm.
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Executive Summary
## Recommended Stack
### Core Technologies
| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go (toolchain) | 1.22, 1.23, `stable` matrix; module declares `go 1.22` | Language runtime, stdlib | PROJECT.md fixes Go 1.22 floor to keep one version of backwards compat; `iter.Seq` features (Go 1.23+) gated by build tags or simply guaranteed by the requirement that one helper method needs Go 1.23 at compile time. **HIGH confidence** — fixed by the project brief. |
| `net/http` | stdlib | HTTP transport | The community-default HTTP client. No reason to pull in `fasthttp`, `resty`, or `go-resty` for a small SDK: the stdlib client is fast enough, integrates with `context.Context`, and pulling third-party clients is a recurring complaint among Go SDK reviewers in 2025. **HIGH confidence**. |
| `encoding/json` (v1) | stdlib | JSON marshaling/unmarshaling | Zero-dep mandatory. v1 is fully sufficient for this API (small response shapes, no hot-path serialization). Go 1.25 ships `encoding/json/v2` as an opt-in experiment via `GOEXPERIMENT=jsonv2`; **do not** require v2 — v1 remains the API the library targets. Optionally add a benchmark cell with `GOEXPERIMENT=jsonv2` to measure when v2 lands stably (likely Go 1.26). **HIGH confidence**. |
| `context` | stdlib | Cancellation & deadlines | Every exported endpoint method takes `ctx context.Context` as first param. Use `http.NewRequestWithContext` (never `http.NewRequest` + `req.WithContext` — the former is preferred since Go 1.13). **HIGH confidence**. |
| `log/slog` | stdlib (Go 1.21+) | Structured logging | The 2025 Go answer. No external logger needed. Library accepts `*slog.Logger` via `WithLogger(...)`; defaults to `slog.Default()`. Library code must **never** log full response bodies (PROJECT.md explicitly prohibits this above Debug). **HIGH confidence**. |
| `time` | stdlib | Time and date handling | `time.Time` is the underlying type for the custom `Date` wrapper. **HIGH confidence**. |
| `errors` | stdlib | Sentinel + wrapping | `errors.Is` / `errors.As` for typed errors per PROJECT.md. Use `fmt.Errorf("%w", err)` for wrapping; declare sentinels as `var ErrX = errors.New("...")`. **HIGH confidence**. |
| `iter` (Go 1.23+) | stdlib | Range-over-func helpers | Used for `Holiday.Range() iter.Seq[time.Time]` per PROJECT.md. Requires Go 1.23 at build for that method. Strategy: either (a) bump module's `go` directive to `1.23` (drops 1.22 from compile target but matches the CI matrix's reality), or (b) keep `go 1.22` and put the `iter.Seq` helper behind `//go:build go1.23` build tag. Recommend **(a)** for simplicity — Go 1.22 has been off mainline support for over a year by 2026-05. **MEDIUM confidence** (depends on user's tolerance for that go-directive bump; flag this for the roadmap phase). |
| `flag` | stdlib | CLI argument parsing for `cmd/ohcli` | The demo CLI is small (two subcommands per PROJECT.md). Stdlib `flag` works with manual subcommand dispatch (`os.Args[1]` switch + per-subcommand `flag.NewFlagSet`). This is the zero-dep correct choice. If the CLI grows beyond ~4 subcommands, revisit `cobra` — but not for v0.1.0. **HIGH confidence** (stdlib `flag` remains acceptable for small CLIs in 2025/2026; it's only "limiting" when POSIX-style `--long=value`, command trees, or auto-generated help/man pages are needed). |
### Supporting Libraries
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/google/go-cmp/cmp` | latest (v0.6.x or newer) | Deep-equal diffing in tests | **Test-only.** Pre-approved in PROJECT.md. Use `cmp.Diff(want, got)` for table-driven tests producing readable diffs. `reflect.DeepEqual` is acceptable but produces unreadable failure output; `cmp` is the de-facto Go testing standard in 2025. **HIGH confidence**. |
#### Libraries explicitly NOT added (with rationale)
| Library | Why not |
|---------|---------|
| `github.com/cenkalti/backoff/v4`, `github.com/avast/retry-go` | Retry logic for this library is ~80 lines of code (exponential backoff + full jitter + `Retry-After` parsing + ctx cancellation). Pulling a dep for that adds supply-chain surface, conflicts with PROJECT.md's zero-runtime-dep rule, and surrenders control over the exact retry semantics consumers see. **HIGH confidence — write our own.** |
| `github.com/hashicorp/go-retryablehttp` | Same as above, plus it wraps the `*http.Client` and obscures the user's own transport chain — bad for a library that exposes `WithHTTPClient`. **HIGH confidence.** |
| `github.com/spf13/cobra`, `github.com/urfave/cli` | Demo CLI has two subcommands. Stdlib `flag` handles this cleanly. Adding `cobra` would pull `spf13/pflag`, `inconshreveable/mousetrap` (Windows-only but always in graph) and several others. **HIGH confidence.** |
| `github.com/bytedance/sonic`, `github.com/goccy/go-json`, `github.com/mailru/easyjson` | Faster than stdlib but with cost: `sonic` requires CGO on some paths and amd64-favoring assembly; `go-json` has had subtle correctness regressions historically; `easyjson` requires code generation. JSON parsing is not the bottleneck for an SDK that calls a public REST API across the public internet — network is. **HIGH confidence — stdlib `encoding/json` is correct.** |
| `cloud.google.com/go/civil`, `github.com/jjeffery/civil`, `github.com/rickb777/date` | All solve "I want a date type without timezone". Each is well-designed, but each is a runtime dependency. Project's `Holiday.Date` field is parsed from `YYYY-MM-DD` strings — a 10-line custom `Date` type wrapping `time.Time` with `MarshalJSON`/`UnmarshalJSON` and `String()` is sufficient. Document that internal representation is `time.Time` at midnight UTC for ergonomics with `time.Date`/`time.Before`/etc. **HIGH confidence.** |
| `github.com/stretchr/testify` | Convenient `assert.Equal` etc., but a heavy dep tree and the Go community has been moving away from it in 2025 in favor of `cmp.Diff` + plain `t.Errorf`. **HIGH confidence — stick with stdlib `testing` + `cmp`.** |
| `github.com/patrickmn/go-cache`, `github.com/coocood/freecache`, `VictoriaMetrics/fastcache` | TTL cache patterns are simple — `sync.RWMutex` + `map[string]entry{value, expiresAt}` + janitor goroutine is ~50 lines. Pulling an external cache surrenders control over eviction policy and TTL semantics. **HIGH confidence — write our own.** |
### Development Tools
| Tool | Purpose | Notes |
|------|---------|-------|
| `golangci-lint` | Aggregated linter runner | Use **v2.x** (released March 2025, current stable as of May 2026 is ~v2.12.x). Migration from v1 is one-command (`golangci-lint migrate`). Configure `.golangci.yml` with `version: "2"`, `linters.default: standard` plus the PROJECT.md-required additions: `govet`, `errcheck`, `staticcheck`, `gosec`, `revive`, `gocritic` (note: `staticcheck` is in the `standard` default set in v2, so this is mostly explicit-enabling for `gosec`, `revive`, `gocritic`). **HIGH confidence**. |
| `govulncheck` | Vulnerability scanner | `go install golang.org/x/vuln/cmd/govulncheck@latest` in CI. Run as `govulncheck ./...`. Only reports vulnerabilities the binary's call graph actually reaches, which keeps the signal-to-noise ratio high. **HIGH confidence**. |
| `goreleaser` | Release pipeline (CLI binaries only — library doesn't need it) | Use **v2.x** (current stable as of May 2026 is ~v2.15.x). `.goreleaser.yaml` with `version: 2`. Cross-compile cmd binaries for linux/darwin/windows × amd64/arm64, set `CGO_ENABLED=0` for static binaries, generate checksums + SLSA-style provenance. Library itself is consumed via `go get` and needs no release artifacts beyond the git tag. **HIGH confidence**. |
| `gofmt` / `gofumpt` | Code formatting | `gofmt` is the floor (required). `gofumpt` (stricter formatter, used as a `golangci-lint` linter) is optional but commonly added in 2025/2026 OSS Go libraries for consistency. Recommend enabling via `gofumpt` linter in `.golangci.yml`. **MEDIUM confidence** (community split — some projects skip it). |
| `go test -race -cover` | Race detector + coverage | Standard tool. PROJECT.md requires `-race`-clean. CI runs `go test -race -coverprofile=cover.out ./...`. **HIGH confidence**. |
| GitHub Actions | CI | Matrix on `go-version: [1.22.x, 1.23.x, stable]`. `actions/setup-go@v5` (current major). Cache modules with the action's built-in caching. **HIGH confidence**. |
| Dependabot or Renovate | Dep update bot | Trivially small dep graph (just `cmp` test-only + Actions versions), but worth turning on for security updates on `actions/setup-go` etc. **HIGH confidence**. |
## Installation
# Library has zero runtime dependencies — no `go get` needed for users beyond
# the library itself.
# In the repo, the test-only dep:
# Dev tooling (one-time install per developer machine):
# Optional but recommended:
## Detailed Pattern Recommendations
### 1. `net/http` patterns (HIGH confidence)
- **Headers** (User-Agent, Accept) — set once via a header-injecting RoundTripper.
- **Logging** (slog Debug-level method+URL+status+duration; never the body).
- **Hook** (`WithRequestHook` invokes user callback after each round trip).
### 2. JSON handling (HIGH confidence)
- **Speed:** `sonic` is ~5x faster on unmarshal, `go-json` ~1.4x faster.
- **Reflection cost:** `encoding/json` uses reflection. For ~50 holidays per
- **Memory:** v1 allocates intermediate `interface{}` values. Same conclusion
### 3. Logging — `log/slog` (HIGH confidence)
### 4. Retry/backoff — hand-rolled (HIGH confidence)
- **Full jitter** (not equal jitter, not decorrelated jitter): formula
- **`Retry-After` parsing** — accept both forms per RFC 7231:
- **Retryable conditions:** network errors (`net.Error.Timeout()`, conn
- **`ctx` cancellation:** use `select { case <-ctx.Done(): ... case <-time.After(delay): }` — never bare `time.Sleep`, which is uninterruptible.
- **Idempotency:** all five endpoints are GETs and safely retryable. Document
- **Random source:** use `math/rand/v2` (Go 1.22+) for jitter — no seeding
### 5. In-memory TTL cache (HIGH confidence)
- `sync.Map` is optimized for write-once read-many or for goroutines that
- `sync.Map` has no convenient iteration for the janitor; its `Range` works
- A plain map under `RWMutex` is easier to reason about, easier to add an
### 6. Testing (HIGH confidence)
- Top-level tests: yes, parallel by default. Each unit test starts its own
- Subtests inside table-driven loops: yes, parallel, with proper variable
- Tests that mutate global state (e.g., `os.Setenv`): **no parallel** —
### 7. CLI in `cmd/ohcli` — stdlib `flag` (HIGH confidence)
### 8. Build/release — `goreleaser` v2 + `golangci-lint` v2 (HIGH confidence)
### 9. `iter.Seq` / range-over-func (MEDIUM-HIGH confidence)
- **Stdlib has adopted it heavily:** `slices.All`, `slices.Values`,
- **Third-party adoption is mixed but rising.** Newer libraries treat it
- **For this library:** exposing one `iter.Seq[time.Time]` helper
### 10. Errors (mentioned in retry section, expanded here for completeness)
## Alternatives Considered
| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Functional options | Config struct (`type Config struct { Timeout time.Duration; ... }` passed to `New(cfg)`) | Fewer options (≤ 3), or when callers benefit from struct literal syntax with named fields. For an SDK with growing optionality, options scale better. |
| stdlib `flag` | `cobra` | CLI grows past 4 subcommands or needs nested commands, completion scripts, auto-generated man pages. |
| stdlib `encoding/json` | `encoding/json/v2` (Go 1.25 experiment) | When v2 stabilizes (likely Go 1.26 promotion); evaluate then. Today's experimental v2 is fine to benchmark but not require. |
| stdlib `encoding/json` | `sonic` / `go-json` | When JSON parse is provably the bottleneck (typically server-side hot paths processing 10K+ records/sec). Not this SDK. |
| Custom `Date` wrapping `time.Time` | `cloud.google.com/go/civil.Date` | If multiple types in the API needed civil dates and the consumer base was already on Google Cloud (the import path is well-known there). For zero-dep, hand-rolled wins. |
| Hand-rolled retry | `hashicorp/go-retryablehttp` | If the SDK ever needs to support arbitrary user-supplied retry policies pluggably. For one canonical policy with reasonable knobs, hand-roll. |
| `sync.RWMutex` + map cache | `sync.Map` cache | If access pattern shifts to many readers / very rare writes on disjoint keys (the workload sync.Map was designed for). Not this SDK. |
| `cmp.Diff` for tests | `testify/assert.Equal` | If the team is already on testify and migration cost outweighs benefit. New code in 2025: prefer `cmp`. |
| `log/slog` | `zap`, `zerolog` | When sub-microsecond logging perf matters (high-throughput servers). Not a client SDK with a few debug logs per HTTP call. |
| Custom RoundTripper chain | Middleware framework (e.g., `go-chi/chi` for clients) | These don't really exist for clients — chi is server-side. Custom chain is the idiom. |
| Hand-rolled small cache | `patrickmn/go-cache`, `karlseguin/ccache` | If LRU eviction or memory bounding becomes a requirement. v0.1.0 doesn't need either. |
## What NOT to Use
| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `http.Get` / `http.Post` package-level helpers | Don't propagate `ctx`; use `http.DefaultClient`; impossible to instrument | `http.NewRequestWithContext` + `c.http.Do(req)` |
| `http.NewRequest` (no ctx) | Deprecated form since Go 1.13 in favor of `NewRequestWithContext`; cancellation can't be wired | `http.NewRequestWithContext` |
| `json.Unmarshal(body, &v)` without size cap | Hostile server can return 10 GiB; library OOMs | `io.LimitReader` then `json.NewDecoder(lr).Decode(&v)` |
| `defer resp.Body.Close()` without draining | Drained connection-pool semantics — sometimes the connection isn't returned to pool | Drain with `io.Copy(io.Discard, ...)` then close |
| `time.Sleep(delay)` in retry loop | Not interruptible by `ctx` cancellation | `select { case <-ctx.Done(): case <-time.After(delay): }` |
| `math/rand` (v1) for jitter | Requires manual seeding (footgun); not concurrent-safe without `rand.New(rand.NewSource(...))` boilerplate | `math/rand/v2` (Go 1.22+) |
| `init()` for client construction | PROJECT.md forbids; also bad practice (untestable, hidden side effects) | Constructor pattern |
| Global mutable state (e.g., shared default cache) | PROJECT.md forbids; thread-safety footgun | Per-Client state |
| `interface{}`-typed `Get/Set` cache | Loses type safety, every read needs a type assertion | Generic cache `cache[T]` if type erasure cost matters; otherwise scoped internal caches per call type |
| `panic` for input validation | Libraries should return errors; panic is reserved for "programmer error" | Return typed errors |
| `slog.SetDefault` from library code | Mutates global state for the entire process | Accept `*slog.Logger` via option |
| `tc := tc` shadow in table-driven loops (Go 1.22+ code) | Redundant since Go 1.22 scopes loop vars per iteration | Drop it; linters may flag it |
| Logging full response bodies | Leaks data into operator logs (PROJECT.md prohibits above Debug) | Log method/URL/status/duration only; truncate body if at Debug |
| `time.Now().Unix()` for deadlines | Loses sub-second precision | `time.Now().Add(d)` for deadlines |
## Stack Patterns by Variant
- Migrate to `github.com/spf13/cobra`
- Because: subcommand routing, POSIX flags, auto-generated help, completion scripts all become real wins past that complexity threshold.
- Adopt `encoding/json/v2` for `Holiday.UnmarshalJSON` if benchmarks show >20% improvement
- Because: stdlib still wins for zero-dep, v2 is faster, and the migration is mostly mechanical with v2's option-driven API.
- Add a parallel method, don't break the existing one
- Because: pre-1.0 the library can break things, but the friction is high enough that adding a new method is cheaper.
- Move from `httptest.NewServer` per-test to a recorded fixture replay tool (e.g., implement a thin `http.RoundTripper` that reads from `testdata/recordings/`)
- Because: maintenance of canned fixtures scales poorly past ~30 distinct request shapes.
- Add a `WithRateLimit(rps int)` option backed by `golang.org/x/time/rate.Limiter`
- Because: `x/time/rate` is the de-facto stdlib-adjacent token bucket; it's the only `golang.org/x/*` dependency that's nearly universal in HTTP clients. **Important:** this would break the zero-dep rule — current scope doesn't need it (POC observed no rate-limit headers).
## Version Compatibility
| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| Go 1.22 module | `cmp` (latest), Go 1.22+ stdlib | `cmp` requires Go 1.21+; safe. |
| Go 1.23 features (`iter.Seq`) | Module `go` directive 1.23+ | Bump directive when adopting `Holiday.Range()`. |
| `math/rand/v2` | Go 1.22+ | Available throughout the support matrix; no compat concern. |
| `t.Setenv` | Go 1.17+ | Safe across matrix. |
| `testing.F.Fuzz` | Go 1.18+ | Safe across matrix. |
| `slog` | Go 1.21+ | Safe across matrix. |
| `GOEXPERIMENT=jsonv2` | Go 1.25+ | Optional/experimental; not on the support floor. |
| `golangci-lint` v2 | Any Go in the matrix | v2.x supports linting code built with Go 1.21+. |
| `goreleaser` v2 | Any Go in the matrix | v2.x builds with any modern Go toolchain. |
| `actions/setup-go@v5` | Latest Actions runner | Major v5 is stable and current as of 2026. |
## Sources
- [Go 1.23 Release Notes](https://go.dev/doc/go1.23) — `iter.Seq` package, range-over-func, stdlib iterator helpers. HIGH confidence.
- [Go 1.24 Release Notes](https://go.dev/doc/go1.24) — `bytes`/`strings` iterator helpers (`Lines`, `SplitSeq`, etc.), generic type aliases. HIGH confidence.
- [Go 1.25 Release Notes](https://tip.golang.org/doc/go1.25) — `encoding/json/v2` experiment, container-native improvements. HIGH confidence.
- [A new experimental Go API for JSON](https://go.dev/blog/jsonv2-exp) — official rationale for json v2, perf and API tradeoffs. HIGH confidence.
- [`log/slog` package docs](https://pkg.go.dev/log/slog) — official slog reference. HIGH confidence.
- [Structured Logging with slog (go.dev blog)](https://go.dev/blog/slog) — official intro and gotchas. HIGH confidence.
- [Welcome to golangci-lint v2 (ldez)](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) — v2 launch post, configuration changes, migration. HIGH confidence (project maintainer).
- [golangci-lint Configuration](https://golangci-lint.run/docs/configuration/) — current v2 config schema. HIGH confidence.
- [Announcing GoReleaser v2](https://goreleaser.com/blog/goreleaser-v2/) — v2 launch and config schema. HIGH confidence.
- [GoReleaser Build docs](https://goreleaser.com/customization/builds/) — build matrix for Go binaries. HIGH confidence.
- [`govulncheck` docs (pkg.go.dev)](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) — official vuln scanner. HIGH confidence.
- [hashicorp/go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp) — reference for Retry-After parsing semantics (used as design reference, not a dep). MEDIUM confidence (third-party project's docs).
- [cenkalti/backoff/v4](https://pkg.go.dev/github.com/cenkalti/backoff/v4) — reference for jitter strategy patterns (design reference, not a dep). MEDIUM confidence.
- [Middleware and RoundTrippers in Go (DEV)](https://dev.to/calvinmclean/middleware-and-roundtrippers-in-go-30pa) — community summary of tripperware pattern. MEDIUM confidence (community blog).
- [Tripperwares: http.Client Middleware (DEV)](https://dev.to/stevenacoffman/tripperwares-http-client-middleware-chaining-roundtrippers-3o00) — chaining pattern. MEDIUM confidence.
- [Implementing an In-Memory Cache in Go (Alex Edwards)](https://www.alexedwards.net/blog/implementing-an-in-memory-cache-in-go) — canonical small Go cache pattern. MEDIUM confidence (well-respected Go author).
- [Parallel Table-Driven Tests in Go (glukhov.org)](https://www.glukhov.org/post/2025/12/parallel-table-driven-tests-in-go/) — 2025 update on Go 1.22 loop-scoping fix and t.Parallel etiquette. MEDIUM confidence.
- [`pkg.go.dev` jjeffery/civil](https://pkg.go.dev/github.com/jjeffery/civil) — comparison reference for civil-date alternatives. MEDIUM confidence.
- [Choosing a Go CLI Library (mt165.co.uk)](https://mt165.co.uk/blog/golang-cli-library/) — survey of Go CLI library tradeoffs. MEDIUM confidence.
- [JetBrains "The Go Ecosystem in 2025"](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025/) — 2025 ecosystem snapshot (frameworks, tools, practices). MEDIUM confidence (industry survey).
- [go-json-experiment/jsonbench](https://github.com/go-json-experiment/jsonbench) — official jsonv2 vs alternatives benchmarks. HIGH confidence (run by Go team / json v2 authors).
- [Go 1.25 JSON v2 Benchmarks (dev.to/ryansgi)](https://dev.to/ryansgi/go-125-json-v2-benchmarks-raptor-escapes-and-a-18-speedup-5cf3) — third-party benchmark write-up. MEDIUM confidence.
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Gold Project Rules
### Rule 1 — Everything in English
### Rule 2 — Never guess; verify or ask
- Verify it (read the source file, run the command, hit the endpoint, check the upstream OpenAPI spec).
- Or stop and ask the user.
- API claims ("the endpoint returns X") — verify by reading the OpenAPI spec or hitting the live endpoint.
- Code claims ("function Y returns Z") — read the source.
- Test claims ("this assertion passes") — run the test.
- Behavior claims ("the retry honors `Retry-After`") — exercise it under a fake transport.
### Rule 3 — Test conventions (testify + one-test-per-prod-function + t.Run)
- `testify` is the de-facto Go test framework in 2025/2026; raw `if got != want { t.Fatalf(...) }` is allowed but tedious for SDKs with rich result types.
- One-test-per-prod-function makes `go test -run TestXxx` predictable and matches IDE "go to test" navigation.
- `t.Run` per case lets CI report each case as a separate row, enables `-run` filtering, and gates failures per-case under `-failfast`.
- `github.com/stretchr/testify` (assert + require) — primary assertion library.
- `github.com/google/go-cmp` — deep-equal diffs when testify's output is insufficient (rare).
## Conventions evolve
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->

<!-- Manual section — NOT managed by gsd-sdk. Safe to edit; survives `gsd-sdk query generate-claude-md` regenerations. -->

## Project Rules (Gold)

Non-negotiable rules. Take precedence over agent recommendations, research conclusions, and generated plans. Full text and rationale: `.planning/codebase/CONVENTIONS.md`.

### Rule 1 — Everything in English

All code, code comments, package docs, godoc strings, test names, commit messages, CHANGELOG entries, README, design docs, and ADRs are written in English. No Polish, no mixed-language identifiers, no non-ASCII identifiers. Exception: `testdata/` fixtures may contain non-English strings only when they reflect real upstream API responses (e.g. `"Wigilia Bożego Narodzenia"` from OpenHolidays).

### Rule 2 — Never guess; verify or ask

If you do not know something with confidence, do not write it as if you do. Either verify it (read the source file, run the command, hit the endpoint, check the upstream OpenAPI spec) or stop and ask the user. Words like *"I think"*, *"probably"*, *"should be"*, *"most likely"* in a draft response signal stop-and-verify before sending. If verification would take longer than asking, ask first.

### Rule 3 — Test conventions (testify + one-per-prod-function + t.Run)

- Use `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require` as the assertion libraries. Both are test-only — they may only appear in `*_test.go` imports.
- Exactly one `TestXxx` function per exported production function. If `holidays.go` exports `func (c *Client) PublicHolidays(...)`, the test file has exactly one `func TestClient_PublicHolidays(t *testing.T)`.
- Every test case lives inside a `t.Run(name, func(t *testing.T) { ... })`. No top-level assertions in the outer `TestXxx` body. Table-driven by default when ≥ 2 cases share setup.
- `require` for preconditions (aborts the case), `assert` for verifications (reports without aborting).

Approved test-only dependencies: `github.com/stretchr/testify` (primary), `github.com/google/go-cmp` (deep-equal diffs when testify is insufficient). Any further test-only dep requires explicit user approval and a `Key Decisions` entry in PROJECT.md.

### Rule 4 — Published releases and tags are immutable; fix forward only

Once a release tag (`vX.Y.Z`) has been pushed to origin OR a GitHub Release has been transitioned out of draft state, both are **frozen**. The only sanctioned response when something is wrong with a published release is to cut a new release that supersedes it — never delete, never rewrite, never amend.

**Forbidden**: `git push --force` on tag refs, `git push origin :refs/tags/vX.Y.Z`, `gh release delete` on non-draft releases, editing a published release's body / title / assets, force-pushing master after a tag has been cut from it, amending a `CHANGELOG.md` entry for an already-published version.

**Allowed**: discarding a release while it is still `draft: true` (drafts are not yet reachable by consumers); cutting a new release that supersedes a broken one (`v0.2.5` supersedes broken `v0.2.4`); adding clarifying entries to `docs/release-runbook.md` §8 "Release history" describing what went wrong.

**Why**: `go get` and the Go module proxy cache module bytes by `(module, version)` — once any caller has fetched a version, those bytes persist in their cache regardless of upstream changes. SLSA / sigstore attestations chain to specific commit + tag + workflow run; rewriting a tag breaks every attestation that referenced it. Audit logs reference prior state by SHA. Trust signal degrades on every silent rewrite.

**Recovery from a bad release**: leave it; open a `fix:` PR; let Release Please cut the next version; document the bad release in `docs/release-runbook.md` §8 (and §6 if it's a recurring class). Precedents on this repo: `v0.2.0`, `v0.2.1`, `v0.2.2` ship with zero binary assets due to various pipeline bugs; `v0.2.4` is a stray draft from an aborted release run. **None have been deleted or rewritten.** `v0.2.3`, `v0.2.5+` are the fix-forward responses; the broken releases stay as audit trail.
