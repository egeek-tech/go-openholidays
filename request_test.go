// Package openholidays — tests for the generic doJSONGet[T any] helper in
// request.go.
//
// One TestXxx function per exported production function per Gold Rule 3.
// doJSONGet is unexported, but it is the single load-bearing helper that
// every Phase 3 endpoint dispatches through, so it carries its own
// dedicated TestDoJSONGet covering the pipeline at the unit level. The
// existing TestClient_Countries (in countries_test.go) exercises the same
// pipeline at the endpoint level after the Plan 03-01 retrofit; this file
// adds the direct unit tests so a regression in the generic helper itself
// is attributable to doJSONGet rather than to a specific endpoint method.
//
// Every test case lives in a t.Run subtest. require for preconditions
// (httptest setup, fixture read). assert for verifications. Tests use
// httptest.NewServer per Phase 2 D-46 — no live network in unit tests.

package openholidays

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoJSONGet covers the generic helper's contract end-to-end:
//
//   - typed-T decode (int slice; Country slice from the live-captured fixture)
//   - defensive nil-ctx guard (returns "openholidays: nil context" before
//     any HTTP dispatch)
//   - 4xx upstream → *APIError populated with StatusCode/Path/Message
//   - 200 + empty body → wraps ErrEmptyResponse
//   - query-string encoding (RawQuery populated when q has entries; left
//     empty when q has none)
//   - 11 MiB streaming body → wraps ErrResponseTooLarge via the
//     mid-truncation gate (limited.N == 0)
//
// Gold Rule 3: exactly one TestDoJSONGet; every case is a t.Run.
func TestDoJSONGet(t *testing.T) {
	t.Parallel()

	t.Run("decodes a JSON array of ints", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[1,2,3]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		got, err := doJSONGet[[]int](context.Background(), c, "/anything", nil)
		require.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, got)
	})

	t.Run("decodes the Countries fixture", func(t *testing.T) {
		t.Parallel()
		body, err := os.ReadFile(filepath.Join("testdata", "countries.json"))
		require.NoError(t, err, "fixture missing — re-capture per Plan 03-01 Task 3")
		t.Logf("fixture captured %s", countriesFixtureCapturedAt)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		got, err := doJSONGet[[]Country](context.Background(), c, "/Countries", nil)
		require.NoError(t, err)
		require.Len(t, got, 2)
		isoCodes := []string{got[0].IsoCode, got[1].IsoCode}
		assert.Contains(t, isoCodes, "PL")
		assert.Contains(t, isoCodes, "DE")
	})

	t.Run("nil ctx returns defensive error before HTTP", func(t *testing.T) {
		t.Parallel()
		// example.invalid is RFC 6761 reserved; if the nil-ctx guard
		// failed to short-circuit, the HTTP dispatch would fail loudly
		// with a DNS error (or hang). The guard MUST fire first.
		c := NewClient(WithBaseURL("http://example.invalid"))
		//nolint:staticcheck // intentionally pass nil context to exercise the defensive guard
		_, err := doJSONGet[[]Country](nil, c, "/Countries", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "openholidays: nil context",
			"defensive guard must return the documented error string")
	})

	t.Run("4xx returns *APIError populated", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail": "Resource missing"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		got, err := doJSONGet[[]Country](context.Background(), c, "/Whatever", nil)
		require.Error(t, err)
		assert.Nil(t, got, "doJSONGet must return the zero value of T on error")

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
		assert.Equal(t, "/Whatever", apiErr.Path)
		assert.Equal(t, "Resource missing", apiErr.Message)
	})

	t.Run("empty body wraps ErrEmptyResponse", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Zero-byte body — json.Decoder.Decode returns io.EOF.
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := doJSONGet[[]Country](context.Background(), c, "/EmptyEndpoint", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEmptyResponse,
			"expected ErrEmptyResponse via errors.Is, got %v", err)
	})

	t.Run("query is encoded into req.URL.RawQuery when non-empty", func(t *testing.T) {
		t.Parallel()
		var observedRawQuery string
		var observedKey string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			observedRawQuery = r.URL.RawQuery
			observedKey = r.URL.Query().Get("k")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		q := url.Values{}
		q.Set("k", "v")
		_, err := doJSONGet[[]int](context.Background(), c, "/q", q)
		require.NoError(t, err)
		assert.Equal(t, "v", observedKey,
			"expected query key k=v, got RawQuery=%q", observedRawQuery)
		assert.NotEmpty(t, observedRawQuery,
			"non-empty url.Values must produce a non-empty RawQuery")
	})

	t.Run("query is empty when q is empty", func(t *testing.T) {
		t.Parallel()
		var observedRawQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			observedRawQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := doJSONGet[[]int](context.Background(), c, "/q", url.Values{})
		require.NoError(t, err)
		assert.Empty(t, observedRawQuery,
			"empty url.Values must NOT set RawQuery (got %q)", observedRawQuery)

		// Repeat with a literal nil — same expectation.
		_, err = doJSONGet[[]int](context.Background(), c, "/q", nil)
		require.NoError(t, err)
		assert.Empty(t, observedRawQuery,
			"nil url.Values must NOT set RawQuery (got %q)", observedRawQuery)
	})

	t.Run("11 MiB body wraps ErrResponseTooLarge", func(t *testing.T) {
		t.Parallel()
		// Stream a structurally valid JSON array of Country entries until
		// the payload exceeds 11 MiB (> 10 MiB cap). The mid-truncation
		// gate (limited.N == 0 after Decode surfaces io.ErrUnexpectedEOF
		// because the closing `]` was never reached) must wrap
		// ErrResponseTooLarge.
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
				n, writeErr := w.Write([]byte(chunk))
				if writeErr != nil {
					return
				}
				written += n
				if flusher != nil && written%(1<<16) < len(chunk) {
					flusher.Flush()
				}
			}
			// Intentionally do NOT write the closing `]`. The client's
			// LimitReader will exhaust before Decode reaches a valid
			// boundary — the mid-truncation gate fires.
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := doJSONGet[[]Country](context.Background(), c, "/Countries", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrResponseTooLarge,
			"expected ErrResponseTooLarge via errors.Is, got %v", err)
	})

	t.Run("4xx body truncated at 4 KiB (apiErrorBodyCap)", func(t *testing.T) {
		t.Parallel()
		big := bytes.Repeat([]byte("X"), 8192) // 8 KiB > 4 KiB cap
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(big)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := doJSONGet[[]Country](context.Background(), c, "/Countries", nil)
		require.Error(t, err)
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr,
			"expected *APIError, got %T: %v", err, err)
		assert.Len(t, apiErr.Body, 4096,
			"APIError.Body length must equal the 4 KiB cap, got %d", len(apiErr.Body))
	})
}
