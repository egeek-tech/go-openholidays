// Package openholidays — client-side input validators.
//
// This file ships three unexported validator functions that Phase 2 endpoint
// methods call BEFORE making any HTTP request. Their job is to reject
// malformed input client-side (ASVS V5.1.3 input-validation control) and
// produce errors that callers can branch on via errors.Is against the
// sentinel surface defined in errors.go.

package openholidays

import (
	"fmt"
	"strings"
)

// validateCountry canonicalizes a country ISO 3166-1 alpha-2 code to uppercase
// and verifies it is exactly 2 ASCII letters in [A-Z]. Returns the canonical
// (uppercase) form, which is what the OpenHolidays API expects on the wire.
//
// Accepts any input case ("PL", "pl", "Pl" all map to "PL") per D-20 / CL-02.
// Returns an empty string and an error wrapping ErrInvalidCountry when the
// input is empty, contains non-ASCII bytes, is the wrong length, contains
// digits, or carries surrounding whitespace.
//
// Error messages quote the ORIGINAL (non-canonicalized) input via %q so
// callers can see exactly what they passed. Country codes are not secrets;
// quoting them is safe per ERR-04.
func validateCountry(code string) (string, error) {
	canon := strings.ToUpper(code)
	if !isTwoASCIIUppers(canon) {
		return "", fmt.Errorf("%w: %q", ErrInvalidCountry, code)
	}
	return canon, nil
}

// validateLanguage canonicalizes a language ISO 639-1 alpha-2 code to lowercase
// and verifies it is exactly 2 ASCII letters in [a-z]. Returns the canonical
// (lowercase) form.
//
// This is a SHAPE-ONLY check — no allowlist is enforced. The set of languages
// the OpenHolidays API supports is the authoritative source; the /Languages
// endpoint is the runtime way to enumerate them. Validating against a
// hard-coded list here would silently drift from upstream and produce
// false-negative rejections.
//
// Accepts any input case ("pl", "PL", "Pl" all map to "pl") per D-21.
// Returns an empty string and an error wrapping ErrInvalidLanguage on
// malformed input.
func validateLanguage(code string) (string, error) {
	canon := strings.ToLower(code)
	if !isTwoASCIILowers(canon) {
		return "", fmt.Errorf("%w: %q", ErrInvalidLanguage, code)
	}
	return canon, nil
}

// validateDateRange enforces two invariants on a [from, to] date window
// passed to a holiday-listing endpoint:
//
//   - from <= to (else ErrInvalidDateRange, D-22 / VALID-02).
//   - to is within 3 calendar years inclusive of from (else
//     ErrDateRangeTooLarge, D-22 / VALID-03). "Within 3 calendar years
//     inclusive" means to is no later than from advanced by 3 years;
//     equivalently, to lies strictly before from advanced by 3 years and
//     1 day. The implementation uses Date.AddDate(3, 0, 1) (promoted from
//     the embedded time.Time), which is calendar-aware and handles
//     leap-year boundaries correctly (for example, 2024-02-29 advanced by
//     3 years and 1 day yields 2027-03-01). Naive duration arithmetic
//     would slip by a day. See Pitfall TZ-2 for the DST/leap-year
//     rationale.
//
// Equal dates (from == to) are accepted: a single-day window is a valid query.
//
// Error messages include the offending from/to dates via the Date.String()
// YYYY-MM-DD form. Dates are not secrets per ERR-04.
func validateDateRange(from, to Date) error {
	if from.After(to) {
		return fmt.Errorf("%w: from=%s to=%s", ErrInvalidDateRange, from, to)
	}
	// "Within 3 calendar years inclusive" means to <= from + 3 years.
	// Equivalent: to is strictly before (from + 3 years + 1 day). The
	// AddDate(3, 0, 1) call uses calendar arithmetic (not duration math)
	// so leap-year boundaries are handled correctly.
	limit := Date{from.AddDate(3, 0, 1)}
	if !to.Before(limit) {
		return fmt.Errorf("%w: from=%s to=%s spans more than 3 years", ErrDateRangeTooLarge, from, to)
	}
	return nil
}

// isTwoASCIIUppers reports whether s is exactly 2 bytes and each byte is in
// [A-Z]. Byte arithmetic (rather than unicode.IsUpper) is intentional: we want
// ASCII-only matching. unicode.IsLetter('Ö') is true, but 'Ö' is not a valid
// ISO 3166-1 alpha-2 character; the byte-range check rejects it cleanly.
func isTwoASCIIUppers(s string) bool {
	if len(s) != 2 {
		return false
	}
	return s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}

// isTwoASCIILowers reports whether s is exactly 2 bytes and each byte is in
// [a-z]. See isTwoASCIIUppers for the byte-arithmetic rationale.
func isTwoASCIILowers(s string) bool {
	if len(s) != 2 {
		return false
	}
	return s[0] >= 'a' && s[0] <= 'z' && s[1] >= 'a' && s[1] <= 'z'
}
