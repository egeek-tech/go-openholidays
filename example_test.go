// Package openholidays_test — runnable Example_* doctests for pkg.go.dev.
//
// Every Example_* function here is rendered by pkg.go.dev next to its target
// symbol. The convention follows Pitfall 7 of the Phase 5 research:
//
//   - Examples with a `// Output:` block are deterministic doctests. They
//     exercise pure helpers and constructors and `go test -run Example` will
//     re-verify the captured stdout matches the comment on every CI run.
//   - Examples without a `// Output:` block are compile-only. They demonstrate
//     methods that issue HTTP against the live OpenHolidays API and would
//     otherwise hit the network during `go test -run Example`. They still
//     verify that the public symbols, argument shapes, and return types
//     match the documented surface.
//
// This file lives in package openholidays_test (the external test package) so
// it exercises only the public surface — exactly what a downstream consumer
// would compile against.
package openholidays_test

import (
	"context"
	"fmt"
	"time"

	"github.com/egeek-tech/go-openholidays"
)

// audit:ok 2026-05-31

// Example_quickstart mirrors the README quickstart verbatim — one canonical
// ≤20-line snippet that fetches a year of Polish public holidays. Compile-only
// because PublicHolidays hits the live API. This example is the single source
// of truth for README §Quickstart (DOC-01).
func Example_quickstart() {
	c := openholidays.NewClient()
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	hs, err := c.PublicHolidays(ctx, openholidays.PublicHolidaysRequest{
		CountryIsoCode: "PL",
		ValidFrom:      openholidays.NewDate(2025, time.January, 1),
		ValidTo:        openholidays.NewDate(2025, time.December, 31),
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("got %d Polish public holidays\n", len(hs))
}

// audit:ok 2026-05-30

// ExampleClient_PublicHolidays demonstrates the canonical
// "fetch one year of public holidays" call against a Client. Compile-only —
// PublicHolidays issues HTTP to the live OpenHolidays API.
func ExampleClient_PublicHolidays() {
	c := openholidays.NewClient()
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hs, err := c.PublicHolidays(ctx, openholidays.PublicHolidaysRequest{
		CountryIsoCode: "PL",
		ValidFrom:      openholidays.NewDate(2025, time.January, 1),
		ValidTo:        openholidays.NewDate(2025, time.December, 31),
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("got %d Polish public holidays\n", len(hs))
}

// audit:ok 2026-05-30

// ExampleClient_SchoolHolidays demonstrates the regional filter — the
// SubdivisionCode argument restricts the upstream result to school holidays
// applying to that administrative subdivision (e.g. "PL-SL" = Śląskie).
// Compile-only — SchoolHolidays issues HTTP to the live OpenHolidays API.
func ExampleClient_SchoolHolidays() {
	c := openholidays.NewClient()
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hs, err := c.SchoolHolidays(ctx, openholidays.SchoolHolidaysRequest{
		CountryIsoCode:  "PL",
		ValidFrom:       openholidays.NewDate(2025, time.January, 1),
		ValidTo:         openholidays.NewDate(2025, time.December, 31),
		SubdivisionCode: "PL-SL",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("got %d Polish school holidays for PL-SL\n", len(hs))
}

// audit:ok 2026-05-30

// ExampleClient_Countries demonstrates the Countries listing endpoint. The
// optional LanguageIsoCode filter narrows the localized Name entries upstream
// returns to a single language. Compile-only — Countries issues HTTP.
func ExampleClient_Countries() {
	c := openholidays.NewClient()
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	countries, err := c.Countries(ctx, openholidays.CountriesRequest{
		LanguageIsoCode: "en",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("got %d supported countries\n", len(countries))
}

// audit:ok 2026-05-30

// ExampleClient_Languages demonstrates the Languages listing endpoint —
// returns every ISO 639-1 language the upstream API can localize its
// responses in. Compile-only — Languages issues HTTP.
func ExampleClient_Languages() {
	c := openholidays.NewClient()
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	langs, err := c.Languages(ctx, openholidays.LanguagesRequest{})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("got %d supported languages\n", len(langs))
}

// audit:ok 2026-05-30

// ExampleClient_Subdivisions demonstrates fetching the administrative-
// subdivision tree for a country. For Poland the response is a flat list of
// 16 województwa; for Germany it nests Bundesländer → Regierungsbezirke.
// Compile-only — Subdivisions issues HTTP.
func ExampleClient_Subdivisions() {
	c := openholidays.NewClient()
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	subs, err := c.Subdivisions(ctx, openholidays.SubdivisionsRequest{
		CountryIsoCode:  "PL",
		LanguageIsoCode: "en",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("got %d Polish subdivisions\n", len(subs))
}

// audit:ok 2026-05-30

// ExampleClient_IsInRegion demonstrates the hierarchical region-membership
// check — unlike Holiday.IsInRegion (a pure flat match) this method may walk
// the upstream /Subdivisions tree to detect whether code is a descendant of
// any subdivision the holiday applies to. Compile-only because the upward
// walk may issue HTTP for hierarchical lookups.
func ExampleClient_IsInRegion() {
	c := openholidays.NewClient()
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h := openholidays.Holiday{
		Nationwide:   false,
		Subdivisions: []openholidays.SubdivisionRef{{Code: "PL-SL"}},
	}
	ok, err := c.IsInRegion(ctx, h, "PL-SL")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("holiday applies in PL-SL: %t\n", ok)
}

// audit:ok 2026-05-30

// ExampleClient_Close demonstrates the idempotent shutdown idiom: deferring
// Close immediately after NewClient guarantees the cache sweeper (when one
// was wired via WithCache) stops on the way out. Close is safe to call from
// any goroutine and returns nil on every invocation after the first.
// Compile-only — no observable output.
func ExampleClient_Close() {
	c := openholidays.NewClient()
	defer c.Close()
	// Calling Close twice is safe — the second call is a no-op.
	_ = c.Close()
}

// audit:ok 2026-05-30

// ExampleNewClient demonstrates functional-options composition — every
// behavior knob (User-Agent, timeout, retry, in-memory cache) is layered on
// the constructor without an options struct. Compile-only — no network is
// touched until an endpoint method is called.
func ExampleNewClient() {
	c := openholidays.NewClient(
		openholidays.WithUserAgent("myapp/1.0"),
		openholidays.WithTimeout(15*time.Second),
		openholidays.WithRetry(3, 250*time.Millisecond),
		openholidays.WithCache(time.Hour),
	)
	defer c.Close()
	_ = c
}

// audit:ok 2026-05-31

// ExampleHoliday_NameFor demonstrates the localized-name lookup. NameFor
// reports whether the requested language was found; on a miss it returns
// ("", false) with no fallback. The Polish literal matches the testdata-fixture
// exception in CONVENTIONS.md Rule 1.
func ExampleHoliday_NameFor() {
	h := openholidays.Holiday{
		Name: []openholidays.LocalizedText{
			{Language: "pl", Text: "Boże Narodzenie"},
			{Language: "en", Text: "Christmas Day"},
		},
	}
	pl, okPL := h.NameFor("pl")
	fmt.Println(pl, okPL)
	xx, okXX := h.NameFor("xx") // not present — no fallback
	fmt.Printf("%q %v\n", xx, okXX)
	// Output:
	// Boże Narodzenie true
	// "" false
}

// audit:ok 2026-05-30

// ExampleHoliday_IsInRegion demonstrates the flat (no-HTTP) region check on
// a Holiday value. A nationwide holiday returns true for any code; a holiday
// with explicit Subdivisions returns true only on a [strings.EqualFold] match.
func ExampleHoliday_IsInRegion() {
	h := openholidays.Holiday{
		Nationwide:   false,
		Subdivisions: []openholidays.SubdivisionRef{{Code: "PL-SL"}},
	}
	fmt.Println(h.IsInRegion("PL-SL"), h.IsInRegion("PL-WP"))
	// Output: true false
}

// audit:ok 2026-05-30

// ExampleHoliday_Days demonstrates the inclusive day-count helper. For a
// holiday spanning 2025-01-01 through 2025-01-07 inclusive, Days returns 7.
func ExampleHoliday_Days() {
	h := openholidays.Holiday{
		StartDate: openholidays.NewDate(2025, time.January, 1),
		EndDate:   openholidays.NewDate(2025, time.January, 7),
	}
	fmt.Println(h.Days())
	// Output: 7
}

// audit:ok 2026-05-30

// ExampleHoliday_Range demonstrates the Go 1.23 range-over-func iterator on
// Holiday. The iterator yields one Date per calendar day in chronological
// order, inclusive on both endpoints.
func ExampleHoliday_Range() {
	h := openholidays.Holiday{
		StartDate: openholidays.NewDate(2025, time.January, 1),
		EndDate:   openholidays.NewDate(2025, time.January, 3),
	}
	for d := range h.Range() {
		fmt.Println(d)
	}
	// Output:
	// 2025-01-01
	// 2025-01-02
	// 2025-01-03
}

// audit:ok 2026-05-31

// ExampleCountry_NameFor demonstrates the localized-name lookup pattern
// shared by Country, Language, and Subdivision. Matching is case-insensitive
// (strings.EqualFold); on a language miss NameFor returns ("", false) with no
// fallback.
func ExampleCountry_NameFor() {
	c := openholidays.Country{
		IsoCode: "PL",
		Name: []openholidays.LocalizedText{
			{Language: "en", Text: "Poland"},
			{Language: "de", Text: "Polen"},
		},
	}
	name, ok := c.NameFor("de")
	fmt.Println(name, ok)
	// Output: Polen true
}

// audit:ok 2026-05-30

// ExampleHolidayType_IsKnown demonstrates the closed-set membership check
// for HolidayType — callers SHOULD gate a switch on Holiday.Type with
// IsKnown so the unknown-value path (upstream schema drift) is explicit.
func ExampleHolidayType_IsKnown() {
	fmt.Println(openholidays.HolidayType("Public").IsKnown(), openholidays.HolidayType("Bogus").IsKnown())
	// Output: true false
}

// audit:ok 2026-05-30

// ExampleNewDate demonstrates the UTC-midnight Date constructor. Date.String
// emits the YYYY-MM-DD wire format used by the upstream API.
func ExampleNewDate() {
	d := openholidays.NewDate(2025, time.December, 24)
	fmt.Println(d)
	// Output: 2025-12-24
}

// audit:ok 2026-05-30

// ExampleParseDate demonstrates parsing a YYYY-MM-DD wire string into a
// UTC-midnight Date. Empty input and malformed strings return a wrapped
// error (see Date.UnmarshalJSON for the JSON form).
func ExampleParseDate() {
	d, err := openholidays.ParseDate("2025-12-24")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(d)
	// Output: 2025-12-24
}
