package render_test

import (
	"testing"

	"github.com/appifyhub/agent-cli/internal/render"
)

func TestDisplayPath(t *testing.T) {
	for _, c := range []struct {
		name, path, home, goos, want string
	}{
		{"under home", "/home/milos/.agent-cli/config.toml", "/home/milos", "linux", "~/.agent-cli/config.toml"},
		{"on macOS", "/Users/milos/.abot", "/Users/milos", "darwin", "~/.abot"},
		{"outside home", "/tmp/agent-test/config.toml", "/home/milos", "linux", "/tmp/agent-test/config.toml"},
		{"exactly home", "/home/milos", "/home/milos", "linux", "~"},
		{"home with trailing slash", "/home/milos/.abot", "/home/milos/", "linux", "~/.abot"},
		{"sibling with the same prefix", "/home/milosx/.abot", "/home/milos", "linux", "/home/milosx/.abot"},
		{"override outside home", "/work/state", "/home/milos", "linux", "/work/state"},
		{"override inside home", "/home/milos/state", "/home/milos", "linux", "~/state"},
		{"no home known", "/home/milos/.abot", "", "linux", "/home/milos/.abot"},
		{"root as home", "/etc/abot", "/", "linux", "/etc/abot"},
		{"windows separators", `C:\Users\milos\.agent-cli\config.toml`, `C:\Users\milos`, "windows", `~\.agent-cli\config.toml`},
		{"windows mixed separators", `C:\Users\milos/.agent-cli`, `C:\Users\milos`, "windows", `~\.agent-cli`},
		{"windows is case-insensitive", `c:\users\MILOS\.abot`, `C:\Users\milos`, "windows", `~\.abot`},
		{"windows outside home", `D:\state\config.toml`, `C:\Users\milos`, "windows", `D:\state\config.toml`},
		{"windows exactly home", `C:\Users\milos`, `C:\Users\milos`, "windows", "~"},
		{"windows sibling with the same prefix", `C:\Users\milosx\.abot`, `C:\Users\milos`, "windows", `C:\Users\milosx\.abot`},
		{"unix path with a backslash in a name", `/home/milos/a\b`, "/home/milos", "linux", `~/a\b`},
	} {
		if got := render.DisplayPath(c.path, c.home, c.goos); got != c.want {
			t.Errorf("%s: DisplayPath(%q, %q, %q) = %q, want %q", c.name, c.path, c.home, c.goos, got, c.want)
		}
	}
}
