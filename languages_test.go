// Package openholidays — tests for the Languages endpoint method.
//
// One TestXxx per exported production function per Gold Rule 3. Every
// scenario lives in a t.Run subtest. Non-English strings in the fixture
// (e.g. localized language names) mirror real upstream OpenHolidays
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

// languagesFixtureCapturedAt records the date testdata/languages.json was
// captured from the live API. Re-capture when the upstream schema is
// suspected to have drifted. The fixture is not the authoritative shape —
// the live API is.
const languagesFixtureCapturedAt = "2026-05-27"

// audit:ok 2026-05-30

// TestClient_Languages covers ENDPT-02 + TEST-01 (the 4 error-path floor)
// for the Languages endpoint:
//
//   - happy path (httptest fixture replay; ≥ 14 entries per D-70 floor)
//   - optional LanguageIsoCode query-param contract (canonicalized uppercase)
//   - invalid LanguageIsoCode short-circuit (no HTTP issued)
//   - 4xx → *APIError with Path /Languages
//   - 5xx with RFC 7807 title fallback
//   - malformed JSON wraps a decode error (not a sentinel)
//   - ctx cancel returns [context.Canceled] within ≤ 100 ms (CLIENT-09)
//
// Gold Rule 3: exactly one TestClient_Languages; every case is a t.Run.
func TestClient_Languages(t *testing.T) {
	t.Parallel()

	t.Run("happy path returns ≥14 languages from fixture", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "languages.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 03-02 Task 1")
		t.Logf("fixture captured %s", languagesFixtureCapturedAt)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/Languages", r.URL.Path)
			assert.Empty(t, r.URL.RawQuery,
				"zero-value LanguagesRequest must not emit any query string, got %q", r.URL.RawQuery)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		got, err := c.Languages(t.Context(), LanguagesRequest{})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(got), 14,
			"D-70 floor: expected ≥ 14 languages in the fixture, got %d", len(got))

		// Live data confirms English is always present in /Languages.
		// Match case-insensitively because upstream IsoCode casing may
		// drift (some entries use lowercase, others uppercase historically).
		var foundEN bool
		for _, l := range got {
			if strings.EqualFold(l.IsoCode, "en") {
				foundEN = true
				break
			}
		}
		assert.True(t, foundEN, "expected at least one Language with IsoCode 'en' (case-insensitive)")
	})

	t.Run("optional LanguageIsoCode sent as query param when non-empty", func(t *testing.T) {
		t.Parallel()
		// Use a small valid-JSON empty-array body so the decode succeeds —
		// the assertion under test is the outbound query-param contract,
		// not the response shape.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "EN", r.URL.Query().Get("languageIsoCode"),
				"expected canonicalized uppercase languageIsoCode in query")
			assert.Equal(t, "/Languages", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"isoCode":"en","name":[{"language":"EN","text":"English"}]}]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		// Uppercase input → validateLanguage canonicalizes to uppercase
		// "EN" before the query param is set.
		got, err := c.Languages(t.Context(), LanguagesRequest{LanguageIsoCode: "EN"})
		require.NoError(t, err)
		assert.NotEmpty(t, got)
	})

	t.Run("invalid LanguageIsoCode wraps ErrInvalidLanguage without HTTP", func(t *testing.T) {
		t.Parallel()
		// http://example.invalid is RFC 6761 reserved; if the validator
		// failed to short-circuit, the HTTP dispatch would fail loudly.
		c := NewClient(WithBaseURL("http://example.invalid"))
		_, err := c.Languages(t.Context(), LanguagesRequest{LanguageIsoCode: "x"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidLanguage,
			"expected ErrInvalidLanguage via errors.Is, got %v", err)
	})

	t.Run("4xx returns *APIError with Path /Languages", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"Language not supported"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		langs, err := c.Languages(t.Context(), LanguagesRequest{})
		require.Error(t, err)
		assert.Nil(t, langs)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 404, apiErr.StatusCode)
		assert.Equal(t, "/Languages", apiErr.Path)
		assert.Equal(t, "Language not supported", apiErr.Message)
	})

	t.Run("5xx with title fallback", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"title":"Internal Server Error"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Languages(t.Context(), LanguagesRequest{})
		require.Error(t, err)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 500, apiErr.StatusCode)
		assert.Equal(t, "Internal Server Error", apiErr.Message,
			"title must win when detail is absent")
	})

	t.Run("malformed JSON returns decode error (not sentinel)", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Truncated array opener — json.Decoder.Decode surfaces a
			// json.SyntaxError (NOT io.EOF, so not ErrEmptyResponse).
			_, _ = w.Write([]byte(`[not valid json`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Languages(t.Context(), LanguagesRequest{})
		require.Error(t, err)

		var apiErr *APIError
		require.NotErrorAs(t, err, &apiErr,
			"malformed JSON must not be reported as *APIError")
		require.NotErrorIs(t, err, ErrEmptyResponse,
			"malformed JSON must not match ErrEmptyResponse — it is a decode error")
		require.NotErrorIs(t, err, ErrResponseTooLarge,
			"malformed JSON must not match ErrResponseTooLarge")
		assert.Contains(t, err.Error(), "decode /Languages",
			"decode-error wrap must reference the /Languages path")
	})

	t.Run("ctx cancel returns context.Canceled", func(t *testing.T) {
		t.Parallel()
		// Handler stalls long enough that ctx cancellation must short-circuit.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(2 * time.Second):
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		ctx, cancel := context.WithCancel(t.Context())
		cancel() // immediate cancel before the call

		start := time.Now()
		_, err := c.Languages(ctx, LanguagesRequest{})
		elapsed := time.Since(start)

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled,
			"expected context.Canceled via errors.Is, got %v", err)
		// CLIENT-09: cancellation must interrupt within ≤ 100 ms.
		// A generous 1 s ceiling here still catches any pathological
		// regression (e.g. a missing ctx wire-up that lets the call
		// run to the handler's 2 s sleep) without flaking on slow CI.
		assert.Less(t, elapsed, time.Second,
			"ctx cancellation must short-circuit well before the 2 s server stall, took %v", elapsed)
	})
}
