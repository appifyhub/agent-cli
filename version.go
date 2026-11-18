// Package agentcli exposes the repository's single version source: the VERSION file.
package agentcli

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version is the release version, read from the VERSION file at build time.
func Version() string {
	return strings.TrimSpace(version)
}
