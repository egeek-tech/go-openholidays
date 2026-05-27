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
package openholidays
