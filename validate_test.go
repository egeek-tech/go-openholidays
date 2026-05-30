// tests for the unexported input validators.
//
// In-package tests are required so validateCountry/validateLanguage/
// validateDateRange (all unexported per ARCHITECTURE.md Pattern 5) can be
// exercised directly. Phase 2 endpoint methods call these validators
// before any HTTP request; the tests below lock the contract those
// callers depend on.

package openholidays

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateCountry covers VALID-01 and CL-02 (case-insensitive accept,
// canonicalize to uppercase, reject empty/short/long/digit/whitespace/
// non-ASCII).
func TestValidateCountry(t *testing.T) {
	t.Parallel()

	type successCase struct {
		name    string
		input   string
		canonOK string
	}
	successCases := []successCase{
		{name: "uppercase passes through", input: "PL", canonOK: "PL"},
		{name: "lowercase canonicalizes", input: "pl", canonOK: "PL"},
		{name: "mixed case canonicalizes", input: "Pl", canonOK: "PL"},
		{name: "additional happy case DE", input: "DE", canonOK: "DE"},
		{name: "additional happy case lowercase fr", input: "fr", canonOK: "FR"},
	}
	for _, tc := range successCases {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			t.Parallel()
			canon, err := validateCountry(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.canonOK, canon)
		})
	}

	type rejectCase struct {
		name  string
		input string
	}
	rejectCases := []rejectCase{
		{name: "empty string", input: ""},
		{name: "one letter", input: "P"},
		{name: "three letters", input: "POL"},
		{name: "letter plus digit", input: "P1"},
		{name: "two digits", input: "11"},
		{name: "leading whitespace", input: " PL"},
		{name: "trailing whitespace", input: "PL "},
		{name: "non-ASCII letters", input: "ŻŻ"},
		{name: "lowercase + non-ASCII", input: "pŻ"},
		{name: "symbol", input: "P-"},
		// W-01 regression cases (Phase 2 D-32). Each input is a real pre-fix bypass:
		// strings.ToUpper folds the input to a 2-byte string of ASCII uppercase
		// letters that the pre-fix isTwoASCIIUppers(canon) check accepted.
		//   "ıA"  U+0131 + 'A'   → ToUpper → "IA"  (2 bytes, both [A-Z])  → BYPASS
		//   "ſA"  U+017F + 'A'   → ToUpper → "SA"  (2 bytes, both [A-Z])  → BYPASS
		//   "ıı"  U+0131 + U+0131 → ToUpper → "II"  (2 bytes, both [A-Z]) → BYPASS
		//   "ſſ"  U+017F + U+017F → ToUpper → "SS"  (2 bytes, both [A-Z]) → BYPASS
		// Post-fix (Task 1 reorder), the ASCII-shape check runs on ORIGINAL bytes
		// (each ı / ſ is 2 bytes in UTF-8, so len("ıA") == 3, len("ıı") == 4)
		// and rejection occurs BEFORE strings.ToUpper is called. The wrapped
		// sentinel is ErrInvalidCountry; the %q-quoted message echoes the
		// ORIGINAL input (ERR-04 / D-23 invariant preserved).
		{name: "W-01 dotless-i + A folds to IA under ToUpper", input: "ıA"},
		{name: "W-01 long-s + A folds to SA under ToUpper", input: "ſA"},
		{name: "W-01 dotless-i x2 folds to II under ToUpper", input: "ıı"},
		{name: "W-01 long-s x2 folds to SS under ToUpper", input: "ſſ"},
	}
	for _, tc := range rejectCases {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			t.Parallel()
			canon, err := validateCountry(tc.input)
			require.Error(t, err)
			assert.Empty(t, canon, "rejected input must return empty canonical form")
			require.ErrorIs(t, err, ErrInvalidCountry,
				"expected error to wrap ErrInvalidCountry, got %v", err)
			// Verify the offending value is quoted in the error message (D-23).
			// We assert the ORIGINAL input shows up between quotes, not the
			// canonicalized form, so callers see exactly what they passed.
			wantSub := "\"" + tc.input + "\""
			assert.Contains(t, err.Error(), wantSub,
				"error must contain the offending value quoted with %%q")
		})
	}
}

// TestValidateLanguage covers VALID-04 (case-insensitive accept, canonicalize
// to uppercase, reject empty/short/long/digit/non-ASCII). Symmetric with
// TestValidateCountry against ErrInvalidLanguage.
func TestValidateLanguage(t *testing.T) {
	t.Parallel()

	type successCase struct {
		name    string
		input   string
		canonOK string
	}
	successCases := []successCase{
		{name: "uppercase passes through", input: "PL", canonOK: "PL"},
		{name: "lowercase canonicalizes", input: "pl", canonOK: "PL"},
		{name: "mixed case canonicalizes", input: "Pl", canonOK: "PL"},
		{name: "additional happy case uppercase EN", input: "EN", canonOK: "EN"},
		{name: "additional happy case lowercase de", input: "de", canonOK: "DE"},
	}
	for _, tc := range successCases {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			t.Parallel()
			canon, err := validateLanguage(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.canonOK, canon)
		})
	}

	type rejectCase struct {
		name  string
		input string
	}
	rejectCases := []rejectCase{
		{name: "empty string", input: ""},
		{name: "one letter", input: "p"},
		{name: "three letters", input: "pol"},
		{name: "letter plus digit", input: "p1"},
		{name: "two digits", input: "11"},
		{name: "leading whitespace", input: " pl"},
		{name: "trailing whitespace", input: "pl "},
		{name: "non-ASCII letters", input: "żż"},
		{name: "uppercase + non-ASCII", input: "Pż"},
		{name: "symbol", input: "p-"},
		// W-01 regression cases (Phase 2 D-32). Each input is a real pre-fix bypass:
		// strings.ToLower folds the input to a 2-byte string of ASCII lowercase
		// letters that the pre-fix isTwoASCIILowers(canon) check accepted.
		//   "KK"  U+212A + U+212A → ToLower → "kk"  (2 bytes, both [a-z]) → BYPASS
		//   "İa"  U+0130 + 'a'   → ToLower → "ia"  (2 bytes, both [a-z]) → BYPASS
		//   "İİ"  U+0130 + U+0130 → ToLower → "ii"  (2 bytes, both [a-z]) → BYPASS
		//   "Ka"  U+212A + 'a'   → ToLower → "ka"  (2 bytes, both [a-z]) → BYPASS
		// Post-fix, the ASCII-shape check on ORIGINAL bytes rejects every case
		// BEFORE strings.ToLower runs. The wrapped sentinel is ErrInvalidLanguage.
		// Note: U+212A Kelvin sign is written via K (UTF-8 e2 84 aa) — not as
		// the visually similar ASCII 'K' (0x4b). The whole point of W-01 is that
		// strings.ToLower folds U+212A → ASCII 'k'; using ASCII 'K' here would
		// make these cases pass the post-fix isTwoASCIILetters check and miss
		// the regression they exist to lock.
		{name: "W-01 Kelvin sign x2 folds to kk under ToLower", input: "KK"},
		{name: "W-01 dotted-I + a folds to ia under ToLower", input: "İa"},
		{name: "W-01 dotted-I x2 folds to ii under ToLower", input: "İİ"},
		{name: "W-01 Kelvin sign + a folds to ka under ToLower", input: "Ka"},
	}
	for _, tc := range rejectCases {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			t.Parallel()
			canon, err := validateLanguage(tc.input)
			require.Error(t, err)
			assert.Empty(t, canon, "rejected input must return empty canonical form")
			require.ErrorIs(t, err, ErrInvalidLanguage,
				"expected error to wrap ErrInvalidLanguage, got %v", err)
			wantSub := "\"" + tc.input + "\""
			assert.Contains(t, err.Error(), wantSub,
				"error must contain the offending value quoted with %%q")
		})
	}
}

// TestValidateDateRange covers VALID-02 (from > to rejected with
// ErrInvalidDateRange) and VALID-03 (>3-year window rejected with
// ErrDateRangeTooLarge). Locks ROADMAP criterion #4 verbatim: exact-3-year
// boundary passes, +1d fails, leap-year boundary passes / +1d fails.
func TestValidateDateRange(t *testing.T) {
	t.Parallel()

	type successCase struct {
		name string
		from Date
		to   Date
	}
	successCases := []successCase{
		{
			name: "equal dates",
			from: NewDate(2025, time.January, 1),
			to:   NewDate(2025, time.January, 1),
		},
		{
			name: "single calendar year",
			from: NewDate(2025, time.January, 1),
			to:   NewDate(2025, time.December, 31),
		},
		{
			name: "exact 3 years inclusive (boundary PASS)",
			from: NewDate(2025, time.January, 1),
			to:   NewDate(2028, time.January, 1),
		},
		{
			name: "leap-year 3 years inclusive (boundary PASS, Pitfall 3)",
			from: NewDate(2024, time.February, 29),
			to:   NewDate(2027, time.February, 28),
		},
	}
	for _, tc := range successCases {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateDateRange(tc.from, tc.to)
			assert.NoError(t, err)
		})
	}

	type rangeCase struct {
		name       string
		from       Date
		to         Date
		wantWrap   error
		mustHaveTo string // expected "to=..." substring in error message
	}
	rejectCases := []rangeCase{
		{
			name:       "from after to by one day",
			from:       NewDate(2025, time.January, 2),
			to:         NewDate(2025, time.January, 1),
			wantWrap:   ErrInvalidDateRange,
			mustHaveTo: "to=2025-01-01",
		},
		{
			name:       "from year after to year",
			from:       NewDate(2025, time.December, 31),
			to:         NewDate(2025, time.January, 1),
			wantWrap:   ErrInvalidDateRange,
			mustHaveTo: "to=2025-01-01",
		},
		{
			name:       "exact 3 years plus 1 day (boundary FAIL)",
			from:       NewDate(2025, time.January, 1),
			to:         NewDate(2028, time.January, 2),
			wantWrap:   ErrDateRangeTooLarge,
			mustHaveTo: "to=2028-01-02",
		},
		{
			name:       "leap-year 3 years plus 1 day (boundary FAIL, Pitfall 3)",
			from:       NewDate(2024, time.February, 29),
			to:         NewDate(2027, time.March, 1),
			wantWrap:   ErrDateRangeTooLarge,
			mustHaveTo: "to=2027-03-01",
		},
		{
			name:       "extreme 25-year range",
			from:       NewDate(2025, time.January, 1),
			to:         NewDate(2050, time.January, 1),
			wantWrap:   ErrDateRangeTooLarge,
			mustHaveTo: "to=2050-01-01",
		},
	}
	for _, tc := range rejectCases {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateDateRange(tc.from, tc.to)
			require.Error(t, err)
			require.ErrorIs(t, err, tc.wantWrap,
				"expected error to wrap %v, got %v", tc.wantWrap, err)
			// D-23: error message must include the offending from/to dates.
			assert.Contains(t, err.Error(), "from="+tc.from.String(),
				"error must include from= prefix and value")
			assert.Contains(t, err.Error(), tc.mustHaveTo,
				"error must include to= prefix and value")
		})
	}
}

// TestValidators_NoSensitiveData is the ERR-04 regression guard. It asserts
// the invariant that validator error messages contain ONLY the offending
// caller-supplied value, never HTTP/transport metadata. Defensive: catches
// future PRs that accidentally splice request/response context into a
// validator error.
//
// The test passes plausible-looking "secret" strings as inputs (they are
// also invalid by shape, so they all reach the error path). The assertion
// is twofold: (1) the offending value DOES appear in the error message
// per D-23, and (2) tokens that indicate HTTP/transport context
// ("http", "://", "Body:", "Authorization") do NOT appear.
//
// Key Decision CL-17 (PROJECT.md): this test is the documented Gold-Rule-3
// exception for a cross-cutting validator invariant. The per-validator
// tests (TestValidateCountry / TestValidateLanguage / TestValidateDateRange)
// continue to assert each validator's individual contract; this test
// asserts the ERR-04 no-leak property that spans ALL validators at once.
// Folding the assertion into each per-validator test would duplicate the
// check three times and dilute the "one property of the whole validator
// surface" framing. Future cross-cutting tests must be argued for
// individually and added to PROJECT.md Key Decisions before landing.
func TestValidators_NoSensitiveData(t *testing.T) {
	t.Parallel()

	// Tokens that must never appear in validator error messages — they
	// indicate that HTTP/transport context has leaked into the error.
	forbiddenSubstrings := []string{"http", "://", "Body:", "Authorization"}

	assertNoLeak := func(t *testing.T, msg string) {
		t.Helper()
		for _, forbidden := range forbiddenSubstrings {
			assert.NotContains(t, msg, forbidden,
				"validator error message must not contain %q (ERR-04 leak guard); got %q",
				forbidden, msg)
		}
	}

	t.Run("validateCountry quotes offending value, no transport leak", func(t *testing.T) {
		t.Parallel()
		_, err := validateCountry("topsecret")
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "\"topsecret\"",
			"error must quote the offending value per D-23")
		assertNoLeak(t, msg)
	})

	t.Run("validateLanguage quotes offending value, no transport leak", func(t *testing.T) {
		t.Parallel()
		_, err := validateLanguage("apikey")
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "\"apikey\"")
		assertNoLeak(t, msg)
	})

	t.Run("validateDateRange includes from/to, no transport leak", func(t *testing.T) {
		t.Parallel()
		err := validateDateRange(
			NewDate(2025, time.January, 1),
			NewDate(2099, time.January, 1),
		)
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "from=2025-01-01")
		assert.Contains(t, msg, "to=2099-01-01")
		assertNoLeak(t, msg)
	})
}
