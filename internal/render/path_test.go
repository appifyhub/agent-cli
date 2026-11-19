package render_test

import (
	"testing"

	"github.com/appifyhub/agent-cli/internal/render"
)

func TestDisplayPath(t *testing.T) {
	for _, c := range []struct {
		name, path, home, goos, want string
	}{
		{"under home", "/home/morpheus/.agent-cli/config.toml", "/home/morpheus", "linux", "~/.agent-cli/config.toml"},
		{"on macOS", "/Users/morpheus/.abot", "/Users/morpheus", "darwin", "~/.abot"},
		{"outside home", "/tmp/agent-test/config.toml", "/home/morpheus", "linux", "/tmp/agent-test/config.toml"},
		{"exactly home", "/home/morpheus", "/home/morpheus", "linux", "~"},
		{"home with trailing slash", "/home/morpheus/.abot", "/home/morpheus/", "linux", "~/.abot"},
		{"sibling with the same prefix", "/home/morpheusx/.abot", "/home/morpheus", "linux", "/home/morpheusx/.abot"},
		{"override outside home", "/work/state", "/home/morpheus", "linux", "/work/state"},
		{"override inside home", "/home/morpheus/state", "/home/morpheus", "linux", "~/state"},
		{"no home known", "/home/morpheus/.abot", "", "linux", "/home/morpheus/.abot"},
		{"root as home", "/etc/abot", "/", "linux", "/etc/abot"},
		{"windows separators", `C:\Users\morpheus\.agent-cli\config.toml`, `C:\Users\morpheus`, "windows", `~\.agent-cli\config.toml`},
		{"windows mixed separators", `C:\Users\morpheus/.agent-cli`, `C:\Users\morpheus`, "windows", `~\.agent-cli`},
		{"windows is case-insensitive", `c:\users\MORPHEUS\.abot`, `C:\Users\morpheus`, "windows", `~\.abot`},
		{"windows outside home", `D:\state\config.toml`, `C:\Users\morpheus`, "windows", `D:\state\config.toml`},
		{"windows exactly home", `C:\Users\morpheus`, `C:\Users\morpheus`, "windows", "~"},
		{"windows sibling with the same prefix", `C:\Users\morpheusx\.abot`, `C:\Users\morpheus`, "windows", `C:\Users\morpheusx\.abot`},
		{"unix path with a backslash in a name", `/home/morpheus/a\b`, "/home/morpheus", "linux", `~/a\b`},
	} {
		if got := render.DisplayPath(c.path, c.home, c.goos); got != c.want {
			t.Errorf("%s: DisplayPath(%q, %q, %q) = %q, want %q", c.name, c.path, c.home, c.goos, got, c.want)
		}
	}
}
