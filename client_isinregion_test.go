// Package openholidays — tests for the hierarchical Client.IsInRegion helper
// and its two unexported support functions.
//
// Gold Rule 3 application: exactly one TestClient_IsInRegion (the exported
// method under test) plus one TestSplitCountryFromSubdivision and one
// TestBuildParentIndex (the unexported helpers have non-trivial logic and
// warrant their own TestXxx). Every case lives in a t.Run subtest; require
// for preconditions, assert for verifications.
//
// CL-09 / D-59 are exercised here. Assumption A3 (DE fixture is the
// live-grounded hierarchical test target because PL is flat) is honored:
// the hierarchical happy path replays testdata/subdivisions_de.json and
// uses the DE-BY/Augsburg pair extracted at runtime from the fixture's
// Children slice, so the test stays resilient to small fixture refreshes.
//
// Cycle-defense regression: the threat-model row T-3-DoS-CycleInChildren is
// regression-locked by the subtest named
// "cycle in upstream tree is bounded by IsInRegion iteration cap (enforcement)".
// Removing the len(parentIdx)+1 cap in client_isinregion.go FAILS that
// subtest via its 2-second timeout. Non-English strings ("Augsburg",
// "Bayern") in the fixture mirror real upstream OpenHolidays responses and
// are admitted per CONVENTIONS.md Rule 1 testdata-fixture exception.

package openholidays

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findFirstWithChildren walks the parsed DE fixture tree and returns the
// first (parent, child) Code pair where parent carries a non-empty Children
// slice and Children[0] is the child. The test uses these literal codes in
// the hierarchical assertions — the test stays resilient to small changes
// in the DE fixture as long as some entry continues to carry Children.
func findFirstWithChildren(t *testing.T, tree []Subdivision) (string, string) {
	t.Helper()
	for _, s := range tree {
		if len(s.Children) > 0 {
			return s.Code, s.Children[0].Code
		}
	}
	t.Fatalf("DE fixture must contain at least one Subdivision with non-empty Children — fixture refresh required (Assumption A3 violated)")
	return "", ""
}

// TestClient_IsInRegion exercises Client.IsInRegion (D-59 / CL-09):
//
//   - empty code → (false, nil) no HTTP
//   - h.Nationwide → (true, nil) no HTTP
//   - flat strings.EqualFold match → (true, nil) no HTTP (case-insensitive)
//   - no flat match + empty Subdivisions → (false, nil) no HTTP
//   - hierarchical descendant match against DE fixture's DE-BY/Augsburg pair
//   - hierarchical miss on an unknown code → (false, nil)
//   - transport error from inner c.Subdivisions surfaces verbatim
//   - cycle in upstream parent-index is bounded by len(parentIdx)+1
//     (enforcement subtest; the cap regression is detected by the 2 s timeout)
//
// Gold Rule 3: exactly one TestClient_IsInRegion; every case is a t.Run.
func TestClient_IsInRegion(t *testing.T) {
	t.Parallel()

	t.Run("empty code returns (false, nil) without HTTP", func(t *testing.T) {
		t.Parallel()
		// http://example.invalid is RFC 6761 reserved; an accidental HTTP
		// dispatch would fail loudly with a DNS error.
		c := NewClient(WithBaseURL("http://example.invalid"))
		h := Holiday{Subdivisions: []SubdivisionRef{{Code: "PL-SL"}}}
		ok, err := c.IsInRegion(context.Background(), h, "")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("Nationwide returns (true, nil) without HTTP", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		h := Holiday{Nationwide: true}
		ok, err := c.IsInRegion(context.Background(), h, "PL-SL")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("flat match on h.Subdivisions returns (true, nil) without HTTP", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		h := Holiday{Subdivisions: []SubdivisionRef{{Code: "PL-SL"}}}
		ok, err := c.IsInRegion(context.Background(), h, "PL-SL")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("flat match is case-insensitive (no HTTP)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		h := Holiday{Subdivisions: []SubdivisionRef{{Code: "PL-SL"}}}
		ok, err := c.IsInRegion(context.Background(), h, "pl-sl")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("no flat match and empty Subdivisions returns (false, nil) without HTTP", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		h := Holiday{Nationwide: false, Subdivisions: nil}
		ok, err := c.IsInRegion(context.Background(), h, "PL-SL")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("hierarchical match against DE fixture", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "subdivisions_de.json"))
		require.NoError(t, err, "DE fixture missing — Plan 03-03 captures testdata/subdivisions_de.json (Assumption A3)")

		// Parse to discover the parent/child code pair at runtime so the test
		// stays resilient to small fixture refreshes.
		var tree []Subdivision
		require.NoError(t, json.Unmarshal(body, &tree))
		parentCode, childCode := findFirstWithChildren(t, tree)
		t.Logf("hierarchical test target: parent=%q child=%q", parentCode, childCode)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/Subdivisions", r.URL.Path)
			assert.Equal(t, "DE", r.URL.Query().Get("countryIsoCode"),
				"country prefix must be derived from h.Subdivisions[0].Code")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		// Holiday declares membership in the parent (e.g. DE-BY); query the
		// child code (e.g. DE-BY-AU). The upward walk must climb from
		// childCode through the parent-index and hit parentCode.
		h := Holiday{Subdivisions: []SubdivisionRef{{Code: parentCode}}}
		ok, err := c.IsInRegion(context.Background(), h, childCode)
		require.NoError(t, err)
		assert.True(t, ok,
			"Client.IsInRegion must walk DE-BY's Children and return true for %q under parent %q",
			childCode, parentCode)
	})

	t.Run("hierarchical miss returns (false, nil)", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "subdivisions_de.json"))
		require.NoError(t, err, "DE fixture missing — Plan 03-03 captures testdata/subdivisions_de.json")

		var tree []Subdivision
		require.NoError(t, json.Unmarshal(body, &tree))
		parentCode, _ := findFirstWithChildren(t, tree)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/Subdivisions", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		h := Holiday{Subdivisions: []SubdivisionRef{{Code: parentCode}}}
		ok, err := c.IsInRegion(context.Background(), h, "ZZ-XX-NEVER")
		require.NoError(t, err)
		assert.False(t, ok,
			"a code that does not exist in the parent-index must return (false, nil)")
	})

	t.Run("transport error from Subdivisions surfaces verbatim", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"title": "Internal Server Error"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		h := Holiday{Subdivisions: []SubdivisionRef{{Code: "DE-BY"}}}
		ok, err := c.IsInRegion(context.Background(), h, "DE-BY-AU")
		require.Error(t, err)
		assert.False(t, ok)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr),
			"expected *APIError via errors.As, got %T: %v", err, err)
		assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
		// WR-04: lock the title-fallback parseAPIMessage path symmetric
		// with sibling endpoint 5xx tests (countries_test.go,
		// languages_test.go, public_holidays_test.go, school_holidays_test.go,
		// subdivisions_test.go). Without these assertions an
		// IsInRegion-mediated regression that lost title-fallback or
		// rewrote the inner path would slip past this subtest alone.
		assert.Equal(t, "/Subdivisions", apiErr.Path,
			"Path must be /Subdivisions (the inner Subdivisions call) — IsInRegion does not rewrite the path")
		assert.Equal(t, "Internal Server Error", apiErr.Message,
			"title must win when detail is absent (parseAPIMessage fallback)")
	})

	t.Run("cycle in upstream tree is bounded by IsInRegion iteration cap (enforcement)", func(t *testing.T) {
		t.Parallel()
		// Synthetic cyclic tree: two top-level subdivisions where each
		// declares the other as its Child. buildParentIndex will produce
		// parentIdx["DE-A"]="DE-B" and parentIdx["DE-B"]="DE-A" — a cycle
		// at the parent-index level. Without the len(parentIdx)+1 cap in
		// IsInRegion's upward walk, the call would loop forever; with the
		// cap it terminates after 3 iterations and returns (false, nil).
		cyclic := []Subdivision{
			{Code: "DE-A", Children: []Subdivision{{Code: "DE-B"}}},
			{Code: "DE-B", Children: []Subdivision{{Code: "DE-A"}}},
		}
		body, err := json.Marshal(cyclic)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/Subdivisions", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		// Holiday declares membership in a code that does NOT appear in the
		// cyclic tree (so the upward walk has no flat match and must rely
		// purely on the iteration cap to terminate).
		h := Holiday{
			Nationwide:   false,
			Subdivisions: []SubdivisionRef{{Code: "DE-X"}},
		}

		type result struct {
			ok  bool
			err error
		}
		done := make(chan result, 1)
		go func() {
			ok, err := c.IsInRegion(context.Background(), h, "DE-A")
			done <- result{ok, err}
		}()

		select {
		case r := <-done:
			require.NoError(t, r.err)
			assert.False(t, r.ok,
				"cycle must terminate via len(parentIdx)+1 cap and return (false, nil)")
		case <-time.After(2 * time.Second):
			t.Fatalf("IsInRegion failed to bound cycle — exceeded 2s (regression: len(parentIdx)+1 cap may have been removed from client_isinregion.go)")
		}
	})

	t.Run("5-node cycle terminates via len(parentIdx)+1 cap (WR-01 regression)", func(t *testing.T) {
		t.Parallel()
		// Deeper cycle than the 2-node test above: A → B → C → D → E → A.
		// buildParentIndex produces 5 entries (DE-A..DE-E); the upward-walk
		// cap is len(parentIdx)+1 = 6 iterations. The cycle's natural length
		// is 5 — the cap fires on the 6th iteration. A regression that
		// tightened the cap to e.g. len(parentIdx)-1 (= 4) would terminate
		// without traversing the full cycle, yet still return (false, nil)
		// on the 2-node test — so this subtest is necessary to lock the
		// "cap upper-bound is meaningfully exercised" contract from WR-01.
		cyclic := []Subdivision{
			{Code: "DE-A", Children: []Subdivision{
				{Code: "DE-B", Children: []Subdivision{
					{Code: "DE-C", Children: []Subdivision{
						{Code: "DE-D", Children: []Subdivision{
							{Code: "DE-E", Children: []Subdivision{{Code: "DE-A"}}},
						}},
					}},
				}},
			}},
		}
		body, err := json.Marshal(cyclic)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/Subdivisions", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		// Holiday declares membership in a code that does NOT appear in the
		// cyclic tree, so the upward walk must rely purely on the cap to
		// terminate.
		h := Holiday{
			Nationwide:   false,
			Subdivisions: []SubdivisionRef{{Code: "DE-X"}},
		}

		type result struct {
			ok  bool
			err error
		}
		done := make(chan result, 1)
		go func() {
			ok, err := c.IsInRegion(context.Background(), h, "DE-A")
			done <- result{ok, err}
		}()

		select {
		case r := <-done:
			require.NoError(t, r.err)
			assert.False(t, r.ok,
				"deeper cycle must terminate via len(parentIdx)+1 cap and return (false, nil)")
		case <-time.After(2 * time.Second):
			t.Fatalf("IsInRegion failed to bound 5-node cycle — exceeded 2s (regression: len(parentIdx)+1 cap may have been tightened below cycle length)")
		}
	})
}

// TestSplitCountryFromSubdivision covers the unexported helper that derives
// the country prefix from a subdivision code (D-59 — input to IsInRegion's
// hierarchical fetch).
func TestSplitCountryFromSubdivision(t *testing.T) {
	t.Parallel()

	t.Run("PL-SL extracts PL", func(t *testing.T) {
		t.Parallel()
		got, ok := splitCountryFromSubdivision("PL-SL")
		assert.True(t, ok)
		assert.Equal(t, "PL", got)
	})

	t.Run("DE-BY extracts DE", func(t *testing.T) {
		t.Parallel()
		got, ok := splitCountryFromSubdivision("DE-BY")
		assert.True(t, ok)
		assert.Equal(t, "DE", got)
	})

	t.Run("DE-BY-AU extracts only the first segment DE", func(t *testing.T) {
		t.Parallel()
		got, ok := splitCountryFromSubdivision("DE-BY-AU")
		assert.True(t, ok)
		assert.Equal(t, "DE", got, "only the first '-' splits; trailing segments are ignored")
	})

	t.Run("no hyphen returns false", func(t *testing.T) {
		t.Parallel()
		got, ok := splitCountryFromSubdivision("PL")
		assert.False(t, ok)
		assert.Equal(t, "", got)
	})

	t.Run("leading hyphen returns false", func(t *testing.T) {
		t.Parallel()
		got, ok := splitCountryFromSubdivision("-SL")
		assert.False(t, ok, "hyphen at position 0 is not a country prefix")
		assert.Equal(t, "", got)
	})
}

// TestBuildParentIndex covers the unexported helper that walks
// Subdivision.Children depth-first into a child→parent map keyed by
// uppercase code. No cycle subtest here — buildParentIndex itself does not
// enforce a cap; the cycle enforcement regression lives in
// TestClient_IsInRegion (which exercises the IsInRegion upward-walk cap,
// the actual mitigation surface for T-3-DoS-CycleInChildren).
func TestBuildParentIndex(t *testing.T) {
	t.Parallel()

	t.Run("flat tree produces empty map", func(t *testing.T) {
		t.Parallel()
		flat := []Subdivision{
			{Code: "PL-SL"},
			{Code: "PL-DS"},
			{Code: "PL-WP"},
		}
		idx := buildParentIndex(flat)
		assert.Empty(t, idx, "top-level subdivisions have no parent — map must be empty")
	})

	t.Run("single-level nesting indexes children to parents", func(t *testing.T) {
		t.Parallel()
		tree := []Subdivision{
			{Code: "DE-BY", Children: []Subdivision{{Code: "DE-BY-AU"}}},
			{Code: "DE-NI"},
		}
		idx := buildParentIndex(tree)
		require.Len(t, idx, 1)
		assert.Equal(t, "DE-BY", idx["DE-BY-AU"],
			"child code must be keyed uppercase and map to its parent")
	})

	t.Run("two-level nesting indexes deeper children", func(t *testing.T) {
		t.Parallel()
		tree := []Subdivision{
			{
				Code: "DE-BY",
				Children: []Subdivision{
					{
						Code:     "DE-BY-AU",
						Children: []Subdivision{{Code: "DE-BY-AU-X"}},
					},
				},
			},
		}
		idx := buildParentIndex(tree)
		require.Len(t, idx, 2)
		assert.Equal(t, "DE-BY", idx["DE-BY-AU"])
		assert.Equal(t, "DE-BY-AU", idx["DE-BY-AU-X"])
	})

	t.Run("keys are normalized to uppercase", func(t *testing.T) {
		t.Parallel()
		tree := []Subdivision{
			{Code: "de-by", Children: []Subdivision{{Code: "de-by-au"}}},
		}
		idx := buildParentIndex(tree)
		require.Len(t, idx, 1)
		assert.Equal(t, "de-by", idx["DE-BY-AU"],
			"buildParentIndex must uppercase the child key so IsInRegion's strings.ToUpper(current) lookup hits")
	})
}
