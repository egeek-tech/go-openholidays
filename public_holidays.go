// Package openholidays — PublicHolidays endpoint method.
//
// This file ships only the PublicHolidays endpoint method and its associated
// PublicHolidaysRequest type per D-64 (each endpoint file is scoped to its
// Request type, its endpoint method ≤ ~30 lines, and any endpoint-specific
// post-decode validation call). The shared HTTP-and-decode pipeline lives in
// request.go's doJSONGet (D-62 / D-63); the post-decode Holiday invariant
// check lives in request.go's validateHolidays (D-65 / CL-12); the three
// client-side input validators (validateCountry, validateLanguage,
// validateDateRange) live in validate.go.
//
// PublicHolidays is the first endpoint in Phase 3 to exercise the full
// "required + optional + post-decode" validator chain (D-54 / D-56) and is
// the canonical analog for the SchoolHolidays endpoint that lands later in
// Wave 1 with one additional optional field.
package openholidays

import (
	"context"
	"net/url"
)

// PublicHolidaysRequest carries the filters supported by the upstream
// /PublicHolidays endpoint. Fields mirror the upstream query parameters
// exactly (D-53 / CL-13): exposing every upstream-supported filter is the
// pattern every sibling Request struct in this phase follows.
//
// Fields:
//
//   - CountryIsoCode is the required ISO 3166-1 alpha-2 country code
//     (case-insensitive; canonicalized to uppercase before being sent on
//     the wire). Empty or malformed values return an error wrapping
//     ErrInvalidCountry without dispatching the HTTP request (D-56).
//   - ValidFrom is the required inclusive lower bound of the date window
//     (YYYY-MM-DD; UTC midnight). Must be ≤ ValidTo and within 3 calendar
//     years of ValidTo (validateDateRange / D-22 / VALID-02 / VALID-03).
//   - ValidTo is the required inclusive upper bound of the date window
//     (YYYY-MM-DD; UTC midnight). See ValidFrom.
//   - LanguageIsoCode is an optional ISO 639-1 two-letter language code
//     (case-insensitive; canonicalized to lowercase before being sent on
//     the wire). When non-empty, restricts the localized Holiday.Name
//     entries upstream returns to that language only. When empty, the
//     parameter is omitted (D-55 / D-56) and the upstream returns all
//     localized names.
//   - SubdivisionCode is an optional administrative subdivision code (e.g.
//     "PL-SL" for Śląskie). Shape-tolerant per D-56: no client-side
//     validator runs; the value is passed through verbatim and the
//     upstream is the authoritative source on which codes it accepts.
//     When empty, the parameter is omitted.
type PublicHolidaysRequest struct {
	// CountryIsoCode is the required ISO 3166-1 alpha-2 country code.
	CountryIsoCode string
	// ValidFrom is the required inclusive lower bound of the date window.
	ValidFrom Date
	// ValidTo is the required inclusive upper bound of the date window.
	ValidTo Date
	// LanguageIsoCode is an optional ISO 639-1 language filter.
	LanguageIsoCode string
	// SubdivisionCode is an optional subdivision-code filter, passed
	// through to the upstream verbatim with no client-side shape check.
	SubdivisionCode string
}

// PublicHolidays fetches the list of public (statutory) holidays for a
// country in a date window from the upstream OpenHolidays API. Each
// returned Holiday carries an ID, a StartDate/EndDate pair (Date.Equal for
// single-day holidays), a Type, a per-language localized Name array
// (look up a specific language via Holiday.NameFor), Nationwide /
// RegionalScope / TemporalScope flags, and the optional Subdivisions /
// Groups / Tags / Comment / Quality fields when the upstream populates them.
//
// Request shape: PublicHolidays takes a PublicHolidaysRequest second
// argument symmetric with every other Phase 3 endpoint method (Countries,
// Languages, Subdivisions, SchoolHolidays). The uniform (ctx, Request)
// shape is locked by D-51 / CL-08.
//
// Per-request timeout: when the Client was constructed with WithTimeout(d)
// and d > 0, PublicHolidays wraps ctx via context.WithTimeout(ctx, d)
// before dispatching. Cancellation of the caller's ctx interrupts the
// in-flight HTTP within ≤ 100 ms (CLIENT-09); errors.Is(err,
// context.Canceled) holds through the fmt.Errorf %w wrap returned on
// transport-level failures.
//
// Error handling:
//
//   - An empty or malformed req.CountryIsoCode returns an error wrapping
//     ErrInvalidCountry without reaching the network (D-56).
//   - req.ValidFrom after req.ValidTo returns an error wrapping
//     ErrInvalidDateRange (D-22 / VALID-02). A window spanning more than
//     3 calendar years (anchored at ValidTo, stepping backward) returns an
//     error wrapping ErrDateRangeTooLarge (D-22 / VALID-03).
//   - A non-empty req.LanguageIsoCode that fails shape validation returns
//     an error wrapping ErrInvalidLanguage without reaching the network.
//   - 4xx and 5xx upstream responses produce *APIError with the StatusCode,
//     a parsed Message (RFC 7807 ProblemDetails priority: detail → title →
//     error), and the raw response body capped at 4 KiB (Phase 1 D-17).
//     Use errors.As(err, &apiErr) to recover the populated value.
//   - 2xx with an empty body returns an error that errors.Is matches
//     against ErrEmptyResponse.
//   - Upstream responses exceeding the 10 MiB cap return an error that
//     errors.Is matches against ErrResponseTooLarge.
//   - A structurally-decodable response that violates the Holiday
//     post-decode invariants (zero StartDate, zero EndDate, or EndDate
//     strictly before StartDate) returns an error that errors.Is matches
//     against ErrMalformedResponse (D-65 / D-66 / CL-12). The error
//     message includes the offending Holiday's ID and the failing
//     predicate so an upstream-regression bug report has actionable
//     diagnostics.
//   - JSON decode failures wrap the underlying error with the
//     "openholidays: decode /PublicHolidays: " prefix.
//
// Concurrent use: the Client is immutable after NewClient, so PublicHolidays
// is safe to call from any goroutine without external synchronization
// (CLIENT-07).
func (c *Client) PublicHolidays(ctx context.Context, req PublicHolidaysRequest) ([]Holiday, error) {
	country, err := validateCountry(req.CountryIsoCode)
	if err != nil {
		return nil, err
	}
	if err := validateDateRange(req.ValidFrom, req.ValidTo); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("countryIsoCode", country)
	q.Set("validFrom", req.ValidFrom.String())
	q.Set("validTo", req.ValidTo.String())
	if req.LanguageIsoCode != "" {
		lang, err := validateLanguage(req.LanguageIsoCode)
		if err != nil {
			return nil, err
		}
		q.Set("languageIsoCode", lang)
	}
	if req.SubdivisionCode != "" {
		q.Set("subdivisionCode", req.SubdivisionCode)
	}
	holidays, err := doJSONGet[[]Holiday](ctx, c, "/PublicHolidays", q)
	if err != nil {
		return nil, err
	}
	if err := validateHolidays(holidays, "/PublicHolidays"); err != nil {
		return nil, err
	}
	return holidays, nil
}
