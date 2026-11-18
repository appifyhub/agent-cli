package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/text"
)

// Failure is an error that already carries its exit code and final, user-facing text.
type Failure struct {
	Code    int
	Message string
}

func (f *Failure) Error() string { return f.Message }

// usageError is a malformed invocation: exit code 2, reduced to the first line of the message.
func usageError(message string) *Failure {
	line, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	return &Failure{Code: ExitUsage, Message: strings.TrimSpace(line)}
}

// Present is the one place that turns an error into stderr output and an exit code.
// An aborted prompt prints the abort notice; any other error prints a single "error: " line,
// and errors that carry no exit code of their own are failures (exit 1).
func Present(w io.Writer, cat *text.Catalog, err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, term.ErrAborted) {
		_, _ = fmt.Fprintln(w, cat.T("error.aborted"))
		return ExitFailure
	}
	failure := &Failure{Code: ExitFailure, Message: err.Error()}
	var known *Failure
	if errors.As(err, &known) {
		failure = known
	}
	_, _ = fmt.Fprintln(w, cat.T("error.line", "message", term.Clean(failure.Message, 0)))
	return failure.Code
}
