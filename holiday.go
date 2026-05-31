// pure-value helper methods on Holiday.
//
// This file declares four side-effect-free methods on Holiday that operate
// only on the value the caller already holds: NameFor, IsInRegion, Days,
// Range. None of them issue I/O, depend on *Client, or mutate the receiver.
// They delegate to existing Phase 1 primitives — pickLocalized (types.go),
// Date.DaysUntil (date.go), NewDate (date.go), and strings.EqualFold — to
// keep behavior consistent with the rest of the type surface.
//
// Naming conventions:
//
//   - NameFor (not Name) — Holiday already exposes a Name []LocalizedText
//     field; a method named Name(lang) would collide. Same CL-05 rationale
//     as Country.NameFor, recorded for Holiday as CL-10.
//   - Range returns iter.Seq[Date] (not iter.Seq[time.Time]) — a deliberate
//     deviation from ROADMAP success criterion #4's literal text so the
//     iterator composes directly with Date math helpers (Equal/Before/After,
//     DaysUntil) without conversion churn. Recorded as CL-11.

package openholidays

import (
	"iter"
	"strings"
)

// NameFor returns the localized holiday name for the given ISO 639-1
// language code and reports whether a matching entry was found. Language
// matching is case-insensitive ([strings.EqualFold]) so "PL" matches a "pl"
// entry. When the requested language is absent, NameFor returns ("", false)
// — it does NOT fall back to another entry, so a false ok unambiguously means
// "not localized in lang" (callers wanting a fallback choose one explicitly).
//
// The accessor is named NameFor (not Name) because Holiday already has a
// Name field of type []LocalizedText — a method named Name(lang) would
// collide with the field. The same shape is used by Country.NameFor,
// Language.NameFor, and Subdivision.NameFor (CL-05 / CL-10).
func (h Holiday) NameFor(lang string) (string, bool) {
	return pickLocalized(h.Name, lang)
}

// audit:ok 2026-05-30

// IsInRegion reports whether the holiday h applies to the administrative
// subdivision identified by code. The match is flat and side-effect-free:
//
//  1. A nationwide holiday returns true for any code (including the empty
//     string) — a nationwide holiday applies everywhere by definition, so
//     "applies in <code>" is true regardless of <code> (WR-06).
//  2. An empty code on a non-nationwide holiday returns false (defensive —
//     no panic, no false positive on hand-built empty input).
//  3. Otherwise, IsInRegion iterates Holiday.Subdivisions and returns true
//     on the first [strings.EqualFold](s.Code, code) match.
//  4. Returns false otherwise.
//
// IsInRegion does not recurse into a subdivision tree — Holiday only carries
// a flat []SubdivisionRef and the upstream-returned subdivisions are
// already top-level matches for the holiday. Callers that need to ask
// whether a child subdivision (e.g. "PL-SL-KAT" under "PL-SL") is covered
// by a holiday that applies to its parent should use Client.IsInRegion
// instead — that method fetches /Subdivisions and walks Subdivision.Children
// to answer hierarchical questions (CL-09).
func (h Holiday) IsInRegion(code string) bool {
	if h.Nationwide {
		return true
	}
	if code == "" {
		return false
	}
	for _, s := range h.Subdivisions {
		if strings.EqualFold(s.Code, code) {
			return true
		}
	}
	return false
}

// Days returns the inclusive count of calendar days the holiday spans.
//
// For a single-day holiday (StartDate == EndDate), Days returns 1. For a
// multi-day holiday, Days returns the inclusive count from StartDate to
// EndDate — for example, the Polish ferie zimowe Śląskie 2025 span
// (2025-01-18 to 2025-01-31) returns 14.
//
// When EndDate is strictly before StartDate — a malformed Holiday the
// endpoint-layer validateHolidays would have rejected but a hand-built
// Holiday can carry — Days returns 0 (defensive clamp, WR-03). Callers
// branching on h.Days() > N therefore get a defined, non-negative value
// for every Holiday they can hold.
//
// The implementation adds 1 to Date.DaysUntil (the exclusive day delta) to
// convert it to an inclusive span count. DaysUntil operates on UTC-midnight
// operands and is therefore calendar-correct across DST boundaries (Phase 1
// D-10 / Pitfall TZ-2).
func (h Holiday) Days() int {
	if h.EndDate.Before(h.StartDate) {
		return 0
	}
	return h.StartDate.DaysUntil(h.EndDate) + 1
}

// audit:ok 2026-05-30

// Range returns an iterator that yields every Date from StartDate to
// EndDate inclusive. For a single-day holiday (StartDate == EndDate), the
// iterator yields exactly one Date. For a multi-day holiday, it yields
// each calendar day in chronological order.
//
// The iterator is single-use per Go 1.23 range-over-func semantics: the
// yield function returns false when the consumer breaks out of the range
// loop, and the body must not call yield again after that. The returned
// closure honors this contract by returning immediately on a false yield.
//
// Every yielded Date is rebuilt via NewDate(year, month, day), so each
// yielded value is at UTC midnight regardless of the receiver's internal
// [time.Time] location. Iteration advances via [time.Time].AddDate(0, 0, 1),
// which is calendar-correct across DST boundaries because the operands
// are UTC-midnight (Phase 1 D-10 / Pitfall TZ-2 / Pitfall 3).
//
// When EndDate is strictly before StartDate, Range yields nothing. Such
// Holiday values are rejected by the endpoint-layer validateHolidays pass
// before they reach the caller for endpoint-returned Holidays, but the
// defensive guard exists so hand-built Holidays do not panic.
//
// The yielded element type is Date, not [time.Time] — a deliberate
// deviation from ROADMAP success criterion #4's literal [iter.Seq][time.Time]
// so the iterator composes directly with Date math helpers (Equal/Before/
// After/Compare/DaysUntil) without conversion churn. Callers that want
// a [time.Time] use the embedded field: `for d := range h.Range() { t := d.Time }`.
// Recorded as CL-11.
func (h Holiday) Range() iter.Seq[Date] {
	return func(yield func(Date) bool) {
		if h.EndDate.Before(h.StartDate) {
			return
		}
		d := NewDate(h.StartDate.Year(), h.StartDate.Month(), h.StartDate.Day())
		for {
			if !yield(d) {
				return
			}
			if !d.Before(h.EndDate) {
				return
			}
			next := d.AddDate(0, 0, 1)
			d = NewDate(next.Year(), next.Month(), next.Day())
		}
	}
}
