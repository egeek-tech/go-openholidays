package openholidays

// Version is the semantic version of the go-openholidays library.
//
// It is the single source of truth for the User-Agent header sent by the
// HTTP client (set in a later phase) and for the --version output of the
// cmd/ohcli demo CLI. The value can be overridden at link time, for example:
//
//	go build -ldflags '-X github.com/egeek-tech/go-openholidays.Version=0.1.1-rc1'
//
// Release Please updates the literal on the next line at each Release PR.
var Version = "1.0.0" // x-release-please-version
