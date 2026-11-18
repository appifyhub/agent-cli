//go:build !windows

package term

import "os"

// SetupConsole is a no-op outside Windows: terminals already speak ANSI and UTF-8.
func SetupConsole(out, err *os.File) Console {
	return Console{ANSI: true, Restore: func() {}}
}
