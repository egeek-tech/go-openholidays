// Package main — ohcli `school` subcommand handler.
//
// This file implements the `school` subcommand: `ohcli school <country>
// <year> [--region CC-RR] [--lang xx] [--format text|json|csv] [--json]
// [--csv]`. The handler mirrors cmdPublic with one extra flag — --region —
// that maps to SchoolHolidaysRequest.SubdivisionCode (CLI-02). The library
// passes SubdivisionCode through to the upstream verbatim with no
// client-side shape check (D-56), so the CLI does no extra validation on
// --region beyond forwarding the string.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	openholidays "github.com/egeek-tech/go-openholidays"
)

// cmdSchool implements `ohcli school <country> <year> [--region CC-RR]
// [flags]`. It returns the process exit code per the same 0/1/2 contract
// cmdPublic documents.
//
// The handler is a one-flag-different sibling of cmdPublic: it adds
// --region (default "") so callers can filter the upstream result to a
// single administrative subdivision (e.g. `--region PL-SL` for the Polish
// ferie zimowe Śląskie cohort — CLI-02 example).
//
// Empty-result message wording branches on whether --region was provided so
// the operator-facing diagnostic identifies the exact filter chain that
// returned zero rows (per CONTEXT.md "Specifics"):
//
//   - region empty: "ohcli: no school holidays found for <country> <year>"
//   - region set  : "ohcli: no school holidays found for <country> <year> (region <CC-RR>)"
func cmdSchool(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("school", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "en", "ISO 639-1 language code for localized names")
	region := fs.String("region", "", "administrative subdivision code filter (e.g. PL-SL)")
	format := fs.String("format", "text", "output format: text, json, or csv")
	asJSON := fs.Bool("json", false, "shortcut for --format=json")
	asCSV := fs.Bool("csv", false, "shortcut for --format=csv")

	// Reorder argv to flags-first form so callers can write
	// `ohcli school PL 2025 --region PL-SL` (positionals first) and have
	// the --region flag still bind correctly. See reorderArgs in main.go.
	if err := fs.Parse(reorderArgs(args, map[string]struct{}{"json": {}, "csv": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "ohcli: school requires <country> and <year>")
		fs.Usage()
		return 2
	}
	country := fs.Arg(0)
	year, err := strconv.Atoi(fs.Arg(1))
	if err != nil || year < 1900 || year > 2100 {
		fmt.Fprintf(stderr, "ohcli: invalid year %q\n", fs.Arg(1))
		return 2
	}
	switch {
	case *asJSON:
		*format = "json"
	case *asCSV:
		*format = "csv"
	}
	if *format != "text" && *format != "json" && *format != "csv" {
		fmt.Fprintf(stderr, "ohcli: invalid format %q (want text|json|csv)\n", *format)
		return 2
	}

	c := newClient()
	defer func() { _ = c.Close() }()

	req := openholidays.SchoolHolidaysRequest{
		CountryIsoCode:  country,
		ValidFrom:       openholidays.NewDate(year, time.January, 1),
		ValidTo:         openholidays.NewDate(year, time.December, 31),
		LanguageIsoCode: *lang,
		SubdivisionCode: *region,
	}
	hs, err := c.SchoolHolidays(ctx, req)
	if err != nil {
		fmt.Fprintf(stderr, "ohcli: %v\n", err)
		return libErrExitCode(err)
	}
	if len(hs) == 0 {
		if *region == "" {
			fmt.Fprintf(stderr, "ohcli: no school holidays found for %s %d\n", country, year)
		} else {
			fmt.Fprintf(stderr, "ohcli: no school holidays found for %s %d (region %s)\n", country, year, *region)
		}
		return 0
	}
	if err := render(stdout, hs, *lang, *format); err != nil {
		fmt.Fprintf(stderr, "ohcli: %v\n", err)
		return 1
	}
	return 0
}
