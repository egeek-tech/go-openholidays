// Package openholidays — microbenchmarks for the perf budget (TEST-11 /
// ROADMAP SC#3).
//
// Two benchmarks:
//
//   - BenchmarkClient_PublicHolidays measures the cold path (no cache,
//     in-memory httptest backend) against the committed PL 2025 public
//     holidays fixture. The ROADMAP SC#3 target is < 500 ms cold for one
//     year of PL public holidays.
//   - BenchmarkClient_Countries measures the cached path against the
//     Countries endpoint. Countries is the only endpoint family the
//     cache covers per RESIL-07 (PublicHolidays / SchoolHolidays are
//     intentionally NOT cached — temporal-data trap). This is the CL-18
//     reinterpretation of SC#3's "< 5 ms cached" budget: the target
//     applies to whichever endpoint the cache actually accelerates, which
//     is Countries / Languages / Subdivisions.
//
// Both benchmarks use httptest.NewServer to serve the committed fixture
// byte-for-byte; benchmarks measure the client end of the pipeline
// (request build + transport + JSON decode + validation) without
// crossing the public internet.
//
// Adding a "cached" sub-benchmark to BenchmarkClient_PublicHolidays
// would be the Pitfall 5 anti-pattern: PublicHolidays bypasses the cache
// by design, so a "cached" measurement there would be misleading. Only
// one WithCache call appears in this file by intent.

package openholidays

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BenchmarkClient_PublicHolidays measures the cold path of
// Client.PublicHolidays against an in-memory httptest backend serving the
// committed PL 2025 fixture. ROADMAP SC#3 target: < 500 ms cold.
func BenchmarkClient_PublicHolidays(b *testing.B) {
	body, err := os.ReadFile(filepath.Join("testdata", "public_holidays_pl_2025.json"))
	require.NoError(b, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	b.Cleanup(srv.Close)

	req := PublicHolidaysRequest{
		CountryIsoCode: "PL",
		ValidFrom:      NewDate(2025, time.January, 1),
		ValidTo:        NewDate(2025, time.December, 31),
	}

	b.Run("cold", func(b *testing.B) {
		c := NewClient(WithBaseURL(srv.URL))
		b.Cleanup(func() { _ = c.Close() })

		ctx := context.Background()
		b.ResetTimer()
		for range b.N {
			if _, err := c.PublicHolidays(ctx, req); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkClient_Countries measures the cached path of Client.Countries
// against an in-memory httptest backend serving the committed countries
// fixture. ROADMAP SC#3 target (CL-18 reinterpretation): < 5 ms cached.
// Countries is the only Holiday-adjacent endpoint family the cache
// covers (per RESIL-07).
func BenchmarkClient_Countries(b *testing.B) {
	body, err := os.ReadFile(filepath.Join("testdata", "countries.json"))
	require.NoError(b, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	b.Cleanup(srv.Close)

	b.Run("cached", func(b *testing.B) {
		c := NewClient(WithBaseURL(srv.URL), WithCache(time.Hour))
		b.Cleanup(func() { _ = c.Close() })

		ctx := context.Background()
		// Warm the cache so the measured loop hits the cacheTransport.
		_, _ = c.Countries(ctx, CountriesRequest{})

		b.ResetTimer()
		for range b.N {
			if _, err := c.Countries(ctx, CountriesRequest{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
