// Package openholidays — Date type for calendar dates (no timezone).
//
// This file implements the custom Date wrapper that every Holiday.StartDate
// and Holiday.EndDate field will use. Date is the foundational type for all
// subsequent Phase 1 work beyond errors.

package openholidays

import (
	"bytes"
	"fmt"
	"time"
)

// dateLayout is the wire format the upstream OpenHolidays API uses for every
// date field. It matches the Go reference time formatting layout for ISO 8601
// year-month-day strings.
const dateLayout = "2006-01-02"

// errEmptyDate signals an empty or null date payload during JSON decode.
// It is intentionally unexported so the public sentinel surface remains at
// the locked size (D-06): external callers cannot pivot on this identity.
// Use errors.Is internally only.
var errEmptyDate = fmt.Errorf("openholidays: empty date string")

// Date is a calendar date (no timezone) returned by the OpenHolidays API.
//
// Internally, Date wraps a time.Time normalized to UTC midnight so the
// embedded time.Time methods (Year, Month, Day, Format, IsZero, ...) work
// naturally without timezone surprises. Construct a Date via NewDate or
// ParseDate, or decode one from a JSON "YYYY-MM-DD" string via the standard
// encoding/json package.
//
// The zero Date{} represents January 1 of year 1 (matching the time.Time
// zero), and round-trips to the JSON literal "0001-01-01". Use IsZero to
// distinguish a populated Date from an absent one.
type Date struct {
	time.Time
}

// NewDate constructs a Date at UTC midnight on the given calendar year,
// month, and day. The returned Date.Location() is always time.UTC and the
// time-of-day fields (Hour, Minute, Second, Nanosecond) are all zero.
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate parses a YYYY-MM-DD string and returns the corresponding
// UTC-midnight Date.
//
// An empty string returns an error wrapping the internal empty-date sentinel.
// Malformed input returns a wrapped time.Parse error containing the offending
// value in quoted form for diagnostics.
func ParseDate(s string) (Date, error) {
	if s == "" {
		return Date{}, errEmptyDate
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("openholidays: invalid date %q: %w", s, err)
	}
	return Date{t}, nil
}

// MarshalJSON emits the Date as a JSON string in YYYY-MM-DD form.
//
// The zero Date{} round-trips to "0001-01-01" — symmetric with time.Time's
// MarshalJSON semantics. Callers detect missing dates via Date.IsZero, not
// by checking against the marshaled string.
func (d Date) MarshalJSON() ([]byte, error) {
	// 12 bytes: 2 quotes + 10-character date.
	buf := make([]byte, 0, 12)
	buf = append(buf, '"')
	buf = d.AppendFormat(buf, dateLayout)
	buf = append(buf, '"')
	return buf, nil
}

// UnmarshalJSON parses YYYY-MM-DD JSON strings into the Date.
//
// Both the JSON literal null and the empty JSON string "" are rejected with
// an error wrapping the internal empty-date sentinel — silent zero values
// are not produced. Non-string JSON tokens (numbers, booleans, objects, ...)
// return a "must be a JSON string" error with the offending bytes echoed
// for diagnostics. Malformed date strings return a wrapped time.Parse error.
//
// On success, the receiver is replaced with a UTC-midnight Date.
func (d *Date) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return fmt.Errorf("openholidays: null is not a valid date: %w", errEmptyDate)
	}
	if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
		return fmt.Errorf("openholidays: date must be a JSON string, got %s", b)
	}
	s := string(b[1 : len(b)-1])
	if s == "" {
		return fmt.Errorf("openholidays: %w", errEmptyDate)
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("openholidays: invalid date %q: %w", s, err)
	}
	*d = Date{t}
	return nil
}

// String returns the Date in YYYY-MM-DD form.
//
// This shadows the embedded time.Time.String() method to avoid the noisy
// "0001-01-01 00:00:00 +0000 UTC" format that time.Time produces by default;
// the YYYY-MM-DD shape matches the JSON wire format and is friendlier in
// CLI table output.
func (d Date) String() string {
	return d.Format(dateLayout)
}

// Equal reports whether two Dates represent the same calendar day.
//
// Both operands are defensively normalized to UTC midnight before comparison
// so a Date constructed outside NewDate/ParseDate (for example via a struct
// literal with a non-UTC time.Time) still compares calendar-correctly.
func (d Date) Equal(other Date) bool {
	return d.toUTCMidnight().Equal(other.toUTCMidnight())
}

// Before reports whether d is strictly before other in calendar order.
// Both operands are normalized to UTC midnight before comparison.
func (d Date) Before(other Date) bool {
	return d.toUTCMidnight().Before(other.toUTCMidnight())
}

// After reports whether d is strictly after other in calendar order.
// Both operands are normalized to UTC midnight before comparison.
func (d Date) After(other Date) bool {
	return d.toUTCMidnight().After(other.toUTCMidnight())
}

// Compare returns -1 if d is before other, 0 if equal, +1 if after.
// Both operands are normalized to UTC midnight before comparison.
func (d Date) Compare(other Date) int {
	return d.toUTCMidnight().Compare(other.toUTCMidnight())
}

// DaysUntil returns the inclusive day count from d to other.
//
// For d == other (same calendar day), DaysUntil returns 1. For other one
// day after d, DaysUntil returns 2. For d strictly after other (a negative
// span), DaysUntil returns a negative integer whose magnitude is the
// inclusive count.
//
// The implementation operates on UTC-midnight operands so the result is
// calendar-correct across DST boundaries (DST cannot perturb a difference
// of UTC-midnight times because both are at 00:00 UTC).
func (d Date) DaysUntil(other Date) int {
	a := d.toUTCMidnight()
	b := other.toUTCMidnight()
	// Both operands are UTC midnight, so Sub returns a clean multiple of 24h
	// — no fractional hours from DST transitions.
	days := int(b.Sub(a).Hours() / 24)
	if days >= 0 {
		return days + 1
	}
	return days - 1
}

// toUTCMidnight is the canonical normalization used by every comparison
// method on Date. It rebuilds the time.Time at UTC midnight using only the
// Year/Month/Day fields of the receiver, defensively erasing any timezone
// or time-of-day component that external code might have introduced via a
// Date{} struct literal.
func (d Date) toUTCMidnight() time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}
