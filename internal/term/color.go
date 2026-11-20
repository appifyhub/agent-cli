package term

// ColorEnabled applies the color policy: a terminal that renders ANSI, NO_COLOR unset or empty, TERM not dumb.
func ColorEnabled(isTerminal bool, getenv func(string) string) bool {
	if !isTerminal {
		return false
	}
	if getenv("NO_COLOR") != "" {
		return false
	}
	return getenv("TERM") != "dumb"
}

// Style is an ANSI presentation style applied only to terminal output.
type Style string

const (
	Heading Style = "\x1b[1;36m"
	Success Style = "\x1b[32m"
	Warning Style = "\x1b[33m"
	Error   Style = "\x1b[31m"
)

// Paint wraps text in a style when color is enabled, leaving plain output unchanged.
func Paint(value string, style Style, enabled bool) string {
	if !enabled || value == "" {
		return value
	}
	return string(style) + value + "\x1b[0m"
}
