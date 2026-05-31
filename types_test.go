// Package openholidays — tests for the domain type contract in types.go.
//
// One TestXxx function per exported production function/type per Gold Rule 3.
// Every test case is wrapped in t.Run; require for preconditions, assert for
// verifications. Non-English strings in fixtures (e.g. "Wigilia Bożego
// Narodzenia", "Śląskie") mirror real upstream OpenHolidays responses and
// are admitted per CONVENTIONS.md Rule 1 testdata-fixture exception.

package openholidays

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// audit:ok 2026-05-30

// TestHolidayType_constants verifies that each of the six HolidayType
// constants stringifies to the exact upstream wire value (TYPES-04, CL-04).
func TestHolidayType_constants(t *testing.T) {
	t.Parallel()

	t.Run("HolidayTypePublic", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Public", string(HolidayTypePublic))
	})
	t.Run("HolidayTypeBank", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Bank", string(HolidayTypeBank))
	})
	t.Run("HolidayTypeOptional", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Optional", string(HolidayTypeOptional))
	})
	t.Run("HolidayTypeSchool", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "School", string(HolidayTypeSchool))
	})
	t.Run("HolidayTypeBackToSchool", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "BackToSchool", string(HolidayTypeBackToSchool))
	})
	t.Run("HolidayTypeEndOfLessons", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "EndOfLessons", string(HolidayTypeEndOfLessons))
	})
}

// audit:ok 2026-05-30

// TestHolidayType_IsKnown locks IN-03 follow-up: IsKnown returns true for
// each of the six documented constants and false for any other value
// (empty string, an upstream-drift value not in the constant set, mixed
// case, and a UUID-shaped string that proves the helper does not silently
// accept arbitrary strings).
func TestHolidayType_IsKnown(t *testing.T) {
	t.Parallel()

	type tc struct {
		name  string
		input HolidayType
		want  bool
	}
	cases := []tc{
		{name: "HolidayTypePublic", input: HolidayTypePublic, want: true},
		{name: "HolidayTypeBank", input: HolidayTypeBank, want: true},
		{name: "HolidayTypeOptional", input: HolidayTypeOptional, want: true},
		{name: "HolidayTypeSchool", input: HolidayTypeSchool, want: true},
		{name: "HolidayTypeBackToSchool", input: HolidayTypeBackToSchool, want: true},
		{name: "HolidayTypeEndOfLessons", input: HolidayTypeEndOfLessons, want: true},
		{name: "empty string is not known", input: HolidayType(""), want: false},
		{name: "upstream-drift Religious is not known", input: HolidayType("Religious"), want: false},
		{name: "case-sensitive: lowercase public is not known", input: HolidayType("public"), want: false},
		{name: "free-form value is not known", input: HolidayType("Whatever"), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := c.input.IsKnown()
			assert.Equal(t, c.want, got,
				"HolidayType(%q).IsKnown() mismatch", string(c.input))
		})
	}
}

// TestRegionalScope_IsKnown verifies IsKnown returns true for each of the three
// documented RegionalScope constants and false for any other value (empty
// string, an upstream-drift value, mixed case, and a free-form string) — the
// same closed-set guarantee as HolidayType.IsKnown.
func TestRegionalScope_IsKnown(t *testing.T) {
	t.Parallel()

	type tc struct {
		name  string
		input RegionalScope
		want  bool
	}
	cases := []tc{
		{name: "RegionalScopeNational", input: RegionalScopeNational, want: true},
		{name: "RegionalScopeRegional", input: RegionalScopeRegional, want: true},
		{name: "RegionalScopeLocal", input: RegionalScopeLocal, want: true},
		{name: "empty string is not known", input: RegionalScope(""), want: false},
		{name: "upstream-drift Continental is not known", input: RegionalScope("Continental"), want: false},
		{name: "case-sensitive: lowercase national is not known", input: RegionalScope("national"), want: false},
		{name: "free-form value is not known", input: RegionalScope("Whatever"), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := c.input.IsKnown()
			assert.Equal(t, c.want, got,
				"RegionalScope(%q).IsKnown() mismatch", string(c.input))
		})
	}
}

// TestTemporalScope_IsKnown verifies IsKnown returns true for each of the two
// documented TemporalScope constants and false for any other value (empty
// string, an upstream-drift value, mixed case, and a free-form string) — the
// same closed-set guarantee as HolidayType.IsKnown.
func TestTemporalScope_IsKnown(t *testing.T) {
	t.Parallel()

	type tc struct {
		name  string
		input TemporalScope
		want  bool
	}
	cases := []tc{
		{name: "TemporalScopeFullDay", input: TemporalScopeFullDay, want: true},
		{name: "TemporalScopeHalfDay", input: TemporalScopeHalfDay, want: true},
		{name: "empty string is not known", input: TemporalScope(""), want: false},
		{name: "upstream-drift Evening is not known", input: TemporalScope("Evening"), want: false},
		{name: "case-sensitive: lowercase fullday is not known", input: TemporalScope("fullday"), want: false},
		{name: "free-form value is not known", input: TemporalScope("Whatever"), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := c.input.IsKnown()
			assert.Equal(t, c.want, got,
				"TemporalScope(%q).IsKnown() mismatch", string(c.input))
		})
	}
}

// audit:ok 2026-05-30

// TestLocalizedText_JSON verifies LocalizedText round-trips against the
// verified upstream wire shape (TYPES-03).
func TestLocalizedText_JSON(t *testing.T) {
	t.Parallel()

	const wireJSON = `{"language":"pl","text":"Wigilia Bożego Narodzenia"}`
	want := LocalizedText{Language: "pl", Text: "Wigilia Bożego Narodzenia"}

	t.Run("marshal", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(want)
		require.NoError(t, err)
		assert.JSONEq(t, wireJSON, string(b))
	})

	t.Run("unmarshal", func(t *testing.T) {
		t.Parallel()
		var got LocalizedText
		require.NoError(t, json.Unmarshal([]byte(wireJSON), &got))
		assert.Equal(t, want, got)
	})
}

// audit:ok 2026-05-30

// TestSubdivisionRef_JSON verifies SubdivisionRef round-trips against the
// verified upstream SubdivisionReference shape (TYPES-03).
func TestSubdivisionRef_JSON(t *testing.T) {
	t.Parallel()

	const wireJSON = `{"code":"PL-SL","shortName":"Śląskie"}`
	want := SubdivisionRef{Code: "PL-SL", ShortName: "Śląskie"}

	t.Run("marshal", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(want)
		require.NoError(t, err)
		assert.JSONEq(t, wireJSON, string(b))
	})

	t.Run("unmarshal", func(t *testing.T) {
		t.Parallel()
		var got SubdivisionRef
		require.NoError(t, json.Unmarshal([]byte(wireJSON), &got))
		assert.Equal(t, want, got)
	})
}

// audit:ok 2026-05-30

// TestGroupRef_JSON verifies GroupRef round-trips against the verified
// upstream GroupReference shape (TYPES-03).
func TestGroupRef_JSON(t *testing.T) {
	t.Parallel()

	const wireJSON = `{"code":"A","shortName":"Group A"}`
	want := GroupRef{Code: "A", ShortName: "Group A"}

	t.Run("marshal", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(want)
		require.NoError(t, err)
		assert.JSONEq(t, wireJSON, string(b))
	})

	t.Run("unmarshal", func(t *testing.T) {
		t.Parallel()
		var got GroupRef
		require.NoError(t, json.Unmarshal([]byte(wireJSON), &got))
		assert.Equal(t, want, got)
	})
}

// audit:ok 2026-05-30

// TestHoliday_JSON is the big wire-shape contract test (TYPES-01). It locks
// the Holiday struct against the verified upstream JSON shape from the live
// OpenAPI spec (2026-05-27): all 13 fields, nullable omitempty behavior on
// Comment/Subdivisions/Groups/Tags, schema-drift Quality tolerance, and
// default lenient decoding of unknown extra fields.
func TestHoliday_JSON(t *testing.T) {
	t.Parallel()

	t.Run("single-day public holiday — nullable fields omitted on marshal", func(t *testing.T) {
		t.Parallel()
		h := Holiday{
			ID:        "11111111-2222-3333-4444-555555555555",
			StartDate: NewDate(2025, time.December, 24),
			EndDate:   NewDate(2025, time.December, 24),
			Type:      HolidayTypePublic,
			Name: []LocalizedText{
				{Language: "pl", Text: "Wigilia Bożego Narodzenia"},
				{Language: "en", Text: "Christmas Eve"},
			},
			Nationwide:    true,
			RegionalScope: "National",
			TemporalScope: "FullDay",
			// Comment, Subdivisions, Groups, Tags intentionally nil — should be omitted.
			// Quality intentionally "" — should be omitted.
		}

		wantJSON := `{
			"id":"11111111-2222-3333-4444-555555555555",
			"startDate":"2025-12-24",
			"endDate":"2025-12-24",
			"type":"Public",
			"name":[
				{"language":"pl","text":"Wigilia Bożego Narodzenia"},
				{"language":"en","text":"Christmas Eve"}
			],
			"nationwide":true,
			"regionalScope":"National",
			"temporalScope":"FullDay"
		}`

		b, err := json.Marshal(h)
		require.NoError(t, err)
		assert.JSONEq(t, wantJSON, string(b), "marshaled Holiday must match upstream wire shape")

		var got Holiday
		require.NoError(t, json.Unmarshal(b, &got))
		assert.Equal(t, h.ID, got.ID)
		assert.True(t, h.StartDate.Equal(got.StartDate), "StartDate round-trip")
		assert.True(t, h.EndDate.Equal(got.EndDate), "EndDate round-trip")
		assert.Equal(t, h.Type, got.Type)
		assert.Equal(t, h.Name, got.Name)
		assert.Equal(t, h.Nationwide, got.Nationwide)
		assert.Equal(t, h.RegionalScope, got.RegionalScope)
		assert.Equal(t, h.TemporalScope, got.TemporalScope)
		assert.Nil(t, got.Comment)
		assert.Nil(t, got.Subdivisions)
		assert.Nil(t, got.Groups)
		assert.Nil(t, got.Tags)
		assert.Empty(t, got.Quality)
	})

	t.Run("multi-day school holiday — all nullable fields populated", func(t *testing.T) {
		t.Parallel()
		// Śląskie ferie zimowe — a multi-day school break (StartDate < EndDate).
		h := Holiday{
			ID:        "22222222-3333-4444-5555-666666666666",
			StartDate: NewDate(2025, time.February, 17),
			EndDate:   NewDate(2025, time.March, 2),
			Type:      HolidayTypeSchool,
			Name: []LocalizedText{
				{Language: "pl", Text: "Ferie zimowe"},
				{Language: "en", Text: "Winter school holidays"},
			},
			Nationwide:    false,
			RegionalScope: "Regional",
			TemporalScope: "FullDay",
			Comment: []LocalizedText{
				{Language: "pl", Text: "Ferie zimowe"},
			},
			Subdivisions: []SubdivisionRef{
				{Code: "PL-SL", ShortName: "Śląskie"},
			},
			Groups: []GroupRef{
				{Code: "A", ShortName: "Group A"},
			},
			Tags:    []string{"winter"},
			Quality: "Stable",
		}

		b, err := json.Marshal(h)
		require.NoError(t, err)

		var got Holiday
		require.NoError(t, json.Unmarshal(b, &got))
		assert.Equal(t, h.ID, got.ID)
		assert.True(t, h.StartDate.Equal(got.StartDate))
		assert.True(t, h.EndDate.Equal(got.EndDate))
		assert.Equal(t, h.Type, got.Type)
		assert.Equal(t, h.Name, got.Name)
		assert.Equal(t, h.Nationwide, got.Nationwide)
		assert.Equal(t, h.RegionalScope, got.RegionalScope)
		assert.Equal(t, h.TemporalScope, got.TemporalScope)
		assert.Equal(t, h.Comment, got.Comment)
		assert.Equal(t, h.Subdivisions, got.Subdivisions)
		assert.Equal(t, h.Groups, got.Groups)
		assert.Equal(t, h.Tags, got.Tags)
		assert.Equal(t, h.Quality, got.Quality)
	})

	t.Run("decode with schema-drift Quality field (Pitfall OH-2)", func(t *testing.T) {
		t.Parallel()
		// "Quality" is observed in real upstream responses but not in the
		// OpenAPI spec. Default lenient decode must accept it.
		raw := []byte(`{
			"id":"33333333-4444-5555-6666-777777777777",
			"startDate":"2025-05-01",
			"endDate":"2025-05-01",
			"type":"Public",
			"name":[{"language":"pl","text":"Święto Pracy"}],
			"nationwide":true,
			"regionalScope":"National",
			"temporalScope":"FullDay",
			"quality":"Verified"
		}`)

		var got Holiday
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, "Verified", got.Quality)
	})

	t.Run("decode tolerates unknown extra field (JSON-1 lenient default)", func(t *testing.T) {
		t.Parallel()
		// Default Go json.Unmarshal is lenient — unknown fields are silently
		// dropped. Strict decoding (DisallowUnknownFields) ships in Phase 4
		// as an opt-in. This locks the Phase 1 contract.
		raw := []byte(`{
			"id":"44444444-5555-6666-7777-888888888888",
			"startDate":"2025-11-11",
			"endDate":"2025-11-11",
			"type":"Public",
			"name":[{"language":"pl","text":"Narodowe Święto Niepodległości"}],
			"nationwide":true,
			"regionalScope":"National",
			"temporalScope":"FullDay",
			"extra_unknown_field":42,
			"another_drift_field":{"nested":"value"}
		}`)

		var got Holiday
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, "44444444-5555-6666-7777-888888888888", got.ID)
		assert.Equal(t, HolidayTypePublic, got.Type)
		assert.True(t, NewDate(2025, time.November, 11).Equal(got.StartDate))
	})
}

// TestCountry_NameFor covers the Country.NameFor accessor (TYPES-05, CL-05):
// exact-match, case-insensitive match, ("", false) on miss, and empty-slice
// handling.
func TestCountry_NameFor(t *testing.T) {
	t.Parallel()

	populated := Country{
		IsoCode: "PL",
		Name: []LocalizedText{
			{Language: "pl", Text: "Polska"},
			{Language: "en", Text: "Poland"},
			{Language: "de", Text: "Polen"},
		},
		OfficialLanguages: []string{"pl"},
	}

	cases := []struct {
		name   string
		c      Country
		lang   string
		want   string
		wantOK bool
	}{
		{name: "exact match pl", c: populated, lang: "pl", want: "Polska", wantOK: true},
		{name: "case-insensitive match PL", c: populated, lang: "PL", want: "Polska", wantOK: true},
		{name: "case-insensitive match Pl", c: populated, lang: "Pl", want: "Polska", wantOK: true},
		{name: "exact match en", c: populated, lang: "en", want: "Poland", wantOK: true},
		{name: "exact match de", c: populated, lang: "de", want: "Polen", wantOK: true},
		{name: "miss returns false, no fallback", c: populated, lang: "xx", want: "", wantOK: false},
		{name: "empty lang returns false", c: populated, lang: "", want: "", wantOK: false},
		{name: "nil Name returns false", c: Country{IsoCode: "PL"}, lang: "pl", want: "", wantOK: false},
		{name: "empty Name slice returns false", c: Country{IsoCode: "PL", Name: []LocalizedText{}}, lang: "pl", want: "", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tc.c.NameFor(tc.lang)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

// TestLanguage_NameFor covers the Language.NameFor accessor (TYPES-05):
// same contract as Country.NameFor.
func TestLanguage_NameFor(t *testing.T) {
	t.Parallel()

	populated := Language{
		IsoCode: "pl",
		Name: []LocalizedText{
			{Language: "pl", Text: "polski"},
			{Language: "en", Text: "Polish"},
			{Language: "de", Text: "Polnisch"},
		},
	}

	cases := []struct {
		name   string
		l      Language
		lang   string
		want   string
		wantOK bool
	}{
		{name: "exact match pl", l: populated, lang: "pl", want: "polski", wantOK: true},
		{name: "case-insensitive match EN", l: populated, lang: "EN", want: "Polish", wantOK: true},
		{name: "exact match de", l: populated, lang: "de", want: "Polnisch", wantOK: true},
		{name: "miss returns false, no fallback", l: populated, lang: "fr", want: "", wantOK: false},
		{name: "nil Name returns false", l: Language{IsoCode: "pl"}, lang: "pl", want: "", wantOK: false},
		{name: "empty Name slice returns false", l: Language{IsoCode: "pl", Name: []LocalizedText{}}, lang: "pl", want: "", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tc.l.NameFor(tc.lang)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

// TestSubdivision_NameFor covers the Subdivision.NameFor accessor (TYPES-05):
// same contract as Country.NameFor.
func TestSubdivision_NameFor(t *testing.T) {
	t.Parallel()

	populated := Subdivision{
		Code:      "PL-SL",
		ShortName: "Śląskie",
		Name: []LocalizedText{
			{Language: "pl", Text: "Śląskie"},
			{Language: "en", Text: "Silesian Voivodeship"},
			{Language: "de", Text: "Woiwodschaft Schlesien"},
		},
		OfficialLanguages: []string{"pl"},
	}

	cases := []struct {
		name   string
		s      Subdivision
		lang   string
		want   string
		wantOK bool
	}{
		{name: "exact match pl", s: populated, lang: "pl", want: "Śląskie", wantOK: true},
		{name: "case-insensitive match PL", s: populated, lang: "PL", want: "Śląskie", wantOK: true},
		{name: "exact match en", s: populated, lang: "en", want: "Silesian Voivodeship", wantOK: true},
		{name: "exact match de", s: populated, lang: "de", want: "Woiwodschaft Schlesien", wantOK: true},
		{name: "miss returns false, no fallback", s: populated, lang: "fr", want: "", wantOK: false},
		{name: "nil Name returns false", s: Subdivision{Code: "PL-SL"}, lang: "pl", want: "", wantOK: false},
		{name: "empty Name slice returns false", s: Subdivision{Code: "PL-SL", Name: []LocalizedText{}}, lang: "pl", want: "", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tc.s.NameFor(tc.lang)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}
