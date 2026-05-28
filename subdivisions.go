// Package openholidays — Subdivisions endpoint method.
//
// This file ships only the Subdivisions endpoint method and its associated
// SubdivisionsRequest type. The HTTP-and-decode pipeline lives in request.go's
// doJSONGet[T] (D-62 / D-63); Subdivisions builds a query, validates the
// inputs client-side per D-56's matrix, and dispatches through the shared
// helper.
//
// File scope is intentionally narrow (D-64): the SubdivisionsRequest type
// and the endpoint method (≤ 30 lines). No HTTP plumbing is duplicated here.

package openholidays

import (
	"context"
	"net/url"
)

// SubdivisionsRequest carries the inputs for the /Subdivisions endpoint.
//
// Fields:
//
//   - CountryIsoCode is REQUIRED. It is the ISO 3166-1 alpha-2 country code
//     (case-insensitive; canonicalized to uppercase before being sent on the
//     wire). The validator runs on every call; an empty or malformed value
//     returns an error wrapping ErrInvalidCountry without reaching the
//     network (D-56).
//
//   - LanguageIsoCode is OPTIONAL. It is the ISO 639-1 two-letter language
//     code (case-insensitive; canonicalized to lowercase before being sent
//     on the wire). When non-empty, the upstream returns only the localized
//     Subdivision.Name / Subdivision.Category / Subdivision.Comment entries
//     in that language. When empty, the parameter is omitted and the
//     upstream returns every supported language for each Subdivision (D-54 /
//     D-55 / CL-13).
//
// Validation: a non-empty LanguageIsoCode is validated client-side via
// validateLanguage (D-56) before any HTTP request is made; a malformed value
// returns an error wrapping ErrInvalidLanguage without reaching the network.
type SubdivisionsRequest struct {
	// CountryIsoCode is the required ISO 3166-1 alpha-2 country code.
	CountryIsoCode string
	// LanguageIsoCode is the optional ISO 639-1 language filter.
	LanguageIsoCode string
}

// Subdivisions fetches the administrative subdivisions of the country named by
// req.CountryIsoCode from the upstream OpenHolidays API. Each returned
// Subdivision carries a Code (e.g. "PL-SL"), a per-language localized Name
// array (look up a specific language via Subdivision.NameFor), a Category
// label, and — for countries whose subdivisions are organized hierarchically
// (e.g. Germany at the Bundesländer→Regierungsbezirke level) — a recursive
// Children slice referencing the same Subdivision type.
//
// Per-request timeout: when the Client was constructed with WithTimeout(d)
// and d > 0, Subdivisions wraps ctx via context.WithTimeout(ctx, d) before
// dispatching. Cancellation of the caller's ctx interrupts the in-flight
// HTTP within ≤ 100 ms (CLIENT-09); errors.Is(err, context.Canceled) holds
// through the fmt.Errorf %w wrap returned on transport-level failures.
//
// Error handling:
//
//   - An empty or malformed req.CountryIsoCode returns an error wrapping
//     ErrInvalidCountry without reaching the network (D-56).
//   - A non-empty req.LanguageIsoCode that fails client-side shape validation
//     returns an error wrapping ErrInvalidLanguage without reaching the
//     network (D-56).
//   - 4xx and 5xx upstream responses produce *APIError with the StatusCode,
//     a parsed Message (RFC 7807 ProblemDetails priority: detail → title →
//     error), and the raw response body capped at 4 KiB (Phase 1 D-17).
//     Use errors.As(err, &apiErr) to recover the populated value.
//   - 2xx with an empty body returns an error that errors.Is matches against
//     ErrEmptyResponse.
//   - Upstream responses exceeding the 10 MiB cap return an error that
//     errors.Is matches against ErrResponseTooLarge.
//   - JSON decode failures wrap the underlying error with the
//     "openholidays: decode /Subdivisions: " prefix.
//
// Recursion: Subdivision.Children is a (potentially deeply nested) slice of
// the same Subdivision type. The recursive shape is the reason Client.IsInRegion
// exists (CL-09) — given a leaf-level region code like "DE-BY-AU", it walks
// the parent chain to detect membership when only the parent code (e.g.
// "DE-BY") appears in Holiday.Subdivisions. See Client.IsInRegion godoc.
//
// Concurrent use: the Client is immutable after NewClient, so Subdivisions
// is safe to call from any goroutine without external synchronization
// (CLIENT-07).
//
// Trust model (IN-04): the upstream is assumed to return only subdivisions
// belonging to the requested country. The library does NOT post-decode-verify
// the country prefix on Subdivision.Code values; a hostile or buggy upstream
// that returns mixed-country codes would produce undefined behavior in
// downstream helpers (in particular Client.IsInRegion's hierarchical walk,
// which keys its parent-index by Subdivision.Code). The current v0.x scope
// is intentionally trust-the-upstream; a post-decode country-prefix filter is
// a v0.2 deviation candidate.
func (c *Client) Subdivisions(ctx context.Context, req SubdivisionsRequest) ([]Subdivision, error) {
	country, err := validateCountry(req.CountryIsoCode)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("countryIsoCode", country)
	if req.LanguageIsoCode != "" {
		lang, err := validateLanguage(req.LanguageIsoCode)
		if err != nil {
			return nil, err
		}
		q.Set("languageIsoCode", lang)
	}
	return doJSONGet[[]Subdivision](ctx, c, "/Subdivisions", q)
}
