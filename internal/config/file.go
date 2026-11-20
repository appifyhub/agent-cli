package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// FileName is the config file inside the home directory.
const FileName = "config.toml"

const header = "# managed by the command-line client; contains no secrets\n"

// File is the content of config.toml: non-secret local state only.
type File struct {
	DeviceID string  `toml:"device_id,omitempty"`
	APIURL   string  `toml:"api_url,omitempty"`
	Token    Token   `toml:"token,omitempty"`
	Default  Default `toml:"default,omitempty"`
}

// Token is metadata about the stored token; the token itself lives in the chosen credential store.
type Token struct {
	Store     string    `toml:"store,omitempty"`
	UserID    string    `toml:"user_id,omitempty"`
	UserName  string    `toml:"user_name,omitempty"`
	Hostname  string    `toml:"hostname,omitempty"`
	ExpiresAt time.Time `toml:"expires_at,omitempty"`
}

// Default is the stored default target: either the chat_* keys or a platform, never both.
type Default struct {
	ChatID       string `toml:"chat_id,omitempty"`
	ChatPlatform string `toml:"chat_platform,omitempty"`
	ChatTitle    string `toml:"chat_title,omitempty"`
	ChatPrivate  bool   `toml:"chat_private,omitempty"`
	Platform     string `toml:"platform,omitempty"`
}

// Path is the config file of a home directory.
func Path(home string) string {
	return filepath.Join(home, FileName)
}

// Load reads the config file. A missing file is an empty File; a file that cannot be read, is not valid
// TOML or holds an invalid device id is a *ReadError. Unknown keys are ignored.
func Load(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, &ReadError{Path: path, Err: err}
	}
	var f File
	if err := toml.Unmarshal(raw, &f); err != nil {
		return File{}, &ReadError{Path: path, Err: err}
	}
	if f.DeviceID != "" && !ValidDeviceID(f.DeviceID) {
		return File{}, &ReadError{Path: path, Err: ErrInvalidDeviceID}
	}
	return f, nil
}

// Save replaces the config file atomically: a temporary file in the same directory (mode 0600) is
// written, synced and renamed over the target, so readers never see a partial file.
func Save(path string, f File) error {
	return save(path, f, os.Rename)
}

func save(path string, f File, rename func(oldPath, newPath string) error) (err error) {
	body, err := toml.Marshal(f)
	if err != nil {
		return &WriteError{Path: path, Err: err}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return &WriteError{Path: path, Err: err}
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			err = &WriteError{Path: path, Err: err}
		}
	}()
	if _, err = tmp.WriteString(header + string(body)); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return rename(tmp.Name(), path)
}
