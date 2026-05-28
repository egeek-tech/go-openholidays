// Package main — ohcli output renderers (text/json/csv).
//
// This file ships the three Holiday renderers and the render dispatcher
// every subcommand handler uses to emit results to stdout. All renderers
// take an io.Writer (not *os.File) so Plan 02's table-driven tests can
// capture output into *bytes.Buffer and assert against it byte-for-byte.
//
// Format inventory (D-03):
//
//   - text — pkg.go.dev/text/tabwriter for the column-aligned default view
//     (DATE, END, NAME, NATIONWIDE, TYPE).
//   - json — pkg.go.dev/encoding/json with two-space indent for human
//     readability and pipe-into-jq friendliness.
//   - csv  — pkg.go.dev/encoding/csv RFC 4180 with a snake_case header row;
//     UTF-8 with no BOM per CONTEXT.md "Specifics" so the file streams
//     cleanly through `awk`, `cut`, and `psql \COPY`.
//
// All three renderers share the same lang argument so Holiday.NameFor can
// resolve the localized name with the same language preference used by the
// upstream filter (--lang xx flag in every subcommand).
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	openholidays "github.com/egeek-tech/go-openholidays"
)

// render dispatches to the per-format renderer for a []Holiday payload. It
// is the single entry point every Holiday-producing subcommand handler
// calls. format must be one of "text", "json", or "csv"; the subcommand
// handler validates the value before calling render, but render itself
// returns a typed error for any unrecognized value so the function remains
// safe to call from tests with arbitrary input.
func render(w io.Writer, hs []openholidays.Holiday, lang, format string) error {
	switch format {
	case "text":
		return renderText(w, hs, lang)
	case "json":
		return renderJSON(w, hs)
	case "csv":
		return renderCSV(w, hs, lang)
	default:
		return fmt.Errorf("ohcli: invalid format %q", format)
	}
}

// renderText writes the column-aligned text view of a []Holiday using
// text/tabwriter (RESEARCH §3.2 Pattern 2). Column order matches the
// research reference: DATE, END, NAME, NATIONWIDE, TYPE. Holiday.NameFor
// resolves the localized name for lang case-insensitively, falling back to
// the first entry when lang is not present (matches the library's
// pickLocalized behavior).
//
// The tabwriter is constructed with minwidth=0, tabwidth=0, padding=2,
// padchar=' ', flags=0 — the standard "two-space padding between columns,
// no leading indent" form used throughout the stdlib examples.
func renderText(w io.Writer, hs []openholidays.Holiday, lang string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DATE\tEND\tNAME\tNATIONWIDE\tTYPE")
	for _, h := range hs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n",
			h.StartDate, h.EndDate, h.NameFor(lang), h.Nationwide, h.Type)
	}
	return tw.Flush()
}

// renderJSON writes the JSON view of a []Holiday using encoding/json
// (RESEARCH §3.2 Pattern 2). Output is two-space-indented so the result is
// readable on a terminal and pipes cleanly into jq. The encoder appends a
// trailing newline (json.Encoder behavior) so the final line ends with
// "\n", matching the Unix convention.
//
// The Holiday struct's json tags are the library's canonical wire shape
// (types.go) so the rendered JSON round-trips back through the upstream
// API and through the library's own Holiday decoder.
func renderJSON(w io.Writer, hs []openholidays.Holiday) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(hs)
}

// renderCSV writes the CSV view of a []Holiday using encoding/csv
// (RESEARCH §3.2 Pattern 2). Output is RFC 4180 compliant with a
// snake_case header row: start_date, end_date, name, nationwide, type,
// subdivision_codes. No UTF-8 BOM is emitted (per CONTEXT.md "Specifics").
//
// Multi-subdivision Holidays serialize their Subdivisions list as a
// semicolon-joined string in the subdivision_codes column. Semicolon is
// chosen over comma so the CSV row stays a single field and round-trips
// through every spreadsheet importer without requiring quoting; downstream
// consumers split on ';' to recover the original list.
func renderCSV(w io.Writer, hs []openholidays.Holiday, lang string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"start_date", "end_date", "name", "nationwide", "type", "subdivision_codes"}); err != nil {
		return err
	}
	for _, h := range hs {
		codes := make([]string, 0, len(h.Subdivisions))
		for _, s := range h.Subdivisions {
			codes = append(codes, s.Code)
		}
		if err := cw.Write([]string{
			h.StartDate.String(),
			h.EndDate.String(),
			h.NameFor(lang),
			strconv.FormatBool(h.Nationwide),
			string(h.Type),
			strings.Join(codes, ";"),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// renderCountries dispatches the per-format renderer for a []Country
// payload — the parallel of render for Country values. The countries
// subcommand calls this instead of render because Client.Countries
// returns []openholidays.Country, not []Holiday.
//
// The renderer set mirrors the Holiday renderer set: text via tabwriter
// (ISO_CODE / NAME / OFFICIAL_LANGUAGES columns), JSON via encoding/json
// with two-space indent, CSV via encoding/csv with a snake_case header
// row (iso_code,name,official_languages). The official_languages CSV
// column joins entries with ';' for the same single-field-per-row reason
// renderCSV joins subdivision codes with ';'.
func renderCountries(w io.Writer, cs []openholidays.Country, lang, format string) error {
	switch format {
	case "text":
		return renderCountriesText(w, cs, lang)
	case "json":
		return renderCountriesJSON(w, cs)
	case "csv":
		return renderCountriesCSV(w, cs, lang)
	default:
		return fmt.Errorf("ohcli: invalid format %q", format)
	}
}

// renderCountriesText writes the column-aligned text view of a []Country
// using text/tabwriter. Columns: ISO_CODE, NAME (localized via
// Country.NameFor), OFFICIAL_LANGUAGES (comma-joined for the human
// reader; CSV uses ';' for parser-friendly separation).
func renderCountriesText(w io.Writer, cs []openholidays.Country, lang string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ISO_CODE\tNAME\tOFFICIAL_LANGUAGES")
	for _, c := range cs {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			c.IsoCode, c.NameFor(lang), strings.Join(c.OfficialLanguages, ","))
	}
	return tw.Flush()
}

// renderCountriesJSON writes the JSON view of a []Country with two-space
// indent so output pipes cleanly into jq.
func renderCountriesJSON(w io.Writer, cs []openholidays.Country) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(cs)
}

// renderCountriesCSV writes the CSV view of a []Country using
// encoding/csv. Header row: iso_code, name, official_languages.
// official_languages joins the language list with ';' so the value stays
// a single field and round-trips through every spreadsheet importer
// without quoting.
func renderCountriesCSV(w io.Writer, cs []openholidays.Country, lang string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"iso_code", "name", "official_languages"}); err != nil {
		return err
	}
	for _, c := range cs {
		if err := cw.Write([]string{
			c.IsoCode,
			c.NameFor(lang),
			strings.Join(c.OfficialLanguages, ";"),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
