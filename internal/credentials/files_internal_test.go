package credentials

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	testToken   = "header.payload.signature"
	testWorkLog = 8
)

func secretFile(t *testing.T, passphrase string) passphraseFile {
	t.Helper()
	return newPassphraseFile(filepath.Join(t.TempDir(), FileName), passphrase, testWorkLog)
}

func TestPlainFileRoundTrip(t *testing.T) {
	f := plainFile{path: filepath.Join(t.TempDir(), FileName)}
	if err := f.Save(testToken); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(f.path)
	if string(raw) != testToken+"\n" {
		t.Errorf("file content %q", raw)
	}
	if got, err := f.Load(); err != nil || got != testToken {
		t.Errorf("Load() = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(f.path)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestPassphraseFileRoundTripAndNoPlaintext(t *testing.T) {
	f := secretFile(t, "correct horse")
	if err := f.Save(testToken); err != nil {
		t.Fatal(err)
	}
	if got, err := f.Load(); err != nil || got != testToken {
		t.Fatalf("Load() = %q, %v", got, err)
	}
	raw, _ := os.ReadFile(f.path)
	for i := 0; i+8 <= len(testToken); i++ {
		if bytes.Contains(raw, []byte(testToken[i:i+8])) {
			t.Fatalf("encrypted file contains the token substring %q", testToken[i:i+8])
		}
	}
	if !strings.HasPrefix(string(raw), "-----BEGIN AGE ENCRYPTED FILE-----") {
		t.Errorf("file is not ASCII-armored: %.40q", raw)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(f.path)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestPassphraseFileWrongPassphrase(t *testing.T) {
	f := secretFile(t, "right")
	if err := f.Save(testToken); err != nil {
		t.Fatal(err)
	}
	f.passphrase = "wrong"
	if _, err := f.Load(); !errors.Is(err, ErrIncorrectPassphrase) {
		t.Errorf("got %v, want ErrIncorrectPassphrase", err)
	}
}

func TestPassphraseFileDamage(t *testing.T) {
	f := secretFile(t, "pw")
	if err := f.Save(testToken); err != nil {
		t.Fatal(err)
	}
	good, _ := os.ReadFile(f.path)
	lines := strings.Split(string(good), "\n")
	flipped := append([]string(nil), lines...)
	body := []byte(flipped[len(flipped)-4])
	body[3] ^= 0x01
	flipped[len(flipped)-4] = string(body)

	cases := map[string]string{
		"truncated":     string(good[:len(good)/2]),
		"bit flipped":   strings.Join(flipped, "\n"),
		"plain token":   testToken + "\n",
		"empty":         "",
		"garbage":       strings.Repeat("x", 200),
		"oversized":     strings.Repeat("a", maxFileSize+10),
		"armor no body": "-----BEGIN AGE ENCRYPTED FILE-----\n-----END AGE ENCRYPTED FILE-----\n",
	}
	for name, content := range cases {
		if err := os.WriteFile(f.path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Load(); !errors.Is(err, ErrDamaged) {
			t.Errorf("%s: got %v, want ErrDamaged", name, err)
		}
	}
}

func TestPassphraseFileRefusesExcessiveWorkFactor(t *testing.T) {
	f := secretFile(t, "pw")
	f.workFactor = 10
	if err := f.Save(testToken); err != nil {
		t.Fatal(err)
	}
	f.maxWork = 9
	if _, err := f.Load(); !errors.Is(err, ErrDamaged) {
		t.Errorf("got %v, want ErrDamaged", err)
	}
	f.maxWork = 10
	if got, err := f.Load(); err != nil || got != testToken {
		t.Errorf("at the cap: %q, %v", got, err)
	}
}

func TestProductionDecryptCapIsTwenty(t *testing.T) {
	if f := newPassphraseFile("x", "pw", 0); f.maxWork != 20 || f.workFactor != 18 {
		t.Errorf("work factors %d/%d, want 18 for writes and 20 for reads", f.workFactor, f.maxWork)
	}
}

func TestMissingAndDeletedFiles(t *testing.T) {
	for name, f := range map[string]Store{
		"plain":      plainFile{path: filepath.Join(t.TempDir(), FileName)},
		"passphrase": secretFile(t, "pw"),
	} {
		if _, err := f.Load(); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: missing file got %v", name, err)
		}
		if err := f.Delete(); err != nil {
			t.Errorf("%s: delete of a missing file: %v", name, err)
		}
		if err := f.Save(testToken); err != nil {
			t.Fatal(err)
		}
		if err := f.Delete(); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Load(); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: deleted file got %v", name, err)
		}
	}
}

func TestSaveReplacesAtomicallyAndLeavesNoTempFiles(t *testing.T) {
	f := plainFile{path: filepath.Join(t.TempDir(), FileName)}
	for _, token := range []string{"first.token.value", "second.token.value"} {
		if err := f.Save(token); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := f.Load(); got != "second.token.value" {
		t.Errorf("got %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(f.path))
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the credentials file", len(entries))
	}
}

func TestFailedWriteKeepsThePreviousFile(t *testing.T) {
	dir := t.TempDir()
	f := plainFile{path: filepath.Join(dir, FileName)}
	if err := f.Save("old.token.value"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := plainFile{path: filepath.Join(dir, "missing-dir", FileName)}
	if err := bad.Save("x"); err == nil {
		t.Fatal("writing into a missing directory must fail")
	}
	if got, _ := f.Load(); got != "old.token.value" {
		t.Errorf("got %q", got)
	}
}
