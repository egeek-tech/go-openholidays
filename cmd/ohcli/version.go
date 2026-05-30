// ohcli version resolution.
//
// This file implements the `ohcli version` resolution chain documented in
// RESEARCH §Pitfall 8: prefer the module-stamped version that
// [runtime/debug.ReadBuildInfo] exposes (set by `go install`, `go build`, and
// goreleaser's ldflags), fall back to the library's openholidays.Version
// constant when ReadBuildInfo reports "(devel)" or no version (the common
// case when the binary is built from inside the module's working tree).

package main

import (
	"runtime/debug"

	openholidays "github.com/egeek-tech/go-openholidays"
)

// audit:ok 2026-05-30

// ohcliVersion resolves the version string printed by `ohcli version` and
// embedded in the User-Agent ("ohcli/<version>") sent by newClient.
//
// Resolution order (Pitfall 8):
//
//  1. [runtime/debug.ReadBuildInfo]().Main.Version when it is set and not the
//     "(devel)" sentinel — populated by `go install ...@<tag>` and by
//     goreleaser via -ldflags.
//  2. openholidays.Version — the library's own version constant; used
//     when the binary is built locally from the working tree (the common
//     developer-loop case) where ReadBuildInfo returns "(devel)".
//
// Returning the library version as the fallback means `go run ./cmd/ohcli
// version` and `./ohcli version` (after `go build`) both print the same
// string the goreleaser-built artifact prints for the matching tag.
func ohcliVersion() string {
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return openholidays.Version
}
