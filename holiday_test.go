// Package openholidays — tests for the Holiday helpers in holiday.go.
//
// Gold Rule 3 application: exactly four TestXxx functions for the four
// exported production methods on Holiday — NameFor, IsInRegion, Days,
// Range. Every test case lives in a t.Run subtest; require for
// preconditions, assert for verifications.
//
// CL-10 (Holiday.NameFor collision-avoiding name) and CL-11
// (Holiday.Range yields iter.Seq[Date] rather than iter.Seq[time.Time])
// are exercised here. Non-English strings ("Wigilia") mirror real
// upstream OpenHolidays responses and are admitted per CONVENTIONS.md
// Rule 1 testdata-fixture exception.

package openholidays

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHoliday_NameFor exercises Holiday.NameFor — case-insensitive match,
// first-entry fallback on miss, empty string on empty Name slice.
func TestHoliday_NameFor(t *testing.T) {
	t.Parallel()

	t.Run("matches Polish entry case-insensitively", func(t *testing.T) {
		t.Parallel()
		h := Holiday{Name: []LocalizedText{
			{Language: "pl", Text: "Wigilia"},
			{Language: "en", Text: "Christmas Eve"},
		}}
		assert.Equal(t, "Wigilia", h.NameFor("pl"))
		assert.Equal(t, "Wigilia", h.NameFor("PL"))
	})

	t.Run("falls back to first entry on miss", func(t *testing.T) {
		t.Parallel()
		h := Holiday{Name: []LocalizedText{
			{Language: "pl", Text: "Wigilia"},
			{Language: "en", Text: "Christmas Eve"},
		}}
		assert.Equal(t, "Wigilia", h.NameFor("xx"))
	})

	t.Run("returns empty on empty Name", func(t *testing.T) {
		t.Parallel()
		h := Holiday{}
		assert.Equal(t, "", h.NameFor("pl"))
	})
}

// TestHoliday_IsInRegion exercises Holiday.IsInRegion — empty-code defense,
// Nationwide short-circuit, case-insensitive Subdivisions[].Code match,
// and the negative cases (no match, no Subdivisions).
func TestHoliday_IsInRegion(t *testing.T) {
	t.Parallel()

	t.Run("empty code returns false even when Nationwide", func(t *testing.T) {
		t.Parallel()
		h := Holiday{Nationwide: true}
		assert.False(t, h.IsInRegion(""))
	})

	t.Run("Nationwide returns true for any non-empty code", func(t *testing.T) {
		t.Parallel()
		h := Holiday{Nationwide: true}
		assert.True(t, h.IsInRegion("PL-SL"))
	})

	t.Run("matches Code in Subdivisions case-insensitively", func(t *testing.T) {
		t.Parallel()
		h := Holiday{Subdivisions: []SubdivisionRef{
			{Code: "PL-SL"},
			{Code: "PL-DS"},
		}}
		assert.True(t, h.IsInRegion("pl-sl"))
		assert.True(t, h.IsInRegion("PL-SL"))
	})

	t.Run("returns false when code is not in Subdivisions and not Nationwide", func(t *testing.T) {
		t.Parallel()
		h := Holiday{Subdivisions: []SubdivisionRef{{Code: "PL-SL"}}}
		assert.False(t, h.IsInRegion("PL-DS"))
	})

	t.Run("returns false when Subdivisions is empty and not Nationwide", func(t *testing.T) {
		t.Parallel()
		h := Holiday{Nationwide: false, Subdivisions: nil}
		assert.False(t, h.IsInRegion("PL-SL"))
	})
}

// TestHoliday_Days exercises Holiday.Days — single-day, 14-day Polish
// ferie zimowe Śląskie span (the canonical D-70 sanity value), and a
// cross-year span. Delegates to Date.DaysUntil which is calendar-correct.
func TestHoliday_Days(t *testing.T) {
	t.Parallel()

	t.Run("single-day returns 1", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			StartDate: NewDate(2025, time.January, 1),
			EndDate:   NewDate(2025, time.January, 1),
		}
		assert.Equal(t, 1, h.Days())
	})

	t.Run("14-day ferie zimowe Śląskie returns 14", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			StartDate: NewDate(2025, time.January, 18),
			EndDate:   NewDate(2025, time.January, 31),
		}
		assert.Equal(t, 14, h.Days())
	})

	t.Run("cross-year span returns 2", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			StartDate: NewDate(2025, time.December, 31),
			EndDate:   NewDate(2026, time.January, 1),
		}
		assert.Equal(t, 2, h.Days())
	})
}

// TestHoliday_Range exercises Holiday.Range — the canonical 14-day Polish
// ferie zimowe Śląskie span, single-day yield, empty yield on malformed
// EndDate<StartDate, early-break consumer behavior, and the UTC-midnight
// invariant on every yielded Date (CL-11 + Pitfall 3).
func TestHoliday_Range(t *testing.T) {
	t.Parallel()

	t.Run("14-day ferie zimowe yields 14 Dates inclusive", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			StartDate: NewDate(2025, time.January, 18),
			EndDate:   NewDate(2025, time.January, 31),
		}
		var dates []Date
		for d := range h.Range() {
			dates = append(dates, d)
		}
		require.Len(t, dates, 14)
		assert.True(t, dates[0].Equal(NewDate(2025, time.January, 18)),
			"first yielded Date must be 2025-01-18, got %s", dates[0])
		assert.True(t, dates[13].Equal(NewDate(2025, time.January, 31)),
			"last yielded Date must be 2025-01-31, got %s", dates[13])
	})

	t.Run("single-day yields exactly one Date", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			StartDate: NewDate(2025, time.December, 25),
			EndDate:   NewDate(2025, time.December, 25),
		}
		var dates []Date
		for d := range h.Range() {
			dates = append(dates, d)
		}
		require.Len(t, dates, 1)
		assert.True(t, dates[0].Equal(NewDate(2025, time.December, 25)))
	})

	t.Run("EndDate before StartDate yields zero", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			StartDate: NewDate(2025, time.January, 31),
			EndDate:   NewDate(2025, time.January, 18),
		}
		var dates []Date
		for d := range h.Range() {
			dates = append(dates, d)
		}
		assert.Empty(t, dates)
	})

	t.Run("early break stops iteration", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			StartDate: NewDate(2025, time.January, 18),
			EndDate:   NewDate(2025, time.January, 31),
		}
		count := 0
		for range h.Range() {
			count++
			if count == 3 {
				break
			}
		}
		assert.Equal(t, 3, count)
	})

	t.Run("every yielded Date is UTC midnight", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			StartDate: NewDate(2025, time.January, 18),
			EndDate:   NewDate(2025, time.January, 31),
		}
		for d := range h.Range() {
			assert.Equal(t, time.UTC, d.Location(),
				"yielded Date %s has non-UTC location %s", d, d.Location())
			assert.Equal(t, 0, d.Hour())
			assert.Equal(t, 0, d.Minute())
			assert.Equal(t, 0, d.Second())
			assert.Equal(t, 0, d.Nanosecond())
		}
	})

	t.Run("non-UTC StartDate yields UTC-midnight first Date (WR-01 regression)", func(t *testing.T) {
		t.Parallel()
		// Hand-build a Holiday with a non-UTC StartDate. Endpoint-returned
		// Holidays would never look like this (validateHolidays canonicalizes
		// to UTC-midnight), but the godoc contract on Range() explicitly
		// promises every yield is normalized through NewDate. WR-01 was the
		// first-iteration drift between the contract and the code.
		cet := time.FixedZone("CET", 3600)
		h := Holiday{
			StartDate: Date{Time: time.Date(2025, time.January, 18, 0, 0, 0, 0, cet)},
			EndDate:   NewDate(2025, time.January, 18),
		}
		var dates []Date
		for d := range h.Range() {
			dates = append(dates, d)
		}
		require.Len(t, dates, 1, "single-day span must yield exactly one Date")
		first := dates[0]
		assert.Equal(t, time.UTC, first.Location(),
			"first yielded Date must be UTC-midnight regardless of StartDate location, got %s", first.Location())
		assert.Equal(t, 0, first.Hour())
		assert.Equal(t, 0, first.Minute())
		assert.Equal(t, 0, first.Second())
		assert.Equal(t, 0, first.Nanosecond())
		assert.Equal(t, 2025, first.Year())
		assert.Equal(t, time.January, first.Month())
		assert.Equal(t, 18, first.Day())
	})
}
