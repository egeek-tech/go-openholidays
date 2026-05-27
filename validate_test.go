// Package openholidays — tests for the unexported input validators.
//
// In-package tests are required so validateCountry/validateLanguage/
// validateDateRange (all unexported per ARCHITECTURE.md Pattern 5) can be
// exercised directly. Phase 2 endpoint methods call these validators
// before any HTTP request; the tests below lock the contract those
// callers depend on.
package openholidays

import (
	"errors"
	"strings"
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
	}
	for _, tc := range rejectCases {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			t.Parallel()
			canon, err := validateCountry(tc.input)
			require.Error(t, err)
			assert.Equal(t, "", canon, "rejected input must return empty canonical form")
			assert.True(t, errors.Is(err, ErrInvalidCountry),
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
// to lowercase, reject empty/short/long/digit/non-ASCII). Symmetric with
// TestValidateCountry against ErrInvalidLanguage.
func TestValidateLanguage(t *testing.T) {
	t.Parallel()

	type successCase struct {
		name    string
		input   string
		canonOK string
	}
	successCases := []successCase{
		{name: "lowercase passes through", input: "pl", canonOK: "pl"},
		{name: "uppercase canonicalizes", input: "PL", canonOK: "pl"},
		{name: "mixed case canonicalizes", input: "Pl", canonOK: "pl"},
		{name: "additional happy case en", input: "en", canonOK: "en"},
		{name: "additional happy case uppercase DE", input: "DE", canonOK: "de"},
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
	}
	for _, tc := range rejectCases {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			t.Parallel()
			canon, err := validateLanguage(tc.input)
			require.Error(t, err)
			assert.Equal(t, "", canon, "rejected input must return empty canonical form")
			assert.True(t, errors.Is(err, ErrInvalidLanguage),
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
			assert.True(t, errors.Is(err, tc.wantWrap),
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
func TestValidators_NoSensitiveData(t *testing.T) {
	t.Parallel()

	// Tokens that must never appear in validator error messages — they
	// indicate that HTTP/transport context has leaked into the error.
	forbiddenSubstrings := []string{"http", "://", "Body:", "Authorization"}

	assertNoLeak := func(t *testing.T, msg string) {
		t.Helper()
		for _, forbidden := range forbiddenSubstrings {
			assert.False(t, strings.Contains(msg, forbidden),
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
