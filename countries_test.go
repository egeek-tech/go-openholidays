// Package openholidays — tests for the Countries endpoint method.
//
// One TestXxx per exported production function per Gold Rule 3. Every
// scenario lives in a t.Run subtest. Non-English strings in the fixture
// (e.g. "Polska", "Deutschland", "Polen") mirror real upstream OpenHolidays
// responses and are admitted per CONVENTIONS.md Rule 1 testdata-fixture
// exception.

package openholidays

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingBody wraps an [http.Response] body and increments an atomic counter
// on construction; the counter decrements on Close (single-decrement guarded
// by a [sync.Once]-style boolean). This lets WR-02 prove drain-then-close
// hygiene deterministically — far more reliable than [runtime.NumGoroutine]()
// which is process-global and races sibling tests' transport pools.
type countingBody struct {
	io.ReadCloser
	open   *atomic.Int32
	closed atomic.Bool
}

func (b *countingBody) Close() error {
	if b.closed.CompareAndSwap(false, true) {
		b.open.Add(-1)
	}
	return b.ReadCloser.Close()
}

// drainCountingTransport wraps an [http.RoundTripper] so every returned
// resp.Body is a countingBody tied to a shared atomic counter. The counter
// is the post-call invariant the test asserts: 0 means every body the
// pipeline opened was also closed.
type drainCountingTransport struct {
	base      http.RoundTripper
	openCount atomic.Int32
}

func (t *drainCountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	t.openCount.Add(1)
	resp.Body = &countingBody{ReadCloser: resp.Body, open: &t.openCount}
	return resp, nil
}

// countriesFixtureCapturedAt records the date testdata/countries.json was
// captured from the live API. Re-capture when the upstream schema is
// suspected to have drifted. The fixture is not the authoritative shape —
// the live API is.
const countriesFixtureCapturedAt = "2026-05-27"

// audit:ok 2026-05-30

// TestClient_Countries covers ENDPT-01 + TRANS-02 + TRANS-03 + the four
// Phase 1 / Phase 2 invariants the endpoint exercises end-to-end:
//
//   - happy path (httptest fixture replay)
//   - RFC 7807 ProblemDetails priority detail → title → error (D-43)
//   - APIError.Body capped at 4 KiB (Phase 1 D-17)
//   - empty body → ErrEmptyResponse (RESEARCH OQ-1)
//   - nil context defensive guard (D-42)
//   - 11 MiB stream → ErrResponseTooLarge with goroutine-leak audit (D-49)
//
// Gold Rule 3: exactly one TestClient_Countries; every case is a t.Run.
func TestClient_Countries(t *testing.T) {
	t.Parallel()

	t.Run("happy path returns PL+DE from fixture", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "countries.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 02-03 Task 2")
		t.Logf("fixture captured %s", countriesFixtureCapturedAt)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		countries, err := c.Countries(t.Context(), CountriesRequest{})
		require.NoError(t, err)
		// Fixture mirrors the full live /Countries response (36 entries
		// as of 2026-05-29). The test asserts on PL + DE specifically
		// because the wider Phase 3 suite depends on them; the precise
		// total is upstream's call and would drift again the moment a
		// new country was added.
		require.GreaterOrEqual(t, len(countries), 2)

		byIso := make(map[string]Country, len(countries))
		for _, c := range countries {
			byIso[c.IsoCode] = c
		}
		require.Contains(t, byIso, "PL")
		require.Contains(t, byIso, "DE")

		// Fixture uses uppercase Language codes ("PL", "DE", "EN").
		// Country.NameFor matches case-insensitively (strings.EqualFold).
		assert.Equal(t, "Polska", byIso["PL"].NameFor("PL"))
		assert.Equal(t, "Polska", byIso["PL"].NameFor("pl"))
		assert.Equal(t, "Deutschland", byIso["DE"].NameFor("de"))
		assert.NotEmpty(t, byIso["DE"].OfficialLanguages)
		assert.NotEmpty(t, byIso["PL"].OfficialLanguages)
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
		countries, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err)
		assert.Nil(t, countries)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 404, apiErr.StatusCode)
		assert.Equal(t, "/Countries", apiErr.Path)
		assert.Equal(t, "Country not supported", apiErr.Message)
		assert.Contains(t, string(apiErr.Body), "Country not supported",
			"APIError.Body must carry the raw upstream bytes")
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
		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 500, apiErr.StatusCode)
		assert.Equal(t, "Internal Server Error", apiErr.Message,
			"title must win when detail is absent")
	})

	t.Run("error field fallback when detail and title absent", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "Service Unavailable"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 503, apiErr.StatusCode)
		assert.Equal(t, "Service Unavailable", apiErr.Message,
			"error must win as the third-priority field")
	})

	t.Run("4xx body truncated at 4 KiB (Phase 1 D-17 cap)", func(t *testing.T) {
		t.Parallel()
		big := bytes.Repeat([]byte("X"), 8192) // 8 KiB > 4 KiB cap
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(big)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Len(t, apiErr.Body, 4096,
			"APIError.Body length must equal the 4 KiB cap (D-17), got %d", len(apiErr.Body))
	})

	t.Run("empty body wraps ErrEmptyResponse", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Zero-byte body. json.Decoder.Decode returns io.EOF.
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEmptyResponse,
			"expected ErrEmptyResponse via errors.Is, got %v", err)
	})

	t.Run("nil context returns defensive error (not sentinel)", func(t *testing.T) {
		t.Parallel()
		// No server — the nil-ctx guard short-circuits before any HTTP.
		c := NewClient(WithBaseURL("http://example.invalid"))
		//nolint:staticcheck // intentionally pass nil context to exercise the defensive guard
		_, err := c.Countries(nil, CountriesRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "openholidays: nil context",
			"defensive guard must return the documented error string")
		require.NotErrorIs(t, err, ErrEmptyResponse,
			"nil-ctx error must NOT match any sentinel (D-42 defensive guard)")
		assert.NotErrorIs(t, err, ErrResponseTooLarge,
			"nil-ctx error must NOT match any sentinel (D-42 defensive guard)")
	})

	t.Run("oversize triggers ErrResponseTooLarge with no body leak (D-49)", func(t *testing.T) {
		t.Parallel()
		// WR-02 fix: replaced the process-global runtime.NumGoroutine()
		// sample with a deterministic per-test counter on resp.Body
		// open/close. The wrapping drainCountingTransport increments
		// openCount on every RoundTrip return and decrements on every
		// countingBody.Close(); the assertion that openCount == 0 after
		// the call proves the doJSONGet drain-then-close defer ran on
		// the oversize-error path. This removes flake risk from sibling
		// t.Parallel tests' goroutine churn and removes the need for a
		// time.Sleep settle pause.

		// Streaming handler: writes a valid JSON Country array, looping
		// until total bytes written exceeds 11 MiB, then closes the
		// outer `]`. The result is structurally valid JSON that exceeds
		// the maxResponseBytes (10 MiB) cap.
		entry := `{"isoCode":"PL","name":[{"language":"EN","text":"Poland"}],"officialLanguages":["PL"]}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			written := 0
			target := 11 << 20 // 11 MiB > 10 MiB cap
			_, _ = w.Write([]byte("["))
			written++
			first := true
			for written < target {
				var chunk string
				if first {
					chunk = entry
					first = false
				} else {
					chunk = "," + entry
				}
				n, err := w.Write([]byte(chunk))
				if err != nil {
					return
				}
				written += n
				if flusher != nil && written%(1<<16) < len(chunk) {
					flusher.Flush()
				}
			}
			_, _ = w.Write([]byte("]"))
		}))
		t.Cleanup(srv.Close)

		countingRT := &drainCountingTransport{base: http.DefaultTransport}
		httpClient := &http.Client{Transport: countingRT}
		c := NewClient(WithBaseURL(srv.URL), WithHTTPClient(httpClient))

		_, err := c.Countries(t.Context(), CountriesRequest{})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrResponseTooLarge,
			"expected ErrResponseTooLarge via errors.Is, got %v", err)

		// Deterministic post-condition: every resp.Body the transport
		// returned must have been closed by the doJSONGet drain-then-close
		// defer. A leak would manifest as openCount > 0.
		assert.Equal(t, int32(0), countingRT.openCount.Load(),
			"response body leaked: %d open bodies after Countries returned (drain failed?)",
			countingRT.openCount.Load())
	})

	t.Run("CR-01 regression: trailing whitespace in separate chunk is NOT oversize", func(t *testing.T) {
		t.Parallel()
		// Reviewer-reproduced false positive: a small (~5 KiB) JSON body
		// followed by trailing whitespace flushed in a separate HTTP chunk
		// previously triggered ErrResponseTooLarge because the post-Decode
		// sentinel-byte read on resp.Body returned n>0 for the leftover
		// newline that the decoder didn't pre-buffer across the chunk
		// boundary. The fix replaces the sentinel-byte read with
		// decoder.More(), which correctly ignores RFC 8259 whitespace.
		entry := `{"isoCode":"PL","name":[{"language":"EN","text":"Poland"}],"officialLanguages":["PL"]}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			_, _ = w.Write([]byte("[" + entry + "]"))
			if flusher != nil {
				flusher.Flush()
			}
			// Trailing whitespace in a SEPARATE chunk — this is what
			// triggered the CR-01 false positive pre-fix.
			_, _ = w.Write([]byte("\n\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		countries, err := c.Countries(t.Context(), CountriesRequest{})
		require.NoError(t, err, "trailing whitespace in a separate chunk must NOT be reported as ErrResponseTooLarge (CR-01)")
		require.NotErrorIs(t, err, ErrResponseTooLarge,
			"CR-01 regression: small body + trailing whitespace must not match ErrResponseTooLarge")
		require.Len(t, countries, 1)
		assert.Equal(t, "PL", countries[0].IsoCode)
	})

	t.Run("optional LanguageIsoCode sent in query when non-empty", func(t *testing.T) {
		t.Parallel()
		// Use the existing committed fixture body so the response shape is
		// stable. The handler asserts that the URL builder sent
		// languageIsoCode=PL (uppercase canonical form per validateLanguage).
		body, err := os.ReadFile(filepath.Join("testdata", "countries.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 03-01 Task 3")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "PL", r.URL.Query().Get("languageIsoCode"),
				"expected canonicalized uppercase languageIsoCode in query")
			assert.Equal(t, "/Countries", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		countries, err := c.Countries(t.Context(), CountriesRequest{LanguageIsoCode: "PL"})
		require.NoError(t, err)
		assert.NotEmpty(t, countries)
	})

	t.Run("empty LanguageIsoCode is omitted from query", func(t *testing.T) {
		t.Parallel()
		body, err := os.ReadFile(filepath.Join("testdata", "countries.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 03-01 Task 3")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Empty(t, r.URL.RawQuery,
				"empty LanguageIsoCode must not produce any query string, got %q", r.URL.RawQuery)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err = c.Countries(t.Context(), CountriesRequest{})
		require.NoError(t, err)
	})

	t.Run("invalid LanguageIsoCode returns ErrInvalidLanguage without HTTP", func(t *testing.T) {
		t.Parallel()
		// http://example.invalid is RFC 6761 reserved; if the validator
		// failed to short-circuit, the HTTP dispatch would fail loudly.
		c := NewClient(WithBaseURL("http://example.invalid"))
		_, err := c.Countries(t.Context(), CountriesRequest{LanguageIsoCode: "X"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidLanguage,
			"expected ErrInvalidLanguage via errors.Is, got %v", err)
	})
}
