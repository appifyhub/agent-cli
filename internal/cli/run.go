package cli

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/term"
)

// Run executes one invocation and returns the process exit code.
func Run(ctx context.Context, deps Deps, args []string) int {
	root, state := newRoot(deps)
	return execute(ctx, root, state, args)
}

// execute runs the command tree; a panic becomes a one-line failure, with the stack only under --verbose.
func execute(ctx context.Context, root *cobra.Command, state *rootState, args []string) (code int) {
	deps := state.deps
	defer func() {
		if recovered := recover(); recovered != nil {
			code = Present(deps.Stderr, deps.Text, &Failure{Code: ExitFailure, Message: deps.Text.T("error.unexpected")}, deps.colorEnabled(term.Err))
			if state.verbose {
				_, _ = fmt.Fprintf(deps.Stderr, "%v\n%s", recovered, debug.Stack())
			}
		}
	}()
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		err = state.helpErr
	}
	return Present(deps.Stderr, deps.Text, err, deps.colorEnabled(term.Err))
}
