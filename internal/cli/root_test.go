package cli_test

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcli "github.com/appifyhub/agent-cli"
	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/cli"
	"github.com/appifyhub/agent-cli/internal/testkit"
	"github.com/appifyhub/agent-cli/internal/text"
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

func TestRootHelpAndCompletionOmitServer(t *testing.T) {
	for _, name := range commandNames(t) {
		if name == "server" {
			t.Fatal("root help lists server")
		}
	}

	got := runOn(t, "linux", "__complete", "")
	if got.Code != 0 {
		t.Fatal(got)
	}
	foundStatus := false
	for _, line := range strings.Split(got.Stdout, "\n") {
		name, _, _ := strings.Cut(line, "\t")
		switch name {
		case "server":
			t.Fatal("root completion lists server")
		case "status":
			foundStatus = true
		}
	}
	if !foundStatus {
		t.Fatalf("root completion lacks status: %+v", got)
	}
}

func TestServerIsUnknown(t *testing.T) {
	d := testkit.NewDeps(testkit.WithHTTP(commandRoundTrip(func(*http.Request) (*http.Response, error) {
		panic("unknown command must not contact the backend")
	})))
	got := testkit.Run(t, d, "server")
	if got.Code != 2 || got.Stdout != "" || got.Stderr != "error: unknown command \"server\" for \"agent-cli\"\n" {
		t.Fatal(got)
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
	for _, command := range []string{"completion", "env", "help", "revoke-all", "status", "chats", "chats default", "notify", "unlock", "skill", "skill show", "skill install", "skill status", "skill remove"} {
		for _, c := range []struct{ goos, golden string }{{"darwin", "unix"}, {"linux", "unix"}, {"windows", "windows"}} {
			path := strings.Fields(command)
			forms := [][]string{append(append([]string{}, path...), "-h"), append(append([]string{}, path...), "--help"), append([]string{"help"}, path...)}
			first := runOn(t, c.goos, forms[0]...)
			if first.Code != 0 || first.Stderr != "" {
				t.Fatalf("%v: %+v", forms[0], first)
			}
			if command == "status" && !strings.Contains(first.Stdout, "Use --offline to show local readiness and the last check time without sending requests or writing state.") {
				t.Errorf("%s: status help does not explain --offline:\n%s", c.goos, first.Stdout)
			}
			testkit.Golden(t, strings.ReplaceAll(command, " ", "_")+"_help_"+c.golden, first.Stdout)
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
	names = append(names, "chats default", "skill show", "skill install", "skill status", "skill remove")
	for _, name := range names {
		res := runOn(t, "linux", append([]string{"help"}, strings.Fields(name)...)...)
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
		{"--version"},
		{"--help"},
		{"help", "completion"},
		{"completion", "bash"},
		{},
		{"completion", "tcsh"},
		{"chats", "--help"},
		{"chats", "default", "--help"},
		{"chats", "default"},
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

func TestChatsPlatformCompletionAndBuildNamedHelp(t *testing.T) {
	r := newConnectRig(t, "linux")
	got := testkit.Run(t, r.deps, "__complete", "chats", "default", "--platform", "")
	if got.Code != 0 || got.Stdout != "telegram\nwhatsapp\n:4\n" || r.backend.Count() != 0 || len(r.keychain.Calls) != 0 || len(r.terminal.Prompts) != 0 {
		t.Fatal(got)
	}
	if _, err := os.Stat(r.home); !os.IsNotExist(err) {
		t.Error("completion created state")
	}
	for _, name := range []string{"agent-cli", "agent-cli-dev"} {
		r.deps.Build.Name = name
		for _, path := range [][]string{{"chats"}, {"chats", "default"}} {
			help := testkit.Run(t, r.deps, append([]string{"help"}, path...)...)
			if help.Code != 0 || help.Stderr != "" || !strings.Contains(help.Stdout, "Usage: "+name+" "+strings.Join(path, " ")) {
				t.Fatal(help)
			}
			for _, variable := range []string{"THE_AGENT_CLI_DEFAULT_CHAT", "THE_AGENT_CLI_DEFAULT_PLATFORM"} {
				if !strings.Contains(help.Stdout, variable) {
					t.Errorf("%v help omits %s", path, variable)
				}
			}
			if strings.Contains(help.Stdout, "agent.cloud.appifyhub.com") {
				t.Error("chats help embeds a hosted backend URL")
			}
		}
	}
}

func skillTestDeps(t *testing.T) (cli.Deps, string, string) {
	t.Helper()
	root := t.TempDir()
	home, cwd := filepath.Join(root, "home"), filepath.Join(root, "work", "nested")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	d := testkit.NewDeps(testkit.WithEnv(map[string]string{"THE_AGENT_CLI_TOKEN": "private-token-value", "THE_AGENT_CLI_PASSPHRASE": "private-passphrase-value", "THE_AGENT_CLI_HOME": filepath.Join(root, "cli-state")}))
	d.UserHomeDir = func() (string, error) { return home, nil }
	d.Getwd = func() (string, error) { return cwd, nil }
	return d, home, cwd
}

func TestSkillGuideAndPointer(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		d, home, cwd := skillTestDeps(t)
		d.GOOS = goos
		d.Text = text.MustNew(goos)
		for _, args := range [][]string{{"skill", "show"}, {"skill", "show", "--pointer"}} {
			got := testkit.Run(t, d, args...)
			if got.Code != 0 || got.Stderr != "" || !strings.Contains(got.Stdout, "agent-cli") {
				t.Fatal(got)
			}
			testkit.AssertNoSecret(t, "private-token-value", got.Stdout, got.Stderr)
			testkit.AssertNoSecret(t, "private-passphrase-value", got.Stdout, got.Stderr)
			if len(args) == 3 {
				if strings.Count(strings.TrimSpace(got.Stdout), "\n") >= 6 || !strings.Contains(got.Stdout, "agent-cli skill show") {
					t.Fatal(got)
				}
			} else {
				suffix := "unix"
				if goos == "windows" {
					suffix = "windows"
				}
				testkit.Golden(t, "skill_guide_"+suffix, got.Stdout)
				if strings.Count(got.Stdout, "\n") > 200 {
					t.Fatal("guide exceeds limit")
				}
			}
		}
		if _, err := os.Stat(home); !os.IsNotExist(err) {
			t.Fatal("show created user home", err)
		}
		if entries, err := os.ReadDir(cwd); err != nil || len(entries) != 0 {
			t.Fatal(entries, err)
		}
	}
	d, _, _ := skillTestDeps(t)
	d.Build = buildinfo.Resolve(buildinfo.Info{Name: "abot-dev"})
	got := testkit.Run(t, d, "skill", "show")
	if got.Code != 0 || got.Stderr != "" || strings.Contains(got.Stdout, "agent-cli") || strings.Contains(got.Stdout, "abot-dev server ") {
		t.Fatal(got)
	}
	for _, want := range []string{"abot-dev notify", "Run abot-dev status --offline first.", "the user may run abot-dev status without --offline."} {
		if !strings.Contains(got.Stdout, want) {
			t.Errorf("named-build skill guide lacks %q:\n%s", want, got.Stdout)
		}
	}
	d.UserHomeDir = func() (string, error) { panic("show must not resolve home") }
	d.Getwd = func() (string, error) { panic("show must not resolve cwd") }
	d.Getenv = func(string) string { panic("show must not read environment") }
	if got := testkit.Run(t, d, "skill", "show"); got.Code != 0 {
		t.Fatal(got)
	}
}

func TestSkillGroupAndValidation(t *testing.T) {
	if a, b := runOn(t, "linux", "skill"), runOn(t, "linux", "skill", "--help"); a != b || a.Code != 0 {
		t.Fatal(a, b)
	}
	for _, args := range [][]string{{"skill", "nope"}, {"skill", "setup"}, {"skill", "install"}, {"skill", "install", "codex", "claude"}, {"skill", "install", "devin"}, {"skill", "status", "--agent", "devin"}, {"skill", "remove", "codex", "--agent", "nope"}, {"skill", "install", "codex", "--user", "--project"}, {"skill", "status", "--user", "--project"}, {"skill", "remove", "--user", "--project"}} {
		d, home, cwd := skillTestDeps(t)
		got := testkit.Run(t, d, args...)
		if got.Code != 2 || got.Stdout != "" || !strings.HasPrefix(got.Stderr, "error: ") || strings.Count(got.Stderr, "\n") != 1 {
			t.Fatal(args, got)
		}
		if _, err := os.Stat(home); !os.IsNotExist(err) {
			t.Fatal("validation created home", err)
		}
		if entries, err := os.ReadDir(cwd); err != nil || len(entries) != 0 {
			t.Fatal("validation wrote files", entries, err)
		}
		if strings.Contains(got.Stderr, "unknown agent") {
			for _, name := range []string{"claude", "codex", "opencode", "pi", "omp", "windsurf", "cursor", "gemini", "antigravity"} {
				if !strings.Contains(got.Stderr, name) {
					t.Fatal(got)
				}
			}
		}
	}
}

func TestSkillInstallStatusRemove(t *testing.T) {
	for _, project := range []bool{false, true} {
		d, home, cwd := skillTestDeps(t)
		scope, root := "--user", home
		if project {
			scope, root = "--project", cwd
		}
		path := filepath.Join(root, ".agents", "skills", "agent-cli", "SKILL.md")
		run := func(wantCode int, want string, args ...string) testkit.Result {
			t.Helper()
			got := testkit.Run(t, d, append([]string{"skill"}, args...)...)
			if got.Code != wantCode || !strings.Contains(got.Stdout+got.Stderr, want) {
				t.Fatal(args, got)
			}
			testkit.AssertNoSecret(t, "private-token-value", got.Stdout, got.Stderr)
			return got
		}
		run(0, "missing", "status", scope, "--agent", "codex")
		run(0, "would install", "install", "codex", scope, "--dry-run")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("dry run wrote", err)
		}
		run(0, "installed", "install", "codex", scope)
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		testkit.AssertNoSecret(t, "private-token-value", string(original))
		testkit.AssertNoSecret(t, "private-passphrase-value", string(original))
		run(0, "unchanged", "install", "codex", scope)
		run(0, "current", "status", scope, "--agent", "codex", "--agent", "cursor")
		edited := append(append([]byte(nil), original...), []byte("edited\n")...)
		if err := os.WriteFile(path, edited, 0o600); err != nil {
			t.Fatal(err)
		}
		run(0, "outdated", "status", scope, "--agent", "codex")
		run(0, "updated", "install", "codex", scope)
		if err := os.WriteFile(path, []byte("foreign skill\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		run(1, "conflict", "install", "codex", scope)
		run(0, "foreign", "remove", "codex", scope)
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != "foreign skill\n" {
			t.Fatal("foreign changed", err)
		}
		run(0, "updated", "install", "codex", scope, "--force")
		run(0, "would remove", "remove", "codex", scope, "--dry-run")
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
		removed := run(0, "removed", "remove", scope, "--all")
		if !strings.Contains(removed.Stdout, "Codex: removed") || !strings.Contains(removed.Stdout, "Cursor: removed") {
			t.Fatal("shared path operated twice", removed)
		}
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Dir(filepath.Dir(path))); err != nil {
			t.Fatal("parent removed", err)
		}
	}
}

func TestSkillNativeLocationsAndIsolation(t *testing.T) {
	d, home, cwd := skillTestDeps(t)
	override := filepath.Join(t.TempDir(), "claude-config")
	d.Getenv = func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return override
		}
		return ""
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(cwd), ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"claude", "codex", "opencode", "pi", "omp", "windsurf", "cursor", "gemini", "antigravity"} {
		for _, project := range []bool{false, true} {
			args := []string{"skill", "install", a}
			root := home
			if project {
				root = cwd
				args = append(args, "--project")
			}
			want := filepath.Join(root, ".agents", "skills", "agent-cli", "SKILL.md")
			if a == "claude" {
				if project {
					want = filepath.Join(cwd, ".claude", "skills", "agent-cli", "SKILL.md")
				} else {
					want = filepath.Join(override, "skills", "agent-cli", "SKILL.md")
				}
			}
			if a == "antigravity" && !project {
				want = filepath.Join(home, ".gemini", "config", "skills", "agent-cli", "SKILL.md")
			}
			if got := testkit.Run(t, d, args...); got.Code != 0 {
				t.Fatal(got)
			}
			if _, err := os.Stat(want); err != nil {
				t.Fatal(a, project, want, err)
			}
		}
	}
	other := d
	other.Build = buildinfo.Resolve(buildinfo.Info{Name: "another-cli"})
	if got := testkit.Run(t, other, "skill", "install", "codex"); got.Code != 0 {
		t.Fatal(got)
	}
	if got := testkit.Run(t, d, "skill", "remove", "--project", "--all"); got.Code != 0 {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "agent-cli", "SKILL.md")); err != nil {
		t.Fatal("project removal affected user", err)
	}
	if got := testkit.Run(t, d, "skill", "remove", "--all"); got.Code != 0 {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "another-cli", "SKILL.md")); err != nil {
		t.Fatal("other build removed", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cwd), ".agents")); !os.IsNotExist(err) {
		t.Fatal("used Git root instead of cwd", err)
	}
}

func TestSkillUnreadableAndWriteFailure(t *testing.T) {
	d, home, _ := skillTestDeps(t)
	path := filepath.Join(home, ".agents", "skills", "agent-cli", "SKILL.md")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	got := testkit.Run(t, d, "skill", "status", "--agent", "codex")
	if got.Code != 0 || got.Stderr != "" || !strings.Contains(got.Stdout, "unreadable") {
		t.Fatal(got)
	}
	got = testkit.Run(t, d, "skill", "install", "codex")
	if got.Code != 1 || !strings.HasPrefix(got.Stderr, "error: ") || !strings.Contains(got.Stderr, "agent-cli") {
		t.Fatal(got)
	}
	d.Getwd = func() (string, error) { return "", errors.New("cwd unavailable") }
	if got := testkit.Run(t, d, "skill", "install", "codex", "--project"); got.Code != 1 {
		t.Fatal(got)
	}
}

func TestSkillCompletion(t *testing.T) {
	for _, args := range [][]string{{"__complete", "skill", "install", ""}, {"__complete", "skill", "status", "--agent", ""}, {"__complete", "skill", "remove", ""}} {
		got := runOn(t, "linux", args...)
		if got.Code != 0 {
			t.Fatal(got)
		}
		for _, name := range []string{"claude", "codex", "opencode", "pi", "omp", "windsurf", "cursor", "gemini", "antigravity"} {
			if !strings.Contains(got.Stdout, name+"\n") {
				t.Fatal(args, got)
			}
		}
	}
}

func TestSkillReportGoldens(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		d, _, _ := skillTestDeps(t)
		d.GOOS = goos
		d.Text = text.MustNew(goos)
		var output strings.Builder
		for _, args := range [][]string{{"install", "codex"}, {"status", "--agent", "codex"}, {"remove", "codex"}} {
			got := testkit.Run(t, d, append([]string{"skill"}, args...)...)
			if got.Code != 0 || got.Stderr != "" {
				t.Fatal(got)
			}
			output.WriteString(got.Stdout)
		}
		testkit.Golden(t, "skill_report_"+goos, output.String())
	}
}

func TestSkillExplicitAll(t *testing.T) {
	d, home, _ := skillTestDeps(t)
	for _, args := range [][]string{{"remove"}, {"install", "codex", "--all"}, {"remove", "codex", "--all"}, {"remove", "codex", "cursor"}, {"remove", "--all=false"}} {
		got := testkit.Run(t, d, append([]string{"skill"}, args...)...)
		if got.Code != 2 || got.Stdout != "" {
			t.Fatal(args, got)
		}
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("invalid selection changed home", err)
	}
	got := testkit.Run(t, d, "skill", "install", "--all", "--dry-run")
	if got.Code != 0 || strings.Count(got.Stdout, "would install") != 9 {
		t.Fatal(got)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("dry run created home", err)
	}
	got = testkit.Run(t, d, "skill", "install", "--all")
	if got.Code != 0 || strings.Count(got.Stdout, "installed") != 9 {
		t.Fatal(got)
	}
	got = testkit.Run(t, d, "skill", "status")
	if got.Code != 0 || strings.Count(got.Stdout, "current") != 9 {
		t.Fatal(got)
	}
	got = testkit.Run(t, d, "skill", "remove", "--all", "--dry-run")
	if got.Code != 0 || strings.Count(got.Stdout, "would remove") != 9 {
		t.Fatal(got)
	}
	got = testkit.Run(t, d, "skill", "remove", "--all")
	if got.Code != 0 || strings.Count(got.Stdout, "removed") != 9 {
		t.Fatal(got)
	}
	got = testkit.Run(t, d, "skill", "status")
	if got.Code != 0 || strings.Count(got.Stdout, "missing") != 9 {
		t.Fatal(got)
	}
}
