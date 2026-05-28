# Phase 4 — Deferred Items

Issues discovered during Phase 4 execution that are out-of-scope for the
current plan and were intentionally NOT auto-fixed. Each is a candidate for
a future cleanup task.

## countries_test.go gofmt diff (pre-existing, Phase 3)

`gofmt -d countries_test.go` reports an extra trailing blank line at EOF.
Last touched by 9730014 (`refactor(03-01): retrofit Countries to
(ctx, CountriesRequest) via doJSONGet`). Not introduced by Plan 04-04;
fixing it would be a one-line `truncate -s -1` change outside this plan's
scope. Defer to a phase-housekeeping pass or a documentation/style PR.

