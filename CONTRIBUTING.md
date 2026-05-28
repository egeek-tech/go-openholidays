# Contributing to go-openholidays

Thanks for considering a contribution. The project is small and the dev loop is intentionally short — run the commands below, open a PR, and the CI matrix takes it from there.

## Dev Loop

- `go test -race ./...` — unit tests with the race detector.
- `go test -race -cover ./...` — coverage report (target ≥ 80% per PROJECT.md).
- `golangci-lint run` — aggregated lint (config in [`.golangci.yml`](./.golangci.yml); v2.x).
- `govulncheck ./...` — vulnerability scan against the resolved call graph.
- `OPENHOLIDAYS_LIVE=1 go test -tags=integration ./...` — integration tests against the live OpenHolidays API. Network access required.
- `go test -fuzz=FuzzUnmarshalHoliday -fuzztime=30s .` — fuzz target for the Holiday decoder.

## Commits

This project uses [Conventional Commits 1.0](https://www.conventionalcommits.org/en/v1.0.0/). The prefix is parsed by goreleaser to bucket entries on the auto-generated GitHub Release changelog.

Examples:

- `feat(cli): add --csv output`
- `fix(retry): parseRetryAfter overflow on negative integer`
- `docs(05): refresh README`
- `test(holiday): cover empty-LocalizedText fallback`
- `chore(deps): bump actions/setup-go to v5`

Breaking changes go in the commit body with a `BREAKING CHANGE:` footer.

## Branch + PR Flow

1. Create a topic branch from `main` — name it after the change (e.g. `fix-retry-overflow`, `feat-csv-output`).
2. Push and open a Pull Request against `main`.
3. The CI matrix (`go-version: [1.23.x, 1.24.x, stable]`) must be green — including lint, race-detector unit tests, and `govulncheck`.
4. PRs are merged via **squash merge** so each landed change is one conventional-commit message on `main` that goreleaser can parse.

Pre-1.0 (`v0.x.y`) we may land breaking changes between minor releases; the changelog will call them out explicitly.

## Code Style

The Gold Project Rules (see [`.planning/codebase/CONVENTIONS.md`](./.planning/codebase/CONVENTIONS.md)) are non-negotiable and take precedence over agent recommendations:

- **Rule 1 — English-only.** Code, comments, godoc, test names, commit messages, README, and design docs are written in English. Non-ASCII strings are allowed only in `testdata/` fixtures and example output blocks that mirror real upstream payloads (e.g. Polish localized holiday names).
- **Rule 2 — Never guess; verify or ask.** Read the source, run the command, or hit the endpoint — do not guess and do not assume.
- **Rule 3 — Tests use testify + one-test-per-prod-function + `t.Run` per case.** `github.com/stretchr/testify/{assert,require}` is the assertion library; `require` for preconditions, `assert` for verifications. Subtests via `t.Run`.

Formatting is `gofmt`-clean. The lint set in `.golangci.yml` enforces `govet`, `errcheck`, `staticcheck`, `gosec`, `revive`, and `gocritic` at minimum.
