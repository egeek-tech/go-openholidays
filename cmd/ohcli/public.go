// ohcli `public` subcommand handler.
//
// This file implements the `public` subcommand: `ohcli public <country>
// <year> [--lang xx] [--format text|json|csv] [--json] [--csv]` per
// RESEARCH §"CLI subcommand handler". The handler owns its own
// [flag.FlagSet] so its flag-parsing errors flow to stderr (D-05) and never
// leak into the other subcommands' state.
//
// Exit code conventions (D-06):
//
//   - 0 — success (including the empty-result path per D-07)
//   - 1 — runtime error (transport/render error from the library)
//   - 2 — usage error (bad flag, missing positional, out-of-range year,
//     invalid --format, or a library-side validation rejection such as
//     ErrInvalidCountry / ErrInvalidLanguage / ErrInvalidDateRange /
//     ErrDateRangeTooLarge — these are caller-shape problems, not
//     runtime failures)

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

// audit:ok 2026-05-30

// cmdPublic implements `ohcli public <country> <year> [flags]`. It returns
// the process exit code per the contract documented above.
//
// The handler validates positional args (country code + year), validates
// the --format flag (with --json / --csv short-form precedence), builds an
// openholidays.PublicHolidaysRequest spanning the full calendar year, and
// dispatches through the shared newClient helper (which honors the
// OPENHOLIDAYS_BASE_URL test seam Plan 02 relies on).
//
// On an empty result the handler writes "ohcli: no public holidays found
// for <country> <year>" to stderr and returns 0 per D-07 — an empty list
// is a valid outcome, not an error. Library validation sentinels
// (ErrInvalidCountry, ErrInvalidLanguage, ErrInvalidDateRange,
// ErrDateRangeTooLarge) return exit 2 (usage error). Any other library
// error or render error returns exit 1.
func cmdPublic(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("public", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "en", "ISO 639-1 language code for localized names")
	format := fs.String("format", "text", "output format: text, json, or csv")
	asJSON := fs.Bool("json", false, "shortcut for --format=json")
	asCSV := fs.Bool("csv", false, "shortcut for --format=csv")

	// Stdlib flag halts at the first non-flag token, so callers writing
	// `ohcli public PL 2025 --json` would otherwise see --json land in
	// the positional set. Reorder argv to flags-first form before parsing
	// so the documented CLI shape `public <country> <year> [--flag]`
	// works regardless of token order.
	if err := fs.Parse(reorderArgs(args, map[string]struct{}{"json": {}, "csv": {}})); err != nil {
		// fs.Parse already wrote the error to stderr via SetOutput.
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "ohcli: public requires <country> and <year>")
		fs.Usage()
		return 2
	}
	country := fs.Arg(0)
	year, err := strconv.Atoi(fs.Arg(1))
	if err != nil || year < minYear || year > maxYear {
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

	req := openholidays.PublicHolidaysRequest{
		CountryIsoCode:  country,
		ValidFrom:       openholidays.NewDate(year, time.January, 1),
		ValidTo:         openholidays.NewDate(year, time.December, 31),
		LanguageIsoCode: *lang,
	}
	hs, err := c.PublicHolidays(ctx, req)
	if err != nil {
		fmt.Fprintf(stderr, "ohcli: %v\n", err)
		return libErrExitCode(err)
	}
	if len(hs) == 0 {
		fmt.Fprintf(stderr, "ohcli: no public holidays found for %s %d\n", country, year)
		return 0
	}
	if err := render(stdout, hs, *lang, *format); err != nil {
		fmt.Fprintf(stderr, "ohcli: %v\n", err)
		return 1
	}
	return 0
}
