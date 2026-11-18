package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fullFile() File {
	return File{
		DeviceID: "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b",
		APIURL:   "https://agent.cloud.appifyhub.com",
		Token: Token{
			Store:     "keychain",
			UserID:    "9f1c2d3e4a5b6c7d8e9f0a1b2c3d4e5f",
			UserName:  "Milos",
			Hostname:  "milos-mbp",
			ExpiresAt: time.Date(2027, 2, 14, 9, 21, 0, 0, time.UTC),
		},
		Default: Default{
			ChatID:       "3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7",
			ChatPlatform: "telegram",
			ChatTitle:    "Milos",
			ChatPrivate:  true,
		},
	}
}

func configIn(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), FileName)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestRoundTripKeepsEveryField(t *testing.T) {
	path := configIn(t)
	want := fullFile()
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Token.ExpiresAt.Equal(want.Token.ExpiresAt) {
		t.Errorf("expires_at %v, want %v", got.Token.ExpiresAt, want.Token.ExpiresAt)
	}
	got.Token.ExpiresAt = want.Token.ExpiresAt
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestRoundTripOfAPlatformDefault(t *testing.T) {
	path := configIn(t)
	want := File{Default: Default{Platform: "whatsapp"}}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got != want {
		t.Errorf("got %+v, %v; want %+v", got, err, want)
	}
}

func TestSavedFileStartsWithTheHeaderAndHoldsOnlyTheModel(t *testing.T) {
	path := configIn(t)
	if err := Save(path, fullFile()); err != nil {
		t.Fatal(err)
	}
	content := read(t, path)
	if !strings.HasPrefix(content, header) {
		t.Errorf("no header in %q", content)
	}
	for _, want := range []string{"device_id", "api_url", "[token]", "store", "user_id", "user_name", "hostname", "expires_at", "[default]", "chat_id", "chat_platform", "chat_title", "chat_private"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in\n%s", want, content)
		}
	}
}

func TestEmptyTablesAreOmitted(t *testing.T) {
	path := configIn(t)
	if err := Save(path, File{DeviceID: "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b"}); err != nil {
		t.Fatal(err)
	}
	if content := read(t, path); strings.Contains(content, "[token]") || strings.Contains(content, "[default]") {
		t.Errorf("empty tables written:\n%s", content)
	}
}

func TestLoadMissingFileIsEmptyAndCreatesNothing(t *testing.T) {
	path := configIn(t)
	got, err := Load(path)
	if err != nil || got != (File{}) {
		t.Errorf("got %+v, %v", got, err)
	}
	if names := entries(t, filepath.Dir(path)); len(names) != 0 {
		t.Errorf("files appeared: %v", names)
	}
}

func TestUnknownKeysAreIgnoredOnReadAndDroppedOnWrite(t *testing.T) {
	path := configIn(t)
	write(t, path, "device_id = \"6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b\"\nfuture = 1\n[token]\nfuture_key = \"x\"\n[future_table]\na = 1\n")
	f, err := Load(path)
	if err != nil || f.DeviceID != "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b" {
		t.Fatalf("got %+v, %v", f, err)
	}
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	if content := read(t, path); strings.Contains(content, "future") {
		t.Errorf("unknown keys survived:\n%s", content)
	}
}

func TestMalformedTOMLIsAReadErrorAndTheFileIsUntouched(t *testing.T) {
	path := configIn(t)
	write(t, path, "device_id = = broken")
	_, err := Load(path)
	var readErr *ReadError
	if !errors.As(err, &readErr) {
		t.Fatalf("got %v, want *ReadError", err)
	}
	if readErr.Path != path || readErr.Reason() == "" || strings.HasPrefix(readErr.Reason(), "toml:") {
		t.Errorf("path %q, reason %q", readErr.Path, readErr.Reason())
	}
	if got := read(t, path); got != "device_id = = broken" {
		t.Errorf("the file changed: %q", got)
	}
}

func TestWrongTypeIsAReadError(t *testing.T) {
	path := configIn(t)
	write(t, path, "device_id = 42\n")
	var readErr *ReadError
	if _, err := Load(path); !errors.As(err, &readErr) {
		t.Errorf("got %v, want *ReadError", err)
	}
}

func TestUnreadablePathIsAReadError(t *testing.T) {
	dir := t.TempDir()
	var readErr *ReadError
	if _, err := Load(dir); !errors.As(err, &readErr) {
		t.Errorf("got %v, want *ReadError", err)
	}
}

func TestInvalidDeviceIDIsAReadErrorNamingTheFile(t *testing.T) {
	for _, id := range []string{"not-a-uuid", "6b8f0d3e2a1c4e559a370c1d2e3f4a5b", "{6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b}", "urn:uuid:6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b"} {
		path := configIn(t)
		write(t, path, "device_id = \""+id+"\"\n")
		_, err := Load(path)
		var readErr *ReadError
		if !errors.As(err, &readErr) || readErr.Path != path || !errors.Is(err, ErrInvalidDeviceID) {
			t.Errorf("%q: got %v", id, err)
		}
	}
}

func TestSavedFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	path := configIn(t)
	write(t, path, "device_id = \"6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b\"\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, fullFile()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, err %v; want 600", info.Mode().Perm(), err)
	}
}

func TestFailedRenameKeepsThePreviousFileAndRemovesTheTemporaryOne(t *testing.T) {
	path := configIn(t)
	if err := Save(path, File{APIURL: "https://a.test"}); err != nil {
		t.Fatal(err)
	}
	before := read(t, path)
	failure := errors.New("disk exploded")
	err := save(path, File{APIURL: "https://b.test"}, func(string, string) error { return failure })
	var writeErr *WriteError
	if !errors.As(err, &writeErr) || !errors.Is(err, failure) || writeErr.Path != path {
		t.Fatalf("got %v, want *WriteError wrapping the failure", err)
	}
	if after := read(t, path); after != before {
		t.Errorf("the file changed:\n%s", after)
	}
	if names := entries(t, filepath.Dir(path)); len(names) != 1 || names[0] != FileName {
		t.Errorf("leftovers: %v", names)
	}
	if f, err := Load(path); err != nil || f.APIURL != "https://a.test" {
		t.Errorf("previous content unreadable: %+v, %v", f, err)
	}
}

func TestSaveIntoAMissingDirectoryFails(t *testing.T) {
	var writeErr *WriteError
	if err := Save(filepath.Join(t.TempDir(), "absent", FileName), File{}); !errors.As(err, &writeErr) {
		t.Errorf("got %v, want *WriteError", err)
	}
}
