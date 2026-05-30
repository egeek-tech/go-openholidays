// hierarchical region-membership helper.
//
// This file ships Client.IsInRegion (D-59 / CL-09) and its two unexported
// support functions splitCountryFromSubdivision and buildParentIndex.
// Client.IsInRegion is the ONE Phase 3 method that issues hidden I/O: it
// fetches /Subdivisions and walks Subdivision.Children to detect whether a
// child-level subdivision code is covered by a holiday whose declared
// Holiday.Subdivisions only contain the parent.
//
// Same fast-path guards as Holiday.IsInRegion (D-58 / WR-06):
//   - h.Nationwide → (true, nil), no HTTP — nationwide wins over empty code
//   - empty code on non-nationwide → (false, nil), no HTTP
//   - flat strings.EqualFold match against h.Subdivisions[].Code → (true, nil),
//     no HTTP
//   - h.Subdivisions empty (and not Nationwide) → (false, nil), no HTTP — no
//     country context to fetch a tree for
//
// Only when none of the fast paths fire does the method issue an HTTP GET via
// c.Subdivisions for the country implied by h.Subdivisions[0].Code's prefix.
//
// Cycle defense: the upward walk in IsInRegion has an explicit iteration cap
// of len(parentIdx)+1 — a cyclic parent-index (e.g. malformed upstream data
// where two entries each claim the other as a child) yields a bounded walk
// and returns (false, nil) instead of looping forever. Per RESEARCH.md
// Pitfall 4 and ASVS V5.1.4. The cap is regression-locked by the
// cycle-enforcement subtest in TestClient_IsInRegion.

package openholidays

import (
	"context"
	"strings"
)

// audit:ok 2026-05-30

// IsInRegion reports whether the holiday h applies to the administrative
// subdivision identified by code, accounting for hierarchical subdivision
// nesting (CL-09 / D-59). Where Holiday.IsInRegion only performs a flat
// match against h.Subdivisions[].Code, this method additionally walks the
// upstream /Subdivisions tree to detect whether code is a descendant of any
// subdivision the holiday applies to.
//
// Fast paths (no HTTP issued):
//
//  1. h.Nationwide → returns (true, nil) — a nationwide holiday applies
//     everywhere, including the empty-string code (WR-06).
//  2. code == "" → returns (false, nil) — defensive guard on non-nationwide
//     holidays only.
//  3. Flat [strings.EqualFold] match against h.Subdivisions[].Code → returns
//     (true, nil).
//  4. len(h.Subdivisions) == 0 (and not Nationwide) → returns (false, nil) —
//     there is no country context to fetch a tree for.
//
// Hierarchical path: when none of the fast paths fire, IsInRegion issues an
// HTTP GET to /Subdivisions for the country implied by the prefix of
// h.Subdivisions[0].Code (e.g. "PL" from "PL-SL", "DE" from "DE-BY"). It
// then builds a child→parent index from the recursive Subdivision.Children
// shape and walks upward from code until either a [strings.EqualFold] match
// against an entry in h.Subdivisions is found (returns (true, nil)) or the
// root is reached (returns (false, nil)). Any error from c.Subdivisions is
// surfaced verbatim as (false, err).
//
// Cost note: this is the ONE Phase 3 method that issues hidden I/O. Repeated
// calls in a hot loop incur a /Subdivisions round-trip per call. Phase 4's
// cache transport will memoize /Subdivisions per (baseURL, countryIsoCode);
// callers that need cheap repeated lookups on Phase 3 should perform the
// fetch once via c.Subdivisions and build their own parent-index helper.
//
// Cycle defense: the upward walk is bounded by len(parentIdx)+1 iterations.
// A cyclic parent-index (malformed upstream data where two entries each
// claim the other as a child) yields a bounded walk and returns
// (false, nil) instead of looping forever — per RESEARCH.md Pitfall 4 and
// ASVS V5.1.4.
//
// Concurrent use: this method is safe to call from any goroutine because
// *Client is immutable after NewClient and the inner c.Subdivisions call is
// itself concurrency-safe (CLIENT-07).
func (c *Client) IsInRegion(ctx context.Context, h Holiday, code string) (bool, error) {
	if h.Nationwide {
		return true, nil
	}
	if code == "" {
		return false, nil
	}
	// Fast path: flat match on Holiday.Subdivisions directly.
	for _, s := range h.Subdivisions {
		if strings.EqualFold(s.Code, code) {
			return true, nil
		}
	}
	// No flat match. The hierarchical path needs a country to fetch the
	// tree for. When h.Subdivisions is empty, there is no country context
	// to derive — return (false, nil) without HTTP.
	if len(h.Subdivisions) == 0 {
		return false, nil
	}
	countryCode, ok := splitCountryFromSubdivision(h.Subdivisions[0].Code)
	if !ok {
		return false, nil
	}
	tree, err := c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: countryCode})
	if err != nil {
		return false, err
	}
	parentIdx := buildParentIndex(tree)
	// Walk upward from `code` until a match against h.Subdivisions or the
	// root is reached. Defensive cycle bound: stop after len(parentIdx)+1
	// iterations — cycles cannot exceed the index size, so this terminates
	// even on malformed upstream data (Pitfall 4).
	current := code
	for i := 0; i <= len(parentIdx); i++ {
		for _, s := range h.Subdivisions {
			if strings.EqualFold(s.Code, current) {
				return true, nil
			}
		}
		parent, found := parentIdx[strings.ToUpper(current)]
		if !found {
			return false, nil // reached root or unknown code
		}
		current = parent
	}
	return false, nil // cycle defense — should never trigger in practice
}

// audit:ok 2026-05-30

// splitCountryFromSubdivision extracts the country prefix from a subdivision
// code by splitting on the first '-' byte. Returns ("PL", true) for "PL-SL",
// ("DE", true) for "DE-BY", ("DE", true) for "DE-BY-AU" (only the first
// segment is taken). Returns ("", false) when the input has no hyphen or the
// hyphen is at position 0.
//
// The function uses [strings.IndexByte] rather than [strings.Split] because only
// the first segment is needed and IndexByte allocates nothing.
func splitCountryFromSubdivision(code string) (string, bool) {
	if i := strings.IndexByte(code, '-'); i > 0 {
		return code[:i], true
	}
	return "", false
}

// audit:ok 2026-05-30

// buildParentIndex walks the recursive Subdivision tree depth-first and
// returns a child→parent map keyed by the uppercase Subdivision.Code.
// Top-level subdivisions (the entries at the root of the input slice) are
// NOT present in the map — they have no parent. Nested entries (children of
// children) are correctly indexed at each depth.
//
// Cycle note: this function does NOT bound its own recursion — it walks
// Subdivision.Children verbatim. A truly cyclic Children pointer at the JSON
// level would loop forever here. In practice the upstream OpenHolidays data
// is curated and never carries cyclic Children. The realistic threat surface
// is parent-index-level cycles (data shapes where, after parent-indexing, A
// claims B as parent and B claims A as parent); that case is defended by
// Client.IsInRegion's upward-walk iteration cap (len(parentIdx)+1) — see the
// godoc on Client.IsInRegion and RESEARCH.md Pitfall 4.
func buildParentIndex(tree []Subdivision) map[string]string {
	idx := make(map[string]string)
	var walk func(parent string, nodes []Subdivision)
	walk = func(parent string, nodes []Subdivision) {
		for _, n := range nodes {
			if parent != "" {
				idx[strings.ToUpper(n.Code)] = parent
			}
			if len(n.Children) > 0 {
				walk(n.Code, n.Children)
			}
		}
	}
	walk("", tree)
	return idx
}
