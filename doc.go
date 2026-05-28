// Package openholidays is a Go client for the OpenHolidays public-holidays
// and school-holidays API (https://www.openholidaysapi.org).
//
// The library exposes public holidays, school holidays, country and language
// metadata, and administrative subdivisions through a clean, well-tested
// Go-first API. It is designed for backend engineers building HR, scheduling,
// education, and leave-management applications — including those that need
// regional school-break granularity (for example, Polish ferie per
// województwo) that competing libraries do not cover.
//
// Design principles:
//   - Zero runtime dependencies (no non-stdlib import outside *_test.go).
//   - Full context.Context propagation on every exported call.
//   - Typed errors inspectable via errors.Is / errors.As.
//
// Quickstart:
//
//	c := openholidays.NewClient()
//	defer c.Close()
//	hs, err := c.PublicHolidays(ctx, openholidays.PublicHolidaysRequest{
//	    CountryIsoCode: "PL",
//	    ValidFrom:      openholidays.NewDate(2025, time.January, 1),
//	    ValidTo:        openholidays.NewDate(2025, time.December, 31),
//	})
//
// See package examples on pkg.go.dev for every exported method — the
// runnable form lives in example_test.go as Example_quickstart and the
// per-method Example_<Symbol> functions.
package openholidays
