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
