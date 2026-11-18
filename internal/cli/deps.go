// Package cli is the command-line front end: the Cobra command tree, help, error presentation and exit codes.
package cli

import (
	"io"
	"net/http"
	"time"

	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/text"
)

// Deps is the only seam of the program: everything that touches the outside world comes through here,
// so every command runs the same way in tests and in production.
type Deps struct {
	Getenv      func(string) string
	UserHomeDir func() (string, error)
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Term        term.Terminal
	Text        *text.Catalog
	Now         func() time.Time
	Location    *time.Location
	GOOS        string
	GOARCH      string
	Build       buildinfo.Info
	// HTTP is the transport of every backend call; nil means the default transport, which honors the proxy environment.
	HTTP http.RoundTripper
}
