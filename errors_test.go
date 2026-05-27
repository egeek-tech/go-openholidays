package openholidays

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSentinelErrors locks ERR-01: each of the 5 sentinels is non-nil,
// carries the "openholidays: " prefix per the project's error-string
// convention, and has a distinct identity from every other sentinel.
func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	type namedSentinel struct {
		name string
		err  error
	}
	all := []namedSentinel{
		{"ErrInvalidCountry", ErrInvalidCountry},
		{"ErrInvalidLanguage", ErrInvalidLanguage},
		{"ErrDateRangeTooLarge", ErrDateRangeTooLarge},
		{"ErrInvalidDateRange", ErrInvalidDateRange},
		{"ErrEmptyResponse", ErrEmptyResponse},
	}

	for _, s := range all {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()

			require.NotNil(t, s.err, "sentinel must be non-nil")

			msg := s.err.Error()
			assert.True(t,
				len(msg) > len("openholidays: ") && msg[:len("openholidays: ")] == "openholidays: ",
				"sentinel message %q must start with %q", msg, "openholidays: ",
			)

			// Identity uniqueness: no sentinel matches any other.
			for _, other := range all {
				if other.name == s.name {
					continue
				}
				assert.Falsef(t,
					errors.Is(s.err, other.err),
					"errors.Is(%s, %s) must be false (distinct identities)",
					s.name, other.name,
				)
			}
		})
	}
}

// TestSentinels_ErrorsIs locks ERR-03 + ROADMAP criterion #2: every sentinel
// is recoverable via errors.Is through a fmt.Errorf("...: %w", sentinel)
// wrapper, and wrapping does not bleed identity into a different sentinel.
func TestSentinels_ErrorsIs(t *testing.T) {
	t.Parallel()

	type namedSentinel struct {
		name string
		err  error
	}
	all := []namedSentinel{
		{"ErrInvalidCountry", ErrInvalidCountry},
		{"ErrInvalidLanguage", ErrInvalidLanguage},
		{"ErrDateRangeTooLarge", ErrDateRangeTooLarge},
		{"ErrInvalidDateRange", ErrInvalidDateRange},
		{"ErrEmptyResponse", ErrEmptyResponse},
	}

	for _, s := range all {
		t.Run(s.name+"/recoverable-through-wrap", func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("context %q: %w", "ZZZ", s.err)
			require.NotNil(t, wrapped, "fmt.Errorf must produce a non-nil error")

			assert.Truef(t,
				errors.Is(wrapped, s.err),
				"errors.Is(wrapped, %s) must be true through %%w",
				s.name,
			)

			// Identity does not bleed: wrapping sentinel X must not satisfy
			// errors.Is against a different sentinel Y.
			for _, other := range all {
				if other.name == s.name {
					continue
				}
				assert.Falsef(t,
					errors.Is(wrapped, other.err),
					"errors.Is(wrapped %s, %s) must be false (no identity bleed)",
					s.name, other.name,
				)
			}
		})
	}
}

// TestAPIError_Error locks D-18: the two output formats based on Message
// emptiness, and the invariant that Body is never serialized into the
// error string (ERR-04 / T-01-02-IL).
func TestAPIError_Error(t *testing.T) {
	t.Parallel()

	type tc struct {
		name string
		err  *APIError
		want string
	}
	cases := []tc{
		{
			name: "empty-message/omits-suffix",
			err:  &APIError{StatusCode: 404, Path: "/Subdivisions"},
			want: "openholidays: api error 404 at /Subdivisions",
		},
		{
			name: "non-empty-message/appends-suffix",
			err:  &APIError{StatusCode: 404, Path: "/Subdivisions", Message: "Country not supported"},
			want: "openholidays: api error 404 at /Subdivisions: Country not supported",
		},
		{
			name: "body-never-in-error-string",
			err: &APIError{
				StatusCode: 500,
				Path:       "/PublicHolidays",
				Body:       []byte("ignored by Error"),
				Message:    "boom",
			},
			want: "openholidays: api error 500 at /PublicHolidays: boom",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			require.NotNil(t, c.err, "*APIError must be non-nil for this case")
			require.Equal(t, c.want, c.err.Error())
		})
	}
}

// TestAPIError_Is locks D-15 + Pitfall 5: wildcard (zero StatusCode) matches
// any *APIError; non-zero StatusCode matches by status only; Path / Body /
// Message on the target are ignored; non-*APIError targets never match.
func TestAPIError_Is(t *testing.T) {
	t.Parallel()

	base := &APIError{
		StatusCode: 404,
		Path:       "/Subdivisions",
		Body:       []byte("{\"detail\":\"not found\"}"),
		Message:    "Country not supported",
	}

	type tc struct {
		name   string
		target error
		want   bool
	}
	cases := []tc{
		{
			name:   "status-match",
			target: &APIError{StatusCode: 404},
			want:   true,
		},
		{
			name:   "wildcard-zero-status",
			target: &APIError{StatusCode: 0},
			want:   true,
		},
		{
			name:   "wildcard-zero-value",
			target: &APIError{},
			want:   true,
		},
		{
			name:   "status-mismatch",
			target: &APIError{StatusCode: 500},
			want:   false,
		},
		{
			name:   "path-ignored-on-target",
			target: &APIError{StatusCode: 404, Path: "/Other"},
			want:   true,
		},
		{
			name:   "message-ignored-on-target",
			target: &APIError{StatusCode: 404, Message: "different"},
			want:   true,
		},
		{
			name:   "body-ignored-on-target",
			target: &APIError{StatusCode: 404, Body: []byte("different body")},
			want:   true,
		},
		{
			name:   "non-apierror-target-never-matches",
			target: ErrInvalidCountry,
			want:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			require.NotNil(t, c.target, "target must be non-nil for this case")

			got := errors.Is(base, c.target)
			assert.Equal(t, c.want, got,
				"errors.Is(base{status=%d}, target=%T{...}) mismatch",
				base.StatusCode, c.target,
			)
		})
	}
}

// TestAPIError_ErrorsAs locks ERR-02 + ROADMAP criterion #3: errors.As
// extracts a populated *APIError from a %w-wrapped chain with all four
// fields preserved end-to-end.
func TestAPIError_ErrorsAs(t *testing.T) {
	t.Parallel()

	t.Run("populated-fields-survive-wrap", func(t *testing.T) {
		t.Parallel()

		original := &APIError{
			StatusCode: 404,
			Path:       "/Subdivisions",
			Body:       []byte("body"),
			Message:    "msg",
		}
		wrapped := fmt.Errorf("transport failure: %w", original)

		var got *APIError
		require.True(t, errors.As(wrapped, &got),
			"errors.As must recover *APIError from %%w-wrapped chain")
		require.NotNil(t, got, "recovered *APIError must be non-nil")

		assert.Equal(t, 404, got.StatusCode, "StatusCode preserved")
		assert.Equal(t, "/Subdivisions", got.Path, "Path preserved")
		assert.Equal(t, "body", string(got.Body), "Body preserved")
		assert.Equal(t, "msg", got.Message, "Message preserved")
	})
}
