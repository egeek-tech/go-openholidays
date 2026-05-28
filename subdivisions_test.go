// Package openholidays — tests for the Subdivisions endpoint method.
//
// One TestXxx per exported production function per Gold Rule 3. Every
// scenario lives in a t.Run subtest. Non-English strings in the fixture
// (e.g. "Śląskie", "Dolny Śląsk") mirror real upstream OpenHolidays
// responses and are admitted per CONVENTIONS.md Rule 1 testdata-fixture
// exception.

package openholidays

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subdivisionsPLFixtureCapturedAt records the date testdata/subdivisions_pl.json
// AND testdata/subdivisions_de.json were captured from the live API (D-69).
// Re-capture when the upstream schema is suspected to have drifted. The
// fixtures are not the authoritative shape — the live API is.
const subdivisionsPLFixtureCapturedAt = "2026-05-27"

// TestClient_Subdivisions covers ENDPT-03 + TEST-01 + Assumption A3 closure:
//
//   - happy path PL (16 województwa, flat) — fixture replay + query-param
//     contract assertion on countryIsoCode + path
//   - happy path DE (16 Bundesländer, ≥ 1 children-bearing entry — the
//     hierarchical shape Plan 7's Client.IsInRegion consumes)
//   - empty CountryIsoCode → ErrInvalidCountry without HTTP (D-56)
//   - invalid LanguageIsoCode → ErrInvalidLanguage without HTTP (D-56)
//   - LanguageIsoCode canonicalized to lowercase on the wire (D-55)
//   - 4xx → *APIError with Path "/Subdivisions"
//   - 5xx → *APIError with title-fallback Message
//   - malformed JSON → decode error (not a sentinel)
//   - ctx cancel → [context.Canceled] within ≤ 100 ms (CLIENT-09)
//
// Gold Rule 3: exactly one TestClient_Subdivisions; every case is a t.Run.
func TestClient_Subdivisions(t *testing.T) {
	t.Parallel()

	t.Run("happy path PL returns 16 województwa with query contract", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "subdivisions_pl.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 03-03 Task 1 (captured %s)", subdivisionsPLFixtureCapturedAt)
		t.Logf("fixture captured %s", subdivisionsPLFixtureCapturedAt)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Query-param contract assertion: the wire format the upstream
			// expects from Subdivisions is /Subdivisions?countryIsoCode=PL.
			assert.Equal(t, "/Subdivisions", r.URL.Path,
				"path must be /Subdivisions (URL builder regression)")
			assert.Equal(t, "PL", r.URL.Query().Get("countryIsoCode"),
				"countryIsoCode must be canonicalized to uppercase PL")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		got, err := c.Subdivisions(t.Context(), SubdivisionsRequest{CountryIsoCode: "PL"})
		require.NoError(t, err)
		// D-70 sanity floor: PL fixture must contain all 16 województwa.
		require.Len(t, got, 16, "PL fixture must contain all 16 województwa (D-70 locked count)")
		for _, s := range got {
			assert.True(t, strings.HasPrefix(s.Code, "PL-"),
				"every PL subdivision Code must start with PL- prefix, got %q", s.Code)
		}
	})

	t.Run("happy path DE includes at least one entry with non-empty Children (Assumption A3)", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "subdivisions_de.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 03-03 Task 1 (captured %s)", subdivisionsPLFixtureCapturedAt)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/Subdivisions", r.URL.Path)
			assert.Equal(t, "DE", r.URL.Query().Get("countryIsoCode"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		got, err := c.Subdivisions(t.Context(), SubdivisionsRequest{CountryIsoCode: "DE"})
		require.NoError(t, err)
		require.Len(t, got, 16, "DE fixture must contain all 16 Bundesländer")

		// Assumption A3: the DE fixture must contain at least one Subdivision
		// whose Children slice is non-empty. Plan 7's Client.IsInRegion walks
		// that recursive tree to test hierarchical region membership; without
		// this entry, the hierarchical test path has no authentic data.
		var withChildren []string
		for _, s := range got {
			if len(s.Children) > 0 {
				withChildren = append(withChildren, s.Code)
			}
		}
		require.NotEmpty(t, withChildren,
			"DE fixture must contain a Children-bearing entry — Plan 7 depends on this for its hierarchical test (Assumption A3)")
		t.Logf("DE subdivisions with non-empty Children: %v", withChildren)
	})

	t.Run("empty CountryIsoCode wraps ErrInvalidCountry without HTTP", func(t *testing.T) {
		t.Parallel()
		// http://example.invalid is RFC 6761 reserved; if the validator
		// failed to short-circuit, the HTTP dispatch would fail loudly.
		c := NewClient(WithBaseURL("http://example.invalid"))
		got, err := c.Subdivisions(t.Context(), SubdivisionsRequest{})
		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, ErrInvalidCountry,
			"expected ErrInvalidCountry via errors.Is, got %v", err)
	})

	t.Run("invalid LanguageIsoCode wraps ErrInvalidLanguage without HTTP", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		_, err := c.Subdivisions(t.Context(), SubdivisionsRequest{
			CountryIsoCode:  "PL",
			LanguageIsoCode: "x",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidLanguage,
			"expected ErrInvalidLanguage via errors.Is, got %v", err)
	})

	t.Run("lowercased languageIsoCode reaches the wire", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "subdivisions_pl.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 03-03 Task 1")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The caller passes "EN" (uppercase); validateLanguage must
			// canonicalize to lowercase before url.Values.Set runs.
			assert.Equal(t, "en", r.URL.Query().Get("languageIsoCode"),
				"languageIsoCode must be canonicalized to lowercase before reaching the wire")
			assert.Equal(t, "PL", r.URL.Query().Get("countryIsoCode"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err = c.Subdivisions(t.Context(), SubdivisionsRequest{
			CountryIsoCode:  "PL",
			LanguageIsoCode: "EN",
		})
		require.NoError(t, err)
	})

	t.Run("4xx returns *APIError with Path /Subdivisions", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail": "Country not supported"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		got, err := c.Subdivisions(t.Context(), SubdivisionsRequest{CountryIsoCode: "PL"})
		require.Error(t, err)
		assert.Nil(t, got)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 404, apiErr.StatusCode)
		assert.Equal(t, "/Subdivisions", apiErr.Path)
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
		_, err := c.Subdivisions(t.Context(), SubdivisionsRequest{CountryIsoCode: "PL"})
		require.Error(t, err)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 500, apiErr.StatusCode)
		assert.Equal(t, "/Subdivisions", apiErr.Path)
		assert.Equal(t, "Internal Server Error", apiErr.Message,
			"title must win when detail is absent")
	})

	t.Run("malformed JSON returns decode error", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`not valid`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Subdivisions(t.Context(), SubdivisionsRequest{CountryIsoCode: "PL"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode /Subdivisions",
			"decode error must carry the /Subdivisions path in its prefix")
		// Must NOT match any of the typed sentinels — a malformed body is a
		// generic JSON decode failure (Phase 3 D-65's ErrMalformedResponse is
		// reserved for *post*-decode Holiday-content checks, not for syntax
		// errors).
		require.NotErrorIs(t, err, ErrEmptyResponse,
			"malformed JSON must not match ErrEmptyResponse")
		require.NotErrorIs(t, err, ErrResponseTooLarge,
			"malformed JSON must not match ErrResponseTooLarge")
		require.NotErrorIs(t, err, ErrInvalidCountry,
			"malformed JSON must not match ErrInvalidCountry")
		assert.NotErrorIs(t, err, ErrInvalidLanguage,
			"malformed JSON must not match ErrInvalidLanguage")
	})

	t.Run("ctx cancel returns context.Canceled", func(t *testing.T) {
		t.Parallel()
		// Handler sleeps so the in-flight HTTP is still running when the
		// caller cancels ctx; PROJECT.md mandates cancellation within ≤ 100 ms.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(200 * time.Millisecond):
				// Cancellation should fire before this deadline.
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("[]"))
			}
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		ctx, cancel := context.WithCancel(t.Context())
		// Cancel after a small delay so the request has dispatched.
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		_, err := c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: "PL"})
		elapsed := time.Since(start)
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled,
			"expected context.Canceled via errors.Is, got %v", err)
		// IN-03: this is the contract-locking ctx-cancel test for the
		// CLIENT-09 ≤ 100 ms interruption bound. The ceiling is
		// 20 ms (cancel-after delay) + 100 ms (CLIENT-09 budget) + 30 ms
		// (scheduler slack) = 150 ms. Sibling tests in countries/languages
		// have intentionally looser ceilings to absorb broader CI flake
		// (see countries_test.go WR-09 comment); this one stays tight so
		// the headline 100 ms contract has a regression detector.
		assert.LessOrEqual(t, elapsed, 150*time.Millisecond,
			"cancellation must interrupt in-flight HTTP within ≤ 100 ms (CLIENT-09); ceiling 150 ms = 20 ms cancel-delay + 100 ms target + 30 ms scheduler slack; elapsed=%s", elapsed)
	})
}
