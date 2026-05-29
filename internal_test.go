// Package openholidays — CLIENT-10 mechanical invariant audit.
//
// This file contains TestNoInitOrGlobalState, the AST-walking test that
// locks the two CLIENT-10 invariants for every future commit:
//
//  1. No production *.go file in the repo root declares func init().
//  2. The only package-level var declarations across all production files
//     are the eight entries on the allowlist (the seven exported sentinels
//     from errors.go plus the unexported errEmptyDate from date.go).
//
// Test-only — appears only in *_test.go imports and never ships to consumers.
// See .planning/phases/01-foundation/01-RESEARCH.md §"Validation Architecture"
// for the AST-walk rationale and §"Common Pitfalls" Pitfall 6 for the
// CLIENT-10 background.

package openholidays

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allowedVars is the closed allowlist of package-level var identifiers that
// the AST audit accepts. Adding a new entry here REQUIRES updating PROJECT.md
// (CLIENT-10 forbids global mutable state by default — every entry is a
// deliberate, reviewed exception).
//
// Entries:
//
//   - ErrInvalidCountry, ErrInvalidLanguage, ErrDateRangeTooLarge,
//     ErrInvalidDateRange, ErrEmptyResponse — the five Phase 1 exported
//     sentinels declared in errors.go (D-13).
//   - ErrResponseTooLarge — the sixth exported sentinel added in Phase 2
//     for the 10 MiB response cap (D-24 / CL-07). Wrapped via [fmt.Errorf]
//     %w from Countries when the post-decode sentinel-byte read detects
//     truncation.
//   - ErrMalformedResponse — the seventh exported sentinel added in
//     Phase 3 Plan 04 for post-decode Holiday schema-drift detection
//     (D-65 / D-66 / CL-12). Wrapped via [fmt.Errorf] %w from
//     validateHolidays in request.go when an upstream Holiday violates
//     the StartDate/EndDate invariants. Listed AFTER ErrResponseTooLarge
//     in chronological-addition order, not alphabetical, so the diff
//     against prior phases stays a one-line append (Pitfall 6 protocol).
//   - errEmptyDate — the unexported sentinel declared in date.go for
//     UnmarshalJSON's null/empty rejection (D-06). Kept unexported so the
//     public sentinel surface stays small.
//   - CacheHitContextKey — the exported context-key var declared in
//     transport_cache.go (Phase 4 Plan 04). The unexported type
//     cacheHitKeyType backs it (Go's context-key idiom — private type,
//     exported var). The var is read-only after package init; consumers
//     detect cache hits via req.Context().Value(openholidays.CacheHitContextKey).
//     Cited as a DEVIATION from CONTEXT.md D-97 step 6 ("allowlist needs
//     NO updates") because the Pattern Mapper finding clarified that the
//     CLIENT-10 AST audit gates ALL exported package-level vars, not just
//     sentinel errors. Listed AFTER errEmptyDate per the chronological-
//     addition order convention (Pitfall 6 protocol — single-line append
//     keeps phase-to-phase diffs minimal).
//   - Version — the exported semantic-version string declared in
//     version.go. Promoted from `const` to `var` so the documented
//     `-ldflags '-X github.com/egeek-tech/go-openholidays.Version=...'`
//     override actually takes effect (the Go linker's -X flag has no
//     effect on a const). The value is read once per Client at
//     construction time for the User-Agent header and is never mutated
//     by library code; it remains effectively immutable after package
//     init (only release-pipeline link-time override mutates it).
//     Listed AFTER CacheHitContextKey per the chronological-addition
//     order convention (Pitfall 6 protocol).
//
// Sentinel error values are immutable identities (built via [errors.New] /
// [fmt.Errorf]) — they are not "mutable state" in the CLIENT-10 sense, but
// they are package-level vars syntactically, so they must be allowlisted.
var allowedVars = map[string]struct{}{
	"ErrInvalidCountry":    {},
	"ErrInvalidLanguage":   {},
	"ErrDateRangeTooLarge": {},
	"ErrInvalidDateRange":  {},
	"ErrEmptyResponse":     {},
	"ErrResponseTooLarge":  {},
	"ErrMalformedResponse": {},
	"errEmptyDate":         {},
	"CacheHitContextKey":   {},
	"Version":              {},
}

// skipDirs are directory names anywhere in the walk that the audit skips.
// Phase 1 has none of these subdirectories, but later phases will add
// cmd/, testdata/, and the planning/git trees should never be scanned.
// Defensive: the allowlist-only audit catches future regressions without
// modification.
//
// IN-05 follow-up: "internal" was previously listed here as a defensive
// skip, but doing so pre-emptively defeated CLIENT-10 for any future
// internal/ package. With "internal" removed, the audit will fail the
// day a non-allowlisted package-level var lands under internal/ —
// which is exactly the moment the maintainer should review (see
// CONTEXT.md D-34 / RESEARCH.md Pitfall 6). Empty-cost change: no
// internal/ package exists yet.
var skipDirs = map[string]struct{}{
	".planning": {},
	".git":      {},
	".claude":   {},
	"testdata":  {},
}

// TestNoInitOrGlobalState walks every production *.go file in the repo and
// asserts the two CLIENT-10 invariants via AST inspection: no init function,
// no unexpected package-level vars.
//
// CLIENT-10 ("no init() side effects, no global mutable state") is a
// constraint inherited from PROJECT.md. This test is the mechanical guard
// that catches regressions at CI time. The fast complementary sanity check
// `grep -rn 'func init' --include='*.go' .` is documented in PLAN 01-06.
func TestNoInitOrGlobalState(t *testing.T) {
	t.Parallel()

	repoRoot, err := findRepoRoot()
	require.NoError(t, err, "could not locate repo root for AST walk")

	var (
		failures  []string
		fileSet   = token.NewFileSet()
		filesSeen int
	)

	walkErr := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip non-allowlisted directories anywhere in the tree.
		if d.IsDir() {
			if _, skip := skipDirs[d.Name()]; skip {
				return filepath.SkipDir
			}
			// Skip the worktree-internal Claude Code directory too if it
			// surfaces under a different name (defensive).
			if strings.HasPrefix(d.Name(), ".claude") {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		// CLIENT-10 applies to production code only. Tests may have
		// package-level vars for setup helpers, TestMain, allowedVars,
		// etc. — they are not the threat the invariant guards.
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		// Phase 5 will introduce cmd/ohcli with its own main package.
		// CLIENT-10 ("no global mutable state") applies to the library;
		// the demo CLI is an external consumer that may want package-level
		// flag.FlagSet vars, etc. Explicitly skip any /cmd/ subtree.
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr == nil {
			// Use forward slashes for portable substring matching.
			relSlashed := filepath.ToSlash(rel)
			if strings.HasPrefix(relSlashed, "cmd/") || strings.Contains(relSlashed, "/cmd/") {
				return nil
			}
		}

		filesSeen++

		file, parseErr := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		require.NoErrorf(t, parseErr, "go/parser failed on %s", path)

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				// CLIENT-10 — Pitfall 6: no init() side effects.
				// A package-level init function has Name "init" and no
				// receiver. Methods named "init" on user types (with a
				// receiver) are syntactically legal and not what we forbid.
				if d.Name != nil && d.Name.Name == "init" && d.Recv == nil {
					pos := fileSet.Position(d.Pos())
					failures = append(failures,
						"init function in "+pos.Filename+":"+strconv.Itoa(pos.Line))
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				// CLIENT-10 — no global mutable state. Walk every name
				// in every spec of the var block (one block may declare
				// multiple identifiers via `var ( ... )`).
				for _, spec := range d.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, ident := range valueSpec.Names {
						if _, allowed := allowedVars[ident.Name]; allowed {
							continue
						}
						pos := fileSet.Position(ident.Pos())
						failures = append(failures,
							"unexpected package-level var '"+ident.Name+"' in "+
								pos.Filename+":"+strconv.Itoa(pos.Line))
					}
				}
			}
		}
		return nil
	})
	require.NoError(t, walkErr, "filepath.WalkDir failed")

	// Sort for deterministic output; report all violations at once so a
	// contributor sees every issue, not just the first.
	sort.Strings(failures)

	t.Run("found_at_least_one_production_go_file", func(t *testing.T) {
		t.Parallel()
		// Sanity guard: if the walk silently skips everything (broken skip
		// logic, wrong starting path), the absence of failures is meaningless.
		// Phase 1 ships at least 4 production files (doc.go, errors.go,
		// date.go, types.go, validate.go, version.go = 6); assert ≥ 4 so
		// the test stays valid as long as the package has any substance.
		assert.GreaterOrEqual(t, filesSeen, 4,
			"AST walk only visited %d production .go files — skip logic likely broken", filesSeen)
	})

	t.Run("no_init_and_no_unexpected_package_vars", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, failures,
			"CLIENT-10 violation(s) detected; see RESEARCH.md Pitfall 6 / Validation Architecture: %v",
			failures)
	})
}

// findRepoRoot locates the directory containing go.mod, starting from the
// test's working directory and walking up. Tests run with their package
// directory as the working directory, so for a single-package repo this is
// the repo root, but the walk-up is defensive against future test layouts.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
