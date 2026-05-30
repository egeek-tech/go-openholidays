// domain type contract for the OpenHolidays API.
//
// This file declares every struct the upstream OpenHolidays API returns:
// Holiday, Country, Language, Subdivision, plus the supporting value types
// LocalizedText, SubdivisionRef, GroupRef, and the HolidayType typed string
// with its six constants. JSON tags match the upstream camelCase wire shape
// exactly. Phase 2's endpoint methods decode upstream bytes directly into
// these structs; Phase 3 helpers operate on them.

package openholidays

import "strings"

// HolidayType is the typed-string enum for Holiday.Type.
//
// The six values below were verified against the live upstream OpenAPI spec
// on 2026-05-27. REQUIREMENTS.md TYPES-04 originally listed
// "Public, School, Bank, Observance" — the upstream actually returns
// "Public, Bank, Optional, School, BackToSchool, EndOfLessons" and never
// returns "Observance". The CL-04 scope clarification (recorded in
// PROJECT.md Key Decisions by Plan 06) ratifies shipping all six real values
// and dropping the spurious "Observance".
type HolidayType string

// HolidayType wire-format constants. Identifiers are PascalCase; values are
// the exact strings the upstream returns (also PascalCase).
const (
	// HolidayTypePublic is a public (statutory) holiday.
	HolidayTypePublic HolidayType = "Public"
	// HolidayTypeBank is a bank holiday (banking-sector observance).
	HolidayTypeBank HolidayType = "Bank"
	// HolidayTypeOptional is an optional / discretionary holiday.
	HolidayTypeOptional HolidayType = "Optional"
	// HolidayTypeSchool is a school holiday (e.g. Polish ferie zimowe).
	HolidayTypeSchool HolidayType = "School"
	// HolidayTypeBackToSchool marks the first instruction day of a term.
	HolidayTypeBackToSchool HolidayType = "BackToSchool"
	// HolidayTypeEndOfLessons marks the last instruction day of a term.
	HolidayTypeEndOfLessons HolidayType = "EndOfLessons"
)

// audit:ok 2026-05-30

// IsKnown reports whether t matches one of the six HolidayType constants
// declared by this package.
//
// HolidayType is a typed-string alias, so upstream is free to return values
// outside the documented set (schema drift, new enum values added without
// spec update). The default lenient decoder accepts unknown values and the
// opt-in strict decoder surfaces unknown *fields* but not unknown enum
// *values* — both decoders flow unknown HolidayType values through
// unchanged. Callers that branch on Holiday.Type SHOULD gate the branch
// on IsKnown to make the unknown-value path explicit:
//
//	if h.Type.IsKnown() {
//	    switch h.Type { ... }
//	} else {
//	    // log / warn / treat as opaque
//	}
//
// The check is O(1) (closed switch over six constants); no map allocation.
func (t HolidayType) IsKnown() bool {
	switch t {
	case HolidayTypePublic,
		HolidayTypeBank,
		HolidayTypeOptional,
		HolidayTypeSchool,
		HolidayTypeBackToSchool,
		HolidayTypeEndOfLessons:
		return true
	}
	return false
}

// LocalizedText is a (language, text) pair returned by the upstream API in
// every localized-string field (Holiday.Name, Holiday.Comment, Country.Name,
// Language.Name, Subdivision.Name, Subdivision.Category, Subdivision.Comment).
//
// Both fields are required by the upstream schema (minLength: 1 each).
type LocalizedText struct {
	// Language is the ISO 639-1 two-letter language code (e.g. "pl", "en").
	Language string `json:"language"`
	// Text is the localized text in the named language.
	Text string `json:"text"`
}

// SubdivisionRef is a lightweight reference (code + short display name)
// embedded in Holiday.Subdivisions when a holiday applies only to specific
// administrative subdivisions (e.g. Polish województwa for ferie zimowe).
//
// The upstream calls this shape SubdivisionReference; the library uses the
// shorter SubdivisionRef name per ARCHITECTURE.md naming guidance.
type SubdivisionRef struct {
	// Code is the subdivision code (e.g. "PL-SL" for Śląskie).
	Code string `json:"code"`
	// ShortName is the human-readable short name of the subdivision.
	ShortName string `json:"shortName"`
}

// GroupRef is a lightweight reference embedded in Holiday.Groups and
// Subdivision.Groups (e.g. Polish ferie cohorts A/B/C/D used to stagger
// school-holiday windows across regions).
//
// The upstream calls this shape GroupReference; the library uses GroupRef
// for symmetry with SubdivisionRef.
type GroupRef struct {
	// Code is the group code (e.g. "A").
	Code string `json:"code"`
	// ShortName is the human-readable short name of the group.
	ShortName string `json:"shortName"`
}

// Holiday represents one public or school holiday returned by the
// OpenHolidays API. Field order, names, and JSON tags match the verified
// upstream OpenAPI spec (2026-05-27).
//
// Multi-day holidays (school holidays, multi-day public observances) have
// StartDate < EndDate. For single-day public holidays, StartDate == EndDate.
// Always use both fields; do not assume a Holiday is a single calendar day
// (Pitfall TZ-3).
//
// Nullable upstream fields (Comment, Subdivisions, Groups, Tags) carry
// `omitempty` so a marshaled Holiday emits the same wire shape upstream
// produces. Quality is a schema-drift field observed in real responses but
// absent from the OpenAPI spec (Pitfall OH-2); the default lenient decoder
// tolerates both its presence and absence.
type Holiday struct {
	// ID is the upstream-assigned UUID identifying the holiday.
	ID string `json:"id"`
	// StartDate is the first calendar day of the holiday (UTC midnight).
	StartDate Date `json:"startDate"`
	// EndDate is the last calendar day of the holiday (UTC midnight).
	// For single-day holidays EndDate equals StartDate.
	EndDate Date `json:"endDate"`
	// Type is the upstream HolidayType enum.
	//
	// The six PascalCase values verified against the upstream OpenAPI spec
	// on 2026-05-27 are HolidayTypePublic, HolidayTypeBank,
	// HolidayTypeOptional, HolidayTypeSchool, HolidayTypeBackToSchool, and
	// HolidayTypeEndOfLessons. Callers MUST be prepared for upstream to
	// return values outside this set: HolidayType is a typed-string alias,
	// so any string the server emits unmarshal-decodes into Type as-is
	// (the default lenient decoder accepts it; the opt-in strict decoder
	// surfaces unknown *fields* but not unknown enum *values*). Use
	// HolidayType.IsKnown to test for membership in the documented set
	// before branching on the value.
	Type HolidayType `json:"type"`
	// Name is the per-language localized name of the holiday (array shape,
	// not a map — Pitfall OH-3).
	Name []LocalizedText `json:"name"`
	// Nationwide reports whether the holiday applies to the entire country.
	// When false, consult Subdivisions for the affected regions.
	Nationwide bool `json:"nationwide"`
	// RegionalScope is the upstream regional-scope marker. The closed value
	// set per spec is "National" / "Regional" / "Local". Shipped as plain
	// string for v0.1.0 (Assumption A4 — typed enums deferred to v0.2 if
	// downstream helpers need to branch on this value).
	RegionalScope string `json:"regionalScope"`
	// TemporalScope is the upstream temporal-scope marker. The closed value
	// set per spec is "FullDay" / "HalfDay". Shipped as plain string for
	// v0.1.0 for the same reason as RegionalScope (Assumption A4).
	TemporalScope string `json:"temporalScope"`
	// Comment is optional per-language commentary on the holiday. Nullable
	// upstream; emitted only when populated.
	Comment []LocalizedText `json:"comment,omitempty"`
	// Subdivisions lists the administrative regions the holiday applies to
	// when Nationwide is false. Nullable upstream.
	Subdivisions []SubdivisionRef `json:"subdivisions,omitempty"`
	// Groups lists the group memberships of the holiday (e.g. Polish ferie
	// cohorts). Nullable upstream.
	Groups []GroupRef `json:"groups,omitempty"`
	// Tags is a free-form tag list. Nullable upstream and newly verified
	// during 2026-05-27 OpenAPI fetch — not previously documented in
	// REQUIREMENTS.md (Assumption A5).
	Tags []string `json:"tags,omitempty"`
	// Quality is a schema-drift field observed in real upstream responses
	// but absent from the OpenAPI spec (Pitfall OH-2). The default lenient
	// decoder tolerates both presence and absence. Phase 4's opt-in strict
	// decoder will surface this field's presence for downstream callers
	// that care.
	Quality string `json:"quality,omitempty"`
}

// Country is the response shape for /Countries returned by the upstream
// OpenHolidays API. Use Country.NameFor to look up the localized country
// name for a given language code.
type Country struct {
	// IsoCode is the ISO 3166-1 alpha-2 country code (uppercase).
	IsoCode string `json:"isoCode"`
	// Name is the per-language localized country name.
	Name []LocalizedText `json:"name"`
	// OfficialLanguages lists the ISO 639-1 codes of the country's
	// official languages.
	OfficialLanguages []string `json:"officialLanguages"`
}

// audit:ok 2026-05-30

// NameFor returns the localized country name for the given ISO 639-1
// language code. Language matching is case-insensitive (strings.EqualFold)
// so "PL" matches a "pl" entry. When the requested language is not found,
// NameFor falls back to the first entry in the Name slice. Returns the
// empty string only when Name is empty.
//
// The accessor is named NameFor (not Name) because Country already has a
// Name field of type []LocalizedText — a method named Name(lang) would
// collide with the field. The same shape is used by Language.NameFor and
// Subdivision.NameFor (CL-05).
func (c Country) NameFor(lang string) string {
	return pickLocalized(c.Name, lang)
}

// Language is the response shape for /Languages returned by the upstream
// OpenHolidays API. Use Language.NameFor to look up the localized language
// name for a given language code.
type Language struct {
	// IsoCode is the ISO 639-1 two-letter language code (lowercase).
	IsoCode string `json:"isoCode"`
	// Name is the per-language localized language name (e.g. one entry
	// per language the API can describe the language in).
	Name []LocalizedText `json:"name"`
}

// audit:ok 2026-05-30

// NameFor returns the localized language name for the given ISO 639-1
// language code. See Country.NameFor for the matching semantics.
func (l Language) NameFor(lang string) string {
	return pickLocalized(l.Name, lang)
}

// Subdivision is the response shape for /Subdivisions returned by the
// upstream OpenHolidays API. Subdivisions can be recursive — Subdivision.Children
// references the same type (e.g. a Polish województwo may contain powiaty).
//
// Use Subdivision.NameFor to look up the localized subdivision name for a
// given language code.
type Subdivision struct {
	// Code is the subdivision code (e.g. "PL-SL").
	Code string `json:"code"`
	// ShortName is the human-readable short name of the subdivision.
	ShortName string `json:"shortName"`
	// Name is the per-language localized full name of the subdivision.
	Name []LocalizedText `json:"name"`
	// Category is the per-language localized category label
	// (e.g. "voivodeship", "region").
	Category []LocalizedText `json:"category"`
	// OfficialLanguages lists the ISO 639-1 codes of the subdivision's
	// official languages.
	OfficialLanguages []string `json:"officialLanguages"`
	// IsoCode is the optional ISO 3166-2 code for the subdivision.
	// Nullable upstream; an empty value indicates the subdivision has no
	// assigned ISO 3166-2 code.
	IsoCode string `json:"isoCode,omitempty"`
	// Comment is optional per-language commentary on the subdivision.
	// Nullable upstream.
	Comment []LocalizedText `json:"comment,omitempty"`
	// Children is the recursive list of nested subdivisions (e.g. powiaty
	// inside a województwo). Nullable upstream.
	Children []Subdivision `json:"children,omitempty"`
	// Groups lists the group memberships of the subdivision (e.g. ferie
	// cohort A/B/C/D for Polish województwa). Nullable upstream.
	Groups []GroupRef `json:"groups,omitempty"`
}

// audit:ok 2026-05-30

// NameFor returns the localized subdivision name for the given ISO 639-1
// language code. See Country.NameFor for the matching semantics.
func (s Subdivision) NameFor(lang string) string {
	return pickLocalized(s.Name, lang)
}

// audit:ok 2026-05-30

// pickLocalized is the shared, unexported helper backing the three NameFor
// accessors. It walks entries linearly and returns the Text of the first
// LocalizedText whose Language matches lang case-insensitively
// (strings.EqualFold). On miss, it falls back to entries[0].Text when
// entries is non-empty, otherwise returns "".
//
// Linear scan is intentional: localized-text slices in this API carry at
// most ~14 entries (one per supported language), so building a map index
// would cost more than the scan it would replace.
func pickLocalized(entries []LocalizedText, lang string) string {
	for _, e := range entries {
		if strings.EqualFold(e.Language, lang) {
			return e.Text
		}
	}
	if len(entries) > 0 {
		return entries[0].Text
	}
	return ""
}
