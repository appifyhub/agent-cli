package testkit

import (
	"context"

	"github.com/appifyhub/agent-cli/internal/term"
)

// FakeTerm is a scripted terminal: TTY flags, queued answers and a record of every prompt shown.
type FakeTerm struct {
	TTYIn, TTYOut, TTYErr bool
	ANSIOK                bool
	Secrets               []string
	Lines                 []string
	Prompts               []string
}

func (f *FakeTerm) IsTTY(s term.Stream) bool {
	switch s {
	case term.In:
		return f.TTYIn
	case term.Out:
		return f.TTYOut
	default:
		return f.TTYErr
	}
}

func (f *FakeTerm) ANSI() bool { return f.ANSIOK }

// ReadSecret records the prompt and returns the next scripted secret, or ErrAborted when none is left.
func (f *FakeTerm) ReadSecret(_ context.Context, prompt string) (string, error) {
	f.Prompts = append(f.Prompts, prompt)
	if len(f.Secrets) == 0 {
		return "", term.ErrAborted
	}
	answer := f.Secrets[0]
	f.Secrets = f.Secrets[1:]
	return answer, nil
}

// ReadLine records the prompt and returns the next scripted line, or ErrAborted when none is left.
func (f *FakeTerm) ReadLine(_ context.Context, prompt string) (string, error) {
	f.Prompts = append(f.Prompts, prompt)
	if len(f.Lines) == 0 {
		return "", term.ErrAborted
	}
	answer := f.Lines[0]
	f.Lines = f.Lines[1:]
	return answer, nil
}
