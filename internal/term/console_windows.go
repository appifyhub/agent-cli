//go:build windows

package term

import (
	"os"

	"golang.org/x/sys/windows"
)

const utf8CodePage = 65001

// SetupConsole enables ANSI processing and the UTF-8 output code page when stdout or stderr is a console;
// redirected output is never touched. Restore puts the previous console modes and code page back.
func SetupConsole(out, err *os.File) Console {
	var undo []func()
	ansi := true
	for _, f := range []*os.File{out, err} {
		h := windows.Handle(f.Fd())
		var mode uint32
		if windows.GetConsoleMode(h, &mode) != nil {
			continue
		}
		if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
			continue
		}
		if windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
			ansi = false
			continue
		}
		undo = append(undo, func() { _ = windows.SetConsoleMode(h, mode) })
	}

	if hasConsole(out, err) {
		if previous, cpErr := windows.GetConsoleOutputCP(); cpErr == nil && previous != utf8CodePage &&
			windows.SetConsoleOutputCP(utf8CodePage) == nil {
			undo = append(undo, func() { _ = windows.SetConsoleOutputCP(previous) })
		}
	}

	return Console{ANSI: ansi, Restore: func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}}
}

func hasConsole(files ...*os.File) bool {
	for _, f := range files {
		var mode uint32
		if windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil {
			return true
		}
	}
	return false
}
