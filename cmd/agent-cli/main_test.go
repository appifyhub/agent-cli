package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// the process-level tests re-execute this test binary as the program itself
const helperEnv = "THE_AGENT_CLI_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		os.Args = []string{"agent-cli"}
		if args := os.Getenv(helperEnv + "_ARGS"); args != "" {
			os.Args = append(os.Args, strings.Split(args, "\x1f")...)
		}
		main()
		return
	}
	os.Exit(m.Run())
}

func runProcess(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0])
	cmd.Env = append(os.Environ(),
		helperEnv+"=1",
		helperEnv+"_ARGS="+strings.Join(args, "\x1f"),
		"THE_AGENT_CLI_HOME="+filepath.Join(t.TempDir(), "missing"),
	)
	var out, errBuf strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		t.Fatal(err)
	}
	return code, out.String(), errBuf.String()
}

func TestExitCodesReachTheOS(t *testing.T) {
	cases := []struct {
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{[]string{"--version"}, 0, "agent-cli ", ""},
		{nil, 0, "agent-cli – command-line access to your agent", ""},
		{[]string{"frobnicate"}, 2, "", "error: unknown command \"frobnicate\" for \"agent-cli\"\n"},
		{[]string{"completion", "tcsh"}, 2, "", "error: unsupported shell \"tcsh\"; use bash, zsh, fish or powershell\n"},
	}
	for _, c := range cases {
		code, stdout, stderr := runProcess(t, c.args...)
		if code != c.wantCode || !strings.HasPrefix(stdout, c.wantStdout) || (c.wantStdout == "" && stdout != "") || stderr != c.wantStderr {
			t.Errorf("%v: code %d, stdout %q, stderr %q", c.args, code, stdout, stderr)
		}
	}
}
