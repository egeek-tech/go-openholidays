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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countriesFixtureCapturedAt records the date testdata/countries.json was
// captured from the live API. Re-capture when the upstream schema is
// suspected to have drifted. The fixture is not the authoritative shape —
// the live API is.
const countriesFixtureCapturedAt = "2026-05-27"

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

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		countries, err := c.Countries(context.Background())
		require.NoError(t, err)
		require.Len(t, countries, 2)

		// Sort by IsoCode for deterministic assertions regardless of
		// upstream ordering.
		sort.Slice(countries, func(i, j int) bool {
			return countries[i].IsoCode < countries[j].IsoCode
		})
		assert.Equal(t, "DE", countries[0].IsoCode)
		assert.Equal(t, "PL", countries[1].IsoCode)

		// Fixture uses uppercase Language codes ("PL", "DE", "EN").
		// Country.NameFor matches case-insensitively (strings.EqualFold).
		assert.Equal(t, "Polska", countries[1].NameFor("PL"))
		assert.Equal(t, "Polska", countries[1].NameFor("pl"))
		assert.Equal(t, "Deutschland", countries[0].NameFor("de"))
		assert.NotEmpty(t, countries[0].OfficialLanguages)
		assert.NotEmpty(t, countries[1].OfficialLanguages)
	})

	t.Run("4xx returns *APIError with detail Message", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail": "Country not supported"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		countries, err := c.Countries(context.Background())
		require.Error(t, err)
		assert.Nil(t, countries)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr),
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 404, apiErr.StatusCode)
		assert.Equal(t, "/Countries", apiErr.Path)
		assert.Equal(t, "Country not supported", apiErr.Message)
		assert.Contains(t, string(apiErr.Body), "Country not supported",
			"APIError.Body must carry the raw upstream bytes")
	})

	t.Run("5xx with title fallback", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"title": "Internal Server Error"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Countries(context.Background())
		require.Error(t, err)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr),
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 500, apiErr.StatusCode)
		assert.Equal(t, "Internal Server Error", apiErr.Message,
			"title must win when detail is absent")
	})

	t.Run("error field fallback when detail and title absent", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "Service Unavailable"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Countries(context.Background())
		require.Error(t, err)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr),
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 503, apiErr.StatusCode)
		assert.Equal(t, "Service Unavailable", apiErr.Message,
			"error must win as the third-priority field")
	})

	t.Run("4xx body truncated at 4 KiB (Phase 1 D-17 cap)", func(t *testing.T) {
		t.Parallel()
		big := bytes.Repeat([]byte("X"), 8192) // 8 KiB > 4 KiB cap
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(big)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Countries(context.Background())
		require.Error(t, err)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr),
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 4096, len(apiErr.Body),
			"APIError.Body length must equal the 4 KiB cap (D-17), got %d", len(apiErr.Body))
	})

	t.Run("empty body wraps ErrEmptyResponse", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Zero-byte body. json.Decoder.Decode returns io.EOF.
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.Countries(context.Background())
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrEmptyResponse),
			"expected ErrEmptyResponse via errors.Is, got %v", err)
	})

	t.Run("nil context returns defensive error (not sentinel)", func(t *testing.T) {
		t.Parallel()
		// No server — the nil-ctx guard short-circuits before any HTTP.
		c := NewClient(WithBaseURL("http://example.invalid"))
		//nolint:staticcheck // intentionally pass nil context to exercise the defensive guard
		_, err := c.Countries(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "openholidays: nil context",
			"defensive guard must return the documented error string")
		assert.False(t, errors.Is(err, ErrEmptyResponse),
			"nil-ctx error must NOT match any sentinel (D-42 defensive guard)")
		assert.False(t, errors.Is(err, ErrResponseTooLarge),
			"nil-ctx error must NOT match any sentinel (D-42 defensive guard)")
	})

	t.Run("oversize triggers ErrResponseTooLarge with no goroutine leak (D-49)", func(t *testing.T) {
		// DO NOT call t.Parallel here — this subtest reads
		// runtime.NumGoroutine() and must not race with sibling
		// subtest goroutine churn.

		// Streaming handler: writes a valid JSON Country array, looping
		// until total bytes written exceeds 11 MiB, then closes the
		// outer `]`. The result is structurally valid JSON that exceeds
		// the maxResponseBytes (10 MiB) cap.
		entry := `{"isoCode":"PL","name":[{"language":"EN","text":"Poland"}],"officialLanguages":["PL"]}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		c := NewClient(WithBaseURL(srv.URL))

		baseGoroutines := runtime.NumGoroutine()
		_, err := c.Countries(context.Background())
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrResponseTooLarge),
			"expected ErrResponseTooLarge via errors.Is, got %v", err)

		// Settle pause: the deferred drain-then-close runs after
		// Countries returns; the streaming handler goroutine on the
		// httptest server side also needs time to exit its Write loop
		// after the client closed the connection. RESEARCH assumption
		// A1 explicitly allows empirical loosening; +5 detects any
		// leak of 6+ goroutines (a real drain failure would leak the
		// transport's body-reader plus its parent and would show ≥ +10
		// in observed runs). 200 ms settle is generous for the
		// streaming server's Write-loop unwind on a closed connection.
		time.Sleep(200 * time.Millisecond)
		afterGoroutines := runtime.NumGoroutine()
		const goroutineSlack = 5
		assert.LessOrEqual(t, afterGoroutines, baseGoroutines+goroutineSlack,
			"goroutine leak suspected: baseline=%d after=%d (drain failed?)",
			baseGoroutines, afterGoroutines)
	})
}

