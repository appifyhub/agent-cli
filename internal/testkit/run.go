package testkit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/cli"
)

// UpdateGoldenEnv names the environment variable that makes Golden rewrite files instead of comparing:
// UPDATE_GOLDEN=1 go test ./...
const UpdateGoldenEnv = "UPDATE_GOLDEN"

// Result is what one invocation produced.
type Result struct {
	Code   int
	Stdout string
	Stderr string
}

// Run executes the CLI with fresh output buffers and returns the exit code and both streams.
func Run(t testing.TB, deps cli.Deps, args ...string) Result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	deps.Stdout, deps.Stderr = &stdout, &stderr
	code := cli.Run(t.Context(), deps, args)
	return Result{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

// Golden compares got with testdata/<name>.golden of the calling package, or rewrites it when
// UPDATE_GOLDEN=1 is set.
func Golden(t testing.TB, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if os.Getenv(UpdateGoldenEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file (run with %s=1 to create it): %v", UpdateGoldenEnv, err)
	}
	if string(want) != got {
		t.Errorf("output differs from %s (run with %s=1 to accept)\n--- want ---\n%s\n--- got ---\n%s", path, UpdateGoldenEnv, want, got)
	}
}

// AssertNoSecret fails when any output contains the secret.
func AssertNoSecret(t testing.TB, secret string, outputs ...string) {
	t.Helper()
	if secret == "" {
		t.Errorf("AssertNoSecret needs a non-empty secret")
		return
	}
	for i, output := range outputs {
		if strings.Contains(output, secret) {
			t.Errorf("secret found in output #%d", i+1)
		}
	}
}
