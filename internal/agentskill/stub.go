package agentskill

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type State string

const (
	Current    State = "current"
	Outdated   State = "outdated"
	Missing    State = "missing"
	Foreign    State = "foreign"
	Unreadable State = "unreadable"
)

type Action string

const (
	Installed    Action = "installed"
	Updated      Action = "updated"
	Unchanged    Action = "unchanged"
	Conflict     Action = "conflict"
	Removed      Action = "removed"
	Absent       Action = "missing"
	Unowned      Action = "foreign"
	WouldInstall Action = "would_install"
	WouldUpdate  Action = "would_update"
	WouldRemove  Action = "would_remove"
)

func owned(raw []byte) bool {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return false
	}
	metadata, marker := false, false
	for _, line := range lines[1:] {
		if line == "---" {
			return marker
		}
		value := strings.TrimSpace(line)
		if value == "" || strings.HasPrefix(value, "#") {
			continue
		}
		if line == value {
			metadata = value == "metadata:"
			continue
		}
		if metadata {
			if version, ok := strings.CutPrefix(value, "agent-cli-stub:"); ok {
				version, _, _ = strings.Cut(strings.TrimSpace(version), " #")
				number, err := strconv.Atoi(version)
				marker = err == nil && number > 0
			}
		}
	}
	return false
}

func Inspect(dir string, stub []byte) (State, error) {
	path := filepath.Join(dir, "SKILL.md")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Missing, nil
	}
	if err != nil {
		return Unreadable, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Foreign, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Unreadable, err
	}
	if bytes.Equal(raw, stub) {
		return Current, nil
	}
	if owned(raw) {
		return Outdated, nil
	}
	return Foreign, nil
}

func Install(dir string, stub []byte, force, dryRun bool) (Action, error) {
	state, err := Inspect(dir, stub)
	if err != nil {
		return "", err
	}
	if state == Current {
		return Unchanged, nil
	}
	if state == Foreign && !force {
		return Conflict, nil
	}
	action := Updated
	if state == Missing {
		action = Installed
	}
	if dryRun {
		if action == Installed {
			return WouldInstall, nil
		}
		return WouldUpdate, nil
	}
	if err := writeStub(dir, stub, os.Rename); err != nil {
		return "", err
	}
	return action, nil
}

func writeStub(dir string, stub []byte, rename func(string, string) error) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- public, secret-free skills require mode 0755
		return err
	}
	tmp, err := os.CreateTemp(dir, ".skill-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(stub); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return rename(tmp.Name(), filepath.Join(dir, "SKILL.md"))
}

func Remove(dir string, stub []byte, dryRun bool) (Action, error) {
	state, err := Inspect(dir, stub)
	if err != nil {
		return "", err
	}
	if state == Missing {
		return Absent, nil
	}
	if state == Foreign {
		return Unowned, nil
	}
	if dryRun {
		return WouldRemove, nil
	}
	if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		if err := os.Remove(dir); err != nil {
			return "", err
		}
	}
	return Removed, nil
}
