// Command ohcli is the demo CLI for the github.com/egeek-tech/go-openholidays
// library. It dogfoods the public library surface (CLI-03) so the value
// proposition for v0.1.0 — public holidays AND school holidays per
// administrative subdivision — is exercised end-to-end from the same module
// that ships the library.
//
// Subcommand dispatch is hand-rolled on top of the stdlib flag package
// (D-03 / CL-21 from PROJECT.md — zero non-stdlib runtime dependencies).
// Each subcommand owns its own flag.FlagSet; usage errors flow to stderr
// with the literal "ohcli: " prefix (D-05) and a non-zero exit code (D-06):
//
//   - exit 0 — success (including the empty-result paths per D-07)
//   - exit 1 — runtime error (HTTP failure, decode failure, render failure)
//   - exit 2 — usage error (missing subcommand, bad flag, bad positional)
//
// All I/O flows through the io.Writer parameters of run and the subcommand
// handlers so Plan 02's table-driven tests can wire bytes.Buffer in place of
// os.Stdout / os.Stderr without re-implementing the CLI.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	openholidays "github.com/egeek-tech/go-openholidays"
)

// usage is the top-level help text printed on stderr for missing/unknown
// subcommands and on stdout for the help subcommand. Format mirrors the
// stdlib `go` tool: one line per subcommand with positional and flag hints.
const usage = `usage: ohcli <command> [flags]

Commands:
  public    <country> <year> [--lang xx] [--format text|json|csv] [--json] [--csv]
  school    <country> <year> [--region CC-RR] [--lang xx] [--format text|json|csv] [--json] [--csv]
  countries [--lang xx] [--format text|json|csv] [--json] [--csv]
  version

Environment:
  OPENHOLIDAYS_BASE_URL  override upstream base URL (test seam; Plan 02)
`

// main is the binary entrypoint. It delegates to run so the integration
// tests in Plan 02 can drive the full subcommand-dispatch pipeline with a
// custom args slice and captured stdout/stderr without calling os.Exit.
func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

// run is the testable entrypoint. It performs subcommand dispatch on
// args[1] and returns the process exit code (0/1/2 per D-06). stdout and
// stderr are accepted as io.Writer — not *os.File — so the Plan 02 tests
// can wire *bytes.Buffer for assertion.
//
// args[0] is the program name (mirrors os.Args layout); args[1] is the
// subcommand; args[2:] is forwarded to the matched handler.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	ctx := context.Background()
	switch args[1] {
	case "public":
		return cmdPublic(ctx, args[2:], stdout, stderr)
	case "school":
		return cmdSchool(ctx, args[2:], stdout, stderr)
	case "countries":
		return cmdCountries(ctx, args[2:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, ohcliVersion())
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "ohcli: unknown command %q\n%s", args[1], usage)
		return 2
	}
}

// newClient constructs the openholidays.Client every subcommand handler
// uses. It applies the canonical CLI defaults — User-Agent
// "ohcli/<version>" so server-side log greps can distinguish CLI traffic
// from library-direct traffic, and a 15-second per-request timeout
// (matches the library's documented default).
//
// The OPENHOLIDAYS_BASE_URL environment variable, when set non-empty, is
// forwarded to openholidays.WithBaseURL — this is the test seam Plan 02
// relies on to point the binary at an httptest.NewServer for the
// table-driven integration tests. Production users do not set this env
// var; the library default (https://openholidaysapi.org) applies.
func newClient() *openholidays.Client {
	opts := []openholidays.Option{
		openholidays.WithUserAgent("ohcli/" + ohcliVersion()),
		openholidays.WithTimeout(15 * time.Second),
	}
	if u := os.Getenv("OPENHOLIDAYS_BASE_URL"); u != "" {
		opts = append(opts, openholidays.WithBaseURL(u))
	}
	return openholidays.NewClient(opts...)
}

// The cmdPublic / cmdSchool / cmdCountries handlers live in their own
// files (public.go / school.go / countries.go) so each subcommand keeps a
// small, focused implementation with its own flag.FlagSet, validation
// chain, request construction, and renderer dispatch.

// reorderArgs splits argv into flags-first form so stdlib flag.Parse can
// consume `ohcli public PL 2025 --json` as well as the equivalent
// flag-first spelling. The stdlib flag package halts at the first
// non-flag token, so `[PL 2025 --json]` would otherwise yield NArg=3
// with --json unparsed. This helper preserves order within each group
// (flags keep their relative order, positionals keep theirs) so
// `--region PL-SL` keeps its value paired with its flag name.
//
// Recognition rules:
//   - A token beginning with '-' is a flag. A bare "-" or "--" is
//     treated as a positional (matches Go's flag package convention).
//   - A token of the form -name=value is a single self-contained flag.
//   - A flag of the form -name value (no '=') consumes the next token
//     as its value when the following arg does not itself start with
//     '-'. This is how stdlib flag parses non-bool flags
//     (`--region PL-SL`).
//   - Bool flags (named in the boolFlags set) consume no value.
//
// boolFlags lists the per-FlagSet bool flag names so the helper can
// distinguish `--json PL` (one bool flag, one positional) from
// `--region PL` (one flag whose value is PL). For ohcli's three
// subcommands the bool flags are exactly --json and --csv; --lang,
// --format, --region are strings.
func reorderArgs(args []string, boolFlags map[string]struct{}) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		// Bare "-" and "--" are positional per stdlib flag convention,
		// as are any tokens that do not begin with '-'.
		if a == "-" || a == "--" || len(a) < 2 || a[0] != '-' {
			positionals = append(positionals, a)
			continue
		}
		// Self-contained flag of form -name=value.
		if hasByte(a, '=') {
			flags = append(flags, a)
			continue
		}
		// Compute the bare flag name (strip leading "-" or "--").
		name := a
		for len(name) > 0 && name[0] == '-' {
			name = name[1:]
		}
		// Bool flag — consumes no value.
		if _, isBool := boolFlags[name]; isBool {
			flags = append(flags, a)
			continue
		}
		// Non-bool flag — consume the following token as its value when
		// the next arg is present and does not itself start with '-'.
		if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] != '-' {
			flags = append(flags, a, args[i+1])
			i++
			continue
		}
		// Trailing non-bool flag with no value — let flag.Parse emit the
		// canonical error (it expects a value).
		flags = append(flags, a)
	}
	return append(flags, positionals...)
}

// hasByte reports whether s contains the byte b. Inlined here to keep
// reorderArgs free of strings/bytes imports in the dispatcher file.
func hasByte(s string, b byte) bool {
	for i := range len(s) {
		if s[i] == b {
			return true
		}
	}
	return false
}
