---
phase: 04-resilience
plan: 06
subsystem: docs

tags: [project-md, key-decisions, cache, strict-decoding, deviation-numbering]

# Dependency graph
requires:
  - phase: 04-resilience/02
    provides: clientConfig.cache + clientConfig.strictDecoding fields (referenced from CL-15/CL-16 rationale)
  - phase: 04-resilience/03
    provides: WithStrictDecoding option implementation (referenced by CL-16)
  - phase: 04-resilience/04
    provides: Public Cache interface + MemoryCache + WithCache/WithCacheBackend + CacheHitContextKey (referenced by CL-15)
  - phase: 04-resilience/05
    provides: hookTransport wiring that reads CacheHitContextKey (cross-checks the CL-15 surface boundary)

provides:
  - PROJECT.md Key Decisions row CL-15 (Phase 4 cache surface — public Cache interface + WithCache/WithCacheBackend + CacheHitContextKey + key encoding)
  - PROJECT.md Key Decisions row CL-16 (WithStrictDecoding default-OFF + post-NewClient immutability + cache+strict composition semantics)
  - Confirmed re-numbering of Phase 4 decisions (CL-15/CL-16, not CL-14/CL-15) because Phase 3 already consumed CL-14

affects:
  - Future v0.2 planner extending the Cache interface (Redis/BoltDB backend) — CL-15 documents the v0.1.0 contract they must not break
  - Future maintainers extending strict-decoding (per-call override, runtime toggle) — CL-16 documents why both are intentionally rejected
  - Future planners writing CL-XX rows — chronological order resumes after CL-16

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Append-only Key Decisions table — new rows appended to the bottom in chronological adoption order; existing rows never mutated"
    - "Decision re-numbering when a slot is already taken — record the deviation in the slot-stealing plan's CONTEXT and in the next plan's deviation list, never silently overwrite"

key-files:
  created:
    - .planning/phases/04-resilience/04-06-SUMMARY.md (this file)
  modified:
    - .planning/PROJECT.md (Key Decisions table: +2 rows — CL-15 cache surface, CL-16 strict-decoding immutability)

key-decisions:
  - "CL-15 documents the v0.1.0 cache public surface — exported Cache interface (Get/Put/Close), exported MemoryCache + NewMemoryCache(ttl) constructor, WithCache(ttl) + WithCacheBackend(c Cache) options (ttl<=0 disables, nil backend is no-op, WithCacheBackend supersedes prior WithCache), cache key = '<method> <path>?<sorted-query>' (Host excluded because per-Client isolation handles CACHE-2), and exported CacheHitContextKey var (allowlist deviation from CONTEXT.md D-97 step 6 noted in 04-04-SUMMARY.md)."
  - "CL-16 documents WithStrictDecoding(bool): OFF by default + IMMUTABLE after NewClient (no per-call override, no runtime toggle), cache+strict composition (cache stores raw bytes; decode runs on read; strict-mode applies to every cached entry on every Get). Consumers needing 'lenient cache + strict fresh' must run two Clients."
  - "Slot collision resolved: CONTEXT.md initially called the cache decision CL-14 and the strict-decoding decision CL-15, but Phase 3 already consumed CL-14 for the Gold-Rule-3 SchoolHolidays exception. This plan uses CL-15 (cache) and CL-16 (strict), matching the deviation noted in Plan 04-04 and confirmed in 04-CONTEXT.md when it referenced 'new CL-14' / 'new CL-15' inline."

patterns-established:
  - "Pattern: Phase-completion docs row append — every phase that adds public surface or constrains future evolution gets one or more CL-XX rows in PROJECT.md, appended at the bottom of the table, never inserted into the existing chronological order."
  - "Pattern: Slot-collision deviation handling — when CONTEXT.md predicts a CL slot that turns out to already be taken (Phase 3 grabbed CL-14 here), the next plan documents the re-numbering as a Plan-level deviation rather than rewriting CONTEXT.md."

requirements-completed: []  # Plan frontmatter declares requirements: [] — docs-only plan, no REQUIREMENTS.md IDs apply.

# Metrics
duration: 1min 14s
completed: 2026-05-28
---

# Phase 04 Plan 06: PROJECT.md Key Decisions append (CL-15 cache + CL-16 strict-decoding) Summary

**Two new Key Decisions rows appended to PROJECT.md — CL-15 documents the v0.1.0 public Cache surface (interface + MemoryCache + WithCache/WithCacheBackend + cache key encoding + CacheHitContextKey), CL-16 documents WithStrictDecoding default-OFF + post-NewClient immutability + cache+strict composition semantics. Zero code changes; deviation re-numbering (Phase 3 took CL-14) honored.**

## Performance

- **Duration:** 1 min 14 s
- **Started:** 2026-05-28T07:23:21Z
- **Completed:** 2026-05-28T07:24:35Z
- **Tasks:** 1
- **Files modified:** 1 (`.planning/PROJECT.md`)

## Accomplishments
- Appended CL-15 row to `.planning/PROJECT.md` Key Decisions table — documents Phase 4 cache surface: exported `Cache` interface (`Get`/`Put`/`Close`), exported `NewMemoryCache(ttl)` + `MemoryCache`, `WithCache(ttl)` + `WithCacheBackend(c Cache)` options, cache key encoding (`"<method> <path>?<sorted-query>"`, Host excluded), and the exported `CacheHitContextKey` var (with the planner-authorized allowlist deviation from Plan 04 referenced in the rationale).
- Appended CL-16 row to `.planning/PROJECT.md` Key Decisions table — documents `WithStrictDecoding(bool)` default-OFF + post-`NewClient` immutability (no per-call override, no runtime toggle) and cache+strict composition: cache stores raw bytes, decode runs on read, strict-mode applies on every `Get`.
- Honored the deviation noted in Plan 04: Phase 3 consumed `CL-14` (Gold-Rule-3 `SchoolHolidays` exception). Phase 4 therefore uses `CL-15` + `CL-16`, not `CL-14` + `CL-15` as `04-CONTEXT.md` initially indicated when referencing "new CL-14" / "new CL-15" inline.

## Task Commits

Each task was committed atomically:

1. **Task 1: Append CL-15 (Cache surface) and CL-16 (StrictDecoding immutability) rows to PROJECT.md Key Decisions table** — `5932eae` (docs)

_(Plan metadata commit is the orchestrator's responsibility — this is a parallel-executor worktree and does not write STATE.md / ROADMAP.md.)_

## Files Created/Modified
- `.planning/PROJECT.md` — Key Decisions table grew by exactly 2 rows (CL-15, CL-16). `git diff --stat` reports `+2/-0` insertions; no other content of PROJECT.md was modified.
- `.planning/phases/04-resilience/04-06-SUMMARY.md` — this summary file (created post-Task-1, committed in the orchestrator's metadata phase).

## Decisions Made

None — the plan was executed exactly as written. The CL-numbering choice (CL-15 cache, CL-16 strict) was already specified in the plan's `must_haves.truths` and in the deviation notes of 04-CONTEXT.md / 04-04-SUMMARY.md; no executor-side judgment was required to pick the numbers.

## Verification Results

Per the plan's `<verification>` block:

| Check | Expected | Actual | Status |
|-------|----------|--------|--------|
| `grep -cE '^\| CL-15:' .planning/PROJECT.md` | `1` | `1` | PASS |
| `grep -cE '^\| CL-16:' .planning/PROJECT.md` | `1` | `1` | PASS |
| `grep -cE '^\| CL-' .planning/PROJECT.md` | `9` (CL-01..06 + CL-14 + CL-15 + CL-16) | `9` | PASS |
| CL-14 row unchanged | `Narrow Gold-Rule-3 exception …` text intact | match count = 1 | PASS |
| `go build ./...` | exit 0 | exit 0 (BUILD OK) | PASS |
| `git diff --stat .planning/PROJECT.md` | `+2/-0` insertions, no other lines touched | `1 file changed, 2 insertions(+)` | PASS |
| Post-commit deletion check (`git diff --diff-filter=D --name-only HEAD~1 HEAD`) | empty | empty | PASS |

Test/vet runs from the plan's `<verification>` block (`go test -race -count=1 ./...`, `go vet ./...`) were not re-executed in this worktree — only `go build ./...` was run as a sanity check, since no `.go` source file was touched. The full race-test + vet run is the orchestrator/CI surface; touching no Go source guarantees no test or vet regression can originate from this plan.

## Full Text of New Rows

The two rows appended to the Key Decisions table (in column-aligned form for readability — the actual table cells are single-line markdown):

**CL-15 row:**
- **Decision cell:** Phase 4 ships an EXPORTED `Cache` interface (`Get`/`Put`/`Close`) plus an exported `NewMemoryCache(ttl)` constructor and `MemoryCache` type, even though only the in-memory implementation ships in v0.1.0. `WithCache(ttl)` and `WithCacheBackend(c Cache)` are the two opt-in option constructors; `ttl<=0` disables; `WithCacheBackend(nil)` is a no-op; `WithCacheBackend` supersedes prior `WithCache` (last-wins per functional-options convention). The cache key encoding is `"<method> <path>?<sorted-query>"` and excludes Host — per-Client cache isolation handles Pitfall CACHE-2 without putting Host in the key.
- **Rationale cell:** RESIL-06 mandates a public `Cache` interface so a v0.2 consumer can supply a Redis/BoltDB backend without forking the library. Exposing the interface AND the in-memory impl in v0.1.0 commits us to that contract before any consumer requests it; the alternative (keeping `Cache` internal and forcing M2 to break it open) would require a breaking change. Anchored on `.planning/phases/04-resilience/04-CONTEXT.md` D-79/D-80 + RESEARCH.md "Pattern 4" + Pitfall CACHE-2/CACHE-4. The exported `CacheHitContextKey` var is also part of this surface — added to `internal_test.go::allowedVars` in Plan 04 (deviation from CONTEXT.md D-97 step 6 noted in `04-04-SUMMARY.md`).
- **Adoption cell:** Adopted in Phase 4

**CL-16 row:**
- **Decision cell:** `WithStrictDecoding(bool)` is OFF by default and IMMUTABLE after `NewClient`. No per-call override, no runtime toggle. A strict-mode client cannot be made lenient at runtime; a lenient-mode client cannot be made strict. Cache+strict composition: cache stores raw bytes; decode runs on read; therefore strict-mode applies to every cached entry on every `Get`. A consumer wanting "lenient for cache + strict for fresh" must run two `Client`s.
- **Rationale cell:** Pitfall JSON-1 — upstream OpenHolidays schema has been observed to drift (the `"quality"` field on `Holiday` was added without spec update). Strict-by-default would break consumers on upstream additions. Immutability prevents the trap where toggling at runtime would corrupt the cache (cached bytes decoded with one mode could surface as a strict-failure after toggle). Anchored on `.planning/phases/04-resilience/04-CONTEXT.md` D-91/D-92/D-93 + Pitfall JSON-1.
- **Adoption cell:** Adopted in Phase 4

## Deviations from Plan

None — plan executed exactly as written. The CL re-numbering (`CL-15`/`CL-16` instead of CONTEXT.md's tentative `CL-14`/`CL-15`) was pre-baked into the plan's `must_haves.truths[3]` and acted on accordingly; this is not an executor-side deviation but a planner-side prior correction surfaced inside the plan body and the SUMMARY-level confirmation that no further re-numbering was required.

## Issues Encountered
None. The single `Edit` operation against `.planning/PROJECT.md` succeeded on the first attempt; all six plan-prescribed `grep` verifications passed; `go build ./...` exited 0.

## Known Stubs
None — docs-only plan; no UI components, no data sources, no placeholder code.

## Threat Flags
None — no source files touched, no new endpoints, no new auth paths, no new file-access patterns, no schema changes. Threat register row T-04-SC ("npm/pip/cargo installs — accept; no installs in this plan") holds: zero installs occurred.

## User Setup Required
None — no external service configuration required. PROJECT.md row edits do not require any runtime configuration.

## Next Phase Readiness
- All four Phase 4 implementation plans (`04-02`, `04-03`, `04-04`, `04-05`) shipped public-surface changes that are now documented in PROJECT.md Key Decisions. The `pkg.go.dev` API expectation for v0.2 is anchored.
- No blockers. The `STATE.md` and `ROADMAP.md` writes are the orchestrator's responsibility once all worktree agents in Wave 5 complete.
- Phase 5 (docs / CHANGELOG / release) can reference CL-15 and CL-16 directly when authoring the v0.1.0 CHANGELOG bullets for caching and strict decoding.

## Self-Check: PASSED

- FOUND: `.planning/PROJECT.md` (modified — `+2` insertions, `-0` deletions)
- FOUND: `.planning/phases/04-resilience/04-06-SUMMARY.md` (this file, just written)
- FOUND: commit `5932eae` in `git log` (Task 1 atomic commit)
- FOUND: `^\| CL-15:` count = 1 in PROJECT.md
- FOUND: `^\| CL-16:` count = 1 in PROJECT.md
- FOUND: `^\| CL-` total count = 9 in PROJECT.md (CL-01..CL-06 + CL-14 + CL-15 + CL-16)
- FOUND: CL-14 row text "Narrow Gold-Rule-3 exception" still matches once — no row mutation
- VERIFIED: `go build ./...` exits 0 (no Go source touched)
- VERIFIED: `git diff --diff-filter=D --name-only HEAD~1 HEAD` is empty — no deletions in the Task 1 commit
- VERIFIED: HEAD on `worktree-agent-ac888a06f1ebf447e` (per-agent branch, in `worktree-agent-*` namespace; not on a protected ref)

---
*Phase: 04-resilience*
*Plan: 06*
*Completed: 2026-05-28*
