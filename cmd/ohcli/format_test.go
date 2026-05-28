// Package main — tests for the ohcli output renderers (renderText,
// renderJSON, renderCSV, renderCountries).
//
// Gold Rule 3 application: exactly one TestXxx per renderer production
// function — TestRenderText, TestRenderJSON, TestRenderCSV,
// TestRenderCountries. Each test holds every scenario inside a t.Run
// subtest with t.Parallel(); require for preconditions, assert for
// verifications.
//
// These are pure-transform tests — no HTTP, no fixtures, only in-memory
// []Holiday / []Country literals constructed against the public library
// types. The analog source-of-truth for the test shape is
// holiday_test.go in the repo root (same testify + t.Run + t.Parallel
// pattern).

package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	openholidays "github.com/egeek-tech/go-openholidays"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderText exercises renderText — the tab-aligned default output
// view written via text/tabwriter. Asserts the header row and that each
// Holiday emits one row containing the start date, localized name,
// nationwide flag, and type. Empty slice produces the header only.
func TestRenderText(t *testing.T) {
	t.Parallel()

	t.Run("happy path two holidays produces header plus two rows", func(t *testing.T) {
		t.Parallel()
		hs := []openholidays.Holiday{
			{
				StartDate:  openholidays.NewDate(2025, time.January, 1),
				EndDate:    openholidays.NewDate(2025, time.January, 1),
				Type:       openholidays.HolidayTypePublic,
				Nationwide: true,
				Name:       []openholidays.LocalizedText{{Language: "en", Text: "New Year's Day"}},
			},
			{
				StartDate:  openholidays.NewDate(2025, time.December, 25),
				EndDate:    openholidays.NewDate(2025, time.December, 25),
				Type:       openholidays.HolidayTypePublic,
				Nationwide: true,
				Name:       []openholidays.LocalizedText{{Language: "en", Text: "Christmas Day"}},
			},
		}
		var buf bytes.Buffer
		require.NoError(t, renderText(&buf, hs, "en"))
		out := buf.String()
		assert.Contains(t, out, "DATE", "header must include DATE column")
		assert.Contains(t, out, "END", "header must include END column")
		assert.Contains(t, out, "NAME", "header must include NAME column")
		assert.Contains(t, out, "NATIONWIDE", "header must include NATIONWIDE column")
		assert.Contains(t, out, "TYPE", "header must include TYPE column")
		assert.Contains(t, out, "2025-01-01", "must emit StartDate in YYYY-MM-DD form")
		assert.Contains(t, out, "2025-12-25", "must emit second holiday's StartDate")
		assert.Contains(t, out, "New Year's Day", "must emit localized name")
		assert.Contains(t, out, "Christmas Day", "must emit second holiday's name")
		assert.Contains(t, out, "true", "must emit boolean Nationwide flag")
		assert.Contains(t, out, "Public", "must emit Type as string")
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		require.Len(t, lines, 3, "two holidays must produce header + 2 data lines")
	})

	t.Run("empty slice prints header only", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		require.NoError(t, renderText(&buf, nil, "en"))
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		require.Len(t, lines, 1, "empty slice must produce header-only output")
		assert.Contains(t, lines[0], "DATE")
		assert.Contains(t, lines[0], "TYPE")
	})

	t.Run("--lang preference selects localized name", func(t *testing.T) {
		t.Parallel()
		hs := []openholidays.Holiday{{
			StartDate: openholidays.NewDate(2025, time.December, 24),
			EndDate:   openholidays.NewDate(2025, time.December, 24),
			Type:      openholidays.HolidayTypePublic,
			Name: []openholidays.LocalizedText{
				{Language: "en", Text: "Christmas Eve"},
				{Language: "pl", Text: "Wigilia"},
			},
		}}
		var buf bytes.Buffer
		require.NoError(t, renderText(&buf, hs, "pl"))
		assert.Contains(t, buf.String(), "Wigilia",
			"renderText must resolve Holiday.NameFor with the requested lang")
		assert.NotContains(t, buf.String(), "Christmas Eve",
			"non-selected localization must not appear")
	})
}

// TestRenderJSON exercises renderJSON — encoding/json with two-space
// indent. Asserts round-trip fidelity (the rendered bytes decode back to
// the input slice) and that the encoder applies the documented indent.
func TestRenderJSON(t *testing.T) {
	t.Parallel()

	t.Run("round-trip one holiday preserves StartDate and Name", func(t *testing.T) {
		t.Parallel()
		hs := []openholidays.Holiday{{
			ID:            "test-uuid",
			StartDate:     openholidays.NewDate(2025, time.January, 1),
			EndDate:       openholidays.NewDate(2025, time.January, 1),
			Type:          openholidays.HolidayTypePublic,
			Name:          []openholidays.LocalizedText{{Language: "en", Text: "New Year's Day"}},
			Nationwide:    true,
			RegionalScope: "National",
			TemporalScope: "FullDay",
		}}
		var buf bytes.Buffer
		require.NoError(t, renderJSON(&buf, hs))

		var got []openholidays.Holiday
		require.NoError(t, json.Unmarshal(buf.Bytes(), &got),
			"renderJSON output must be valid JSON that decodes back into []Holiday")
		require.Len(t, got, 1)
		assert.True(t, got[0].StartDate.Equal(hs[0].StartDate),
			"StartDate must round-trip losslessly")
		assert.Equal(t, hs[0].Name[0].Text, got[0].Name[0].Text,
			"Name text must round-trip")
		assert.Equal(t, hs[0].Type, got[0].Type, "Type must round-trip")
	})

	t.Run("two-space indent is applied", func(t *testing.T) {
		t.Parallel()
		hs := []openholidays.Holiday{{
			StartDate: openholidays.NewDate(2025, time.January, 1),
			EndDate:   openholidays.NewDate(2025, time.January, 1),
			Type:      openholidays.HolidayTypePublic,
			Name:      []openholidays.LocalizedText{{Language: "en", Text: "X"}},
		}}
		var buf bytes.Buffer
		require.NoError(t, renderJSON(&buf, hs))
		// SetIndent("", "  ") produces "\n  " before each indented field.
		assert.Contains(t, buf.String(), "\n  ",
			"output must use two-space indentation per renderJSON SetIndent contract")
	})

	t.Run("empty slice emits valid JSON empty array", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		require.NoError(t, renderJSON(&buf, nil))
		var got []openholidays.Holiday
		require.NoError(t, json.Unmarshal(buf.Bytes(), &got),
			"empty-slice output must still be valid JSON")
		assert.Empty(t, got, "round-tripped empty slice must be empty")
	})
}

// TestRenderCSV exercises renderCSV — RFC 4180 encoding/csv output with
// a snake_case header row. Asserts the header literal, one data row per
// Holiday, semicolon-joined subdivision codes, and RFC-4180 quoting for
// names that contain commas.
func TestRenderCSV(t *testing.T) {
	t.Parallel()

	t.Run("header row plus one data row for single-holiday slice", func(t *testing.T) {
		t.Parallel()
		hs := []openholidays.Holiday{{
			StartDate:  openholidays.NewDate(2025, time.January, 1),
			EndDate:    openholidays.NewDate(2025, time.January, 1),
			Type:       openholidays.HolidayTypePublic,
			Nationwide: true,
			Name:       []openholidays.LocalizedText{{Language: "en", Text: "New Year"}},
		}}
		var buf bytes.Buffer
		require.NoError(t, renderCSV(&buf, hs, "en"))
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		require.Len(t, lines, 2, "single holiday must produce header + 1 data row")
		assert.Equal(t, "start_date,end_date,name,nationwide,type,subdivision_codes",
			lines[0], "header must be exact snake_case literal")
		assert.Contains(t, lines[1], "2025-01-01,2025-01-01,New Year,true,Public",
			"data row must contain start,end,name,nationwide,type prefix")
	})

	t.Run("name with embedded comma is RFC-4180 quoted", func(t *testing.T) {
		t.Parallel()
		hs := []openholidays.Holiday{{
			StartDate: openholidays.NewDate(2025, time.January, 1),
			EndDate:   openholidays.NewDate(2025, time.January, 1),
			Type:      openholidays.HolidayTypePublic,
			Name:      []openholidays.LocalizedText{{Language: "en", Text: "Friday, Holiday"}},
		}}
		var buf bytes.Buffer
		require.NoError(t, renderCSV(&buf, hs, "en"))
		// encoding/csv must double-quote any field containing a comma.
		assert.Contains(t, buf.String(), `"Friday, Holiday"`,
			"embedded comma must be RFC-4180 quoted by encoding/csv")
		// Verify the output parses back through encoding/csv cleanly.
		r := csv.NewReader(strings.NewReader(buf.String()))
		records, err := r.ReadAll()
		require.NoError(t, err, "RFC-4180-quoted output must round-trip through csv.Reader")
		require.Len(t, records, 2)
		assert.Equal(t, "Friday, Holiday", records[1][2],
			"quoted name must round-trip to the original literal")
	})

	t.Run("subdivision codes are joined with semicolon", func(t *testing.T) {
		t.Parallel()
		hs := []openholidays.Holiday{{
			StartDate: openholidays.NewDate(2025, time.January, 20),
			EndDate:   openholidays.NewDate(2025, time.February, 2),
			Type:      openholidays.HolidayTypeSchool,
			Name:      []openholidays.LocalizedText{{Language: "en", Text: "Winter break"}},
			Subdivisions: []openholidays.SubdivisionRef{
				{Code: "PL-SL"},
				{Code: "PL-WP"},
			},
		}}
		var buf bytes.Buffer
		require.NoError(t, renderCSV(&buf, hs, "en"))
		assert.Contains(t, buf.String(), "PL-SL;PL-WP",
			"multi-subdivision Holiday must serialize codes joined by ';' per renderCSV contract")
	})

	t.Run("empty slice emits header only", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		require.NoError(t, renderCSV(&buf, nil, "en"))
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		require.Len(t, lines, 1)
		assert.Equal(t, "start_date,end_date,name,nationwide,type,subdivision_codes", lines[0])
	})
}

// TestRenderCountries exercises renderCountries — the Country-payload
// parallel of render. Asserts JSON round-trip, CSV header literal, and
// the text dispatcher path.
func TestRenderCountries(t *testing.T) {
	t.Parallel()

	t.Run("json format emits decodable []Country", func(t *testing.T) {
		t.Parallel()
		cs := []openholidays.Country{{
			IsoCode:           "PL",
			Name:              []openholidays.LocalizedText{{Language: "en", Text: "Poland"}},
			OfficialLanguages: []string{"pl"},
		}}
		var buf bytes.Buffer
		require.NoError(t, renderCountries(&buf, cs, "en", "json"))
		assert.Contains(t, buf.String(), `"PL"`, "iso code must appear in JSON output")
		assert.Contains(t, buf.String(), `"Poland"`, "localized name must appear in JSON output")

		var got []openholidays.Country
		require.NoError(t, json.Unmarshal(buf.Bytes(), &got),
			"JSON output must decode back to []Country")
		require.Len(t, got, 1)
		assert.Equal(t, "PL", got[0].IsoCode)
	})

	t.Run("csv format header is exact snake_case literal", func(t *testing.T) {
		t.Parallel()
		cs := []openholidays.Country{{
			IsoCode:           "PL",
			Name:              []openholidays.LocalizedText{{Language: "en", Text: "Poland"}},
			OfficialLanguages: []string{"pl", "de"},
		}}
		var buf bytes.Buffer
		require.NoError(t, renderCountries(&buf, cs, "en", "csv"))
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		require.Len(t, lines, 2, "single country must produce header + 1 data row")
		assert.Equal(t, "iso_code,name,official_languages", lines[0],
			"header must be exact snake_case literal")
		assert.Contains(t, lines[1], "pl;de",
			"official_languages must be joined with ';' per renderCountriesCSV contract")
	})

	t.Run("text format includes header columns and data", func(t *testing.T) {
		t.Parallel()
		cs := []openholidays.Country{{
			IsoCode:           "PL",
			Name:              []openholidays.LocalizedText{{Language: "en", Text: "Poland"}},
			OfficialLanguages: []string{"pl"},
		}}
		var buf bytes.Buffer
		require.NoError(t, renderCountries(&buf, cs, "en", "text"))
		out := buf.String()
		assert.Contains(t, out, "ISO_CODE")
		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "OFFICIAL_LANGUAGES")
		assert.Contains(t, out, "PL")
		assert.Contains(t, out, "Poland")
	})

	t.Run("invalid format returns typed error", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := renderCountries(&buf, nil, "en", "ical")
		require.Error(t, err, "unknown format must return an error")
		assert.Contains(t, err.Error(), "ical",
			"error must echo the offending format value")
	})
}
