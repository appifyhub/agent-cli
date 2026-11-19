package render_test

import (
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/render"
	"github.com/appifyhub/agent-cli/internal/text"
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

func TestChatTableSample(t *testing.T) {
	rows := []render.ChatRow{
		{ID: "3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7", Platform: "telegram", Title: "Morpheus", Private: true},
		{ID: "9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e", Platform: "whatsapp", Title: "Morpheus", Private: true},
		{ID: "c0de1234c0de1234c0de1234c0de1234", Platform: "telegram", Title: "Dev Team"},
	}
	want := "  ID                                PLATFORM  TITLE     PRIVATE   (* = default)\n" +
		"* 3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7  telegram  Morpheus  yes\n" +
		"  9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e  whatsapp  Morpheus  yes\n" +
		"  c0de1234c0de1234c0de1234c0de1234  telegram  Dev Team  no\n"
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			if got := render.ChatTable(rows, rows[0].ID, text.MustNew(goos)); got != want {
				t.Errorf("ChatTable() = %q, want %q", got, want)
			}
		})
	}
}

func TestChatTableCleanAndUnicode(t *testing.T) {
	rows := []render.ChatRow{
		{ID: "\x1b[31méé\x1b[0m", Platform: "\x1b]0;hidden\aMätrix", Title: "  Né\x00o\n\t\u202eTeam  ", Private: true},
		{ID: "b", Platform: "matrix", Title: "\x1b[2J\t\n"},
	}
	want := "  ID  PLATFORM  TITLE     PRIVATE   (* = default)\n" +
		"  éé  Mätrix    Néo Team  yes\n" +
		"  b   matrix    untitled  no\n"
	if got := render.ChatTable(rows, "", text.MustNew("linux")); got != want {
		t.Errorf("ChatTable() = %q, want %q", got, want)
	}
}

func TestChatTableLongTitle(t *testing.T) {
	title := strings.Repeat("é", 80)
	rows := []render.ChatRow{
		{ID: "a", Platform: "matrix", Title: title},
		{ID: "b", Platform: "matrix", Title: "Neo", Private: true},
	}
	want := "  ID  PLATFORM  TITLE" + strings.Repeat(" ", 77) + "PRIVATE   (* = default)\n" +
		"  a   matrix    " + title + "  no\n" +
		"  b   matrix    Neo" + strings.Repeat(" ", 79) + "yes\n"
	if got := render.ChatTable(rows, "", text.MustNew("linux")); got != want {
		t.Errorf("ChatTable() = %q, want %q", got, want)
	}
}

func TestChatTableDefaultMarkers(t *testing.T) {
	rows := []render.ChatRow{
		{ID: "ab", Platform: "telegram", Title: "Neo"},
		{ID: "cd", Platform: "whatsapp", Title: "Morpheus", Private: true},
	}
	for _, c := range []struct {
		name, defaultID, first, second string
	}{
		{"case insensitive", "AB", "* ", "  "},
		{"second row", "CD", "  ", "* "},
		{"no default", "", "  ", "  "},
		{"platform default", "whatsapp", "  ", "  "},
		{"missing chat", "ef", "  ", "  "},
	} {
		t.Run(c.name, func(t *testing.T) {
			want := "  ID  PLATFORM  TITLE     PRIVATE   (* = default)\n" +
				c.first + "ab  telegram  Neo       no\n" +
				c.second + "cd  whatsapp  Morpheus  yes\n"
			if got := render.ChatTable(rows, c.defaultID, text.MustNew("linux")); got != want {
				t.Errorf("ChatTable() = %q, want %q", got, want)
			}
		})
	}
}

func TestChatTableUntitledAndEmpty(t *testing.T) {
	cat := text.MustNew("linux")
	want := "  ID  PLATFORM  TITLE  PRIVATE   (* = default)\n"
	if got := render.ChatTable(nil, "", cat); got != want {
		t.Errorf("ChatTable(nil) = %q, want %q", got, want)
	}
	rows := []render.ChatRow{{ID: "ab", Platform: "matrix"}}
	want = "  ID  PLATFORM  TITLE     PRIVATE   (* = default)\n" +
		"  ab  matrix    untitled  no\n"
	if got := render.ChatTable(rows, "", cat); got != want {
		t.Errorf("ChatTable(untitled) = %q, want %q", got, want)
	}
}
