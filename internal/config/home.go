package config

import (
	"os"
	"path/filepath"
)

// ResolveHome returns the absolute state directory: THE_AGENT_CLI_HOME when set, otherwise
// ".<name>" inside the user home directory. It never touches the disk.
func ResolveHome(getenv func(string) string, userHome func() (string, error), name string) (string, error) {
	if override := getenv(EnvHome); override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", &HomeError{Path: override, Err: err}
		}
		return abs, nil
	}
	home, err := userHome()
	if err != nil || home == "" {
		return "", ErrHomeUnavailable
	}
	return filepath.Join(home, "."+name), nil
}

// EnsureHome creates the state directory with owner-only permissions; only commands that write state call it.
func EnsureHome(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return &HomeError{Path: dir, Err: err}
	}
	return nil
}

// CheckWritable proves that files can be created in dir by creating and removing a temporary one.
func CheckWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return &HomeError{Path: dir, Err: err}
	}
	name := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return &HomeError{Path: dir, Err: closeErr}
	}
	if removeErr != nil {
		return &HomeError{Path: dir, Err: removeErr}
	}
	return nil
}
