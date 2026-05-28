// Package main — ohcli `countries` subcommand handler.
//
// This file implements the `countries` subcommand: `ohcli countries
// [--lang xx] [--format text|json|csv] [--json] [--csv]`. Unlike cmdPublic
// and cmdSchool, countries takes no positional arguments — every filter
// flows through flags.
//
// The handler dispatches through openholidays.Client.Countries, which
// returns []openholidays.Country (NOT []Holiday). Rendering therefore
// flows through renderCountries (declared in format.go) rather than the
// Holiday-specific render dispatcher.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	openholidays "github.com/egeek-tech/go-openholidays"
)

// cmdCountries implements `ohcli countries [flags]`. It returns the
// process exit code per the same 0/1/2 contract cmdPublic documents.
//
// The handler validates that no positional arguments were supplied (D-06
// usage error → exit 2), validates --format (with --json / --csv
// short-form precedence), and dispatches to Client.Countries with an
// optional language filter from --lang.
//
// On an empty result the handler writes "ohcli: no countries found" to
// stderr and returns 0 (D-07 — empty is a valid outcome). Library errors
// and render errors emit "ohcli: %v" on stderr and return exit code 1.
func cmdCountries(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("countries", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "en", "ISO 639-1 language code for localized names")
	format := fs.String("format", "text", "output format: text, json, or csv")
	asJSON := fs.Bool("json", false, "shortcut for --format=json")
	asCSV := fs.Bool("csv", false, "shortcut for --format=csv")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "ohcli: countries takes no positional args")
		fs.Usage()
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

	req := openholidays.CountriesRequest{LanguageIsoCode: *lang}
	cs, err := c.Countries(ctx, req)
	if err != nil {
		fmt.Fprintf(stderr, "ohcli: %v\n", err)
		return 1
	}
	if len(cs) == 0 {
		fmt.Fprintln(stderr, "ohcli: no countries found")
		return 0
	}
	if err := renderCountries(stdout, cs, *lang, *format); err != nil {
		fmt.Fprintf(stderr, "ohcli: %v\n", err)
		return 1
	}
	return 0
}
