package cli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/config"
)

// apiURL is the backend address in effect: the environment, then the config file when the home
// directory is known, then the build default. It reads the config file and writes nothing.
func (s *rootState) apiURL() (string, error) {
	d := s.deps
	var file config.File
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	switch {
	case err == nil:
		if file, err = config.Load(config.Path(home)); err != nil {
			return "", translate(d.Text, d.Build.Name, err)
		}
	case !errors.Is(err, config.ErrHomeUnavailable):
		return "", translate(d.Text, d.Build.Name, err)
	}
	url, err := config.APIURL(d.Getenv, file, d.Build.APIURL)
	if err != nil {
		return "", translate(d.Text, d.Build.Name, err)
	}
	return url, nil
}

// newClient builds the backend client for a validated address. With --verbose it announces each
// request and describes error answers on stderr.
func (s *rootState) newClient(apiURL string) *api.Client {
	d := s.deps
	options := api.Options{
		BaseURL:   apiURL,
		Build:     d.Build,
		GOOS:      d.GOOS,
		GOARCH:    d.GOARCH,
		Transport: d.HTTP,
	}
	if s.verbose {
		options.OnRequest = func() { _, _ = fmt.Fprintln(d.Stderr, d.Text.T("verbose.calling_backend")) }
		options.OnError = func(status, code int) {
			if code == 0 {
				_, _ = fmt.Fprintln(d.Stderr, d.Text.T("verbose.http_error", "status", strconv.Itoa(status)))
				return
			}
			_, _ = fmt.Fprintln(d.Stderr, d.Text.T("verbose.http_error_code", "status", strconv.Itoa(status), "code", strconv.Itoa(code)))
		}
	}
	return api.New(options)
}

// failure turns an error into the Failure the user sees.
func (s *rootState) failure(err error) error {
	return translate(s.deps.Text, s.deps.Build.Name, err)
}
