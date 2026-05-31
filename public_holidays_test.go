// Package openholidays — tests for the PublicHolidays endpoint method and
// the validateHolidays post-decode helper.
//
// One TestXxx per exported production function per Gold Rule 3:
// TestClient_PublicHolidays covers the endpoint method;
// TestValidateHolidays covers the unexported validateHolidays helper
// (the helper is private to the package and Gold Rule 3 still applies for
// every package function the rest of the package exercises — the test
// pins the invariant set in isolation from the HTTP pipeline).
//
// Every scenario lives in a t.Run subtest. Non-English strings in the
// fixture (e.g. "Wigilia Bożego Narodzenia", "Trzech Króli") mirror real
// upstream OpenHolidays responses and are admitted per CONVENTIONS.md Rule
// 1 testdata-fixture exception.

package openholidays

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publicHolidaysPL2025FixtureCapturedAt records the date
// testdata/public_holidays_pl_2025.json was captured from the live API.
// Re-capture when the upstream schema is suspected to have drifted. The
// fixture is not the authoritative shape — the live API is. D-69.
const publicHolidaysPL2025FixtureCapturedAt = "2026-05-27"

// audit:ok 2026-05-31

// TestClient_PublicHolidays covers ENDPT-04 + TEST-01 (4 error paths per
// endpoint) + the D-70 sanity assertions on the live PL 2025 fixture +
// the new ErrMalformedResponse subtest gated by CL-12.
//
// Gold Rule 3: exactly one TestClient_PublicHolidays; every case is a
// t.Run. require for preconditions, assert for verifications.
func TestClient_PublicHolidays(t *testing.T) {
	t.Parallel()

	t.Run("happy path PL 2025 returns 14 holidays incl. Wigilia 2025-12-24", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "public_holidays_pl_2025.json"))
		require.NoError(t, err, "fixture missing — re-capture from live API per Plan 03-04 Task 2 (captured %s)",
			publicHolidaysPL2025FixtureCapturedAt)
		t.Logf("fixture captured %s", publicHolidaysPL2025FixtureCapturedAt)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// URL-builder contract assertions (RESEARCH.md Pattern 3):
			// asserting inside the handler catches a regression that
			// mis-spells a query-param key — without these the test
			// would pass on the fixture body regardless of what the
			// client sent.
			assert.Equal(t, "/PublicHolidays", r.URL.Path)
			q := r.URL.Query()
			assert.Equal(t, "PL", q.Get("countryIsoCode"),
				"country code must be uppercased canonical form")
			assert.Equal(t, "2025-01-01", q.Get("validFrom"))
			assert.Equal(t, "2025-12-31", q.Get("validTo"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		holidays, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		require.Len(t, holidays, 14, "fixture captured %s — re-capture if upstream shape drifted",
			publicHolidaysPL2025FixtureCapturedAt)

		// D-70 sanity assert: locate Wigilia Bożego Narodzenia by its
		// Polish localized name and verify the StartDate. The literal
		// string carries Polish diacritics because it mirrors real
		// upstream bytes (CONVENTIONS.md Rule 1 testdata exception).
		var wigilia *Holiday
		for i := range holidays {
			if name, ok := holidays[i].NameFor("pl"); ok && name == "Wigilia Bożego Narodzenia" {
				wigilia = &holidays[i]
				break
			}
		}
		require.NotNil(t, wigilia,
			"Wigilia Bożego Narodzenia not found in PL 2025 fixture (captured %s)",
			publicHolidaysPL2025FixtureCapturedAt)
		assert.True(t, wigilia.StartDate.Equal(NewDate(2025, time.December, 24)),
			"Wigilia must start on 2025-12-24, got %s", wigilia.StartDate)
	})

	t.Run("validation error: empty CountryIsoCode wraps ErrInvalidCountry", func(t *testing.T) {
		t.Parallel()
		// http://example.invalid is RFC 6761 reserved; if the validator
		// failed to short-circuit, the HTTP dispatch would fail loudly.
		c := NewClient(WithBaseURL("http://example.invalid"))
		holidays, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			ValidFrom: NewDate(2025, time.January, 1),
			ValidTo:   NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.Nil(t, holidays)
		assert.ErrorIs(t, err, ErrInvalidCountry,
			"expected ErrInvalidCountry via errors.Is, got %v", err)
	})

	t.Run("validation error: from > to wraps ErrInvalidDateRange", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		holidays, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2026, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.Nil(t, holidays)
		assert.ErrorIs(t, err, ErrInvalidDateRange,
			"expected ErrInvalidDateRange via errors.Is, got %v", err)
	})

	t.Run("validation error: window > 3 years wraps ErrDateRangeTooLarge", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		holidays, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2020, time.January, 1),
			ValidTo:        NewDate(2025, time.January, 1),
		})
		require.Error(t, err)
		assert.Nil(t, holidays)
		assert.ErrorIs(t, err, ErrDateRangeTooLarge,
			"expected ErrDateRangeTooLarge via errors.Is, got %v", err)
	})

	t.Run("validation error: invalid LanguageIsoCode wraps ErrInvalidLanguage", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		holidays, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode:  "PL",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
			LanguageIsoCode: "X", // not 2 ASCII letters
		})
		require.Error(t, err)
		assert.Nil(t, holidays)
		assert.ErrorIs(t, err, ErrInvalidLanguage,
			"expected ErrInvalidLanguage via errors.Is, got %v", err)
	})

	t.Run("4xx returns *APIError with detail Message", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail": "Country not supported"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		holidays, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.Nil(t, holidays)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 404, apiErr.StatusCode)
		assert.Equal(t, "/PublicHolidays", apiErr.Path)
		assert.Equal(t, "Country not supported", apiErr.Message)
	})

	t.Run("5xx with title fallback", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"title": "Internal Server Error"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 500, apiErr.StatusCode)
		assert.Equal(t, "/PublicHolidays", apiErr.Path)
		assert.Equal(t, "Internal Server Error", apiErr.Message,
			"title must win when detail is absent")
	})

	t.Run("malformed JSON wraps ErrMalformedResponse", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"this": "is not an array of Holiday"`)) // missing closing brace
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		// A malformed body now matches the single ErrMalformedResponse sentinel
		// (syntax/type errors and post-decode schema-drift alike); the underlying
		// *json.SyntaxError / *json.UnmarshalTypeError stays recoverable via
		// errors.As. It must NOT match the other typed sentinels.
		require.NotErrorIs(t, err, ErrEmptyResponse)
		require.NotErrorIs(t, err, ErrResponseTooLarge)
		require.ErrorIs(t, err, ErrMalformedResponse)
		assert.NotErrorIs(t, err, ErrInvalidCountry)
	})

	t.Run("ctx cancel returns context.Canceled", func(t *testing.T) {
		t.Parallel()
		// Handler blocks until the client disconnects; the test cancels
		// ctx after a short delay, exercising the in-flight cancellation
		// path (CLIENT-09: within ≤ 100 ms).
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		t.Cleanup(srv.Close)

		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled,
			"expected context.Canceled via errors.Is, got %v", err)
	})

	t.Run("malformed holiday (EndDate before StartDate) wraps ErrMalformedResponse", func(t *testing.T) {
		t.Parallel()
		// Server returns a structurally-valid Holiday JSON that
		// nonetheless violates the validateHolidays invariant: EndDate
		// strictly before StartDate. "type":"Public" matches
		// HolidayTypePublic (verified in types.go line 29) so decoding
		// succeeds and validateHolidays is reached. This is the CL-12
		// contract: callers branch on errors.Is(err, ErrMalformedResponse).
		bad := `[{
			"id":"bad-uuid","startDate":"2025-12-25","endDate":"2025-01-01",
			"type":"Public","name":[{"language":"en","text":"X"}],
			"nationwide":true,"regionalScope":"National","temporalScope":"FullDay"
		}]`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(bad))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		holidays, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.Nil(t, holidays, "endpoint must return nil holidays on validateHolidays failure")
		assert.ErrorIs(t, err, ErrMalformedResponse,
			"expected ErrMalformedResponse via errors.Is, got %v", err)
	})

	t.Run("optional SubdivisionCode reaches the wire when non-empty", func(t *testing.T) {
		t.Parallel()
		body, err := os.ReadFile(filepath.Join("testdata", "public_holidays_pl_2025.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 03-04 Task 2")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "PL-SL", r.URL.Query().Get("subdivisionCode"),
				"SubdivisionCode must be passed through verbatim per D-56")
			assert.Equal(t, "/PublicHolidays", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		holidays, err := c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode:  "PL",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
			SubdivisionCode: "PL-SL",
		})
		require.NoError(t, err)
		assert.Len(t, holidays, 14)
	})

	t.Run("empty SubdivisionCode is omitted from query", func(t *testing.T) {
		t.Parallel()
		body, err := os.ReadFile(filepath.Join("testdata", "public_holidays_pl_2025.json"))
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Empty(t, r.URL.Query().Get("subdivisionCode"),
				"empty SubdivisionCode must not produce a subdivisionCode query parameter")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err = c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
	})

	t.Run("optional LanguageIsoCode is canonicalized to uppercase on the wire", func(t *testing.T) {
		t.Parallel()
		body, err := os.ReadFile(filepath.Join("testdata", "public_holidays_pl_2025.json"))
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "PL", r.URL.Query().Get("languageIsoCode"),
				"LanguageIsoCode must be canonicalized to uppercase per validateLanguage")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err = c.PublicHolidays(t.Context(), PublicHolidaysRequest{
			CountryIsoCode:  "PL",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
			LanguageIsoCode: "pl", // lowercase input → strings.ToUpper must lift it to "PL"
		})
		require.NoError(t, err)
	})
}

// audit:ok 2026-05-30

// TestValidateHolidays exercises the validateHolidays helper in request.go
// in isolation from the HTTP pipeline (Gold Rule 3 dedicated function per
// exported / package-internal production function under test). The
// invariants pinned here are the contract that the endpoint method's
// post-decode pass relies on.
func TestValidateHolidays(t *testing.T) {
	t.Parallel()

	t.Run("valid slice of three holidays returns nil", func(t *testing.T) {
		t.Parallel()
		hs := []Holiday{
			{
				ID:        "a",
				StartDate: NewDate(2025, time.January, 1),
				EndDate:   NewDate(2025, time.January, 1),
			},
			{
				ID:        "b",
				StartDate: NewDate(2025, time.June, 15),
				EndDate:   NewDate(2025, time.June, 16),
			},
			{
				ID:        "c",
				StartDate: NewDate(2025, time.December, 25),
				EndDate:   NewDate(2025, time.December, 26),
			},
		}
		err := validateHolidays(hs, "/PublicHolidays")
		assert.NoError(t, err)
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		t.Parallel()
		err := validateHolidays(nil, "/PublicHolidays")
		assert.NoError(t, err, "validateHolidays must accept nil/empty slices")
	})

	t.Run("zero StartDate wraps ErrMalformedResponse with the holiday ID", func(t *testing.T) {
		t.Parallel()
		hs := []Holiday{
			{
				ID:        "valid-prefix",
				StartDate: NewDate(2025, time.January, 1),
				EndDate:   NewDate(2025, time.January, 1),
			},
			{
				ID:      "deadbeef-zero-start",
				EndDate: NewDate(2025, time.June, 15),
				// StartDate is the zero Date — Date.IsZero() == true.
			},
		}
		err := validateHolidays(hs, "/PublicHolidays")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMalformedResponse,
			"expected ErrMalformedResponse via errors.Is, got %v", err)
		assert.Contains(t, err.Error(), "deadbeef-zero-start",
			"error message must include the offending holiday's ID")
		assert.Contains(t, err.Error(), "zero StartDate",
			"error message must name the failing predicate")
	})

	t.Run("zero EndDate wraps ErrMalformedResponse with the holiday ID", func(t *testing.T) {
		t.Parallel()
		hs := []Holiday{
			{
				ID:        "deadbeef-zero-end",
				StartDate: NewDate(2025, time.January, 1),
				// EndDate is the zero Date.
			},
		}
		err := validateHolidays(hs, "/PublicHolidays")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMalformedResponse,
			"expected ErrMalformedResponse via errors.Is, got %v", err)
		assert.Contains(t, err.Error(), "deadbeef-zero-end")
		assert.Contains(t, err.Error(), "zero EndDate")
	})

	t.Run("EndDate before StartDate wraps ErrMalformedResponse with both dates", func(t *testing.T) {
		t.Parallel()
		hs := []Holiday{
			{
				ID:        "deadbeef-out-of-order",
				StartDate: NewDate(2025, time.December, 25),
				EndDate:   NewDate(2025, time.January, 1),
			},
		}
		err := validateHolidays(hs, "/PublicHolidays")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMalformedResponse,
			"expected ErrMalformedResponse via errors.Is, got %v", err)
		assert.Contains(t, err.Error(), "deadbeef-out-of-order")
		assert.Contains(t, err.Error(), "2025-12-25",
			"error message must echo StartDate in YYYY-MM-DD form")
		assert.Contains(t, err.Error(), "2025-01-01",
			"error message must echo EndDate in YYYY-MM-DD form")
	})

	t.Run("single-day holiday EndDate==StartDate is accepted", func(t *testing.T) {
		t.Parallel()
		// The upstream emits single-day holidays as StartDate==EndDate.
		// validateHolidays must NOT reject this canonical shape.
		hs := []Holiday{
			{
				ID:        "single-day",
				StartDate: NewDate(2025, time.November, 11),
				EndDate:   NewDate(2025, time.November, 11),
			},
		}
		err := validateHolidays(hs, "/PublicHolidays")
		assert.NoError(t, err)
	})

	t.Run("path appears in the wrapped error message", func(t *testing.T) {
		t.Parallel()
		hs := []Holiday{
			{
				ID:      "x",
				EndDate: NewDate(2025, time.January, 1),
			},
		}
		err := validateHolidays(hs, "/SchoolHolidays")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/SchoolHolidays",
			"path argument must be echoed so multi-endpoint failures surface their origin")
	})
}
