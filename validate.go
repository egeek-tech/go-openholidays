// client-side input validators.
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

// audit:ok 2026-05-30

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
	// ASCII-shape check on ORIGINAL bytes BEFORE any case canonicalization
	// (W-01 fix: closes the Unicode case-fold bypass where, e.g., "ıA"
	// — U+0131 dotless-i followed by 'A' — would survive ToUpper to
	// produce the 2-byte ASCII-uppercase string "IA" and pass the
	// over-permissive post-fold len-2 check.)
	if !isTwoASCIILetters(code) {
		return "", fmt.Errorf("%w: %q", ErrInvalidCountry, code)
	}
	return strings.ToUpper(code), nil
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
	// ASCII-shape check on ORIGINAL bytes BEFORE any case canonicalization
	// (W-01 fix: mirror of validateCountry; rejects U+212A Kelvin sign,
	// U+0130 Latin capital I with dot above, and similar fold-to-ASCII
	// characters BEFORE strings.ToLower canonicalizes them.)
	if !isTwoASCIILetters(code) {
		return "", fmt.Errorf("%w: %q", ErrInvalidLanguage, code)
	}
	return strings.ToLower(code), nil
}

// validateDateRange enforces two invariants on a [from, to] date window
// passed to a holiday-listing endpoint:
//
//   - from <= to (else ErrInvalidDateRange, D-22 / VALID-02).
//   - to is within 3 calendar years inclusive of from (else
//     ErrDateRangeTooLarge, D-22 / VALID-03). "Within 3 calendar years
//     inclusive" anchors the window at to: from must lie no earlier than
//     to.AddDate(-3, 0, 0). Equivalently, from is rejected when it is
//     strictly before to advanced backward by 3 calendar years.
//
// Why backward-from-to rather than forward-from-from: Go's time.AddDate
// normalizes overflow toward later dates. Advancing 2024-02-29 forward by
// 3 years lands on 2027-03-01 (since 2027 is not a leap year), which makes
// the forward-anchored "from + 3 years + 1 day" formulation produce an
// off-by-one for any leap-day from. Anchoring the window at to and
// stepping backward by 3 calendar years avoids that asymmetry: 2027-02-28
// stepped back by 3 years lands on 2024-02-28 (no overflow), so a from of
// 2024-02-29 is acceptable; 2027-03-01 stepped back lands on 2024-03-01,
// which rejects from=2024-02-29 — the documented Pitfall 3 leap-year
// boundary. See ROADMAP success criterion #4 for the locked test cases.
//
// Equal dates (from == to) are accepted: a single-day window is a valid query.
//
// Error messages include the offending from/to dates via the Date.String()
// YYYY-MM-DD form. Dates are not secrets per ERR-04.
func validateDateRange(from, to Date) error {
	if from.After(to) {
		return fmt.Errorf("%w: from=%s to=%s", ErrInvalidDateRange, from, to)
	}
	// Anchor the 3-calendar-year window at to and step backward, then check
	// whether from precedes that lower bound. Calendar arithmetic via
	// AddDate (D-22, Pitfall 3, TZ-2 mitigation) handles leap-year
	// boundaries correctly without the forward-overflow asymmetry that
	// from.AddDate(3, 0, 1) exhibits for leap-day from values.
	lowerBound := Date{to.AddDate(-3, 0, 0)}
	if from.Before(lowerBound) {
		return fmt.Errorf("%w: from=%s to=%s spans more than 3 years", ErrDateRangeTooLarge, from, to)
	}
	return nil
}

// audit:ok 2026-05-30

// isTwoASCIILetters reports whether s is exactly 2 bytes and each byte is
// an ASCII letter (A-Z or a-z). Byte arithmetic (rather than [unicode.IsLetter])
// is intentional and mandatory: the W-01 fix requires that Unicode characters
// that fold to ASCII through [strings.ToUpper] / [strings.ToLower] (e.g. U+212A
// Kelvin sign → 'k' under ToLower) are rejected here, BEFORE canonicalization runs.
func isTwoASCIILetters(s string) bool {
	if len(s) != 2 {
		return false
	}
	for i := range 2 {
		b := s[i]
		if (b < 'A' || b > 'Z') && (b < 'a' || b > 'z') {
			return false
		}
	}
	return true
}
