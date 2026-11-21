package cli

import (
	"fmt"
	"io"
	"strconv"

	"github.com/appifyhub/agent-cli/internal/api"
)

// newClient builds the backend client for a validated address. With --verbose it announces each
// request and describes error answers on stderr.
func (s *rootState) newClient(apiURL string) *api.Client {
	return s.clientWithDiagnostics(apiURL, s.deps.Stderr)
}

func (s *rootState) clientWithDiagnostics(apiURL string, diagnostics io.Writer) *api.Client {
	d := s.deps
	options := api.Options{
		BaseURL:   apiURL,
		Build:     d.Build,
		GOOS:      d.GOOS,
		GOARCH:    d.GOARCH,
		Transport: d.HTTP,
	}
	if s.verbose {
		options.OnRequest = func() { _, _ = fmt.Fprintln(diagnostics, d.Text.T("verbose.calling_backend")) }
		options.OnError = func(status, code int) {
			if code == 0 {
				_, _ = fmt.Fprintln(diagnostics, d.Text.T("verbose.http_error", "status", strconv.Itoa(status)))
				return
			}
			_, _ = fmt.Fprintln(diagnostics, d.Text.T("verbose.http_error_code", "status", strconv.Itoa(status), "code", strconv.Itoa(code)))
		}
	}
	return api.New(options)
}

// failure turns an error into the Failure the user sees.
func (s *rootState) failure(err error) error {
	return translate(s.deps.Text, s.deps.Build.Name, err)
}
