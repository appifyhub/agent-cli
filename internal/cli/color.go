package cli

import (
	"strings"

	"github.com/appifyhub/agent-cli/internal/term"
)

func (d *Deps) colorEnabled(stream term.Stream) bool {
	return d.Term.ANSI() && term.ColorEnabled(d.Term.IsTTY(stream), d.Getenv)
}

func (d *Deps) styled(stream term.Stream, style term.Style, value string) string {
	return term.Paint(value, style, d.colorEnabled(stream))
}

func (d *Deps) styledFirstLine(stream term.Stream, style term.Style, value string) string {
	if !d.colorEnabled(stream) {
		return value
	}
	first, rest, found := strings.Cut(value, "\n")
	first = term.Paint(first, style, true)
	if !found {
		return first
	}
	return first + "\n" + rest
}
