// Package main — tests for cmdPublic, the ohcli `public` subcommand.
//
// Gold Rule 3 application: exactly one TestCmdPublic per the cmdPublic
// production function. Every scenario lives in a t.Run subtest; require
// for preconditions, assert for verifications.
//
// TestCmdPublic exercises the full dispatcher pipeline: argv → run →
// cmdPublic → newClient → openholidays.Client.PublicHolidays → render*.
// Each scenario stands up an httptest.NewServer with a per-scenario
// handler, plumbs the seam via t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL),
// then asserts exit code + stdout + stderr.
//
// Test-file location note: this file lives at cmd/ohcli/public_test.go.
// testdata/*.json lives at the repo root, so fixture loads use
// filepath.Join("..", "..", "testdata", name) — two parents up from
// cmd/ohcli/ to reach the module root.
//
// t.Setenv + t.Parallel constraint (Go testing contract): the runtime
// forbids t.Parallel inside a subtest that calls t.Setenv. Each leaf
// t.Run below uses t.Setenv to inject OPENHOLIDAYS_BASE_URL, so leaves
// DO NOT call t.Parallel. The outer TestCmdPublic still calls
// t.Parallel(), so the whole TestCmdPublic runs in parallel with other
// top-level TestXxx functions — only its own subtests run sequentially.

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	openholidays "github.com/egeek-tech/go-openholidays"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCmdPublic exercises every code path of cmdPublic: happy paths for
// each output mode, the D-07 empty-result branch, the D-06 usage-error
// exit code 2, the runtime-error exit code 1, and the D-05 stderr
// "ohcli: " prefix on every error path.
//
// NOTE: the outer TestCmdPublic does NOT call t.Parallel(). The Go
// testing runtime forbids t.Setenv inside any test whose ancestors
// (including the parent test) called t.Parallel — so making the parent
// parallel would crash every leaf subtest that injects
// OPENHOLIDAYS_BASE_URL via t.Setenv. Test isolation across files is
// provided by go test's package-level scheduling instead.
func TestCmdPublic(t *testing.T) {
	t.Run("text output for PL 2025 prints 14-row table", func(t *testing.T) {
		// No t.Parallel here — t.Setenv forbids combining with t.Parallel.
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "public_holidays_pl_2025.json"))
		require.NoError(t, err, "PL 2025 public holidays fixture must be present")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Server-side query-string assertions catch URL-builder
			// regressions the fixture body alone would mask.
			assert.Equal(t, "/PublicHolidays", r.URL.Path)
			assert.Equal(t, "PL", r.URL.Query().Get("countryIsoCode"))
			assert.Equal(t, "2025-01-01", r.URL.Query().Get("validFrom"))
			assert.Equal(t, "2025-12-31", r.URL.Query().Get("validTo"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL", "2025"}, &stdout, &stderr)
		require.Equal(t, 0, code, "happy path must exit 0; stderr=%q", stderr.String())
		assert.Contains(t, stdout.String(), "DATE",
			"text-mode output must include the DATE header")
		assert.Contains(t, stdout.String(), "2025-01-01",
			"text-mode output must include the first holiday's StartDate")
		assert.Empty(t, stderr.String(),
			"happy path must not write to stderr")
	})

	t.Run("--json output is valid JSON []Holiday with 14 entries", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "public_holidays_pl_2025.json"))
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL", "2025", "--json"}, &stdout, &stderr)
		require.Equal(t, 0, code, "stderr=%q", stderr.String())

		var got []openholidays.Holiday
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &got),
			"stdout must be valid JSON decodable to []Holiday")
		require.Len(t, got, 14,
			"PL 2025 fixture carries 14 public holidays per Phase 3 golden capture")
	})

	t.Run("--csv output starts with snake_case header", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "public_holidays_pl_2025.json"))
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL", "2025", "--csv"}, &stdout, &stderr)
		require.Equal(t, 0, code, "stderr=%q", stderr.String())
		lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
		require.Greater(t, len(lines), 1, "csv output must have at least a header + data row")
		assert.Equal(t, "start_date,end_date,name,nationwide,type,subdivision_codes",
			lines[0], "first line must be the exact snake_case header")
	})

	t.Run("empty result writes stderr + exit 0 + empty stdout per D-07", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL", "2025"}, &stdout, &stderr)
		require.Equal(t, 0, code, "empty result must exit 0 per D-07")
		assert.Empty(t, stdout.String(),
			"empty result must leave stdout empty so downstream pipes don't break (D-07)")
		assert.Contains(t, stderr.String(), "ohcli: no public holidays found for PL 2025",
			"empty-result diagnostic must carry D-05 prefix and exact D-07 wording")
	})

	t.Run("usage error missing year returns exit 2", func(t *testing.T) {
		// No live HTTP needed — argv validation short-circuits before dispatch.
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL"}, &stdout, &stderr)
		require.Equal(t, 2, code, "missing positional must exit 2 (usage error per D-06)")
		assert.Contains(t, stderr.String(), "ohcli:",
			"diagnostic must carry the D-05 prefix")
		assert.Contains(t, stderr.String(), "public requires",
			"diagnostic must name the offending subcommand and the missing positional")
	})

	t.Run("usage error invalid year value returns exit 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL", "abc"}, &stdout, &stderr)
		require.Equal(t, 2, code, "non-numeric year must exit 2")
		assert.Contains(t, stderr.String(), "ohcli: invalid year",
			"diagnostic must carry D-05 prefix and name the failure mode")
	})

	t.Run("usage error year out of bounds returns exit 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		// 9999 > 2100 upper bound; the same code path that catches
		// non-numeric years catches out-of-range values.
		code := run([]string{"ohcli", "public", "PL", "9999"}, &stdout, &stderr)
		require.Equal(t, 2, code, "year above 2100 must exit 2")
		assert.Contains(t, stderr.String(), "ohcli: invalid year")
	})

	t.Run("usage error invalid format returns exit 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL", "2025", "--format=ical"}, &stdout, &stderr)
		require.Equal(t, 2, code, "unknown --format value must exit 2")
		assert.Contains(t, stderr.String(), "ohcli: invalid format",
			"diagnostic must name the bad format value")
	})

	t.Run("runtime error server 500 returns exit 1", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"title":"Internal Server Error"}`))
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL", "2025"}, &stdout, &stderr)
		require.Equal(t, 1, code,
			"upstream 5xx must exit 1 (runtime error per D-06)")
		assert.Contains(t, stderr.String(), "ohcli: ",
			"runtime-error diagnostic must carry the D-05 prefix")
		assert.Empty(t, stdout.String(),
			"runtime error must leave stdout empty (no partial render)")
	})

	t.Run("library validation error (invalid country) returns exit 2 not exit 1", func(t *testing.T) {
		// CR-02 regression: ErrInvalidCountry from the library MUST map
		// to exit 2 (usage error per D-06), NOT exit 1. The validation
		// happens client-side before any HTTP call, so no httptest server
		// is needed — the library's validateCountry rejects "XXX" because
		// it isn't a 2-letter ISO 3166-1 alpha-2 code.
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "XXX", "2025"}, &stdout, &stderr)
		require.Equal(t, 2, code,
			"library validation rejection must exit 2 (usage error per D-06)")
		assert.Contains(t, stderr.String(), "ohcli: ",
			"validation-error diagnostic must carry the D-05 prefix")
		assert.Contains(t, stderr.String(), "invalid country",
			"stderr should surface the ErrInvalidCountry sentinel message")
		assert.Empty(t, stdout.String(),
			"validation error must leave stdout empty")
	})

	t.Run("--lang reaches the wire as uppercase canonical form", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "public_holidays_pl_2025.json"))
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Library canonicalizes the language code to uppercase.
			assert.Equal(t, "PL", r.URL.Query().Get("languageIsoCode"),
				"--lang must reach the upstream as uppercase per validateLanguage")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "public", "PL", "2025", "--lang", "PL"}, &stdout, &stderr)
		require.Equal(t, 0, code, "stderr=%q", stderr.String())
	})
}
