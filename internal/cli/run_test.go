package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/text"
)

// testkit imports this package, so these tests build their own Deps
func testDeps() (Deps, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return Deps{
		Getenv:   func(string) string { return "" },
		Stdin:    strings.NewReader(""),
		Stdout:   &stdout,
		Stderr:   &stderr,
		Term:     plainTerminal{},
		Text:     text.MustNew("linux"),
		Now:      time.Now,
		Location: time.UTC,
		GOOS:     "linux",
		GOARCH:   "amd64",
		Build:    buildinfo.Resolve(buildinfo.Info{}),
	}, &stdout, &stderr
}

type plainTerminal struct{}

func (plainTerminal) IsTTY(term.Stream) bool { return false }
func (plainTerminal) ANSI() bool             { return false }
func (plainTerminal) ReadSecret(context.Context, string) (string, error) {
	return "", term.ErrAborted
}

func (plainTerminal) ReadLine(context.Context, string) (string, error) {
	return "", term.ErrAborted
}

func runWith(extra *cobra.Command, args ...string) (int, string, string) {
	deps, stdout, stderr := testDeps()
	root, state := newRoot(deps)
	root.AddCommand(extra)
	state.guardArgs(extra)
	code := execute(context.Background(), root, state, args)
	return code, stdout.String(), stderr.String()
}

func panicking() *cobra.Command {
	return &cobra.Command{Use: "boom", RunE: func(*cobra.Command, []string) error { panic("kaboom") }}
}

func TestPanicIsOneLineWithoutTrace(t *testing.T) {
	code, stdout, stderr := runWith(panicking(), "boom")
	if code != ExitFailure || stdout != "" || stderr != "error: unexpected internal error\n" {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestPanicPrintsTraceUnderVerbose(t *testing.T) {
	for _, args := range [][]string{{"boom", "-v"}, {"-v", "boom"}, {"boom", "--verbose"}} {
		code, _, stderr := runWith(panicking(), args...)
		if code != ExitFailure || !strings.HasPrefix(stderr, "error: unexpected internal error\n") {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
		if !strings.Contains(stderr, "kaboom") || !strings.Contains(stderr, "goroutine") {
			t.Errorf("%v: stack trace missing from %q", args, stderr)
		}
	}
}

func TestCommandErrorsAreFailuresNotUsageErrors(t *testing.T) {
	failing := &cobra.Command{Use: "fail", RunE: func(*cobra.Command, []string) error {
		return fmt.Errorf("writing: %w", errors.New("broken pipe"))
	}}
	code, _, stderr := runWith(failing, "fail")
	if code != ExitFailure || stderr != "error: writing: broken pipe\n" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestCommandsWithoutValidatorRejectArguments(t *testing.T) {
	leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
	code, _, stderr := runWith(leaf, "leaf", "extra")
	if code != ExitUsage || !strings.HasPrefix(stderr, "error: ") || strings.Count(stderr, "\n") != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}

	group := &cobra.Command{Use: "group"}
	group.AddCommand(&cobra.Command{Use: "child", RunE: func(*cobra.Command, []string) error { return nil }})
	code, _, stderr = runWith(group, "group", "nope")
	if code != ExitUsage || stderr != "error: unknown command \"nope\" for \"agent-cli group\"\n" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestPresent(t *testing.T) {
	cat := text.MustNew("linux")
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantOut  string
	}{
		{"nil", nil, ExitOK, ""},
		{"aborted prompt", term.ErrAborted, ExitFailure, "Aborted.\n"},
		{"wrapped abort", fmt.Errorf("prompt: %w", term.ErrAborted), ExitFailure, "Aborted.\n"},
		{"failure keeps its code", &Failure{Code: ExitRateLimited, Message: "slow down"}, ExitRateLimited, "error: slow down\n"},
		{"wrapped failure", fmt.Errorf("x: %w", &Failure{Code: ExitChatUnreachable, Message: "gone"}), ExitChatUnreachable, "error: gone\n"},
		{"usage error keeps the first line", usageError("bad input\n\nDid you mean this?\n\tstatus"), ExitUsage, "error: bad input\n"},
		{"plain error is a failure", errors.New("disk full"), ExitFailure, "error: disk full\n"},
		{"escape sequences are stripped", &Failure{Code: ExitFailure, Message: "a\x1b[31mb\nc"}, ExitFailure, "error: ab c\n"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if code := Present(&buf, cat, c.err, false); code != c.wantCode {
			t.Errorf("%s: code = %d, want %d", c.name, code, c.wantCode)
		}
		if buf.String() != c.wantOut {
			t.Errorf("%s: output = %q, want %q", c.name, buf.String(), c.wantOut)
		}
	}
}

func TestExitCodesAreTheContract(t *testing.T) {
	got := []int{ExitOK, ExitFailure, ExitUsage, ExitNotConnected, ExitChatUnreachable, ExitInsufficientCredits, ExitRateLimited}
	for i, code := range got {
		if code != i {
			t.Errorf("exit code #%d = %d", i, code)
		}
	}
}

func TestShellAssignmentEscaping(t *testing.T) {
	value := "a'b\\c;$(echo unsafe)"
	for _, shell := range []string{"sh", "bash", "zsh", "fish", "pwsh", "powershell"} {
		got := shellAssignment(shell, value)
		if !strings.HasSuffix(got, "\n") || strings.Count(got, "\n") != 1 {
			t.Fatalf("%s: %q", shell, got)
		}
		if shell == "pwsh" || shell == "powershell" {
			if got != "$env:THE_AGENT_CLI_TOKEN = 'a''b\\c;$(echo unsafe)'\n" {
				t.Fatal(got)
			}
		} else if shell == "fish" {
			if got != "set -gx THE_AGENT_CLI_TOKEN 'a'\\''b\\\\c;$(echo unsafe)'\n" {
				t.Fatal(got)
			}
		} else if runtime.GOOS != "windows" {
			cmd := exec.CommandContext(t.Context(), "sh", "-c", `eval "$1"; test "$THE_AGENT_CLI_TOKEN" = "$2"`, "sh", got, value)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v %s", shell, err, out)
			}
		}
	}
}

func TestSkillGuideCoversCommandTreeAndExitCodes(t *testing.T) {
	d, _, _ := testDeps()
	root, state := newRoot(d)
	guide := d.Text.T("skill.guide", "name", d.Build.Name, "unlock", d.Text.T("skill.unlock", "name", d.Build.Name))
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Name() == "skill" {
			return
		}
		if cmd != root && cmd.Name() != "completion" && cmd.Name() != "help" && !strings.Contains(guide, cmd.CommandPath()) {
			t.Errorf("guide missing %s", cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	linux := state.skillStub()
	state.deps.Text = text.MustNew("windows")
	if !bytes.Equal(linux, state.skillStub()) {
		t.Fatal("stub differs by OS")
	}
	description := state.deps.Text.T("skill.stub_description", "name", d.Build.Name)
	if len([]rune(description)) > 300 || strings.ContainsAny(description, "<>") {
		t.Fatal(description)
	}
	windows := state.deps.Text.T("skill.guide", "name", d.Build.Name, "unlock", state.deps.Text.T("skill.unlock", "name", d.Build.Name))
	if strings.ReplaceAll(windows, state.deps.Text.T("skill.unlock", "name", d.Build.Name), d.Text.T("skill.unlock", "name", d.Build.Name)) != guide {
		t.Fatal("guide differs beyond shell instruction")
	}
}
