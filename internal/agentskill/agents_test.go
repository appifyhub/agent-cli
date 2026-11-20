package agentskill

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testStub(name string) []byte {
	return []byte(strings.ReplaceAll("---\nname: {name}\nmetadata:\n  agent-cli-stub: 1\n---\n\nRun {name} skill show.\n", "{name}", name))
}

func TestAgentDirectories(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo", "nested")
	if err := os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, a := range Supported() {
		userParent, projectParent := filepath.Join(root, ".agents", "skills"), filepath.Join(cwd, ".agents", "skills")
		switch a.Name {
		case "claude":
			userParent = filepath.Join(root, ".claude", "skills")
			projectParent = filepath.Join(cwd, ".claude", "skills")
		case "antigravity":
			userParent = filepath.Join(root, ".gemini", "config", "skills")
		}
		for _, c := range []struct {
			project    bool
			root, want string
		}{{false, root, filepath.Join(userParent, "my-cli")}, {true, cwd, filepath.Join(projectParent, "my-cli")}} {
			if got := Directory(a, c.root, "", "my-cli", c.project); got != c.want {
				t.Errorf("%s: %q want %q", a.Name, got, c.want)
			}
		}
	}
	claude, _ := Lookup("claude")
	override := filepath.Join(root, "custom")
	if got := Directory(claude, root, override, "my-cli", false); got != filepath.Join(override, "skills", "my-cli") {
		t.Fatal(got)
	}
	if got := Directory(claude, cwd, override, "my-cli", true); got != filepath.Join(cwd, ".claude", "skills", "my-cli") {
		t.Fatal(got)
	}
	if _, ok := Lookup("devin"); ok {
		t.Fatal("unsupported agent accepted")
	}
	if runtime.GOOS == "windows" {
		if got := Directory(claude, `C:\Users\Neo`, "", "my-cli", false); got != `C:\Users\Neo\.claude\skills\my-cli` {
			t.Fatal(got)
		}
	}
}

func TestStubLifecycle(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "my-cli")
	stub := testStub("my-cli")
	if state, err := Inspect(dir, stub); err != nil || state != Missing {
		t.Fatal(state, err)
	}
	if action, err := Install(dir, stub, false, true); err != nil || action != WouldInstall {
		t.Fatal(action, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry run created directory", err)
	}
	if action, err := Install(dir, stub, false, false); err != nil || action != Installed {
		t.Fatal(action, err)
	}
	path := filepath.Join(dir, "SKILL.md")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if state, err := Inspect(dir, stub); err != nil || state != Current {
		t.Fatal(state, err)
	}
	if action, err := Install(dir, stub, false, false); err != nil || action != Unchanged {
		t.Fatal(action, err)
	}
	after, err := os.Stat(path)
	if err != nil || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("idempotent install rewrote file", err)
	}
	if runtime.GOOS != "windows" {
		if after.Mode().Perm() != 0o644 {
			t.Fatal(after.Mode())
		}
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0o755 {
			t.Fatal(info, err)
		}
	}
	for _, raw := range [][]byte{append(bytes.Clone(stub), []byte("edited\n")...), bytes.Replace(stub, []byte("agent-cli-stub: 1"), []byte("agent-cli-stub: 42"), 1)} {
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if state, err := Inspect(dir, stub); err != nil || state != Outdated {
			t.Fatal(state, err)
		}
		if action, err := Install(dir, stub, false, true); err != nil || action != WouldUpdate {
			t.Fatal(action, err)
		}
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, raw) {
			t.Fatal("dry update changed file", err)
		}
		if action, err := Install(dir, stub, false, false); err != nil || action != Updated {
			t.Fatal(action, err)
		}
	}
	if action, err := Remove(dir, stub, true); err != nil || action != WouldRemove {
		t.Fatal(action, err)
	}
	if state, _ := Inspect(dir, stub); state != Current {
		t.Fatal("dry removal changed file", state)
	}
	if action, err := Remove(dir, stub, false); err != nil || action != Removed {
		t.Fatal(action, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("empty skill directory remains", err)
	}
	if _, err := os.Stat(parent); err != nil {
		t.Fatal("parent removed", err)
	}
	if action, err := Remove(dir, stub, false); err != nil || action != Absent {
		t.Fatal(action, err)
	}
}

func TestForeignAndUnreadableSkills(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	stub := testStub("my-cli")
	for _, raw := range []string{"someone else\nagent-cli-stub: 1\n", "---\nname: foreign\n---\nmetadata:\n  agent-cli-stub: 1\n", "---\nmetadata:\n  agent-cli-stub: 1\n"} {
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if state, err := Inspect(dir, stub); err != nil || state != Foreign {
			t.Fatal(state, err)
		}
		if action, err := Install(dir, stub, false, false); err != nil || action != Conflict {
			t.Fatal(action, err)
		}
		if action, err := Remove(dir, stub, false); err != nil || action != Unowned {
			t.Fatal(action, err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != raw {
			t.Fatal("foreign content changed", err)
		}
	}
	if action, err := Install(dir, stub, true, false); err != nil || action != Updated {
		t.Fatal(action, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if state, err := Inspect(dir, stub); state != Unreadable || err == nil {
		t.Fatal(state, err)
	}
}

func TestRenameFailurePreservesOldSkill(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	old := []byte("old skill\n")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("rename denied")
	if err := writeStub(dir, testStub("my-cli"), func(string, string) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, old) {
		t.Fatal("old file lost", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" {
		t.Fatal("temporary file remains", entries, err)
	}
}

func TestRemovePreservesOtherFilesAndBuilds(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "my-cli")
	other := filepath.Join(parent, "another-cli")
	for _, name := range []string{"my-cli", "another-cli"} {
		if _, err := Install(filepath.Join(parent, name), testStub(name), false, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(dir, testStub("my-cli"), false); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "notes.md")); err != nil || string(raw) != "keep\n" {
		t.Fatal(err)
	}
	if state, err := Inspect(other, testStub("another-cli")); err != nil || state != Current {
		t.Fatal(state, err)
	}
}

func TestSymlinkIsNotOwned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs extra privileges")
	}
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "SKILL.md")
	stub := testStub("my-cli")
	if err := os.WriteFile(target, stub, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if action, err := Remove(dir, stub, false); err != nil || action != Unowned {
		t.Fatal(action, err)
	}
	if _, err := Install(dir, stub, true, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, stub) {
		t.Fatal("symlink target changed", err)
	}
	if info, err := os.Lstat(filepath.Join(dir, "SKILL.md")); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("force did not replace link", err)
	}
}

func TestStubTextIsPortable(t *testing.T) {
	raw := testStub("my-cli")
	if bytes.Contains(raw, []byte("\r")) || !strings.Contains(string(raw), "name: my-cli\n") {
		t.Fatal(string(raw))
	}
}
