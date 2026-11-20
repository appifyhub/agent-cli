// Package credentials keeps the access token on this machine: in the operating system's credential
// store, in a passphrase-encrypted file, or in a plain file. It selects the store at connect time,
// finds the token again later and removes it.
package credentials

import (
	"path/filepath"

	"github.com/appifyhub/agent-cli/internal/term"
)

// Mode is where the token lives.
type Mode string

const (
	ModeKeychain   Mode = "keychain"
	ModePassphrase Mode = "passphrase"
	ModeUnsafe     Mode = "unsafe"
)

// Valid reports whether m is one of the three modes.
func (m Mode) Valid() bool {
	return m == ModeKeychain || m == ModePassphrase || m == ModeUnsafe
}

// Store holds one token. Load returns ErrNotFound when nothing is stored, and Delete succeeds
// when nothing is stored.
type Store interface {
	Save(token string) error
	Load() (string, error)
	Delete() error
}

// Prompts are the texts shown when a passphrase is asked for; they come from the catalog.
type Prompts struct {
	Choose     string
	Repeat     string
	Passphrase string
}

// Env is everything the package needs from the outside world.
type Env struct {
	Getenv   func(string) string
	Term     term.Terminal
	Keychain Keychain
	// Service is the keychain service: the command name of the build.
	Service string
	// Home is the absolute state directory.
	Home string
	// WorkFactor is the scrypt work factor of new passphrase files; zero means DefaultWorkFactor.
	WorkFactor int
	Prompts    Prompts
}

func (e Env) filePath() string {
	return filepath.Join(e.Home, FileName)
}

// store builds the store of a mode. The passphrase matters only for the passphrase mode, and the
// device id only for the keychain, where it is the account.
func (e Env) store(mode Mode, deviceID, passphrase string) Store {
	switch mode {
	case ModeKeychain:
		return keychainStore{keychain: e.Keychain, service: e.Service, account: deviceID}
	case ModePassphrase:
		return newPassphraseFile(e.filePath(), passphrase, e.WorkFactor)
	default:
		return plainFile{path: e.filePath()}
	}
}

// sameLocation reports whether two modes keep the token in the same place: both file modes use
// the one credentials file.
func sameLocation(a, b Mode) bool {
	return (a == ModeKeychain) == (b == ModeKeychain)
}
