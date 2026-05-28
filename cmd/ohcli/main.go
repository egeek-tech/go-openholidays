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

// The cmdPublic / cmdSchool / cmdCountries stubs below exist only so the
// Task 1 acceptance gate — `go build ./cmd/ohcli` exits 0 — passes before
// Task 3 lands the real implementations in dedicated public.go / school.go /
// countries.go files. Task 3 deletes these three stubs (and the
// `var _ = newClient` no-op reference below) when it splits the handlers
// out into their own files.

// cmdPublic is the Task 1 placeholder for the `public` subcommand. It is
// replaced by the real handler in cmd/ohcli/public.go in Task 3.
func cmdPublic(_ context.Context, _ []string, _, stderr io.Writer) int {
	fmt.Fprintln(stderr, "ohcli: public subcommand not yet implemented")
	return 1
}

// cmdSchool is the Task 1 placeholder for the `school` subcommand. It is
// replaced by the real handler in cmd/ohcli/school.go in Task 3.
func cmdSchool(_ context.Context, _ []string, _, stderr io.Writer) int {
	fmt.Fprintln(stderr, "ohcli: school subcommand not yet implemented")
	return 1
}

// cmdCountries is the Task 1 placeholder for the `countries` subcommand. It
// is replaced by the real handler in cmd/ohcli/countries.go in Task 3.
func cmdCountries(_ context.Context, _ []string, _, stderr io.Writer) int {
	fmt.Fprintln(stderr, "ohcli: countries subcommand not yet implemented")
	return 1
}

// Task 1 no-op reference keeping newClient reachable for go vet. Task 3
// removes this line when the real handlers (which call newClient) land.
var _ = newClient
