# go-openholidays

[![CI](https://github.com/egeek-tech/go-openholidays/actions/workflows/ci.yml/badge.svg)](https://github.com/egeek-tech/go-openholidays/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/egeek-tech/go-openholidays/branch/master/graph/badge.svg)](https://codecov.io/gh/egeek-tech/go-openholidays)
[![Go Report Card](https://goreportcard.com/badge/github.com/egeek-tech/go-openholidays)](https://goreportcard.com/report/github.com/egeek-tech/go-openholidays)
[![Go Reference](https://pkg.go.dev/badge/github.com/egeek-tech/go-openholidays.svg)](https://pkg.go.dev/github.com/egeek-tech/go-openholidays)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

An idiomatic, dependency-light Go client for the public [OpenHolidays API](https://www.openholidaysapi.org). Public holidays + school holidays per administrative subdivision (e.g. Polish *ferie* per województwo).

## Install

```sh
go get github.com/egeek-tech/go-openholidays
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/egeek-tech/go-openholidays"
)

func main() {
    c := openholidays.NewClient()
    defer func() { _ = c.Close() }()
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    hs, err := c.PublicHolidays(ctx, openholidays.PublicHolidaysRequest{
        CountryIsoCode: "PL",
        ValidFrom:      openholidays.NewDate(2025, time.January, 1),
        ValidTo:        openholidays.NewDate(2025, time.December, 31),
    })
    if err != nil {
        fmt.Println("error:", err)
        return
    }
    fmt.Printf("got %d Polish public holidays\n", len(hs))
}
```

The full surface — including every option and helper — is documented on [pkg.go.dev](https://pkg.go.dev/github.com/egeek-tech/go-openholidays). The runnable form of this quickstart lives at [`example_test.go`](./example_test.go) as `Example_quickstart`.

### School holidays per region

School-break granularity *per administrative subdivision* is the differentiator — e.g. Polish *ferie zimowe* for a single województwo. Reusing the `c` and `ctx` from above:

```go
hs, err := c.SchoolHolidays(ctx, openholidays.SchoolHolidaysRequest{
    CountryIsoCode:  "PL",
    ValidFrom:       openholidays.NewDate(2025, time.January, 1),
    ValidTo:         openholidays.NewDate(2025, time.December, 31),
    SubdivisionCode: "PL-SL", // OpenHolidays' own code scheme: PL-SL = Świętokrzyskie (not ISO 3166-2)
    // GroupCode:    "A",     // optional: filter to one ferie cohort (A/B/C/D)
})
if err != nil {
    fmt.Println("error:", err)
    return
}
fmt.Printf("got %d school-holiday spans for PL-SL\n", len(hs))
```

Every endpoint has a runnable example in [`example_test.go`](./example_test.go); these also render under the **Examples** tab on [pkg.go.dev](https://pkg.go.dev/github.com/egeek-tech/go-openholidays) (`ExampleClient_SchoolHolidays`, `ExampleClient_Subdivisions`, and so on).

## Public API

| Surface | Symbols |
|---------|---------|
| Endpoint methods | `Client.PublicHolidays`, `Client.SchoolHolidays`, `Client.Countries`, `Client.Languages`, `Client.Subdivisions` |
| Helpers | `Holiday.NameFor`, `Holiday.IsInRegion`, `Holiday.Days`, `Holiday.Range`, `Client.IsInRegion` |
| Localized names | `Country.NameFor`, `Language.NameFor`, `Subdivision.NameFor` |
| Constructors | `NewClient`, `NewDate`, `ParseDate`, `NewMemoryCache` |
| Opt-in middleware | `WithRetry`, `WithMaxRetryWait`, `WithCache`, `WithCacheBackend`, `WithRequestHook`, `WithStrictDecoding` |
| Configuration | `WithBaseURL`, `WithTimeout`, `WithUserAgent`, `WithLogger`, `WithHTTPClient` |
| Error sentinels | `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrInvalidDateRange`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`, `ErrResponseTooLarge`, `ErrMalformedResponse`, `*APIError` |

## CLI

A small demo CLI ships alongside the library:

```sh
go install github.com/egeek-tech/go-openholidays/cmd/ohcli@latest

ohcli public PL 2025
ohcli school PL 2025 --region PL-SL
ohcli countries --json
```

## Verifying release binaries

Released `ohcli` archives carry SLSA build-provenance attestations, signed via GitHub's keyless (Sigstore/Fulcio) flow, and verifiable with the [`gh` CLI](https://cli.github.com/) (≥ 2.49). Exit 0 means verified:

```sh
VERSION=1.0.0
gh release download "v$VERSION" --repo egeek-tech/go-openholidays --pattern 'ohcli_*_linux_amd64.tar.gz'
gh attestation verify "ohcli_${VERSION}_linux_amd64.tar.gz" --repo egeek-tech/go-openholidays
```

> **Verify the archive, not the binary.** The attested subjects are the released `.tar.gz`/`.zip` archives listed in `checksums.txt` — not the unpacked `ohcli` binary and not `checksums.txt` itself. Verifying either of those returns `HTTP 404: Not Found`. Likewise, a binary built locally with `go install …@latest` or `go build` is never attested, so a 404 there is expected.

For a hardened check, pin the signing workflow with `--signer-workflow egeek-tech/go-openholidays/.github/workflows/release-please.yml`. Note that `--source-ref refs/tags/…` would fail, because the signing ref is `refs/heads/master`.

## Architecture

See [docs/design.md](./docs/design.md) for the RoundTripper chain, cache architecture, retry semantics, and error model.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for the dev loop.

## License

MIT — see [LICENSE](./LICENSE).
