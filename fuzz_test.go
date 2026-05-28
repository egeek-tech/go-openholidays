// Package openholidays — fuzz targets for the two JSON decode paths most
// likely to surface panic-class bugs under adversarial input (TEST-07).
//
// Two fuzz targets per TEST-07:
//
//   - FuzzParseLocalizedText decodes a JSON LocalizedText slice and calls
//     pickLocalized (the shared NameFor backend used by Holiday.NameFor,
//     Country.NameFor, Language.NameFor, and Subdivision.NameFor). The
//     invariant is panic-freedom — both the json.Unmarshal call and the
//     helper itself must accept any input bytes without crashing the
//     process.
//   - FuzzUnmarshalHoliday exercises encoding/json.Unmarshal into the
//     []Holiday slice the upstream endpoints return. The same invariant
//     holds: the stdlib decoder must never panic, regardless of how
//     malformed the bytes are.
//
// Seed corpus combines committed upstream fixtures (loaded via os.ReadFile
// and registered with F.Add) with three hand-curated adversarial seeds per
// target. Hand-curated files also live under testdata/fuzz/<Name>/ so
// `go test -fuzz` discovers them on first run.
//
// Both targets are same-package (`package openholidays`) so FuzzParseLocalizedText
// can reach the unexported pickLocalized helper directly.

package openholidays

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// FuzzParseLocalizedText exercises json.Unmarshal into []LocalizedText
// followed by pickLocalized. Both operations must never panic, regardless
// of input bytes. The decoder is allowed to return an error; the helper
// is allowed to return "" — only panics fail the fuzz run.
func FuzzParseLocalizedText(f *testing.F) {
	// Seed from committed upstream fixtures so the fuzzer explores
	// neighborhoods of real-world responses (per RESEARCH §1.4).
	for _, name := range []string{"public_holidays_pl_2025.json", "school_holidays_pl_2025.json"} {
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(body)
	}

	// Hand-curated adversarial seeds.
	f.Add([]byte("[]"))
	f.Add([]byte(`[{"language":"pl","text":"Wigilia"},{"language":"","text":""}]`))
	f.Add([]byte(`[{"language":"xx","text":null}]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var entries []LocalizedText
		// Return value intentionally ignored — the invariant is panic-freedom.
		_ = json.Unmarshal(data, &entries)
		_ = pickLocalized(entries, "pl")
		_ = pickLocalized(entries, "")
	})
}

// FuzzUnmarshalHoliday exercises json.Unmarshal into []Holiday. The
// invariant is panic-freedom; errors are acceptable.
func FuzzUnmarshalHoliday(f *testing.F) {
	// Seed from committed upstream fixtures.
	for _, name := range []string{"public_holidays_pl_2025.json", "school_holidays_pl_2025.json"} {
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(body)
	}

	// Hand-curated adversarial seeds.
	f.Add([]byte("{}"))
	f.Add([]byte(`{"id":"","startDate":"2025-01-01","endDate":"2024-12-31","type":"Public","name":[]}`))
	f.Add([]byte(`[{"id":"x","startDate":null,"endDate":"2025-01-01","type":"","name":null}]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var hs []Holiday
		// Return value intentionally ignored — the invariant is panic-freedom.
		_ = json.Unmarshal(data, &hs)
	})
}
