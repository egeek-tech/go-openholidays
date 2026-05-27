// Package openholidays — Languages endpoint method.
//
// This file ships only the Languages endpoint method and its associated
// LanguagesRequest type. The HTTP-and-decode pipeline lives in request.go
// (D-62 / D-63); Languages dispatches through the generic doJSONGet
// helper declared there, instantiated with the []Language result type.
//
// The signature follows the uniform (ctx, Request) shape that every
// Phase 3 endpoint method shares (D-51 / D-52 / CL-08). The zero-value
// LanguagesRequest{} reproduces the upstream's unfiltered /Languages
// response verbatim and is the recommended call shape when no filter
// is needed.

package openholidays

import (
	"context"
	"net/url"
)

// LanguagesRequest carries the optional filter exposed by the upstream
// /Languages endpoint. The zero value (LanguagesRequest{}) requests the
// upstream's unfiltered list and is the recommended call shape when no
// filter is needed.
//
// Fields:
//
//   - LanguageIsoCode is an optional ISO 639-1 two-letter language code
//     (case-insensitive; canonicalized to lowercase before being sent on
//     the wire). When non-empty, the request includes the corresponding
//     languageIsoCode query parameter and the upstream returns only the
//     localized Language.Name entries in that language. When empty, the
//     parameter is omitted and the upstream returns all localized names
//     for each Language (D-54 / D-55 / CL-13).
//
// Validation: non-empty LanguageIsoCode is validated client-side via
// validateLanguage (D-56) before any HTTP request is made; a malformed
// value returns an error wrapping ErrInvalidLanguage without reaching the
// network.
type LanguagesRequest struct {
	// LanguageIsoCode is an optional ISO 639-1 language filter.
	LanguageIsoCode string
}

// Languages fetches the list of supported languages from the upstream
// OpenHolidays API. Each returned Language carries an IsoCode (ISO 639-1
// lowercase on the wire) and a per-language localized Name array (look
// up a specific language via Language.NameFor).
//
// Request shape: Languages takes a LanguagesRequest second argument so its
// signature is symmetric with every other Phase 3 endpoint (Countries,
// Subdivisions, PublicHolidays, SchoolHolidays). The zero value
// LanguagesRequest{} reproduces the upstream's unfiltered /Languages
// behavior (D-51 / D-52 / CL-08). The optional LanguageIsoCode filter
// restricts the returned Language.Name entries to that language only.
//
// Per-request timeout: when the Client was constructed with WithTimeout(d)
// and d > 0, Languages wraps ctx via context.WithTimeout(ctx, d) before
// dispatching. Cancellation of the caller's ctx interrupts the in-flight
// HTTP within ≤ 100 ms (CLIENT-09); errors.Is(err, context.Canceled) holds
// through the fmt.Errorf %w wrap returned on transport-level failures.
//
// Error handling:
//
//   - A non-empty req.LanguageIsoCode that fails client-side shape
//     validation returns an error wrapping ErrInvalidLanguage without
//     reaching the network (D-56).
//   - 4xx and 5xx upstream responses produce *APIError with the StatusCode,
//     a parsed Message (RFC 7807 ProblemDetails priority: detail → title →
//     error), and the raw response body capped at 4 KiB (Phase 1 D-17).
//     Use errors.As(err, &apiErr) to recover the populated value.
//   - 2xx with an empty body returns an error that errors.Is matches against
//     ErrEmptyResponse.
//   - Upstream responses exceeding the 10 MiB cap return an error that
//     errors.Is matches against ErrResponseTooLarge.
//   - JSON decode failures wrap the underlying error with the
//     "openholidays: decode /Languages: " prefix.
//
// Concurrent use: the Client is immutable after NewClient, so Languages is
// safe to call from any goroutine without external synchronization
// (CLIENT-07).
func (c *Client) Languages(ctx context.Context, req LanguagesRequest) ([]Language, error) {
	q := url.Values{}
	if req.LanguageIsoCode != "" {
		lang, err := validateLanguage(req.LanguageIsoCode)
		if err != nil {
			return nil, err
		}
		q.Set("languageIsoCode", lang)
	}
	return doJSONGet[[]Language](ctx, c, "/Languages", q)
}
