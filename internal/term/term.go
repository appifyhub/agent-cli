// Package term owns the standard streams: TTY detection, prompts, color policy, text sanitization
// and the Windows console setup. It is the only package besides main that touches os.Stdout and os.Stderr.
package term

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ErrAborted is returned when the user interrupts a prompt or input ends before an answer.
var ErrAborted = errors.New("aborted")

// Stream names one of the three standard streams.
type Stream int

const (
	In Stream = iota
	Out
	Err
)

// Terminal is everything the commands need to know about and ask from the user's terminal.
// Prompts are written to stderr, so stdout carries only command results.
type Terminal interface {
	IsTTY(s Stream) bool
	ANSI() bool
	ReadSecret(ctx context.Context, prompt string) (string, error)
	ReadLine(ctx context.Context, prompt string) (string, error)
}

// Options configure the real terminal.
type Options struct {
	// Mask is echoed once per typed rune of a secret.
	Mask string
	// ANSI tells whether escape sequences render on the output streams (see SetupConsole).
	ANSI bool
}

// Real is the Terminal backed by the process's standard files.
type Real struct {
	in, out, err *os.File
	reader       *bufio.Reader
	opts         Options
}

// New builds the real terminal over the given standard files.
func New(in, out, err *os.File, opts Options) *Real {
	return &Real{in: in, out: out, err: err, reader: bufio.NewReader(in), opts: opts}
}

func (r *Real) IsTTY(s Stream) bool {
	f := r.err
	switch s {
	case In:
		f = r.in
	case Out:
		f = r.out
	}
	return term.IsTerminal(int(f.Fd()))
}

func (r *Real) ANSI() bool {
	return r.opts.ANSI
}

// ReadLine prints the prompt on stderr and reads one line of canonical input.
// End of input and a cancelled context return ErrAborted.
func (r *Real) ReadLine(ctx context.Context, prompt string) (string, error) {
	if _, err := io.WriteString(r.err, prompt); err != nil {
		return "", err
	}
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := r.reader.ReadString('\n')
		if err != nil && line == "" {
			done <- result{err: err}
			return
		}
		done <- result{line: strings.TrimRight(line, "\r\n")}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			_, _ = io.WriteString(r.err, "\n")
			return "", ErrAborted
		}
		return res.line, nil
	case <-ctx.Done():
		_, _ = io.WriteString(r.err, "\n")
		return "", ErrAborted
	}
}

// ReadSecret prompts on stderr and reads a line from stdin in raw mode, echoing the mask once per rune.
func (r *Real) ReadSecret(ctx context.Context, prompt string) (string, error) {
	fd := int(r.in.Fd())
	raw := func() (func(), error) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return nil, err
		}
		return func() { _ = term.Restore(fd, state) }, nil
	}
	return readSecret(ctx, r.reader, r.err, raw, r.opts.Mask, prompt)
}
