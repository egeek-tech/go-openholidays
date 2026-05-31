## Summary

<!-- What does this change do, and why? -->

Closes #

## Type of change

<!-- Pick the type that matches what the change actually IS (Project Rule 6 —
     never inflate or downgrade to manipulate release behavior). This repo
     squash-merges, so the PR title becomes the conventional commit, e.g.
     `fix(retry): parseRetryAfter overflow`. -->

- [ ] `feat` — new capability *(cuts a release)*
- [ ] `fix` — corrects a defect: wrong behavior or misleading public docs *(cuts a release)*
- [ ] `perf` — performance improvement *(cuts a release)*
- [ ] `deps` — dependency change *(cuts a release)*
- [ ] `docs` — consumer-facing documentation *(cuts a release)*
- [ ] `chore` / `refactor` / `test` / `ci` / `build` / `style` — no consumer-facing effect *(no release)*

## Breaking changes

<!-- v1.0+ is strict SemVer. If this breaks the public API, describe it here and
     add a `BREAKING CHANGE:` footer to the commit body. Otherwise: "None". -->

None

## Checklist

- [ ] PR title is a valid Conventional Commit, and the type honestly reflects the change (Rule 6)
- [ ] `go test -race ./...` passes
- [ ] `golangci-lint run` is clean
- [ ] `govulncheck ./...` is clean
- [ ] `gofmt`-clean (no formatting diffs)
- [ ] New / changed exported symbols have doc comments
- [ ] Tests follow one-test-per-prod-function with a `t.Run` per case (Rule 3)
- [ ] Any modified function had its `// audit:ok` line removed in the same commit (Rule 5)
- [ ] No new runtime dependency — non-stdlib imports stay confined to `*_test.go` (zero-dep policy)
- [ ] Integration tests considered if HTTP behavior changed (`OPENHOLIDAYS_LIVE=1 go test -tags=integration ./...`)
