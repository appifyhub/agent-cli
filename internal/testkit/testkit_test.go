package testkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentcli "github.com/appifyhub/agent-cli"
	"github.com/appifyhub/agent-cli/internal/term"
)

type recorder struct {
	testing.TB
	failures int
}

func (r *recorder) Helper()               {}
func (r *recorder) Errorf(string, ...any) { r.failures++ }
func (r *recorder) Fatalf(string, ...any) { r.failures++ }

func TestGoldenUpdateAndCompare(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	t.Setenv(UpdateGoldenEnv, "1")
	Golden(t, "sample", "hello\n")
	t.Setenv(UpdateGoldenEnv, "")

	written, err := os.ReadFile(filepath.Join(dir, "testdata", "sample.golden"))
	if err != nil || string(written) != "hello\n" {
		t.Fatalf("golden not written: %q, %v", written, err)
	}
	Golden(t, "sample", "hello\n")

	rec := &recorder{TB: t}
	Golden(rec, "sample", "different\n")
	if rec.failures == 0 {
		t.Fatal("a differing golden must fail")
	}
	rec = &recorder{TB: t}
	Golden(rec, "missing", "x")
	if rec.failures == 0 {
		t.Fatal("a missing golden must fail")
	}
}

func TestFakeTermScriptedPrompts(t *testing.T) {
	ft := &FakeTerm{TTYIn: true, Secrets: []string{"one", "two"}, Lines: []string{"yes"}}
	ctx := context.Background()

	if !ft.IsTTY(term.In) || ft.IsTTY(term.Out) || ft.IsTTY(term.Err) {
		t.Fatal("tty flags are scripted per stream")
	}
	for _, want := range []string{"one", "two"} {
		got, err := ft.ReadSecret(ctx, "Passphrase: ")
		if err != nil || got != want {
			t.Fatalf("ReadSecret = %q, %v; want %q", got, err, want)
		}
	}
	if _, err := ft.ReadSecret(ctx, "Passphrase: "); !errors.Is(err, term.ErrAborted) {
		t.Fatalf("exhausted secrets must abort, got %v", err)
	}
	if got, err := ft.ReadLine(ctx, "Confirm: "); err != nil || got != "yes" {
		t.Fatalf("ReadLine = %q, %v", got, err)
	}
	if len(ft.Prompts) != 4 || ft.Prompts[3] != "Confirm: " {
		t.Fatalf("prompts recorded wrong: %q", ft.Prompts)
	}
}

func TestAssertNoSecretFindsAPlantedSecret(t *testing.T) {
	clean := &recorder{TB: t}
	AssertNoSecret(clean, "hunter2", "all good", "nothing here")
	if clean.failures != 0 {
		t.Fatal("clean outputs must pass")
	}
	leaky := &recorder{TB: t}
	AssertNoSecret(leaky, "hunter2", "fine", "password is hunter2 oops")
	if leaky.failures != 1 {
		t.Fatalf("planted secret must be reported once, got %d", leaky.failures)
	}
}

func TestRunReturnsCodeAndStreams(t *testing.T) {
	res := Run(t, NewDeps(), "--version")
	if res.Code != 0 || res.Stdout != "agent-cli "+agentcli.Version()+" (linux/amd64, unknown)\n" || res.Stderr != "" {
		t.Fatalf("unexpected result %+v", res)
	}
	res = Run(t, NewDeps(), "nope")
	if res.Code != 2 || res.Stdout != "" || res.Stderr == "" {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestDepsOptions(t *testing.T) {
	clock := &FakeClock{Time: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	loc := time.FixedZone("X", 3600)
	deps := NewDeps(WithEnv(map[string]string{"A": "1"}), WithClock(clock, loc), WithPlatform("windows", "arm64"))
	if deps.Getenv("A") != "1" || deps.Getenv("B") != "" {
		t.Fatal("env map must be served")
	}
	if !deps.Now().Equal(clock.Time) || deps.Location != loc {
		t.Fatal("clock and location must be injected")
	}
	if deps.GOOS != "windows" || deps.GOARCH != "arm64" {
		t.Fatal("platform must be injected")
	}
}
