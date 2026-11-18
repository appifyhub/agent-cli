// Package e2e_test runs the binaries built by tools/build, one per build variant, on the current OS.
// scripts/e2e.sh builds them and points the tests at them through binDirEnv; without it the tests skip.
package e2e_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

const binDirEnv = "THE_AGENT_CLI_E2E_BIN_DIR"

type result struct {
	code           int
	stdout, stderr string
}

// variants reads the build list from tools/variants, the one place the builds are defined.
func variants(t *testing.T) []string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "tools", "variants"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^VARIANTS="([^"]+)"$`).FindStringSubmatch(string(content))
	if match == nil {
		t.Fatal("tools/variants defines no VARIANTS")
	}
	return strings.Fields(match[1])
}

func version(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(content))
}

func binary(t *testing.T, name string) string {
	t.Helper()
	dir := os.Getenv(binDirEnv)
	if dir == "" {
		t.Skip(binDirEnv + " is not set; run scripts/e2e.sh")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("..", dir)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing binary: %v", err)
	}
	return path
}

// run executes the binary with piped streams, without any THE_AGENT_CLI_* variable from the caller,
// and with a home directory that must stay absent: informational commands write nothing.
func run(t *testing.T, bin string, args ...string) result {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	cmd := exec.CommandContext(t.Context(), bin, args...)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "THE_AGENT_CLI_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "THE_AGENT_CLI_HOME="+home)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	r := result{}
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		r.code = exitErr.ExitCode()
	default:
		t.Fatal(err)
	}
	r.stdout, r.stderr = stdout.String(), stderr.String()
	for _, out := range []string{r.stdout, r.stderr} {
		if strings.ContainsRune(out, '\x1b') {
			t.Errorf("%v: escape sequence in piped output %q", args, out)
		}
		if !utf8.ValidString(out) {
			t.Errorf("%v: output is not valid UTF-8: %q", args, out)
		}
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%v: created the home directory", args)
	}
	return r
}

func TestBinaries(t *testing.T) {
	ver := version(t)
	for _, name := range variants(t) {
		t.Run(name, func(t *testing.T) {
			bin := binary(t, name)
			quoted := regexp.QuoteMeta(name)

			t.Run("version", func(t *testing.T) {
				r := run(t, bin, "--version")
				want := regexp.MustCompile(`^` + quoted + ` ` + regexp.QuoteMeta(ver) + ` \(` + runtime.GOOS + `/` + runtime.GOARCH + `, \d{4}-\d{2}-\d{2}\)\n$`)
				if r.code != 0 || r.stderr != "" || !want.MatchString(r.stdout) {
					t.Errorf("code %d, stdout %q, stderr %q; want %s", r.code, r.stdout, r.stderr, want)
				}
				r = run(t, bin, "--version", "-v")
				want = regexp.MustCompile(`\n  commit: [0-9a-f]{7,}\n$`)
				if r.code != 0 || !want.MatchString(r.stdout) {
					t.Errorf("verbose: code %d, stdout %q; want the build commit", r.code, r.stdout)
				}
			})

			t.Run("root help", func(t *testing.T) {
				r := run(t, bin)
				for _, part := range []string{name + " – ", "\nUsage: " + name + " <command> [flags]\n", "\nCommands:\n", "  completion  ", "  help  ", "\nExit codes: "} {
					if !strings.Contains(r.stdout, part) {
						t.Errorf("root help lacks %q:\n%s", part, r.stdout)
					}
				}
				if r.code != 0 || r.stderr != "" {
					t.Errorf("code %d, stderr %q", r.code, r.stderr)
				}
				if h := run(t, bin, "--help"); h != r {
					t.Errorf("--help differs from the bare command: %+v", h)
				}
			})

			t.Run("command help", func(t *testing.T) {
				viaHelp, viaFlag := run(t, bin, "help", "completion"), run(t, bin, "completion", "-h")
				if viaHelp.code != 0 || viaHelp != viaFlag || !strings.Contains(viaHelp.stdout, "Usage: "+name+" completion") {
					t.Errorf("help completion %+v, completion -h %+v", viaHelp, viaFlag)
				}
			})

			t.Run("completion", func(t *testing.T) {
				for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
					r := run(t, bin, "completion", shell)
					if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, name) {
						t.Errorf("%s: code %d, stderr %q, script mentions %s: %t", shell, r.code, r.stderr, name, strings.Contains(r.stdout, name))
					}
				}
			})

			t.Run("usage errors exit 2", func(t *testing.T) {
				cases := []struct {
					args   []string
					stderr string
				}{
					{[]string{"nope"}, "error: unknown command \"nope\" for \"" + name + "\"\n"},
					{[]string{"completion", "tcsh"}, "error: unsupported shell \"tcsh\"; use bash, zsh, fish or powershell\n"},
					{[]string{"--nope"}, ""},
				}
				for _, c := range cases {
					r := run(t, bin, c.args...)
					if r.code != 2 || r.stdout != "" || !strings.HasPrefix(r.stderr, "error: ") || (c.stderr != "" && r.stderr != c.stderr) {
						t.Errorf("%v: code %d, stdout %q, stderr %q", c.args, r.code, r.stdout, r.stderr)
					}
				}
			})
		})
	}
}
