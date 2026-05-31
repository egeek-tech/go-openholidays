package openholidays

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// audit:ok 2026-05-30
func TestNewDate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		year  int
		month time.Month
		day   int
	}{
		{"typical", 2025, time.December, 24},
		{"leap_year_feb_29", 2024, time.February, 29},
		{"year_one", 1, time.January, 1},
		{"end_of_year", 2025, time.December, 31},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := NewDate(tc.year, tc.month, tc.day)

			assert.Equal(t, tc.year, d.Year(), "Year")
			assert.Equal(t, tc.month, d.Month(), "Month")
			assert.Equal(t, tc.day, d.Day(), "Day")
			assert.Equal(t, time.UTC, d.Location(), "Location must be UTC")
			assert.Equal(t, 0, d.Hour(), "Hour must be 0")
			assert.Equal(t, 0, d.Minute(), "Minute must be 0")
			assert.Equal(t, 0, d.Second(), "Second must be 0")
			assert.Equal(t, 0, d.Nanosecond(), "Nanosecond must be 0")
		})
	}
}

// audit:ok 2026-05-30
func TestParseDate(t *testing.T) {
	t.Parallel()

	t.Run("empty_string_returns_errEmptyDate", func(t *testing.T) {
		t.Parallel()
		d, err := ParseDate("")
		require.Error(t, err)
		require.ErrorIs(t, err, errEmptyDate, "err must wrap errEmptyDate")
		assert.True(t, d.IsZero(), "Date must be zero on error")
	})

	t.Run("valid_2025_12_24", func(t *testing.T) {
		t.Parallel()
		d, err := ParseDate("2025-12-24")
		require.NoError(t, err)
		assert.True(t, d.Equal(NewDate(2025, time.December, 24)))
		assert.Equal(t, time.UTC, d.Location())
	})

	t.Run("malformed_returns_wrapped_parse_error", func(t *testing.T) {
		t.Parallel()
		_, err := ParseDate("not-a-date")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `openholidays: invalid date "not-a-date"`)
	})

	t.Run("month_out_of_range_2025_13_01", func(t *testing.T) {
		t.Parallel()
		_, err := ParseDate("2025-13-01")
		require.Error(t, err)
	})

	t.Run("non_leap_year_feb_29_2025", func(t *testing.T) {
		t.Parallel()
		_, err := ParseDate("2025-02-29")
		require.Error(t, err)
	})

	t.Run("leap_year_feb_29_2024_ok", func(t *testing.T) {
		t.Parallel()
		d, err := ParseDate("2024-02-29")
		require.NoError(t, err)
		assert.True(t, d.Equal(NewDate(2024, time.February, 29)))
	})
}

// audit:ok 2026-05-30
func TestDate_MarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("typical_date_2025_12_24", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(NewDate(2025, time.December, 24))
		require.NoError(t, err)
		assert.Equal(t, []byte(`"2025-12-24"`), b)
	})

	t.Run("zero_date_marshals_to_0001_01_01", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(Date{})
		require.NoError(t, err)
		assert.Equal(t, []byte(`"0001-01-01"`), b)
	})

	t.Run("roundtrip_locks_roadmap_criterion_1", func(t *testing.T) {
		t.Parallel()
		// ROADMAP success criterion #1, literal form.
		b, err := json.Marshal(NewDate(2025, time.December, 24))
		require.NoError(t, err)
		var d Date
		require.NoError(t, json.Unmarshal(b, &d))
		assert.True(t, d.Equal(NewDate(2025, time.December, 24)))
	})
}

func TestDate_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("null_returns_errEmptyDate", func(t *testing.T) {
		t.Parallel()
		var d Date
		err := d.UnmarshalJSON([]byte("null"))
		require.Error(t, err)
		require.ErrorIs(t, err, errEmptyDate, "must wrap errEmptyDate")
		assert.True(t, d.IsZero(), "receiver unchanged on error")
	})

	t.Run("empty_json_string_returns_errEmptyDate", func(t *testing.T) {
		t.Parallel()
		var d Date
		err := d.UnmarshalJSON([]byte(`""`))
		require.Error(t, err)
		require.ErrorIs(t, err, errEmptyDate, "must wrap errEmptyDate")
		assert.True(t, d.IsZero(), "receiver unchanged on error")
	})

	t.Run("valid_2025_12_24", func(t *testing.T) {
		t.Parallel()
		var d Date
		require.NoError(t, d.UnmarshalJSON([]byte(`"2025-12-24"`)))
		assert.True(t, d.Equal(NewDate(2025, time.December, 24)))
		assert.Equal(t, time.UTC, d.Location())
	})

	t.Run("non_string_json_number_123", func(t *testing.T) {
		t.Parallel()
		var d Date
		err := d.UnmarshalJSON([]byte("123"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "date must be a JSON string")
		assert.True(t, d.IsZero())
	})

	t.Run("non_string_json_boolean_true", func(t *testing.T) {
		t.Parallel()
		var d Date
		err := d.UnmarshalJSON([]byte("true"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "date must be a JSON string")
		assert.True(t, d.IsZero())
	})

	t.Run("malformed_date_string_returns_wrapped_parse_error", func(t *testing.T) {
		t.Parallel()
		var d Date
		err := d.UnmarshalJSON([]byte(`"not-a-date"`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `openholidays: invalid date "not-a-date"`)
		assert.True(t, d.IsZero())
	})

	t.Run("non_string_token_bounded_in_error_short_input", func(t *testing.T) {
		// IN-05 follow-up: short non-string tokens echo as-is.
		t.Parallel()
		var d Date
		err := d.UnmarshalJSON([]byte("12345"))
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "date must be a JSON string")
		assert.Contains(t, msg, "12345",
			"short caller input must be echoed verbatim")
		assert.NotContains(t, msg, "truncated",
			"short input must not carry a truncation suffix")
		assert.True(t, d.IsZero())
	})

	t.Run("non_string_token_bounded_in_error_oversized_input", func(t *testing.T) {
		// IN-05 follow-up: oversized non-string tokens are capped and
		// labeled with the total byte count.
		t.Parallel()
		oversized := bytes.Repeat([]byte("A"), 200)
		var d Date
		err := d.UnmarshalJSON(oversized)
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "date must be a JSON string")
		assert.Contains(t, msg, "(truncated, 200 total bytes)",
			"oversized caller input must be labeled with byte count")
		// 200 bytes of 'A' must not all appear in the error string.
		assert.NotContains(t, msg, string(oversized),
			"oversized caller input must not be echoed in full")
		assert.True(t, d.IsZero())
	})

	t.Run("non_string_token_non_printable_bytes_sanitized", func(t *testing.T) {
		// IN-05 follow-up: non-printable bytes (NUL, control codes, raw
		// binary, multi-byte UTF-8) are replaced with '?' before reaching
		// the operator-visible error string. This protects log integrity
		// when a caller invokes UnmarshalJSON directly with attacker-
		// controlled bytes.
		t.Parallel()
		// 0x00 (NUL), 0x01 (SOH), 0x7F (DEL), 0xFF (out of 7-bit ASCII).
		nonPrintable := []byte{0x00, 0x01, 0x7F, 0xFF}
		var d Date
		err := d.UnmarshalJSON(nonPrintable)
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "date must be a JSON string")
		// The exact substring "????" must appear: four masked bytes.
		assert.Contains(t, msg, "????",
			"non-printable bytes must be replaced with '?'")
		// Defense: literal control codes must not survive into the message.
		for _, c := range nonPrintable {
			assert.NotContains(t, msg, string([]byte{c}),
				"non-printable byte 0x%02x must not appear verbatim", c)
		}
		assert.True(t, d.IsZero())
	})
}

// audit:ok 2026-05-30
func TestDate_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		date Date
		want string
	}{
		{"typical_2025_12_24", NewDate(2025, time.December, 24), "2025-12-24"},
		{"zero_date", Date{}, "0001-01-01"},
		{"leap_day_2024_02_29", NewDate(2024, time.February, 29), "2024-02-29"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.date.String())
		})
	}
}

// audit:ok 2026-05-30
func TestDate_Equal(t *testing.T) {
	t.Parallel()

	t.Run("same_ymd_returns_true", func(t *testing.T) {
		t.Parallel()
		a := NewDate(2025, time.December, 24)
		b := NewDate(2025, time.December, 24)
		assert.True(t, a.Equal(b))
	})

	t.Run("different_day_returns_false", func(t *testing.T) {
		t.Parallel()
		a := NewDate(2025, time.December, 24)
		b := NewDate(2025, time.December, 25)
		assert.False(t, a.Equal(b))
	})

	t.Run("non_utc_midnight_still_normalizes", func(t *testing.T) {
		t.Parallel()
		// External struct-literal Date with non-midnight UTC time.
		nonMidnight := Date{time.Date(2025, time.December, 24, 12, 30, 0, 0, time.UTC)}
		assert.True(t, nonMidnight.Equal(NewDate(2025, time.December, 24)),
			"toUTCMidnight should erase the time-of-day component")
	})

	t.Run("different_location_still_normalizes", func(t *testing.T) {
		t.Parallel()
		// Date built in a fixed -01:00 zone — Year/Month/Day from local view
		// remain 2025-12-24, so toUTCMidnight normalizes both operands to the
		// same UTC midnight 2025-12-24.
		fz := time.FixedZone("UTC-1", -3600)
		nonUTC := Date{time.Date(2025, time.December, 24, 23, 0, 0, 0, fz)}
		assert.True(t, nonUTC.Equal(NewDate(2025, time.December, 24)),
			"Pitfall TZ-1: defensive UTC normalization on non-UTC Date input")
	})
}

// audit:ok 2026-05-30
func TestDate_Before(t *testing.T) {
	t.Parallel()

	t.Run("earlier_is_before_later", func(t *testing.T) {
		t.Parallel()
		assert.True(t, NewDate(2025, time.January, 1).Before(NewDate(2025, time.December, 31)))
	})

	t.Run("later_is_not_before_earlier", func(t *testing.T) {
		t.Parallel()
		assert.False(t, NewDate(2025, time.December, 31).Before(NewDate(2025, time.January, 1)))
	})

	t.Run("equal_is_not_before", func(t *testing.T) {
		t.Parallel()
		d := NewDate(2025, time.June, 15)
		assert.False(t, d.Before(d), "Before is strict")
	})
}

// audit:ok 2026-05-30
func TestDate_After(t *testing.T) {
	t.Parallel()

	t.Run("later_is_after_earlier", func(t *testing.T) {
		t.Parallel()
		assert.True(t, NewDate(2025, time.December, 31).After(NewDate(2025, time.January, 1)))
	})

	t.Run("earlier_is_not_after_later", func(t *testing.T) {
		t.Parallel()
		assert.False(t, NewDate(2025, time.January, 1).After(NewDate(2025, time.December, 31)))
	})

	t.Run("equal_is_not_after", func(t *testing.T) {
		t.Parallel()
		d := NewDate(2025, time.June, 15)
		assert.False(t, d.After(d), "After is strict")
	})
}

// audit:ok 2026-05-30
func TestDate_Compare(t *testing.T) {
	t.Parallel()

	t.Run("less_than_returns_minus_one", func(t *testing.T) {
		t.Parallel()
		got := NewDate(2025, time.January, 1).Compare(NewDate(2025, time.December, 31))
		assert.Equal(t, -1, got)
	})

	t.Run("equal_returns_zero", func(t *testing.T) {
		t.Parallel()
		got := NewDate(2025, time.June, 15).Compare(NewDate(2025, time.June, 15))
		assert.Equal(t, 0, got)
	})

	t.Run("greater_than_returns_plus_one", func(t *testing.T) {
		t.Parallel()
		got := NewDate(2025, time.December, 31).Compare(NewDate(2025, time.January, 1))
		assert.Equal(t, 1, got)
	})
}

func TestDate_DaysUntil(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from Date
		to   Date
		want int
	}{
		{
			name: "same_day_returns_0",
			from: NewDate(2025, time.June, 15),
			to:   NewDate(2025, time.June, 15),
			want: 0,
		},
		{
			name: "one_day_later_returns_1",
			from: NewDate(2025, time.June, 15),
			to:   NewDate(2025, time.June, 16),
			want: 1,
		},
		{
			name: "slaskie_ferie_zimowe_span_13",
			// 2025-02-17 to 2025-03-02 is a 13-day exclusive delta (14 inclusive
			// via Holiday.Days — the Śląskie ferie zimowe span).
			from: NewDate(2025, time.February, 17),
			to:   NewDate(2025, time.March, 2),
			want: 13,
		},
		{
			name: "negative_direction_minus_13",
			from: NewDate(2025, time.March, 2),
			to:   NewDate(2025, time.February, 17),
			want: -13,
		},
		{
			name: "us_eastern_dst_crossing_march_2025",
			// US DST began 2025-03-09. 2025-03-01..2025-03-31 is a 30-day delta.
			// Regression check: implementation must NOT rely on local time arithmetic.
			from: NewDate(2025, time.March, 1),
			to:   NewDate(2025, time.March, 31),
			want: 30,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.from.DaysUntil(tc.to))
		})
	}
}

// audit:ok 2026-05-30

// FuzzDateUnmarshal enforces the JSON-3 invariant: (*Date).UnmarshalJSON
// must NEVER panic for any input byte sequence. Seed corpus exercises the
// canonical failure modes (null, empty JSON string, valid, non-string,
// malformed) plus an empty byte slice. See CONTEXT.md D-12 and CL-03.
func FuzzDateUnmarshal(f *testing.F) {
	f.Add([]byte(`"2025-12-24"`)) // happy path
	f.Add([]byte("null"))         // null literal — rejected via errEmptyDate
	f.Add([]byte(`""`))           // empty JSON string — rejected via errEmptyDate
	f.Add([]byte(`"2024-02-29"`)) // leap-year boundary
	f.Add([]byte("123"))          // non-string JSON number
	f.Add([]byte(`"not-a-date"`)) // malformed date string
	f.Add([]byte(""))             // empty byte slice
	f.Fuzz(func(_ *testing.T, b []byte) {
		var d Date
		// Return value intentionally ignored — the invariant is panic-freedom.
		_ = d.UnmarshalJSON(b)
	})
}
