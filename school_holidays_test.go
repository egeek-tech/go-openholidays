// Package openholidays — tests for the SchoolHolidays endpoint method.
//
// One TestXxx per exported production function per Gold Rule 3:
// TestClient_SchoolHolidays covers the endpoint method. Every scenario
// lives in a t.Run subtest. require for preconditions, assert for
// verifications.
//
// Non-English strings in the fixture (e.g. "Ferie zimowe", "Wiosenna
// przerwa świąteczna", "Zimowa przerwa świąteczna", "Ferie letnie")
// mirror real upstream OpenHolidays responses and are admitted per
// CONVENTIONS.md Rule 1 testdata-fixture exception.

package openholidays

import (
	"context"
	"errors"
	"fmt"
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

// schoolHolidaysPL2025FixtureCapturedAt records the date
// testdata/school_holidays_pl_2025.json was captured from the live API.
// Re-capture when the upstream schema is suspected to have drifted. The
// fixture is not the authoritative shape — the live API is. D-69.
const schoolHolidaysPL2025FixtureCapturedAt = "2026-05-27"

// TestClient_SchoolHolidays covers ENDPT-05 + TEST-01 (4 error paths per
// endpoint) + the D-70 sanity assertions on the live PL 2025 fixture (7
// school periods total; at least one "Ferie zimowe" entry carries the
// PL-SL subdivision — the seam Plan 6's Holiday.IsInRegion test depends
// on) + the ErrMalformedResponse subtest gated by CL-12 + the two
// query-contract subtests for the optional SubdivisionCode and GroupCode
// pass-through fields (D-56).
//
// Gold Rule 3: exactly one TestClient_SchoolHolidays; every case is a
// t.Run.
func TestClient_SchoolHolidays(t *testing.T) {
	t.Parallel()

	t.Run("happy path PL 2025 returns 7 periods incl. Ferie zimowe with PL-SL", func(t *testing.T) {
		t.Parallel()

		body, err := os.ReadFile(filepath.Join("testdata", "school_holidays_pl_2025.json"))
		require.NoError(t, err, "fixture missing — re-capture from live API per Plan 03-05 Task 1 (captured %s)",
			schoolHolidaysPL2025FixtureCapturedAt)
		t.Logf("fixture captured %s", schoolHolidaysPL2025FixtureCapturedAt)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// URL-builder contract assertions (RESEARCH.md Pattern 3):
			// asserting inside the handler catches a regression that
			// mis-spells a query-param key — without these the test
			// would pass on the fixture body regardless of what the
			// client sent.
			assert.Equal(t, "/SchoolHolidays", r.URL.Path)
			q := r.URL.Query()
			assert.Equal(t, "PL", q.Get("countryIsoCode"),
				"country code must be uppercased canonical form")
			assert.Equal(t, "2025-01-01", q.Get("validFrom"))
			assert.Equal(t, "2025-12-31", q.Get("validTo"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		require.Len(t, holidays, 7, "fixture captured %s — re-capture if upstream shape drifted",
			schoolHolidaysPL2025FixtureCapturedAt)

		// D-70 sanity assert: locate a "Ferie zimowe" entry that carries
		// the PL-SL subdivision (Śląskie). Plan 6's Holiday.IsInRegion
		// test depends on this fixture entry. The upstream emits the PL
		// language tag in uppercase ("PL"), so NameFor("pl") relies on
		// pickLocalized's case-insensitive match (strings.EqualFold).
		// We accept any case-mismatched form of "ferie zimowe" via
		// strings.EqualFold because the plan explicitly says "with
		// case-insensitive comparison ok".
		var ferieZimoweWithSL *Holiday
		for i := range holidays {
			plName := holidays[i].NameFor("pl")
			if !strings.EqualFold(plName, "ferie zimowe") {
				continue
			}
			for _, sub := range holidays[i].Subdivisions {
				if strings.EqualFold(sub.Code, "PL-SL") {
					ferieZimoweWithSL = &holidays[i]
					break
				}
			}
			if ferieZimoweWithSL != nil {
				break
			}
		}
		require.NotNil(t, ferieZimoweWithSL,
			"Ferie zimowe entry with PL-SL subdivision not found in PL 2025 fixture (captured %s) — Plan 6 Holiday.IsInRegion test depends on this fixture entry",
			schoolHolidaysPL2025FixtureCapturedAt)
		// Sanity: confirm the entry IS multi-day (every PL ferie window
		// spans more than 1 calendar day — Plan 6 helpers exercise this).
		assert.True(t, ferieZimoweWithSL.EndDate.After(ferieZimoweWithSL.StartDate),
			"Ferie zimowe must span multiple days, got Start=%s End=%s",
			ferieZimoweWithSL.StartDate, ferieZimoweWithSL.EndDate)
	})

	t.Run("validation error: empty CountryIsoCode wraps ErrInvalidCountry", func(t *testing.T) {
		t.Parallel()
		// http://example.invalid is RFC 6761 reserved; if the validator
		// failed to short-circuit, the HTTP dispatch would fail loudly.
		c := NewClient(WithBaseURL("http://example.invalid"))
		holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			ValidFrom: NewDate(2025, time.January, 1),
			ValidTo:   NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.Nil(t, holidays)
		assert.True(t, errors.Is(err, ErrInvalidCountry),
			"expected ErrInvalidCountry via errors.Is, got %v", err)
	})

	t.Run("validation error: from > to wraps ErrInvalidDateRange", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2026, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.Nil(t, holidays)
		assert.True(t, errors.Is(err, ErrInvalidDateRange),
			"expected ErrInvalidDateRange via errors.Is, got %v", err)
	})

	t.Run("validation error: invalid LanguageIsoCode wraps ErrInvalidLanguage", func(t *testing.T) {
		t.Parallel()
		c := NewClient(WithBaseURL("http://example.invalid"))
		holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode:  "PL",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
			LanguageIsoCode: "X", // not 2 ASCII letters
		})
		require.Error(t, err)
		assert.Nil(t, holidays)
		assert.True(t, errors.Is(err, ErrInvalidLanguage),
			"expected ErrInvalidLanguage via errors.Is, got %v", err)
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
		holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.Nil(t, holidays)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr),
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 404, apiErr.StatusCode)
		assert.Equal(t, "/SchoolHolidays", apiErr.Path)
		assert.Equal(t, "Country not supported", apiErr.Message)
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
		_, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr),
			"expected *APIError, got %T: %v", err, err)
		assert.Equal(t, 500, apiErr.StatusCode)
		assert.Equal(t, "/SchoolHolidays", apiErr.Path)
		assert.Equal(t, "Internal Server Error", apiErr.Message,
			"title must win when detail is absent")
	})

	t.Run("malformed JSON wraps decode error (no sentinel)", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"this": "is not an array of Holiday"`)) // missing closing brace
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		// Malformed JSON must NOT match any of the typed sentinels.
		assert.False(t, errors.Is(err, ErrEmptyResponse))
		assert.False(t, errors.Is(err, ErrResponseTooLarge))
		assert.False(t, errors.Is(err, ErrMalformedResponse))
		assert.False(t, errors.Is(err, ErrInvalidCountry))
	})

	t.Run("ctx cancel returns context.Canceled", func(t *testing.T) {
		t.Parallel()
		// Handler blocks until the client disconnects; the test cancels
		// ctx after a short delay, exercising the in-flight cancellation
		// path (CLIENT-09: within ≤ 100 ms).
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		t.Cleanup(srv.Close)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.SchoolHolidays(ctx, SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled),
			"expected context.Canceled via errors.Is, got %v", err)
	})

	t.Run("optional GroupCode reaches the wire when non-empty", func(t *testing.T) {
		t.Parallel()
		// Empty-array body is structurally valid and validateHolidays
		// accepts it (no entries to iterate). The handler asserts the
		// GroupCode pass-through; the response shape is irrelevant.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/SchoolHolidays", r.URL.Path)
			assert.Equal(t, "A", r.URL.Query().Get("groupCode"),
				"GroupCode must be passed through verbatim per D-56")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
			GroupCode:      "A",
		})
		require.NoError(t, err)
		assert.Empty(t, holidays, "empty fixture must round-trip to empty slice")
	})

	t.Run("optional SubdivisionCode reaches the wire when non-empty", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/SchoolHolidays", r.URL.Path)
			assert.Equal(t, "PL-SL", r.URL.Query().Get("subdivisionCode"),
				"SubdivisionCode must be passed through verbatim per D-56")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode:  "PL",
			ValidFrom:       NewDate(2025, time.January, 1),
			ValidTo:         NewDate(2025, time.December, 31),
			SubdivisionCode: "PL-SL",
		})
		require.NoError(t, err)
		assert.Empty(t, holidays)
	})

	t.Run("malformed holiday (zero StartDate) wraps ErrMalformedResponse", func(t *testing.T) {
		t.Parallel()
		// "0001-01-01" round-trips to Date{} zero value via
		// Date.UnmarshalJSON (time.Parse succeeds; the resulting
		// time.Time IS the zero value). validateHolidays' IsZero check
		// then fires. "type":"School" matches HolidayTypeSchool so
		// decoding succeeds.
		bad := `[{
			"id":"bad-uuid","startDate":"0001-01-01","endDate":"2025-12-31",
			"type":"School","name":[{"language":"en","text":"X"}],
			"nationwide":true,"regionalScope":"National","temporalScope":"FullDay"
		}]`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(bad))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(WithBaseURL(srv.URL))
		holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.Error(t, err)
		assert.Nil(t, holidays, "endpoint must return nil holidays on validateHolidays failure")
		assert.True(t, errors.Is(err, ErrMalformedResponse),
			"expected ErrMalformedResponse via errors.Is, got %v", err)
	})
}

// TestClient_SchoolHolidays_IsInRegion_FerieZimowe is the SC#2-integrated
// characterization test: it loads testdata/school_holidays_pl_2025.json
// through the SchoolHolidays endpoint (httptest server), locates all four
// "Ferie zimowe" entries, and calls Holiday.IsInRegion("PL-SL") on each.
// Per the golden PL 2025 fixture, exactly cohort 1 (Jan 20 - Feb 2)
// carries PL-SL in its subdivisions; cohorts 2-4 do not.
//
// This is a Gold-Rule-3 narrow exception recorded as CL-14 in
// .planning/PROJECT.md Key Decisions: there is no new exported
// production function being tested here. The function exists to close
// the SC2-COMBINED gap from .planning/phases/03-endpoints-helpers/
// 03-VERIFICATION.md by providing a single integrated test scenario
// that satisfies the literal ROADMAP SC#2 wording — "correctly
// identifies the Śląskie ferie zimowe cohort while excluding the other
// three regional cohorts" — without splitting the proof across two
// unrelated test functions (school_holidays_test.go for "fixture has
// the entry" + holiday_test.go for "IsInRegion logic is correct").
// The CL-14 exception is explicitly scoped to THIS test only; future
// SchoolHolidays-related tests must continue to live inside the single
// TestClient_SchoolHolidays t.Run tree per Gold Rule 3.
func TestClient_SchoolHolidays_IsInRegion_FerieZimowe(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join("testdata", "school_holidays_pl_2025.json"))
	require.NoError(t, err, "fixture missing — re-capture from live API per Plan 03-05 Task 1 (captured %s)",
		schoolHolidaysPL2025FixtureCapturedAt)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithBaseURL(srv.URL))
	holidays, err := c.SchoolHolidays(context.Background(), SchoolHolidaysRequest{
		CountryIsoCode: "PL",
		ValidFrom:      NewDate(2025, time.January, 1),
		ValidTo:        NewDate(2025, time.December, 31),
	})
	require.NoError(t, err)
	require.Len(t, holidays, 7, "fixture must have 7 entries (captured %s)",
		schoolHolidaysPL2025FixtureCapturedAt)

	// Collect every "Ferie zimowe" entry from the fixture in StartDate
	// order. Per the golden fixture, there are exactly 4 such entries
	// (the four regional cohorts of the Polish school winter break).
	var ferieZimowe []Holiday
	for _, h := range holidays {
		if h.NameFor("pl") == "Ferie zimowe" {
			ferieZimowe = append(ferieZimowe, h)
		}
	}
	require.Len(t, ferieZimowe, 4,
		"fixture must contain exactly 4 Ferie zimowe cohorts (captured %s) — re-capture if upstream drifted",
		schoolHolidaysPL2025FixtureCapturedAt)

	// Cohort 1 (2025-01-20 .. 2025-02-02) is the only cohort carrying
	// PL-SL. The other three cohorts do not. Subtests below give a
	// per-cohort name so a CI failure points at the exact cohort that
	// regressed.
	type cohortCase struct {
		idx      int
		start    Date
		end      Date
		wantPLSL bool
	}
	cohorts := []cohortCase{
		{0, NewDate(2025, time.January, 20), NewDate(2025, time.February, 2), true},
		{1, NewDate(2025, time.January, 27), NewDate(2025, time.February, 9), false},
		{2, NewDate(2025, time.February, 3), NewDate(2025, time.February, 16), false},
		{3, NewDate(2025, time.February, 17), NewDate(2025, time.March, 2), false},
	}

	for _, tc := range cohorts {
		t.Run(formatCohortName(tc.idx, tc.start, tc.end, tc.wantPLSL), func(t *testing.T) {
			t.Parallel()
			require.Less(t, tc.idx, len(ferieZimowe),
				"fixture has fewer Ferie zimowe entries than expected — cohort index %d out of range", tc.idx)
			h := ferieZimowe[tc.idx]
			// Cohort identity check: dates must match the expected
			// window for the named cohort. If the upstream re-ordered
			// entries this fires loudly instead of silently testing
			// the wrong cohort.
			assert.True(t, h.StartDate.Equal(tc.start),
				"cohort %d StartDate mismatch: want %s, got %s — fixture may have re-ordered (captured %s)",
				tc.idx, tc.start, h.StartDate, schoolHolidaysPL2025FixtureCapturedAt)
			assert.True(t, h.EndDate.Equal(tc.end),
				"cohort %d EndDate mismatch: want %s, got %s — fixture may have re-ordered (captured %s)",
				tc.idx, tc.end, h.EndDate, schoolHolidaysPL2025FixtureCapturedAt)
			// The actual SC#2 assertion:
			got := h.IsInRegion("PL-SL")
			assert.Equal(t, tc.wantPLSL, got,
				"cohort %d IsInRegion(\"PL-SL\") = %v, want %v (subdivisions=%v)",
				tc.idx, got, tc.wantPLSL, h.Subdivisions)
		})
	}
}

// formatCohortName builds a stable, human-readable subtest name from the
// cohort index, date window, and expected IsInRegion("PL-SL") result.
// Kept package-private and adjacent to the only call site.
func formatCohortName(idx int, start Date, end Date, wantPLSL bool) string {
	expected := "excludes_PL-SL"
	if wantPLSL {
		expected = "matches_PL-SL"
	}
	return fmt.Sprintf("cohort_%d_%s_to_%s_%s",
		idx+1, start.Format("2006-01-02"), end.Format("2006-01-02"), expected)
}
