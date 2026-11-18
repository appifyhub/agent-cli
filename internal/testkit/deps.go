// Package testkit holds the test-only fakes and helpers; it is imported only from tests.
package testkit

import (
	"strings"
	"time"

	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/cli"
	"github.com/appifyhub/agent-cli/internal/text"
)

// FakeClock is a settable clock.
type FakeClock struct {
	Time time.Time
}

func (c *FakeClock) Now() time.Time { return c.Time }

// Option customizes the Deps built by NewDeps.
type Option func(*cli.Deps)

// NewDeps builds Deps for a linux/amd64 run of a default build: empty environment, empty stdin,
// a non-interactive FakeTerm, a fixed clock and UTC. Run replaces Stdout and Stderr.
func NewDeps(opts ...Option) cli.Deps {
	deps := cli.Deps{
		Getenv:   func(string) string { return "" },
		Stdin:    strings.NewReader(""),
		Term:     &FakeTerm{},
		Text:     text.MustNew("linux"),
		Now:      (&FakeClock{Time: time.Date(2026, 11, 14, 10, 21, 0, 0, time.UTC)}).Now,
		Location: time.UTC,
		GOOS:     "linux",
		GOARCH:   "amd64",
		Build:    buildinfo.Resolve(buildinfo.Info{}),
	}
	for _, opt := range opts {
		opt(&deps)
	}
	return deps
}

// WithEnv serves the given variables; everything else reads as unset.
func WithEnv(env map[string]string) Option {
	return func(d *cli.Deps) { d.Getenv = func(k string) string { return env[k] } }
}

func WithStdin(input string) Option {
	return func(d *cli.Deps) { d.Stdin = strings.NewReader(input) }
}

func WithTerm(t *FakeTerm) Option {
	return func(d *cli.Deps) { d.Term = t }
}

// WithPlatform sets GOOS and GOARCH and loads the catalog for that GOOS.
func WithPlatform(goos, goarch string) Option {
	return func(d *cli.Deps) {
		d.GOOS, d.GOARCH = goos, goarch
		d.Text = text.MustNew(goos)
	}
}

// WithBuild sets the build identity; empty fields get the defaults.
func WithBuild(info buildinfo.Info) Option {
	return func(d *cli.Deps) { d.Build = buildinfo.Resolve(info) }
}

func WithClock(c *FakeClock, loc *time.Location) Option {
	return func(d *cli.Deps) { d.Now, d.Location = c.Now, loc }
}
