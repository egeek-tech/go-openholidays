// Package main — tests for cmdSchool, the ohcli `school` subcommand.
//
// Gold Rule 3 application: exactly one TestCmdSchool per the cmdSchool
// production function. Every scenario lives in a t.Run subtest; require
// for preconditions, assert for verifications.
//
// TestCmdSchool exercises the full dispatcher pipeline plus the --region
// flag (CLI-02 — Polish ferie zimowe Śląskie cohort). The server-side
// handler asserts the subdivisionCode query parameter reaches the wire,
// catching URL-builder regressions the fixture body alone would mask.
//
// The empty-result test asserts the CONTEXT.md "Specifics" wording —
// "ohcli: no school holidays found for PL 2025 (region PL-XX)" — so a
// regression in the parenthesized region suffix surfaces immediately.
//
// t.Setenv + t.Parallel: leaf subtests that call t.Setenv DO NOT call
// t.Parallel (Go testing contract forbids the combination). The outer
// TestCmdSchool still calls t.Parallel().

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// audit:ok 2026-05-30

// TestCmdSchool exercises every code path of cmdSchool: --region happy
// path with subdivisionCode wire assertion, --region empty-result with
// the parenthesized D-07 wording, no-region happy path (subdivisionCode
// must be absent from the URL), and the D-06 usage/runtime error paths.
//
// NOTE: the outer TestCmdSchool does NOT call t.Parallel(). The Go
// testing runtime forbids t.Setenv inside any test whose ancestors
// called t.Parallel — every leaf below injects OPENHOLIDAYS_BASE_URL.
func TestCmdSchool(t *testing.T) {
	t.Run("--region PL-SL filters request and renders text output", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "school_holidays_pl_2025.json"))
		require.NoError(t, err, "PL 2025 school holidays fixture must be present")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/SchoolHolidays", r.URL.Path)
			assert.Equal(t, "PL", r.URL.Query().Get("countryIsoCode"))
			assert.Equal(t, "PL-SL", r.URL.Query().Get("subdivisionCode"),
				"--region must reach upstream as the subdivisionCode query param (CLI-02)")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "school", "PL", "2025", "--region", "PL-SL"}, &stdout, &stderr)
		require.Equal(t, 0, code, "stderr=%q", stderr.String())
		assert.Contains(t, stdout.String(), "2025",
			"text-mode output must include the year in StartDate rendering")
		assert.Contains(t, stdout.String(), "DATE",
			"text-mode output must include the DATE header")
	})

	t.Run("empty result with --region prints D-07 specifics wording on stderr", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "PL-XX", r.URL.Query().Get("subdivisionCode"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "school", "PL", "2025", "--region", "PL-XX"}, &stdout, &stderr)
		require.Equal(t, 0, code, "empty result must exit 0 per D-07")
		assert.Empty(t, stdout.String(),
			"empty result must leave stdout empty (D-07)")
		assert.Contains(t, stderr.String(),
			"ohcli: no school holidays found for PL 2025 (region PL-XX)",
			"D-07 'Specifics' wording must include the parenthesized region suffix verbatim")
	})

	t.Run("no --region keeps subdivisionCode empty on the wire", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "school_holidays_pl_2025.json"))
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Empty(t, r.URL.Query().Get("subdivisionCode"),
				"absent --region must not produce a subdivisionCode query parameter")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "school", "PL", "2025"}, &stdout, &stderr)
		require.Equal(t, 0, code, "stderr=%q", stderr.String())
	})

	t.Run("empty result without --region prints non-parenthesized wording", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "school", "PL", "2025"}, &stdout, &stderr)
		require.Equal(t, 0, code)
		assert.Contains(t, stderr.String(), "ohcli: no school holidays found for PL 2025",
			"empty-result diagnostic must carry the D-05 prefix")
		assert.NotContains(t, stderr.String(), "(region",
			"no --region means no parenthesized region suffix")
	})

	t.Run("usage error missing year returns exit 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "school", "PL"}, &stdout, &stderr)
		require.Equal(t, 2, code, "missing positional must exit 2 (usage error per D-06)")
		assert.Contains(t, stderr.String(), "ohcli:",
			"diagnostic must carry the D-05 prefix")
		assert.Contains(t, stderr.String(), "school requires",
			"diagnostic must name the offending subcommand")
	})

	t.Run("usage error invalid year returns exit 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "school", "PL", "abc"}, &stdout, &stderr)
		require.Equal(t, 2, code)
		assert.Contains(t, stderr.String(), "ohcli: invalid year")
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
		code := run([]string{"ohcli", "school", "PL", "2025"}, &stdout, &stderr)
		require.Equal(t, 1, code,
			"upstream 5xx must exit 1 (runtime error per D-06)")
		assert.Contains(t, stderr.String(), "ohcli: ",
			"runtime-error diagnostic must carry the D-05 prefix")
	})
}
