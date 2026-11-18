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
	// Bare prints the message as it is, without the "error: " prefix: the not-connected guidance.
	Bare bool
	// Silent prints nothing: the command already showed its result and only sets the exit code.
	Silent bool
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
	if failure.Silent {
		return failure.Code
	}
	message := term.Clean(failure.Message, 0)
	if !failure.Bare {
		message = cat.T("error.line", "message", message)
	}
	_, _ = fmt.Fprintln(w, message)
	return failure.Code
}
