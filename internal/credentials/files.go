package credentials

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
)

const (
	// FileName is the credentials file inside the home directory.
	FileName = "credentials"

	// DefaultWorkFactor is the scrypt work factor of new passphrase files: about one second.
	DefaultWorkFactor = 18
	// maxWorkFactor is the highest scrypt work factor a file may ask for when it is decrypted, so a
	// hostile file cannot make the CLI allocate gigabytes.
	maxWorkFactor = 20
	// maxFileSize bounds what is read from a credentials file.
	maxFileSize = 64 << 10
)

type plainFile struct {
	path string
}

func (f plainFile) Save(token string) error {
	return writeAtomic(f.path, []byte(token+"\n"))
}

func (f plainFile) Load() (string, error) {
	raw, err := readLimited(f.path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", ErrDamaged
	}
	return token, nil
}

func (f plainFile) Delete() error {
	return removeFile(f.path)
}

type passphraseFile struct {
	path       string
	passphrase string
	// workFactor is used for writes; maxWork bounds reads.
	workFactor int
	maxWork    int
}

func newPassphraseFile(path, passphrase string, workFactor int) passphraseFile {
	if workFactor == 0 {
		workFactor = DefaultWorkFactor
	}
	return passphraseFile{path: path, passphrase: passphrase, workFactor: workFactor, maxWork: maxWorkFactor}
}

func (f passphraseFile) Save(token string) error {
	recipient, err := age.NewScryptRecipient(f.passphrase)
	if err != nil {
		return err
	}
	recipient.SetWorkFactor(f.workFactor)
	var out bytes.Buffer
	armored := armor.NewWriter(&out)
	encrypted, err := age.Encrypt(armored, recipient)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(encrypted, token); err != nil {
		return err
	}
	if err := encrypted.Close(); err != nil {
		return err
	}
	if err := armored.Close(); err != nil {
		return err
	}
	return writeAtomic(f.path, out.Bytes())
}

// Load decrypts the file. A wrong passphrase is ErrIncorrectPassphrase; anything else that does not
// decrypt to a token, including a file asking for more work than the cap, is ErrDamaged.
func (f passphraseFile) Load() (string, error) {
	raw, err := readLimited(f.path)
	if err != nil {
		return "", err
	}
	identity, err := age.NewScryptIdentity(f.passphrase)
	if err != nil {
		return "", err
	}
	identity.SetMaxWorkFactor(f.maxWork)
	reader, err := age.Decrypt(armor.NewReader(bytes.NewReader(raw)), identity)
	if err != nil {
		if errors.Is(err, age.ErrIncorrectIdentity) {
			return "", ErrIncorrectPassphrase
		}
		return "", ErrDamaged
	}
	token, err := io.ReadAll(io.LimitReader(reader, maxFileSize))
	if err != nil || len(token) == 0 {
		return "", ErrDamaged
	}
	return string(token), nil
}

func (f passphraseFile) Delete() error {
	return removeFile(f.path)
}

// readLimited reads a credentials file; a missing file is ErrNotFound and an oversized one is damaged.
func readLimited(path string) ([]byte, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxFileSize {
		return nil, ErrDamaged
	}
	return raw, nil
}

func removeFile(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// writeAtomic replaces path with data through a temporary file in the same directory (mode 0600),
// so readers never see a partial file.
func writeAtomic(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
