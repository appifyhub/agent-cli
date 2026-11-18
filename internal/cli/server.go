package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/text"
)

const healthyStatus = "ok"

func (s *rootState) serverCommand() *cobra.Command {
	cat := s.deps.Text
	return withHelp(&cobra.Command{
		Use:   "server",
		Short: cat.T("command.server.short"),
		Long:  cat.T("command.server.long"),
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return s.runServer(cmd.Context()) },
	}, cat.T("command.server.examples", "name", s.deps.Build.Name), cat.T("command.server.environment"), cat.T("command.server.exit_codes"))
}

// runServer asks the backend for its health and prints the report. It needs no token and writes no file.
func (s *rootState) runServer(ctx context.Context) error {
	d := s.deps
	apiURL, err := s.apiURL()
	if err != nil {
		return err
	}
	if err := config.ValidateAPIURL(apiURL); err != nil {
		return s.failure(err)
	}
	client := s.newClient(apiURL)
	start := d.Now()
	health, err := client.Health(ctx)
	elapsed := d.Now().Sub(start)
	if err != nil {
		return s.serverFailure(apiURL, err)
	}
	if _, err := fmt.Fprint(d.Stdout, serverReport(d.Text, apiURL, health, elapsed.Milliseconds())); err != nil {
		return err
	}
	if health.Status != healthyStatus {
		return &Failure{Code: ExitFailure, Silent: true}
	}
	return nil
}

// serverFailure names the address in the errors only this command has: the user typed or configured
// it, so it is the first thing to check. Rate limiting and anything else use the shared table.
func (s *rootState) serverFailure(apiURL string, err error) error {
	cat := s.deps.Text
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return s.failure(err)
	}
	switch {
	case apiErr.Kind == api.KindUnreachable:
		return &Failure{Code: ExitFailure, Message: cat.T("command.server.unreachable", "url", apiURL, "reason", reason(apiErr.Cause))}
	case apiErr.Kind == api.KindUnexpected && apiErr.Status >= 200 && apiErr.Status < 300:
		return &Failure{Code: ExitFailure, Message: cat.T("command.server.not_agent", "url", apiURL)}
	case apiErr.Kind == api.KindServerFault, apiErr.Kind == api.KindFailure, apiErr.Kind == api.KindUnexpected:
		return &Failure{Code: ExitFailure, Message: cat.T("command.server.http_error", "url", apiURL, "status", strconv.Itoa(apiErr.Status))}
	}
	return s.failure(err)
}

// reason is the innermost cause of a transport failure, like "connection refused" or "no such host",
// without the request URL the HTTP client repeats in its messages.
func reason(err error) string {
	for err != nil {
		next := errors.Unwrap(err)
		if next == nil {
			break
		}
		err = next
	}
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

// serverReport lays out the labels and values: the address without its scheme, the status, the
// version when there is one, every other detail the server shared in alphabetical order, and the
// time the answer took. Everything the server said goes through term.Clean.
func serverReport(cat *text.Catalog, apiURL string, h api.Health, milliseconds int64) string {
	type line struct{ label, value string }
	lines := []line{
		{cat.T("command.server.label_server"), term.Clean(displayAddress(apiURL), 0)},
		{cat.T("command.server.label_status"), term.Clean(h.Status, 200)},
	}
	if h.Version != "" {
		lines = append(lines, line{cat.T("command.server.label_version"), term.Clean(h.Version, 200)})
	}
	extra := append([]api.Field(nil), h.Extra...)
	sort.SliceStable(extra, func(i, j int) bool { return extra[i].Name < extra[j].Name })
	for _, f := range extra {
		if label := term.Clean(f.Name, 60); label != "" {
			lines = append(lines, line{label, term.Clean(f.Value, 200)})
		}
	}
	lines = append(lines, line{cat.T("command.server.label_response"), cat.T("command.server.response", "ms", strconv.FormatInt(milliseconds, 10))})

	width := 0
	for _, l := range lines {
		width = max(width, len([]rune(l.label)))
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.label + strings.Repeat(" ", width-len([]rune(l.label))) + "  " + l.value + "\n")
	}
	return b.String()
}

// displayAddress is the API address without its scheme.
func displayAddress(apiURL string) string {
	if _, rest, found := strings.Cut(apiURL, "://"); found {
		return rest
	}
	return apiURL
}
