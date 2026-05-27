package openholidays

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateCountry_Smoke is the RED-phase smoke test that locks the basic
// contract of validateCountry before the full table-driven test lands in
// Task 2. It exists so the TDD cycle can run end-to-end (RED -> GREEN) for
// Task 1's production code.
func TestValidateCountry_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("canonicalizes lowercase to uppercase", func(t *testing.T) {
		t.Parallel()
		canon, err := validateCountry("pl")
		require.NoError(t, err)
		assert.Equal(t, "PL", canon)
	})

	t.Run("rejects empty string with ErrInvalidCountry", func(t *testing.T) {
		t.Parallel()
		_, err := validateCountry("")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidCountry))
	})
}

// TestValidateLanguage_Smoke locks the basic contract of validateLanguage.
func TestValidateLanguage_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("canonicalizes uppercase to lowercase", func(t *testing.T) {
		t.Parallel()
		canon, err := validateLanguage("PL")
		require.NoError(t, err)
		assert.Equal(t, "pl", canon)
	})

	t.Run("rejects empty string with ErrInvalidLanguage", func(t *testing.T) {
		t.Parallel()
		_, err := validateLanguage("")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidLanguage))
	})
}

// TestValidateDateRange_Smoke locks the basic contract of validateDateRange.
func TestValidateDateRange_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("equal dates accepted", func(t *testing.T) {
		t.Parallel()
		d := NewDate(2025, time.January, 1)
		assert.NoError(t, validateDateRange(d, d))
	})

	t.Run("from after to wraps ErrInvalidDateRange", func(t *testing.T) {
		t.Parallel()
		from := NewDate(2025, time.December, 31)
		to := NewDate(2025, time.January, 1)
		err := validateDateRange(from, to)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidDateRange))
	})
}
