//go:build integration

// Package openholidays — fixture refresh utility (live API).
//
// This file is compiled only when -tags=integration is supplied to go test
// AND has effect only when OPENHOLIDAYS_LIVE=1 is also set. Both gates must
// be true to issue any HTTP request to the live upstream; either being unset
// causes the test to skip silently (compile-time exclusion in the first
// case, runtime t.Skip in the second).
//
// Canonical run command — regenerate every Phase 3 fixture from the live
// OpenHolidays API:
//
//	OPENHOLIDAYS_LIVE=1 go test -tags=integration -update \
//	    -run TestUpdateFixtures ./...
//
// The -update flag controls whether TestUpdateFixtures OVERWRITES the
// testdata/*.json files (true) or only verifies the live response matches
// the committed fixture (false; this becomes a useful drift-detection mode
// in CI nightly).
//
// Operating modes:
//
//   - With -update set: live capture is pretty-printed via json.Indent (four-
//     space indent) and a trailing '\n' byte is appended to the pretty buffer
//     so the written bytes round-trip through `git diff` against the committed
//     fixtures without spurious whitespace deltas. The result is written
//     atomically to testdata/<fixture>.json (temp file in the same directory
//     + os.Rename — single-filesystem rename is POSIX-atomic, so either the
//     old fixture remains intact or the new one is fully written; never a
//     half-written file).
//
//   - With -update unset (drift-detection mode): the live response body is
//     compared byte-for-byte against the committed fixture via
//     require.Equal. A failure reports DRIFT and aborts; no fixture write
//     occurs in this mode under any circumstance.
//
// Sanity-check rationale — nonEmptyJSONArray:
// Protects against the upstream returning [] during a transient outage or
// 200 OK with a non-array body during a partial deploy. Without this guard,
// a transient outage during the developer's -update run would corrupt every
// committed fixture.
//
// Build-tag rationale — D-67, D-68:
// Declaring the -update flag in this build-tagged file only means it is
// invisible during normal go test ./... runs (the file is not compiled in).
// Running `go test -update` WITHOUT `-tags=integration` correctly errors
// with "flag provided but not defined: -update", which is the loud failure
// mode we want — not a silent no-op.
//
// See .planning/phases/03-endpoints-helpers/03-RESEARCH.md §"Pattern 5"
// (lines 616-773) and 03-CONTEXT.md D-67/D-68 for the full design.

package openholidays

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// updateFixtures is true when -update is supplied. False (the default)
// means TestUpdateFixtures runs in drift-detection mode: the live response
// is fetched and compared to the committed fixture, but no fixture is
// overwritten.
//
// Declared once in THIS build-tagged file only — invisible during normal
// `go test ./...` runs (which omit -tags=integration). Consequence: running
// `go test -update` WITHOUT `-tags=integration` correctly fails with
// "flag provided but not defined: -update". That is the intended failure
// mode; a silently-ignored flag would be a more confusing footgun.
//
// The package-level var is allowed under CLIENT-10 even though it is not
// in internal_test.go's allowedVars map because the AST walker explicitly
// skips files with the `_test.go` suffix (internal_test.go lines 129-131).
// Test files are exempt from the no-global-mutable-state invariant per the
// same allowlist comment.
var updateFixtures = flag.Bool("update", false,
	"regenerate testdata/*.json fixtures from the live API")

// updateFixturesGuardEnv names the environment variable that gates the
// live-HTTP path. Together with the //go:build integration tag this forms
// the double gate documented in D-67 / D-68.
const updateFixturesGuardEnv = "OPENHOLIDAYS_LIVE"

// nonEmptyJSONArray is the sanity check run against every live response
// body before it is allowed to overwrite a committed fixture. It rejects
// non-JSON bodies, non-array bodies, and empty JSON arrays.
//
// Empty arrays in particular are observed during transient upstream
// outages (DB failover, deploy bleed-through); without this guard the
// fixture-refresh run would silently corrupt every fixture by writing
// `[]` into each one.
func nonEmptyJSONArray(b []byte) error {
	var v []json.RawMessage
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("not a JSON array: %w", err)
	}
	if len(v) == 0 {
		return errors.New("empty array — refusing to overwrite fixture")
	}
	return nil
}

// readAll reads up to max bytes from r and aborts the test on any I/O
// error. Cap is 11 MiB (slightly above the production 10 MiB cap in
// request.go) so legitimate boundary responses are not truncated by the
// helper itself; corruption from a hostile streaming upstream is still
// bounded.
func readAll(t *testing.T, r io.Reader, maxBytes int) []byte {
	t.Helper()
	lr := io.LimitReader(r, int64(maxBytes))
	b, err := io.ReadAll(lr)
	require.NoError(t, err, "failed to read live response body")
	return b
}

// TestUpdateFixtures captures every Phase 3 fixture from the live upstream
// in a single run. Operates in one of two modes per the -update flag (see
// the file header for full semantics).
//
// Captures are ordered so a partial failure leaves the remaining fixtures
// untouched — each subtest writes to a temp file in testdata/ and only
// renames it into place after the sanity check passes.
//
// Skips silently when OPENHOLIDAYS_LIVE != "1" so a developer who supplies
// -tags=integration without setting the env var does not accidentally hit
// the live API.
//
// Gold Rule 3 compliance: this is the single TestXxx in this file and
// every capture is dispatched through a t.Run subtest named after the
// fixture file. No top-level assertions outside the t.Run blocks.
func TestUpdateFixtures(t *testing.T) {
	if os.Getenv(updateFixturesGuardEnv) != "1" {
		t.Skipf("set %s=1 to enable live-API capture", updateFixturesGuardEnv)
	}

	// 30-second per-call cap is more than enough for any single endpoint
	// even with mild upstream latency. The Phase 2 production default is
	// 15s; the refresher uses 30s to give the slowest endpoint
	// (Subdivisions per country) extra headroom on a cold day.
	client := http.Client{Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	type capture struct {
		path     string             // upstream path (no host, no leading scheme)
		query    string             // raw query string (no leading '?')
		fixture  string             // testdata/<fixture> filename
		validate func([]byte) error // sanity check run before overwrite
	}
	captures := []capture{
		{"/Countries", "", "countries.json", nonEmptyJSONArray},
		{"/Languages", "", "languages.json", nonEmptyJSONArray},
		{
			"/Subdivisions",
			"countryIsoCode=PL&languageIsoCode=EN",
			"subdivisions_pl.json",
			nonEmptyJSONArray,
		},
		{
			"/Subdivisions",
			"countryIsoCode=DE&languageIsoCode=EN",
			"subdivisions_de.json",
			nonEmptyJSONArray,
		},
		{
			"/PublicHolidays",
			"countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31&languageIsoCode=PL",
			"public_holidays_pl_2025.json",
			nonEmptyJSONArray,
		},
		{
			"/SchoolHolidays",
			"countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31&languageIsoCode=EN",
			"school_holidays_pl_2025.json",
			nonEmptyJSONArray,
		},
	}

	// baseURL pins against the package-level defaultBaseURL const so
	// the fixture refresher always targets the same upstream as a
	// default-constructed Client (IN-02 follow-up — no more drift
	// between the fixture refresher and the production default).
	baseURL := defaultBaseURL

	for _, c := range captures {
		t.Run(c.fixture, func(t *testing.T) {
			url := baseURL + c.path
			if c.query != "" {
				url += "?" + c.query
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			require.NoError(t, err)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("User-Agent", "go-openholidays-fixture-refresh/"+Version)

			resp, err := client.Do(req)
			require.NoError(t, err, "live HTTP failed — aborting overwrite")
			t.Cleanup(func() { _ = resp.Body.Close() })

			require.Equalf(t, http.StatusOK, resp.StatusCode,
				"live API returned non-200 %d for %s — aborting overwrite",
				resp.StatusCode, url)

			body := readAll(t, resp.Body, 11<<20)
			require.NoErrorf(t, c.validate(body),
				"live response failed sanity check — aborting overwrite for %s",
				c.fixture)

			// Both modes normalize the live body via json.Indent before
			// comparing or writing. The upstream serves minified JSON; the
			// committed fixtures are pretty-printed (four-space indent — see
			// testdata/languages.json). A byte-for-byte comparison of
			// minified-vs-pretty would always report DRIFT, so drift
			// detection must compare pretty-vs-pretty after normalization.
			// A trailing '\n' is appended to the pretty buffer so its bytes
			// match the committed on-disk format (all fixtures end with one
			// newline byte). (*bytes.Buffer).WriteByte never returns an
			// error per stdlib docs, so no error wrap is needed.
			var pretty bytes.Buffer
			require.NoError(t, json.Indent(&pretty, body, "", "    "))
			pretty.WriteByte('\n')

			if !*updateFixtures {
				// Drift-detection mode: compare the committed fixture
				// against the normalized live response. A diff is reported
				// as DRIFT and the developer must decide whether to
				// re-capture via the overwrite mode.
				committed, err := os.ReadFile(filepath.Join("testdata", c.fixture))
				require.NoError(t, err, "committed fixture missing — run with -update to seed it")
				require.Equalf(t, string(committed), pretty.String(),
					"DRIFT: live response for %s differs from committed fixture",
					c.fixture)
				return
			}

			// Overwrite mode: write the pretty-printed body atomically via
			// temp file + os.Rename — never a half-written fixture.

			tmp, err := os.CreateTemp("testdata", c.fixture+".tmp-*")
			require.NoError(t, err)
			// IN-05: defer Close alongside Remove so a require.NoError
			// abort between CreateTemp and the explicit Close below does
			// not leak the file handle until the test binary exits.
			// Close on an already-closed file returns an error we ignore
			// (the explicit Close below has already returned its error
			// to require), so the doubled close is harmless. Remove on
			// the original temp name is a no-op when the rename has
			// already moved the entry.
			defer func() {
				_ = tmp.Close()
				_ = os.Remove(tmp.Name())
			}()

			_, err = tmp.Write(pretty.Bytes())
			require.NoError(t, err)
			require.NoError(t, tmp.Close())

			target := filepath.Join("testdata", c.fixture)
			require.NoError(t, os.Rename(tmp.Name(), target))
			t.Logf("captured %s (%d bytes pretty-printed)", c.fixture, pretty.Len())
		})
	}
}
