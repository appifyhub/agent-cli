package term

import (
	"bufio"
	"context"
	"io"
	"unicode/utf8"
)

const (
	keyCtrlC     = 0x03
	keyCtrlD     = 0x04
	keyBackspace = 0x08
	keyNewline   = '\n'
	keyEnter     = '\r'
	keyCtrlU     = 0x15
	keyEscape    = 0x1b
	keyDelete    = 0x7f

	// raw mode disables output post-processing, so line breaks need an explicit carriage return
	rawEOL = "\r\n"
)

// rawMode switches the terminal to raw mode and returns the function that restores the saved state.
type rawMode func() (restore func(), err error)

// readSecret enters raw mode, prints the prompt and reads a masked line. The terminal is restored on
// every path, including a cancelled context while the read is still blocked.
func readSecret(ctx context.Context, in *bufio.Reader, out io.Writer, raw rawMode, mask, prompt string) (string, error) {
	restore, err := raw()
	if err != nil {
		return "", err
	}
	defer restore()
	if _, err := io.WriteString(out, prompt); err != nil {
		return "", err
	}

	type result struct {
		secret string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		secret, err := readMasked(in, out, mask)
		done <- result{secret, err}
	}()
	select {
	case res := <-done:
		return res.secret, res.err
	case <-ctx.Done():
		_, _ = io.WriteString(out, rawEOL)
		return "", ErrAborted
	}
}

// readMasked reads runes until Enter. Printable runes echo the mask, Backspace and DEL erase one rune,
// Ctrl-U clears the line, escape sequences and other control bytes are ignored, and Ctrl-C, Ctrl-D on
// an empty line or the end of input abort.
func readMasked(in *bufio.Reader, out io.Writer, mask string) (string, error) {
	write := func(s string) { _, _ = io.WriteString(out, s) }
	var secret []rune
	for {
		r, _, err := in.ReadRune()
		if err != nil {
			write(rawEOL)
			return "", ErrAborted
		}
		switch r {
		case keyEnter, keyNewline:
			if r == keyEnter && in.Buffered() > 0 {
				if next, _ := in.Peek(1); next[0] == keyNewline {
					_, _ = in.ReadByte()
				}
			}
			write(rawEOL)
			return string(secret), nil
		case keyCtrlC:
			write(rawEOL)
			return "", ErrAborted
		case keyCtrlD:
			if len(secret) == 0 {
				write(rawEOL)
				return "", ErrAborted
			}
		case keyBackspace, keyDelete:
			if len(secret) > 0 {
				secret = secret[:len(secret)-1]
				write("\b \b")
			}
		case keyCtrlU:
			for range secret {
				write("\b \b")
			}
			secret = secret[:0]
		case keyEscape:
			skipEscape(in)
		default:
			if r == utf8.RuneError || r < 0x20 || (r >= 0x80 && r <= 0x9f) {
				continue
			}
			secret = append(secret, r)
			write(mask)
		}
	}
}

// skipEscape consumes the rest of an escape sequence: CSI and SS3 up to the final byte, otherwise one byte.
func skipEscape(in *bufio.Reader) {
	intro, err := in.ReadByte()
	if err != nil || (intro != '[' && intro != 'O') {
		return
	}
	for {
		b, err := in.ReadByte()
		if err != nil || (b >= 0x40 && b <= 0x7e) {
			return
		}
	}
}
