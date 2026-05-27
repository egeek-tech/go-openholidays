# Phase 2: Transport — Discussion Log

**Session date:** 2026-05-27
**Mode:** default (4 single-question turns, no flags)
**Areas selected for discussion:** 4 (all presented gray areas)

This log is for human reference only. Downstream agents (researcher, planner, executor) read `02-CONTEXT.md`, not this file.

---

## Area 1 — Oversized-body error (TRANS-02)

**Framing:** When `io.LimitReader` returns a body at exactly 10 MiB+1, what error shape do callers see? Phase 1 locked a 5-sentinel public surface (CL-01). Adding a 6th sentinel needs the same kind of justification CL-01 provided: a *semantically distinct, branch-worthy* failure mode.

### Q1: How should the oversized-body (>10 MiB) failure surface to callers?

**Options presented:**
1. **New exported sentinel `ErrResponseTooLarge` (Recommended)** — 6th sentinel; `errors.Is(err, ErrResponseTooLarge)` works; becomes CL-07.
2. Unexported `errResponseTooLarge` wrapped via `%w` — caller cannot `errors.Is` from outside.
3. Inline `fmt.Errorf`, no sentinel — simplest but loses `errors.Is`.

**User selected:** Option 1 — New exported sentinel `ErrResponseTooLarge` (Recommended).

**Decision captured as:** D-24 (the sentinel) + D-25 (the unexported `maxResponseBytes` constant). CL-07 entry must be added to PROJECT.md `Key Decisions`.

---

## Area 2 — Timeout enforcement strategy

**Framing:** `WithTimeout(d)` defaults to 15s (CLIENT-06). The constraint that bites is CLIENT-09: `ctx.Done()` must interrupt within ≤ 100 ms. `http.Client.Timeout` alone doesn't satisfy this on TLS-handshake or body-read paths; per-request `context.WithTimeout` does.

### Q1: How should `WithTimeout(d)` be enforced?

**Options presented:**
1. **Per-request `context.WithTimeout` only (Recommended)** — ctx is the single source of truth; CLIENT-09 cleanly satisfied.
2. Both layers (defense-in-depth) — extra timer interactions; documenting precedence becomes a doc burden.
3. `http.Client.Timeout` only — fails CLIENT-09's ≤ 100 ms on TLS / body-read paths.

**User selected:** Option 1 — Per-request `context.WithTimeout(ctx, c.timeout)` (Recommended).

**Decisions captured as:** D-26 (`WithTimeout` sets `cfg.timeout` only), D-27 (per-request `context.WithTimeout` wrap in each endpoint), D-28 (default 15s; `WithTimeout(0)` = no SDK-imposed deadline).

---

## Area 3 — Transport scaffold scope

**Framing:** TRANS-04 requires only `headerTransport + loggingTransport` for Phase 2. ARCHITECTURE.md describes the full future chain (`retry → cache → hook → logging → headers → underlying`). How much do we scaffold now?

### Q1: How much of the future RoundTripper chain should Phase 2 scaffold?

**Options presented:**
1. **Minimal: header + logging only (Recommended)** — Phase 2 ships exactly the two required RoundTrippers; later phases edit `buildTransport` directly.
2. No-op placeholders for retry / cache / hook — dead code on the wire now; harder to test the stubs.
3. Pluggable middleware-list mechanism (`[]Middleware` + `WithMiddleware(mw)` Option) — extra abstraction; expands public API pre-1.0.

**User selected:** Option 1 — Minimal: header + logging only (Recommended).

**Decisions captured as:** D-29 (chain order: `req → headerTransport → loggingTransport → underlying`), D-30 (`headerTransport.RoundTrip` clones req before mutating headers; caller-supplied headers win), D-31 (`loggingTransport.RoundTrip` emits OBS-02 fields at Debug; never reads body; `attempt` hardcoded to 1 in Phase 2).

---

## Area 4 — Phase 1 W-01 fix timing

**Framing:** W-01 is the validator Unicode case-fold bypass surfaced by the Phase 1 code review (e.g., `strings.ToLower("KK")` where `KK` is two Kelvin signs U+212A produces ASCII `"kk"` that passes the ASCII shape check applied *after* canonicalization). It does NOT affect Phase 2 (Countries takes no validated params), but it MUST be sound before Phase 3 endpoints take country codes from callers.

### Q1: When should W-01 be fixed?

**Options presented:**
1. **Phase 2 fixes it (Recommended)** — Small plan in Phase 2; ~10 min of work; closes the gap before any HTTP wiring exposes it.
2. Phase 1.1 decimal hotfix via `/gsd:plan-phase 1 --gaps` — Heavier process for a 4-line fix; clean audit trail.
3. Wait for Phase 3 — Risk: forgets to fix; ships the bug into a real call path.

**User selected:** Option 1 — Phase 2 fixes it (Recommended).

**Decisions captured as:** D-32 (the fix in `validate.go` — ASCII shape check before canonicalize; byte-level predicate), D-33 (no CL row — defect fix against locked contract, not a deviation), D-34 (W-02/W-03/W-04 remain on backlog).

---

## Deferred items (parked, not in Phase 2)

- Environment-driven base URL override — explicitly rejected by ARCHITECTURE.md.
- `WithMiddleware` pluggable middleware list — kept off the v0.x public surface.
- Configurable 10 MiB cap — PROJECT.md cap is locked.
- Phase 1 W-02 / W-03 / W-04 — handled in a later cleanup phase.
- `go.uber.org/goleak` as test dep — Phase 2 uses `runtime.NumGoroutine` delta for the leak audit instead.
- Debug-only body preview logging — Phase 4 may revisit when upstream schema drift becomes a real signal.

---

## Open questions for downstream

None. All Phase 2 gray areas were decided.

---

*Generated by /gsd-discuss-phase 2 on 2026-05-27.*
