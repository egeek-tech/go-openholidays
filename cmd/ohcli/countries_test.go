// Package main — tests for cmdCountries, the ohcli `countries` subcommand.
//
// Gold Rule 3 application: exactly one TestCmdCountries per the
// cmdCountries production function. Every scenario lives in a t.Run
// subtest; require for preconditions, assert for verifications.
//
// TestCmdCountries differs from TestCmdPublic / TestCmdSchool in two
// ways: (a) the subcommand takes no positional arguments — extra args
// must exit 2 with the "ohcli: countries takes no positional args"
// diagnostic; and (b) the result type is []Country, not []Holiday, so
// the dispatch flows through renderCountries instead of render.
//
// t.Setenv + t.Parallel: leaf subtests that call t.Setenv DO NOT call
// t.Parallel.

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// audit:ok 2026-05-30

// TestCmdCountries exercises every code path of cmdCountries: happy
// path (default text format), --format=csv (header literal assertion),
// the positional-args rejection (D-06 exit 2), and the D-07 empty-result
// branch.
//
// NOTE: the outer TestCmdCountries does NOT call t.Parallel(). The Go
// testing runtime forbids t.Setenv inside any test whose ancestors
// called t.Parallel — every leaf below injects OPENHOLIDAYS_BASE_URL.
func TestCmdCountries(t *testing.T) {
	t.Run("text output happy path returns exit 0 with header and data", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "countries.json"))
		require.NoError(t, err, "countries fixture must be present")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/Countries", r.URL.Path,
				"countries subcommand must dispatch to /Countries upstream path")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "countries"}, &stdout, &stderr)
		require.Equal(t, 0, code, "stderr=%q", stderr.String())
		assert.Contains(t, stdout.String(), "ISO_CODE",
			"text-mode output must include the ISO_CODE header column")
		assert.Contains(t, stdout.String(), "PL",
			"fixture carries PL — must surface in text output")
		assert.Empty(t, stderr.String(),
			"happy path must not write to stderr")
	})

	t.Run("--format=csv produces snake_case header literal", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "countries.json"))
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "countries", "--format=csv"}, &stdout, &stderr)
		require.Equal(t, 0, code, "stderr=%q", stderr.String())
		lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
		require.Greater(t, len(lines), 1, "csv output must have a header + at least one data row")
		assert.Equal(t, "iso_code,name,official_languages", lines[0],
			"first line must be the exact snake_case header for countries")
	})

	t.Run("positional arg rejected with exit 2", func(t *testing.T) {
		// No HTTP needed — argv validation short-circuits before dispatch.
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "countries", "extra"}, &stdout, &stderr)
		require.Equal(t, 2, code, "positional arg must exit 2 (usage error per D-06)")
		assert.Contains(t, stderr.String(),
			"ohcli: countries takes no positional args",
			"diagnostic must use the exact wording with D-05 prefix")
	})

	t.Run("empty result writes stderr + exit 0 per D-07", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		}))
		t.Cleanup(srv.Close)
		t.Setenv("OPENHOLIDAYS_BASE_URL", srv.URL)

		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "countries"}, &stdout, &stderr)
		require.Equal(t, 0, code, "empty result must exit 0 per D-07")
		assert.Empty(t, stdout.String(),
			"empty result must leave stdout empty (D-07)")
		assert.Contains(t, stderr.String(), "ohcli: no countries found",
			"empty-result diagnostic must carry D-05 prefix and exact D-07 wording")
	})

	t.Run("usage error invalid format returns exit 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "countries", "--format=ical"}, &stdout, &stderr)
		require.Equal(t, 2, code, "unknown --format value must exit 2")
		assert.Contains(t, stderr.String(), "ohcli: invalid format",
			"diagnostic must name the bad format value")
	})
}
