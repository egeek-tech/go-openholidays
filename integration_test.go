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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// audit:ok 2026-05-30

// hasLang reports whether entries contains a localized-text entry whose
// Language matches lang case-insensitively (strings.EqualFold). Unlike
// NameFor / pickLocalized (types.go), it does NOT fall back to entries[0] on
// a language miss — it returns true only when some entry's Language genuinely
// matches the requested code.
//
// This is the Layer-3 anti-fallback guard for the language-casing bug class
// (quick task 260530-dvc): when the library lowercased languageIsoCode the
// case-sensitive upstream returned English-only names, yet NameFor("PL")
// still returned a non-empty string (the English entries[0] fallback), so a
// non-empty assertion on the accessor passed under the bug. hasLang(entries,
// "PL") instead returns false in that scenario, so an assertion built on it
// FAILS when the requested localization is silently absent.
func hasLang(entries []LocalizedText, lang string) bool {
	for _, e := range entries {
		if strings.EqualFold(e.Language, lang) {
			return true
		}
	}
	return false
}

// audit:ok 2026-05-30

// TestIntegration_PublicHolidays exercises Client.PublicHolidays against the
// live OpenHolidays API across PL and DE with the three-layer assertion model
// (spec §3): Layer-1 count canaries, Layer-2 exact localized-name pins, and
// Layer-3 hasLang anti-fallback membership. It covers:
//
//   - PL 2025: 14 public holidays (count canary, Phase 3 golden truth).
//   - PL 2025-01-01 (lang=PL): New Year → "Nowy Rok" (the pin that would have
//     caught the 260530-dvc language-casing bug).
//   - DE 2025 (lang=DE): 21 public holidays (count canary); New Year →
//     "Neujahr" pin + hasLang("DE"); the regional holiday "Heilige Drei
//     Könige" is Nationwide==false with Subdivisions {DE-ST, DE-BW, DE-BY} so
//     Holiday.IsInRegion("DE-BY") is true and IsInRegion("DE-HH") is false.
//
// Drift in a count canary indicates the upstream schema or holiday list
// changed and requires investigation before the next release. All pinned
// values are from the 2026-05-30 live probe (spec §6a).
//
// Skips silently when OPENHOLIDAYS_LIVE != "1" so a developer who supplies
// -tags=integration without setting the env var does not accidentally hit
// the live API.
//
// Not parallelized — live-API tests serialize against the public free
// upstream to avoid any chance of overlapping requests stressing the
// volunteer-run service.
func TestIntegration_PublicHolidays(t *testing.T) {
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

	t.Run("DE 2025 has 21 public holidays (count canary)", func(t *testing.T) {
		hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode:  "DE",
			LanguageIsoCode: "DE",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		require.Len(t, hs, 21,
			"DE 2025 has 21 public holidays per the 2026-05-30 live probe (spec §6a) — "+
				"if this fails the upstream schema or DE holiday list drifted")
	})

	t.Run("DE 2025 New Year is Neujahr (localization pin + hasLang)", func(t *testing.T) {
		hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode:  "DE",
			LanguageIsoCode: "DE",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		// Search for New Year (2025-01-01); do not assume slice ordering.
		var newYear Holiday
		var found bool
		for _, h := range hs {
			if h.StartDate.Equal(NewDate(2025, time.January, 1)) {
				newYear, found = h, true
				break
			}
		}
		require.True(t, found, "DE 2025 must contain a holiday starting 2025-01-01")
		// Under the 260530-dvc bug the pin reads "New Year's Day" and
		// hasLang("DE") is false — both assertions fail (2026-05-30 probe).
		assert.Equal(t, "Neujahr", newYear.NameFor("DE"))
		assert.True(t, hasLang(newYear.Name, "DE"),
			"raw Name slice must carry a DE entry (anti-fallback guard for 260530-dvc)")
	})

	t.Run("DE Heilige Drei Könige is regional (Nationwide=false + Subdivisions + IsInRegion)", func(t *testing.T) {
		hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode:  "DE",
			LanguageIsoCode: "DE",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		// Locate the regional holiday by its German name (2025-01-06).
		var epiphany Holiday
		var found bool
		for _, h := range hs {
			if h.NameFor("DE") == "Heilige Drei Könige" {
				epiphany, found = h, true
				break
			}
		}
		require.True(t, found,
			"DE 2025 must contain \"Heilige Drei Könige\" (2026-05-30 probe)")
		assert.False(t, epiphany.Nationwide,
			"Heilige Drei Könige is regional, not nationwide (2026-05-30 probe)")
		assert.NotEmpty(t, epiphany.Subdivisions,
			"a regional holiday must carry Subdivisions")
		// Subdivisions are {DE-ST, DE-BW, DE-BY} per spec §6a (2026-05-30 probe).
		assert.True(t, epiphany.IsInRegion("DE-BY"),
			"Bavaria (DE-BY) observes Heilige Drei Könige")
		// DE-HH (Hamburg) is NOT in the set — proves the flat match is
		// selective, not a blanket true.
		assert.False(t, epiphany.IsInRegion("DE-HH"),
			"Hamburg (DE-HH) does not observe Heilige Drei Könige (2026-05-30 probe)")
	})
}

// audit:ok 2026-05-30

// TestIntegration_SchoolHolidays exercises Client.SchoolHolidays against the
// live OpenHolidays API across PL and DE with the three-layer assertion model
// (spec §3). It covers:
//
//   - PL 2025: 7 school-holiday periods (count canary, Phase 3 golden truth).
//   - PL 2025 ferie zimowe per województwo (the project's headline core
//     value): the cohort whose window is 2025-01-20..02-02 is non-nationwide,
//     carries an EMPTY Groups slice (cohorts are distinguished by their
//     Subdivisions and date window, NOT by group codes — spec §6a), and
//     Holiday.IsInRegion is true for PL-SL (in that first window) yet false
//     for PL-SK (in the LAST window 2025-02-17..03-02). This mirrors the
//     hermetic TestClient_SchoolHolidays_IsInRegion_FerieZimowe.
//   - PL school holidays localized (lang=PL): hasLang("PL") membership (the
//     endpoint lacks rich localization today, and §6a pins no exact name, so
//     hasLang is the robust assertion — Gold Rule 2: no guessed name).
//   - DE school holidays non-empty (lang=DE): hasLang("DE") on the first
//     period (§6a pins no DE school count — assert non-emptiness + DE
//     localization only).
//
// Skips silently when OPENHOLIDAYS_LIVE != "1" so a developer who supplies
// -tags=integration without setting the env var does not accidentally hit
// the live API.
//
// Not parallelized — live-API tests serialize against the public free
// upstream to avoid any chance of overlapping requests stressing the
// volunteer-run service.
func TestIntegration_SchoolHolidays(t *testing.T) {
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

	t.Run("PL 2025 ferie zimowe per województwo (core value)", func(t *testing.T) {
		hs, err := c.SchoolHolidays(ctx, SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		// Isolate the first ferie-zimowe cohort by its window start
		// (2025-01-20..02-02 per spec §6a). Search; do not index by position.
		var firstCohort Holiday
		var found bool
		for _, h := range hs {
			if h.Type == HolidayTypeSchool && !h.Nationwide &&
				h.StartDate.Equal(NewDate(2025, time.January, 20)) {
				firstCohort, found = h, true
				break
			}
		}
		require.True(t, found,
			"PL 2025 must contain a non-nationwide ferie-zimowe cohort starting "+
				"2025-01-20 (spec §6a; mirrors the hermetic FerieZimowe test)")
		assert.False(t, firstCohort.Nationwide,
			"ferie zimowe are regional (per województwo), not nationwide")
		assert.Empty(t, firstCohort.Groups,
			"live ferie-zimowe Groups are empty — cohorts are distinguished by "+
				"Subdivisions + date window, not group codes (spec §6a)")
		// PL-SL is in the first window (2025-01-20..02-02) per spec §6a.
		assert.True(t, firstCohort.IsInRegion("PL-SL"),
			"PL-SL falls in the first ferie-zimowe window (2026-05-30 probe)")
		// PL-SK is in the LAST window (2025-02-17..03-02), NOT the first —
		// proves the window→województwo mapping is real, not a blanket true.
		assert.False(t, firstCohort.IsInRegion("PL-SK"),
			"PL-SK is in the last ferie-zimowe window, not the first (2026-05-30 probe)")
	})

	t.Run("PL school holidays localized (hasLang)", func(t *testing.T) {
		hs, err := c.SchoolHolidays(ctx, SchoolHolidaysRequest{
			CountryIsoCode:  "PL",
			LanguageIsoCode: "PL",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		require.NotEmpty(t, hs)
		// §6a pins no exact Polish school-holiday name; assert hasLang
		// membership (the anti-fallback guard) rather than guess a name
		// (Gold Rule 2).
		assert.True(t, hasLang(hs[0].Name, "PL"),
			"raw Name slice must carry a PL entry (anti-fallback guard for 260530-dvc)")
	})

	t.Run("DE school holidays non-empty with DE localization", func(t *testing.T) {
		hs, err := c.SchoolHolidays(ctx, SchoolHolidaysRequest{
			CountryIsoCode:  "DE",
			LanguageIsoCode: "DE",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		require.NotEmpty(t, hs,
			"DE 2025 must have at least one school-holiday period")
		// §6a pins no DE school-holiday count — assert DE localization only.
		assert.True(t, hasLang(hs[0].Name, "DE"),
			"raw Name slice must carry a DE entry (anti-fallback guard for 260530-dvc)")
	})
}

// audit:ok 2026-05-30

// TestIntegration_Countries exercises Client.Countries against the live
// OpenHolidays API with the three-layer assertion model (spec §3):
//
//   - default: 36 countries (count canary); the list contains PL and DE by
//     uppercase IsoCode; PL.OfficialLanguages contains "PL".
//   - lang=DE: Germany → "Deutschland", Poland → "Polen" pins + hasLang("DE")
//     on the German entry. Filtering by language returns a single-language
//     Name array, so hasLang membership is exact (spec §6a, 2026-05-30 probe).
//
// Not parallelized; skips silently when OPENHOLIDAYS_LIVE != "1".
func TestIntegration_Countries(t *testing.T) {
	if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
		t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
	}

	c := NewClient(WithTimeout(15 * time.Second))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("default lists PL and DE by uppercase IsoCode + 36 count canary", func(t *testing.T) {
		cs, err := c.Countries(ctx, CountriesRequest{})
		require.NoError(t, err)
		require.Len(t, cs, 36,
			"OpenHolidays lists 36 countries per the 2026-05-30 live probe (spec §6a) — "+
				"if this fails the upstream country list drifted")
		// Locate PL and DE by uppercase IsoCode; search, do not index.
		var pl Country
		var hasPL, hasDE bool
		for _, country := range cs {
			switch country.IsoCode {
			case "PL":
				pl, hasPL = country, true
			case "DE":
				hasDE = true
			}
		}
		assert.True(t, hasPL, "country list must contain PL by uppercase IsoCode")
		assert.True(t, hasDE, "country list must contain DE by uppercase IsoCode")
		// PL.OfficialLanguages == ["PL"] per spec §6a (2026-05-30 probe).
		assert.Contains(t, pl.OfficialLanguages, "PL",
			"PL official languages must contain PL (2026-05-30 probe)")
	})

	t.Run("lang=DE localizes country names (pin + hasLang)", func(t *testing.T) {
		cs, err := c.Countries(ctx, CountriesRequest{LanguageIsoCode: "DE"})
		require.NoError(t, err)
		var germany, poland Country
		var hasGermany, hasPoland bool
		for _, country := range cs {
			switch country.IsoCode {
			case "DE":
				germany, hasGermany = country, true
			case "PL":
				poland, hasPoland = country, true
			}
		}
		require.True(t, hasGermany, "lang=DE result must contain DE")
		require.True(t, hasPoland, "lang=DE result must contain PL")
		// Under the 260530-dvc bug these read English ("Germany"/"Poland") and
		// hasLang("DE") is false (2026-05-30 probe; spec §6a).
		assert.Equal(t, "Deutschland", germany.NameFor("DE"))
		assert.Equal(t, "Polen", poland.NameFor("DE"))
		assert.True(t, hasLang(germany.Name, "DE"),
			"language-filtered Country.Name is single-language → hasLang(DE) exact (spec §6a)")
	})
}

// audit:ok 2026-05-30

// TestIntegration_Languages exercises Client.Languages against the live
// OpenHolidays API:
//
//   - default: 30 languages (count canary); the list contains PL, DE, and EN
//     by uppercase IsoCode.
//   - lang=PL: the IsoCode "PL" entry carries a PL-language Name
//     ("polski") → hasLang("PL") is true. No exact localized language name is
//     pinned (§6a gives none; Gold Rule 2 forbids guessing). The 2026-05-30
//     probe confirmed only the IsoCode "PL" entry carries a PL-language Name
//     under lang=PL (the DE/EN entries return English-only names), so the
//     hasLang assertion targets the "PL" entry specifically.
//
// Not parallelized; skips silently when OPENHOLIDAYS_LIVE != "1".
func TestIntegration_Languages(t *testing.T) {
	if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
		t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
	}

	c := NewClient(WithTimeout(15 * time.Second))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("default lists PL, DE, EN by uppercase IsoCode + 30 count canary", func(t *testing.T) {
		ls, err := c.Languages(ctx, LanguagesRequest{})
		require.NoError(t, err)
		require.Len(t, ls, 30,
			"OpenHolidays lists 30 languages per the 2026-05-30 live probe (spec §6a) — "+
				"if this fails the upstream language list drifted")
		var hasPL, hasDE, hasEN bool
		for _, l := range ls {
			switch l.IsoCode {
			case "PL":
				hasPL = true
			case "DE":
				hasDE = true
			case "EN":
				hasEN = true
			}
		}
		assert.True(t, hasPL, "language list must contain PL by uppercase IsoCode")
		assert.True(t, hasDE, "language list must contain DE by uppercase IsoCode")
		assert.True(t, hasEN, "language list must contain EN by uppercase IsoCode")
	})

	t.Run("lang=PL localizes language names (hasLang)", func(t *testing.T) {
		ls, err := c.Languages(ctx, LanguagesRequest{LanguageIsoCode: "PL"})
		require.NoError(t, err)
		// Pick the IsoCode "PL" entry: the 2026-05-30 probe showed only it
		// carries a PL-language Name ("polski") under lang=PL. Do NOT pin the
		// exact name — §6a gives none and Gold Rule 2 forbids guessing.
		var plLang Language
		var found bool
		for _, l := range ls {
			if l.IsoCode == "PL" {
				plLang, found = l, true
				break
			}
		}
		require.True(t, found, "lang=PL result must contain the PL language entry")
		assert.True(t, hasLang(plLang.Name, "PL"),
			"the PL language entry's raw Name must carry a PL entry "+
				"(anti-fallback guard for 260530-dvc; 2026-05-30 probe)")
	})
}

// audit:ok 2026-05-30

// TestIntegration_Subdivisions exercises Client.Subdivisions against the live
// OpenHolidays API:
//
//   - PL: 16 województwa (count canary); every Code has the "PL-" prefix.
//   - PL lang=PL: PL-SK → "Śląskie", PL-SL → "Świętokrzyskie" name pins +
//     hasLang("PL"); Category (a []LocalizedText field with NO NameFor
//     accessor) carries "województwo" for the PL entry. The OpenHolidays Code
//     is its own scheme, NOT ISO 3166-2 — ISO 3166-2 swaps PL-SL/PL-SK
//     (2026-05-30 probe; spec §6a).
//   - DE: 16 Bundesländer (count canary); every Code has the "DE-" prefix;
//     DE-BY is present.
//
// Not parallelized; skips silently when OPENHOLIDAYS_LIVE != "1".
func TestIntegration_Subdivisions(t *testing.T) {
	if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
		t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
	}

	c := NewClient(WithTimeout(15 * time.Second))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("PL has 16 województwa, codes ^PL- (count canary)", func(t *testing.T) {
		subs, err := c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: "PL"})
		require.NoError(t, err)
		require.Len(t, subs, 16,
			"PL has 16 województwa per the 2026-05-30 live probe (spec §6a) — "+
				"if this fails the upstream subdivision list drifted")
		for _, s := range subs {
			assert.True(t, strings.HasPrefix(s.Code, "PL-"),
				"every PL subdivision Code must have the PL- prefix; got %q", s.Code)
		}
	})

	t.Run("PL lang=PL localizes name + category (pins + hasLang)", func(t *testing.T) {
		subs, err := c.Subdivisions(ctx, SubdivisionsRequest{
			CountryIsoCode:  "PL",
			LanguageIsoCode: "PL",
		})
		require.NoError(t, err)
		var sk, sl Subdivision
		var hasSK, hasSL bool
		for _, s := range subs {
			switch s.Code {
			case "PL-SK":
				sk, hasSK = s, true
			case "PL-SL":
				sl, hasSL = s, true
			}
		}
		require.True(t, hasSK, "PL subdivisions must contain Code PL-SK")
		require.True(t, hasSL, "PL subdivisions must contain Code PL-SL")
		// OpenHolidays Code is NOT ISO 3166-2: the live API maps PL-SK →
		// Śląskie and PL-SL → Świętokrzyskie, whereas ISO 3166-2 swaps these
		// (2026-05-30 probe; spec §6a).
		assert.Equal(t, "Śląskie", sk.NameFor("PL"))
		assert.Equal(t, "Świętokrzyskie", sl.NameFor("PL"))
		assert.True(t, hasLang(sk.Name, "PL"),
			"PL-SK raw Name must carry a PL entry (anti-fallback guard for 260530-dvc)")
		// Category has NO NameFor accessor — iterate for the PL entry directly.
		var catPL string
		for _, e := range sk.Category {
			if strings.EqualFold(e.Language, "PL") {
				catPL = e.Text
				break
			}
		}
		assert.Equal(t, "województwo", catPL,
			"PL-SK Category (PL) must be \"województwo\" (2026-05-30 probe)")
		assert.True(t, hasLang(sk.Category, "PL"),
			"PL-SK raw Category must carry a PL entry")
	})

	t.Run("DE has 16 Bundesländer, codes ^DE- (count canary)", func(t *testing.T) {
		subs, err := c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: "DE"})
		require.NoError(t, err)
		require.Len(t, subs, 16,
			"DE has 16 Bundesländer per the 2026-05-30 live probe (spec §6a)")
		var hasBY bool
		for _, s := range subs {
			assert.True(t, strings.HasPrefix(s.Code, "DE-"),
				"every DE subdivision Code must have the DE- prefix; got %q", s.Code)
			if s.Code == "DE-BY" {
				hasBY = true
			}
		}
		assert.True(t, hasBY, "DE subdivisions must contain Code DE-BY (Bayern)")
	})
}

// audit:ok 2026-05-30

// TestIntegration_ClientIsInRegion exercises Client.IsInRegion — the ONE
// Phase-3 helper that issues hidden I/O — live, driving its hierarchical tree
// walk (the behavior it adds over the flat Holiday.IsInRegion). A code already
// present in h.Subdivisions[] would hit the flat fast-path
// (client_isinregion.go) and return true with ZERO HTTP, so it does NOT
// exercise the tree walk. The positive case therefore queries a DESCENDANT
// code not itself in h.Subdivisions, forcing the /Subdivisions fetch +
// buildParentIndex + upward child→parent walk.
//
// The scenario is discovered from live DE data at runtime (mirroring the
// hermetic client_isinregion_test.go findFirstWithChildren pattern) so it
// stays resilient to upstream refreshes: the live DE tree carries
// Subdivision.Children (2026-05-30 probe).
//
// Cases:
//   - descendant code returns true THROUGH the tree walk;
//   - an unrelated code returns false (tree walk finds no ancestor).
//
// Not parallelized; skips silently when OPENHOLIDAYS_LIVE != "1".
func TestIntegration_ClientIsInRegion(t *testing.T) {
	if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
		t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
	}

	c := NewClient(WithTimeout(15 * time.Second))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	// Fetch the live DE subdivision tree and discover a genuine parent/child
	// descent (the live DE tree carries Subdivision.Children — 2026-05-30
	// probe + the hermetic findFirstWithChildren assumption).
	tree, err := c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: "DE"})
	require.NoError(t, err)
	require.NotEmpty(t, tree)

	var parentCode, childCode string
	var found bool
	for _, s := range tree {
		if len(s.Children) > 0 {
			parentCode, childCode = s.Code, s.Children[0].Code
			found = true
			break
		}
	}
	require.True(t, found,
		"live DE subdivision tree must carry at least one entry with non-empty "+
			"Children — required to exercise the hierarchical walk (matches the "+
			"hermetic findFirstWithChildren assumption; 2026-05-30 probe)")
	t.Logf("tree-walk target: parent=%q child=%q", parentCode, childCode)

	// Synthetic Holiday with ONLY the PARENT code in Subdivisions (constructing
	// a value type in test code is fine — this is not production logic).
	h := Holiday{Nationwide: false, Subdivisions: []SubdivisionRef{{Code: parentCode}}}

	t.Run("descendant code returns true THROUGH the live tree walk (the reason Client.IsInRegion exists)", func(t *testing.T) {
		// childCode is only a DESCENDANT (not in h.Subdivisions) so no
		// fast-path fires: this forces the /Subdivisions fetch +
		// buildParentIndex + upward walk to return true.
		ok, err := c.IsInRegion(ctx, h, childCode)
		require.NoError(t, err)
		assert.True(t, ok,
			"Client.IsInRegion must climb from descendant %q to parent %q via the "+
				"live /Subdivisions tree walk (not the flat fast-path)", childCode, parentCode)
	})

	t.Run("unrelated code returns false (tree walk finds no ancestor)", func(t *testing.T) {
		// "ZZ-XX-NEVER" is absent from the DE tree: with no flat match the
		// method fetches /Subdivisions, walks the parent-index, finds no
		// ancestor, and returns false — the tree walk in the FALSE direction.
		ok, err := c.IsInRegion(ctx, h, "ZZ-XX-NEVER")
		require.NoError(t, err)
		assert.False(t, ok,
			"a code absent from the DE tree must return false after the walk")
	})
}

// audit:ok 2026-05-30

// TestIntegration_ErrorPaths exercises live error/validation behavior:
//
//   - unknown but well-formed country "ZZ": passes the shape-only
//     validateCountry and reaches the upstream. The 2026-05-30 probe observed
//     the upstream returns a 2xx with an EMPTY result for ZZ (no *APIError),
//     so this asserts require.NoError + assert.Empty (Gold Rule 2: assert the
//     observed behavior, not an assumed 4xx).
//   - malformed country "Z" (one letter): fails isTwoASCIILetters client-side
//     and never hits the network → [errors.Is](err, ErrInvalidCountry). This
//     is deterministic from validate.go, independent of §6a.
//
// Not parallelized; skips silently when OPENHOLIDAYS_LIVE != "1".
func TestIntegration_ErrorPaths(t *testing.T) {
	if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
		t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
	}

	c := NewClient(WithTimeout(15 * time.Second))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("unknown but well-formed country", func(t *testing.T) {
		// "ZZ" passes the shape-only validateCountry and reaches upstream.
		// Observed 2026-05-30: upstream returns 2xx with an empty list (no
		// *APIError). Assert what was observed (Gold Rule 2).
		hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode: "ZZ",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.NoError(t, err,
			"ZZ is well-formed and reaches upstream; observed 2xx-empty on 2026-05-30")
		assert.Empty(t, hs,
			"upstream returns an empty result for the unknown country ZZ (2026-05-30 probe)")
	})

	t.Run("malformed country rejected client-side (no network)", func(t *testing.T) {
		// "Z" is one letter → fails isTwoASCIILetters before any HTTP.
		hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode: "Z",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.ErrorIs(t, err, ErrInvalidCountry,
			"a one-letter country code must fail client-side with ErrInvalidCountry "+
				"(deterministic from validate.go)")
		assert.Empty(t, hs, "no holidays returned when validation fails before the network")
	})
}
