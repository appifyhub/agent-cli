package term

import (
	"strings"
	"unicode"
)

// Clean neutralizes text that originates outside the process before it reaches a terminal: escape
// sequences, control characters and bidirectional overrides are removed, whitespace runs collapse to
// one space, and a positive limit cuts the result to that many runes.
func Clean(s string, limit int) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	written, pendingSpace := 0, false
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == 0x1b:
			i = skipSequence(runes, i)
		case r == 0x9b:
			i = skipCSI(runes, i)
		case r == 0x90 || r == 0x9d || r == 0x98 || r == 0x9e || r == 0x9f:
			i = skipString(runes, i)
		case unicode.IsSpace(r):
			pendingSpace = written > 0
		case unicode.IsControl(r), isBidiControl(r):
		default:
			if limit > 0 && written+boolToInt(pendingSpace) >= limit {
				return b.String()
			}
			if pendingSpace {
				b.WriteByte(' ')
				written++
				pendingSpace = false
			}
			b.WriteRune(r)
			written++
		}
	}
	return b.String()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// isBidiControl reports the invisible direction marks and overrides that can make text display
// differently from what it says.
func isBidiControl(r rune) bool {
	switch {
	case r == 0x061c, r == 0x200e, r == 0x200f:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// skipSequence returns the index of the last rune of the escape sequence that starts at runes[i].
func skipSequence(runes []rune, i int) int {
	if i+1 >= len(runes) {
		return i
	}
	switch runes[i+1] {
	case '[':
		return skipCSI(runes, i+1)
	case ']', 'P', 'X', '^', '_':
		// OSC, DCS, SOS, PM and APC carry a payload up to the string terminator
		return skipString(runes, i+1)
	default:
		return i + 1
	}
}

// skipCSI skips parameters and intermediates up to the final byte.
func skipCSI(runes []rune, i int) int {
	for j := i + 1; j < len(runes); j++ {
		if runes[j] >= 0x40 && runes[j] <= 0x7e {
			return j
		}
	}
	return len(runes) - 1
}

// skipString skips a control string up to BEL, ST (0x9c) or ESC \; an unterminated string runs to the end.
func skipString(runes []rune, i int) int {
	for j := i + 1; j < len(runes); j++ {
		switch {
		case runes[j] == 0x07, runes[j] == 0x9c:
			return j
		case runes[j] == 0x1b && j+1 < len(runes) && runes[j+1] == '\\':
			return j + 1
		}
	}
	return len(runes) - 1
}
