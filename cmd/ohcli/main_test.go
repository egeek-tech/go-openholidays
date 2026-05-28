// Package main — tests for the ohcli top-level dispatcher (run).
//
// Gold Rule 3 application: exactly one TestRun per the run production
// function. Every scenario lives inside a t.Run subtest with
// t.Parallel(); require for preconditions, assert for verifications.
//
// TestRun exercises pure dispatch behavior — no HTTP, no fixtures —
// covering missing subcommand, unknown subcommand, version, and the
// help / -h / --help branches. The subcommand-specific tests live in
// public_test.go, school_test.go, and countries_test.go.

package main

import (
	"bytes"
	"strings"
	"testing"

	openholidays "github.com/egeek-tech/go-openholidays"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun exercises the run dispatcher's pre-subcommand paths: missing
// argv, unknown command, version, and the three help spellings.
func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("no args returns exit 2 with usage on stderr", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli"}, &stdout, &stderr)
		require.Equal(t, 2, code, "missing subcommand must exit 2 (usage error per D-06)")
		assert.Contains(t, stderr.String(), "usage:",
			"missing subcommand must print usage to stderr")
		assert.Empty(t, stdout.String(),
			"missing subcommand must not write to stdout")
	})

	t.Run("unknown command returns exit 2 with ohcli prefix", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "frobnicate"}, &stdout, &stderr)
		require.Equal(t, 2, code, "unknown subcommand must exit 2 (usage error per D-06)")
		assert.Contains(t, stderr.String(), "ohcli: unknown command",
			"unknown-command diagnostic must carry the D-05 'ohcli: ' prefix")
		assert.Contains(t, stderr.String(), "frobnicate",
			"diagnostic must echo the offending subcommand")
	})

	t.Run("version prints library Version to stdout", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		code := run([]string{"ohcli", "version"}, &stdout, &stderr)
		require.Equal(t, 0, code, "version must exit 0")
		// ohcliVersion falls back to openholidays.Version when the binary
		// is built from the working tree (no debug.ReadBuildInfo tag).
		// Either result is acceptable so long as the stdout matches the
		// resolved version exactly with a trailing newline.
		expected := ohcliVersion() + "\n"
		assert.Equal(t, expected, stdout.String(),
			"version output must equal ohcliVersion()+\"\\n\"")
		// The library Version constant must be a prefix of the resolved
		// version string for the default pseudo-version case (Pitfall 8
		// fallback semantics); when the goreleaser-stamped tag wins, the
		// resolved string is the tag itself. Both shapes must remain
		// non-empty.
		assert.NotEmpty(t, openholidays.Version,
			"library Version constant must not be empty")
		assert.Empty(t, stderr.String(),
			"version must not write to stderr")
	})

	t.Run("help prints usage to stdout", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			argv []string
		}{
			{name: "help subcommand", argv: []string{"ohcli", "help"}},
			{name: "--help long flag", argv: []string{"ohcli", "--help"}},
			{name: "-h short flag", argv: []string{"ohcli", "-h"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				var stdout, stderr bytes.Buffer
				code := run(tc.argv, &stdout, &stderr)
				require.Equal(t, 0, code, "%s must exit 0", tc.name)
				assert.Contains(t, stdout.String(), "usage:",
					"%s must print usage to stdout", tc.name)
				assert.True(t, strings.Contains(stdout.String(), "public") &&
					strings.Contains(stdout.String(), "school") &&
					strings.Contains(stdout.String(), "countries"),
					"%s usage must list every subcommand", tc.name)
				assert.Empty(t, stderr.String(),
					"%s must not write to stderr", tc.name)
			})
		}
	})
}
