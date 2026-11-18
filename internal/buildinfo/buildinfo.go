// Package buildinfo is the identity of the binary: its command name, version, commit, build date and default URLs.
package buildinfo

import (
	"fmt"
	"time"

	agentcli "github.com/appifyhub/agent-cli"
)

const (
	defaultName       = "agent-cli"
	defaultCommit     = "unknown"
	defaultDate       = "unknown"
	defaultAPIURL     = "http://localhost"
	defaultProductURL = "https://agent.appifyhub.com"
)

// set at build time with -X (see tools/build and .goreleaser.yaml); an empty value counts as unset.
// The version is not among them: it comes from the VERSION file, the single source of truth.
var (
	Name       = defaultName
	Commit     = defaultCommit
	Date       = defaultDate
	APIURL     = defaultAPIURL
	ProductURL = defaultProductURL
)

// Info is a snapshot of the build identity with defaults applied.
type Info struct {
	Name       string
	Version    string
	Commit     string
	Date       string
	APIURL     string
	ProductURL string
}

// Current returns the build identity of this binary.
func Current() Info {
	return Resolve(Info{
		Name:       Name,
		Commit:     Commit,
		Date:       Date,
		APIURL:     APIURL,
		ProductURL: ProductURL,
	})
}

// Resolve replaces empty fields with their defaults; the version defaults to the VERSION file.
func Resolve(i Info) Info {
	return Info{
		Name:       orDefault(i.Name, defaultName),
		Version:    orDefault(i.Version, agentcli.Version()),
		Commit:     orDefault(i.Commit, defaultCommit),
		Date:       orDefault(i.Date, defaultDate),
		APIURL:     orDefault(i.APIURL, defaultAPIURL),
		ProductURL: orDefault(i.ProductURL, defaultProductURL),
	}
}

// DisplayDate shows RFC 3339 and YYYY-MM-DD dates as YYYY-MM-DD and anything else as is.
func (i Info) DisplayDate() string {
	if t, err := time.Parse(time.RFC3339, i.Date); err == nil {
		return t.Format(time.DateOnly)
	}
	if t, err := time.Parse(time.DateOnly, i.Date); err == nil {
		return t.Format(time.DateOnly)
	}
	return i.Date
}

// VersionLine is the exact output of --version.
func (i Info) VersionLine(goos, goarch string) string {
	return fmt.Sprintf("%s %s (%s/%s, %s)", i.Name, i.Version, goos, goarch, i.DisplayDate())
}

// CommitLine is the extra --version line printed with --verbose.
func (i Info) CommitLine() string {
	return "  commit: " + i.Commit
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
