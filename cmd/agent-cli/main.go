// Command agent-cli is the command-line client of The Agent. This file only wires real dependencies;
// everything else lives under internal/.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/cli"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/text"
)

func main() {
	os.Exit(run())
}

// run returns the exit code instead of exiting, so the console is restored before os.Exit skips defers.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	console := term.SetupConsole(os.Stdout, os.Stderr)
	defer console.Restore()

	catalog := text.MustNew(runtime.GOOS)
	deps := cli.Deps{
		Getenv:   os.Getenv,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Term:     term.New(os.Stdin, os.Stdout, os.Stderr, term.Options{Mask: catalog.T("glyph.mask"), ANSI: console.ANSI}),
		Text:     catalog,
		Now:      time.Now,
		Location: time.Local,
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		Build:    buildinfo.Current(),
	}
	return cli.Run(ctx, deps, os.Args[1:])
}
