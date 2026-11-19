package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
)

func envOf(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func userHome(path string) func() (string, error) {
	return func() (string, error) { return path, nil }
}

func TestResolveHomeDefaultsToDotNameInUserHome(t *testing.T) {
	for _, name := range []string{"agent-cli", "abot", "abot-dev"} {
		got, err := config.ResolveHome(envOf(nil), userHome("/home/morpheus"), name)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join("/home/morpheus", "."+name); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestResolveHomeWithWindowsStyleUserHome(t *testing.T) {
	got, err := config.ResolveHome(envOf(nil), userHome(`C:\Users\morpheus`), "abot")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, `C:\Users\morpheus`) || !strings.HasSuffix(got, ".abot") {
		t.Errorf("got %q", got)
	}
}

func TestResolveHomeOverrideWins(t *testing.T) {
	dir := t.TempDir()
	failing := func() (string, error) {
		t.Error("the user home must not be asked when the override is set")
		return "", nil
	}
	got, err := config.ResolveHome(envOf(map[string]string{config.EnvHome: dir}), failing, "agent-cli")
	if err != nil || got != dir {
		t.Errorf("got %q, %v; want %q", got, err, dir)
	}
}

func TestResolveHomeRelativeOverrideIsMadeAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.ResolveHome(envOf(map[string]string{config.EnvHome: "state"}), userHome("/home/morpheus"), "agent-cli")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cwd, "state"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveHomeEmptyOverrideCountsAsUnset(t *testing.T) {
	got, err := config.ResolveHome(envOf(map[string]string{config.EnvHome: ""}), userHome("/home/morpheus"), "abot")
	if err != nil || got != filepath.Join("/home/morpheus", ".abot") {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestResolveHomeUnavailable(t *testing.T) {
	failing := func() (string, error) { return "", errors.New("$HOME is not defined") }
	empty := func() (string, error) { return "", nil }
	for name, fn := range map[string]func() (string, error){"error": failing, "empty": empty} {
		if _, err := config.ResolveHome(envOf(nil), fn, "agent-cli"); !errors.Is(err, config.ErrHomeUnavailable) {
			t.Errorf("%s: got %v, want ErrHomeUnavailable", name, err)
		}
	}
}

func TestResolveHomeNeverCreatesTheDirectory(t *testing.T) {
	root := t.TempDir()
	got, err := config.ResolveHome(envOf(nil), userHome(root), "agent-cli")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the directory %q exists after a read-only resolve (%v)", got, err)
	}
	if _, err := config.Load(config.Path(got)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the directory %q exists after loading a missing file (%v)", got, err)
	}
}

func TestEnsureHomeCreatesOwnerOnlyDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "home")
	if err := config.EnsureHome(dir); err != nil {
		t.Fatal(err)
	}
	if err := config.EnsureHome(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("stat: %v, %v", info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Errorf("mode %o, want 700", info.Mode().Perm())
	}
}

func TestCheckWritableLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	if err := config.CheckWritable(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("entries %v, err %v", entries, err)
	}
}

func TestCheckWritableFailsOnUnusableLocation(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := config.CheckWritable(notADir)
	var homeErr *config.HomeError
	if !errors.As(err, &homeErr) {
		t.Fatalf("got %v, want *HomeError", err)
	}
	if homeErr.Path != notADir || homeErr.Reason() == "" || strings.Contains(homeErr.Reason(), notADir) {
		t.Errorf("path %q, reason %q", homeErr.Path, homeErr.Reason())
	}
}

func TestEnsureHomeFailsOnUnusableLocation(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var homeErr *config.HomeError
	if err := config.EnsureHome(filepath.Join(notADir, "home")); !errors.As(err, &homeErr) {
		t.Errorf("got %v, want *HomeError", err)
	}
}
