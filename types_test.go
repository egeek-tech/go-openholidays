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
		assert.Equal(t, "", got.Quality)
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
// exact-match, case-insensitive match, fallback-to-first on miss, and
// empty-slice handling.
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
		name string
		c    Country
		lang string
		want string
	}{
		{name: "exact match pl", c: populated, lang: "pl", want: "Polska"},
		{name: "case-insensitive match PL", c: populated, lang: "PL", want: "Polska"},
		{name: "case-insensitive match Pl", c: populated, lang: "Pl", want: "Polska"},
		{name: "exact match en", c: populated, lang: "en", want: "Poland"},
		{name: "exact match de", c: populated, lang: "de", want: "Polen"},
		{name: "miss falls back to first entry", c: populated, lang: "xx", want: "Polska"},
		{name: "empty lang falls back to first entry", c: populated, lang: "", want: "Polska"},
		{name: "nil Name returns empty string", c: Country{IsoCode: "PL"}, lang: "pl", want: ""},
		{name: "empty Name slice returns empty string", c: Country{IsoCode: "PL", Name: []LocalizedText{}}, lang: "pl", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.c.NameFor(tc.lang))
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
		name string
		l    Language
		lang string
		want string
	}{
		{name: "exact match pl", l: populated, lang: "pl", want: "polski"},
		{name: "case-insensitive match EN", l: populated, lang: "EN", want: "Polish"},
		{name: "exact match de", l: populated, lang: "de", want: "Polnisch"},
		{name: "miss falls back to first entry", l: populated, lang: "fr", want: "polski"},
		{name: "nil Name returns empty string", l: Language{IsoCode: "pl"}, lang: "pl", want: ""},
		{name: "empty Name slice returns empty string", l: Language{IsoCode: "pl", Name: []LocalizedText{}}, lang: "pl", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.l.NameFor(tc.lang))
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
		name string
		s    Subdivision
		lang string
		want string
	}{
		{name: "exact match pl", s: populated, lang: "pl", want: "Śląskie"},
		{name: "case-insensitive match PL", s: populated, lang: "PL", want: "Śląskie"},
		{name: "exact match en", s: populated, lang: "en", want: "Silesian Voivodeship"},
		{name: "exact match de", s: populated, lang: "de", want: "Woiwodschaft Schlesien"},
		{name: "miss falls back to first entry", s: populated, lang: "fr", want: "Śląskie"},
		{name: "nil Name returns empty string", s: Subdivision{Code: "PL-SL"}, lang: "pl", want: ""},
		{name: "empty Name slice returns empty string", s: Subdivision{Code: "PL-SL", Name: []LocalizedText{}}, lang: "pl", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.s.NameFor(tc.lang))
		})
	}
}
