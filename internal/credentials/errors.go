package credentials

import (
	"errors"
	"io/fs"
	"strings"
)

var (
	// ErrNotFound means a store holds no secret for this machine.
	ErrNotFound = errors.New("no secret is stored")
	// ErrTooBig means the credential store refused a secret for its size.
	ErrTooBig = errors.New("the secret is too big for the credential store")
	// ErrIncorrectPassphrase means the passphrase does not open the credentials file.
	ErrIncorrectPassphrase = errors.New("incorrect passphrase")
	// ErrDamaged means the credentials file cannot be read as what its mode says it is.
	ErrDamaged = errors.New("the credentials file is damaged")
	// ErrLocked means the token is passphrase-protected and no passphrase can be asked for.
	ErrLocked = errors.New("the token is locked")
	// ErrNotConnected means no token is stored, or the stored one is gone.
	ErrNotConnected = errors.New("not connected")
	// ErrInvalidEnvToken means THE_AGENT_CLI_TOKEN is not shaped like a CLI token.
	ErrInvalidEnvToken = errors.New("THE_AGENT_CLI_TOKEN is not a valid CLI token")

	// ErrNoKeychain means the keychain was requested explicitly and cannot be used.
	ErrNoKeychain = errors.New("no keychain is available")
	// ErrNoPassphraseSource means the passphrase mode was requested explicitly and there is no way to ask.
	ErrNoPassphraseSource = errors.New("a passphrase is required")
	// ErrNoStore means no mode can be used without --store unsafe.
	ErrNoStore = errors.New("no store is available")
	// ErrPassphraseMismatch means the two typed passphrases differ.
	ErrPassphraseMismatch = errors.New("passphrases do not match")
	// ErrPassphraseEmpty means an empty passphrase was submitted.
	ErrPassphraseEmpty = errors.New("passphrase cannot be empty")
)

// StoreFlagError is a --store value that names no mode.
type StoreFlagError struct {
	Value string
}

func (e *StoreFlagError) Error() string { return "invalid --store " + e.Value }

// StoreOp is what was being done to a store when it failed.
type StoreOp int

const (
	OpRead StoreOp = iota
	OpWrite
	OpRemove
)

// StoreError is a failure of a credential store other than a missing secret. The token is kept.
type StoreError struct {
	Op   StoreOp
	Mode Mode
	Err  error
}

func (e *StoreError) Error() string { return string(e.Mode) + ": " + e.Err.Error() }
func (e *StoreError) Unwrap() error { return e.Err }

// Reason is the first line of the cause, without the path the operating system repeats in its messages.
func (e *StoreError) Reason() string {
	err := e.Err
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	line, _, _ := strings.Cut(err.Error(), "\n")
	return strings.TrimSpace(line)
}
