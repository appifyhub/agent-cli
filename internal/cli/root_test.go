package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcli "github.com/appifyhub/agent-cli"
	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

func runOn(t *testing.T, goos string, args ...string) testkit.Result {
	t.Helper()
	return testkit.Run(t, testkit.NewDeps(testkit.WithPlatform(goos, "amd64")), args...)
}

func TestRootHelpGolden(t *testing.T) {
	for _, c := range []struct{ goos, golden string }{{"darwin", "unix"}, {"linux", "unix"}, {"windows", "windows"}} {
		res := runOn(t, c.goos, "--help")
		if res.Code != 0 || res.Stderr != "" {
			t.Fatalf("%s: code %d, stderr %q", c.goos, res.Code, res.Stderr)
		}
		testkit.Golden(t, "root_help_"+c.golden, res.Stdout)
	}
}

func TestRootHelpSpecLines(t *testing.T) {
	res := runOn(t, "linux", "--help")
	lines := strings.Split(res.Stdout, "\n")
	if lines[0] != "agent-cli – command-line access to your agent: connect this machine and send notifications to your chats. More at https://agent.appifyhub.com" {
		t.Errorf("description line: %q", lines[0])
	}
	if lines[1] != "Usage: agent-cli <command> [flags]" {
		t.Errorf("usage line: %q", lines[1])
	}
	for _, want := range []string{
		"Flags: -v/--verbose, -h/--help, --version",
		"Environment: THE_AGENT_CLI_TOKEN, THE_AGENT_CLI_DEFAULT_CHAT, THE_AGENT_CLI_DEFAULT_PLATFORM, THE_AGENT_CLI_PASSPHRASE, THE_AGENT_CLI_API_URL, THE_AGENT_CLI_HOME, NO_COLOR",
		"Exit codes: 0 ok · 1 failure · 2 usage error · 3 not connected or locked · 4 chat unreachable · 5 insufficient credits · 6 rate limited",
	} {
		if !strings.Contains(res.Stdout, want+"\n") {
			t.Errorf("missing line %q", want)
		}
	}
}

func TestEveryHelpFormPrintsTheSameText(t *testing.T) {
	want := runOn(t, "linux", "--help")
	for _, args := range [][]string{{"-h"}, {"help"}, {}} {
		got := runOn(t, "linux", args...)
		if got.Code != 0 || got.Stdout != want.Stdout || got.Stderr != "" {
			t.Errorf("%v: %+v", args, got)
		}
	}
}

func TestCommandNameComesFromTheBuild(t *testing.T) {
	for _, name := range []string{"agent-cli", "agent-cli-dev", "abot", "abot-dev"} {
		deps := testkit.NewDeps(testkit.WithBuild(buildinfo.Info{Name: name}))
		lines := strings.Split(testkit.Run(t, deps, "--help").Stdout, "\n")
		if !strings.HasPrefix(lines[0], name+" – command-line access") || lines[1] != "Usage: "+name+" <command> [flags]" {
			t.Errorf("%s: help starts with %q / %q", name, lines[0], lines[1])
		}
		if v := testkit.Run(t, deps, "--version"); !strings.HasPrefix(v.Stdout, name+" ") {
			t.Errorf("%s: version line %q", name, v.Stdout)
		}
		if h := testkit.Run(t, deps, "completion", "-h"); !strings.Contains(h.Stdout, "Usage: "+name+" completion") {
			t.Errorf("%s: subcommand help %q", name, h.Stdout)
		}
		if c := testkit.Run(t, deps, "completion", "bash"); !strings.Contains(c.Stdout, "-F __start_"+name+" "+name+"\n") {
			t.Errorf("%s: completion script is not registered for the command name", name)
		}
	}
}

func TestProductURLComesFromTheBuild(t *testing.T) {
	deps := testkit.NewDeps(testkit.WithBuild(buildinfo.Info{ProductURL: "https://example.test/agent"}))
	first, _, _ := strings.Cut(testkit.Run(t, deps, "--help").Stdout, "\n")
	if !strings.HasSuffix(first, "More at https://example.test/agent") {
		t.Errorf("first line: %q", first)
	}
}

func TestVersionOutput(t *testing.T) {
	deps := testkit.NewDeps(
		testkit.WithPlatform("darwin", "arm64"),
		testkit.WithBuild(buildinfo.Info{Version: "0.1.0", Commit: "1a2b3c4", Date: "2026-11-14"}),
	)
	const line = "agent-cli 0.1.0 (darwin/arm64, 2026-11-14)\n"
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--version"}, line},
		{[]string{"--version", "-v"}, line + "  commit: 1a2b3c4\n"},
		{[]string{"-v", "--version"}, line + "  commit: 1a2b3c4\n"},
		{[]string{"--version", "--verbose"}, line + "  commit: 1a2b3c4\n"},
	}
	for _, c := range cases {
		if res := testkit.Run(t, deps, c.args...); res.Code != 0 || res.Stdout != c.want || res.Stderr != "" {
			t.Errorf("%v: %+v, want stdout %q", c.args, res, c.want)
		}
	}
	local := testkit.Run(t, testkit.NewDeps(), "--version")
	if local.Stdout != "agent-cli "+agentcli.Version()+" (linux/amd64, unknown)\n" {
		t.Errorf("local build: %q", local.Stdout)
	}
}

func TestVersionIsOnlyARootFlag(t *testing.T) {
	if res := runOn(t, "linux", "completion", "bash", "--version"); res.Code != 2 || !strings.HasPrefix(res.Stderr, "error: unknown flag: --version") {
		t.Errorf("%+v", res)
	}
}

func TestVerboseIsAcceptedBeforeAndAfterASubcommand(t *testing.T) {
	for _, args := range [][]string{
		{"-v", "completion", "bash"},
		{"completion", "bash", "-v"},
		{"completion", "-v", "bash"},
		{"completion", "bash", "--verbose"},
	} {
		if res := runOn(t, "linux", args...); res.Code != 0 || res.Stdout == "" || res.Stderr != "" {
			t.Errorf("%v: code %d, stderr %q", args, res.Code, res.Stderr)
		}
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		res := runOn(t, "linux", "completion", shell)
		if res.Code != 0 || res.Stderr != "" || !strings.Contains(res.Stdout, "agent-cli") || len(res.Stdout) < 200 {
			t.Errorf("%s: code %d, stderr %q", shell, res.Code, res.Stderr)
		}
	}
}

func TestCompletionRejectsOtherShells(t *testing.T) {
	for _, args := range [][]string{{"completion", "tcsh"}, {"completion"}, {"completion", "bash", "zsh"}} {
		res := runOn(t, "linux", args...)
		if res.Code != 2 || res.Stdout != "" || !strings.HasPrefix(res.Stderr, "error: ") || strings.Count(res.Stderr, "\n") != 1 {
			t.Errorf("%v: %+v", args, res)
		}
	}
	if res := runOn(t, "linux", "completion", "tcsh"); res.Stderr != "error: unsupported shell \"tcsh\"; use bash, zsh, fish or powershell\n" {
		t.Errorf("tcsh: %q", res.Stderr)
	}
}

func TestUsageErrorsAreOneLine(t *testing.T) {
	cases := []struct {
		args       []string
		wantStderr string
	}{
		{[]string{"frobnicate"}, "error: unknown command \"frobnicate\" for \"agent-cli\"\n"},
		{[]string{"completion", "--nope"}, "error: unknown flag: --nope\n"},
		{[]string{"--nope"}, "error: unknown flag: --nope\n"},
		{[]string{"help", "nope"}, "error: unknown help topic \"nope\"\n"},
		{[]string{"\x1b[31mred"}, "error: unknown command \"red\" for \"agent-cli\"\n"},
	}
	for _, c := range cases {
		if res := runOn(t, "linux", c.args...); res.Code != 2 || res.Stdout != "" || res.Stderr != c.wantStderr {
			t.Errorf("%q: %+v, want stderr %q", c.args, res, c.wantStderr)
		}
	}
}

func TestCommandHelpGoldens(t *testing.T) {
	for _, command := range []string{"completion", "env", "server", "help"} {
		for _, c := range []struct{ goos, golden string }{{"darwin", "unix"}, {"linux", "unix"}, {"windows", "windows"}} {
			forms := [][]string{{command, "-h"}, {command, "--help"}, {"help", command}}
			first := runOn(t, c.goos, forms[0]...)
			if first.Code != 0 || first.Stderr != "" {
				t.Fatalf("%v: %+v", forms[0], first)
			}
			testkit.Golden(t, command+"_help_"+c.golden, first.Stdout)
			for _, args := range forms[1:] {
				if res := runOn(t, c.goos, args...); res != first {
					t.Errorf("%v differs from %v:\n%s\n---\n%s", args, forms[0], res.Stdout, first.Stdout)
				}
			}
		}
	}
}

// commandNames reads the command names off the root help, the way a user finds them.
func commandNames(t *testing.T) []string {
	t.Helper()
	var names []string
	inCommands := false
	for _, line := range strings.Split(runOn(t, "linux", "help").Stdout, "\n") {
		switch {
		case line == "Commands:":
			inCommands = true
		case inCommands && strings.HasPrefix(line, "  "):
			names = append(names, strings.Fields(line)[0])
		case inCommands:
			return names
		}
	}
	return names
}

func TestEveryCommandHelpListsExamplesEnvironmentAndExitCodes(t *testing.T) {
	names := commandNames(t)
	if len(names) < 4 {
		t.Fatalf("found only %v", names)
	}
	for _, name := range names {
		res := runOn(t, "linux", "help", name)
		for _, want := range []string{"\nExamples:\n", "\nEnvironment: ", "\nExit codes: "} {
			if !strings.Contains(res.Stdout, want) {
				t.Errorf("help of %q lacks %q:\n%s", name, strings.TrimSpace(want), res.Stdout)
			}
		}
	}
}

func TestInformationalCommandsLeaveTheHomeMissing(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "missing-home")
	deps := testkit.NewDeps(testkit.WithEnv(map[string]string{"THE_AGENT_CLI_HOME": home}))
	t.Chdir(root)

	for _, args := range [][]string{
		{"--version"}, {"--help"}, {"help", "completion"}, {"completion", "bash"}, {}, {"completion", "tcsh"},
	} {
		testkit.Run(t, deps, args...)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Errorf("THE_AGENT_CLI_HOME was created: %v", err)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Errorf("commands wrote files: %v, %v", entries, err)
	}
}
