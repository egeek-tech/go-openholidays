// Countries endpoint method.
//
// This file ships only the Countries endpoint method and its associated
// CountriesRequest type. The HTTP-and-decode pipeline that Phase 2 inlined
// here moved to request.go in Phase 3 (D-62 / D-63); Countries (and every
// other Phase 3 endpoint) now dispatches through the generic helper
// declared there.
//
// Phase 3 also retrofits the Phase 2 single-arg Countries signature
// to the uniform (ctx, CountriesRequest) shape that every endpoint
// method shares (D-51 / D-52 / CL-08). The zero-value CountriesRequest{}
// reproduces the Phase 2 observable behavior verbatim.

package openholidays

import (
	"context"
	"net/url"
)

// CountriesRequest carries the optional filter exposed by the upstream
// /Countries endpoint. The zero value (CountriesRequest{}) reproduces the
// Phase 2 unfiltered behavior verbatim and is the recommended call shape
// when no filter is needed.
//
// Fields:
//
//   - LanguageIsoCode is an optional ISO 639-1 two-letter language code
//     (case-insensitive; canonicalized to lowercase before being sent on
//     the wire). When non-empty, the request includes the corresponding
//     languageIsoCode query parameter and the upstream returns only the
//     localized Country.Name entries in that language. When empty, the
//     parameter is omitted and the upstream returns all localized names
//     for each Country (D-54 / D-55 / CL-13).
//
// Validation: non-empty LanguageIsoCode is validated client-side via
// validateLanguage (D-56) before any HTTP request is made; a malformed
// value returns an error wrapping ErrInvalidLanguage without reaching the
// network.
type CountriesRequest struct {
	// LanguageIsoCode is an optional ISO 639-1 language filter.
	LanguageIsoCode string
}

// audit:ok 2026-05-30

// Countries fetches the list of supported countries from the upstream
// OpenHolidays API. Each returned Country carries an IsoCode, a per-language
// localized Name array (look up a specific language via Country.NameFor),
// and the country's OfficialLanguages list.
//
// Request shape: Countries takes a CountriesRequest second argument so its
// signature is symmetric with every other Phase 3 endpoint (Languages,
// Subdivisions, PublicHolidays, SchoolHolidays). The zero value
// CountriesRequest{} reproduces the Phase 2 single-arg Countries(ctx)
// behavior verbatim (D-51 / D-52 / CL-08). The optional LanguageIsoCode
// filter restricts the returned Country.Name entries to that language only.
//
// Per-request timeout: when the Client was constructed with WithTimeout(d)
// and d > 0, Countries wraps ctx via [context.WithTimeout](ctx, d) before
// dispatching. Cancellation of the caller's ctx interrupts the in-flight
// HTTP within ≤ 100 ms (CLIENT-09); [errors.Is](err, [context.Canceled]) holds
// through the [fmt.Errorf] %w wrap returned on transport-level failures.
//
// Error handling:
//
//   - A non-empty req.LanguageIsoCode that fails client-side shape
//     validation returns an error wrapping ErrInvalidLanguage without
//     reaching the network (D-56).
//   - 4xx and 5xx upstream responses produce *APIError with the StatusCode,
//     a parsed Message (RFC 7807 ProblemDetails priority: detail → title →
//     error), and the raw response body capped at 4 KiB (Phase 1 D-17).
//     Use [errors.As](err, &apiErr) to recover the populated value.
//   - 2xx with an empty body returns an error that [errors.Is] matches against
//     ErrEmptyResponse.
//   - Upstream responses exceeding the 10 MiB cap return an error that
//     [errors.Is] matches against ErrResponseTooLarge.
//   - JSON decode failures wrap the underlying error with the
//     "openholidays: decode /Countries: " prefix.
//
// Concurrent use: the Client is immutable after NewClient, so Countries is
// safe to call from any goroutine without external synchronization
// (CLIENT-07).
func (c *Client) Countries(ctx context.Context, req CountriesRequest) ([]Country, error) {
	q := url.Values{}
	if req.LanguageIsoCode != "" {
		lang, err := validateLanguage(req.LanguageIsoCode)
		if err != nil {
			return nil, err
		}
		q.Set("languageIsoCode", lang)
	}
	return doJSONGet[[]Country](ctx, c, "/Countries", q)
}
