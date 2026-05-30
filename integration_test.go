//go:build integration

// Package openholidays — live-API integration tests for TEST-08.
//
// This file is compiled only when -tags=integration is supplied to go test
// AND has effect only when OPENHOLIDAYS_LIVE=1 is also set. Both gates must
// be true to issue any HTTP request to the live upstream; either being unset
// causes the test to skip silently (compile-time exclusion in the first
// case, runtime t.Skip in the second). The double gate exactly mirrors
// update_fixtures_test.go and the D-67 / D-68 design captured in
// .planning/phases/03-endpoints-helpers/03-CONTEXT.md.
//
// Purpose — surface upstream API drift (schema changes, new fields, removed
// endpoints, holiday-list adjustments) on a nightly cadence rather than at
// v0.1.0 release time. The Phase 3 golden fixtures define the expected
// shape; these integration tests assert the live API still matches.
//
// Two assertions captured here, both anchored to Phase 3 golden fixtures
// (testdata/public_holidays_pl_2025.json,
// testdata/school_holidays_pl_2025.json):
//
//   - PL 2025 public holidays: exactly 14 entries (incl. the new
//     2025-12-24 Wigilia entry that landed upstream in 2025).
//   - PL 2025 school-holiday periods: exactly 7 (4 staggered ferie zimowe
//     cohorts + wiosenna przerwa świąteczna + ferie letnie + zimowa przerwa
//     świąteczna).
//
// Canonical run command — exercise live API against the golden assertions:
//
//	OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...
//
// The nightly integration.yml workflow (Plan 06) supplies both gates. Local
// developers running the default `go test ./...` see no live calls.
//
// Uses t.Context() + context.WithTimeout — Pitfall 1 from
// .planning/phases/05-distribution/05-RESEARCH.md was the original
// motivation for pinning to context.Background(); after the Go floor
// moved to 1.24 (2026-05-29 floor bump) t.Context() is universally
// available across the CI matrix and provides automatic per-test
// cancellation on teardown.

package openholidays

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestIntegration_PublicHolidays_PL_2025 exercises Client.PublicHolidays
// against the live OpenHolidays API and asserts the Phase 3 golden truth
// that PL 2025 has exactly 14 public holidays. Drift in this count
// indicates the upstream schema or the PL public-holiday list changed and
// requires investigation before the next release.
//
// Skips silently when OPENHOLIDAYS_LIVE != "1" so a developer who supplies
// -tags=integration without setting the env var does not accidentally hit
// the live API.
//
// Not parallelized — live-API tests serialize against the public free
// upstream to avoid any chance of overlapping requests stressing the
// volunteer-run service.
func TestIntegration_PublicHolidays_PL_2025(t *testing.T) {
	if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
		t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
	}

	c := NewClient(WithTimeout(15 * time.Second))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("14 public holidays for PL 2025", func(t *testing.T) {
		hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		require.Len(t, hs, 14,
			"PL 2025 has 14 public holidays per Phase 3 golden fixture — "+
				"if this fails the upstream schema or PL holiday list drifted")
	})

	// Regression guard for the language-code casing fix (quick task
	// 260530-dvc): validateLanguage must canonicalize LanguageIsoCode to
	// UPPERCASE so the case-sensitive upstream returns localized names.
	// Before the fix it lowercased the code and the API silently returned
	// English. Like the rest of this function, it only runs under
	// OPENHOLIDAYS_LIVE=1 (gated at the top of the parent test).
	t.Run("PL 2025 with LanguageIsoCode=PL returns Polish names", func(t *testing.T) {
		hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode:  "PL",
			LanguageIsoCode: "PL",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.January, 1),
		})
		require.NoError(t, err)
		require.NotEmpty(t, hs)
		require.Equal(t, "Nowy Rok", hs[0].NameFor("PL"),
			"library must send an uppercase languageIsoCode so upstream returns "+
				"localized names; regression guard for quick task 260530-dvc "+
				"(validateLanguage ToLower->ToUpper). Before the fix this was "+
				"\"New Year's Day\".")
	})
}

// TestIntegration_SchoolHolidays_PL_2025 exercises Client.SchoolHolidays
// against the live OpenHolidays API and asserts the Phase 3 golden truth
// that PL 2025 has exactly 7 school-holiday periods (4 staggered ferie
// zimowe cohorts + wiosenna przerwa świąteczna + ferie letnie + zimowa
// przerwa świąteczna). Drift in this count indicates the upstream schema
// or the PL school-holiday calendar changed and requires investigation
// before the next release.
//
// Skips silently when OPENHOLIDAYS_LIVE != "1" so a developer who supplies
// -tags=integration without setting the env var does not accidentally hit
// the live API.
//
// Not parallelized — live-API tests serialize against the public free
// upstream to avoid any chance of overlapping requests stressing the
// volunteer-run service.
func TestIntegration_SchoolHolidays_PL_2025(t *testing.T) {
	if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
		t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
	}

	c := NewClient(WithTimeout(15 * time.Second))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("7 school-holiday periods for PL 2025", func(t *testing.T) {
		hs, err := c.SchoolHolidays(ctx, SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		require.Len(t, hs, 7,
			"PL 2025 has 7 school-holiday periods per Phase 3 golden fixture")
	})
}
