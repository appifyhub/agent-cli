package term

// Console is the result of preparing the console for output: whether ANSI sequences render,
// and how to undo the setup. Restore must be called before the process exits.
type Console struct {
	ANSI    bool
	Restore func()
}
