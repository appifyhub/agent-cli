package credentials

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const probeAccount = ".probe"

// Keychain is the operating system's credential store.
type Keychain interface {
	Set(service, account, secret string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

// SystemKeychain is the real credential store: the macOS keychain, the Windows Credential Manager
// or a Secret Service on Linux. On macOS the secret is handed to /usr/bin/security through its
// standard input, never through its arguments.
type SystemKeychain struct{}

func (SystemKeychain) Set(service, account, secret string) error {
	return mapKeyringError(keyring.Set(service, account, secret))
}

func (SystemKeychain) Get(service, account string) (string, error) {
	secret, err := keyring.Get(service, account)
	return secret, mapKeyringError(err)
}

func (SystemKeychain) Delete(service, account string) error {
	return mapKeyringError(keyring.Delete(service, account))
}

func mapKeyringError(err error) error {
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, keyring.ErrSetDataTooBig):
		return ErrTooBig
	}
	return err
}

// Probe proves the keychain is usable by writing and removing a throwaway entry. Any error means
// it is not usable. The entry has a fixed account, so one left by an interrupted probe is overwritten
// by the next one.
func Probe(k Keychain, service string) error {
	if err := k.Set(service, probeAccount, "probe"); err != nil {
		return err
	}
	return k.Delete(service, probeAccount)
}

type keychainStore struct {
	keychain Keychain
	service  string
	account  string
}

func (s keychainStore) Save(token string) error {
	return s.keychain.Set(s.service, s.account, token)
}

func (s keychainStore) Load() (string, error) {
	return s.keychain.Get(s.service, s.account)
}

func (s keychainStore) Delete() error {
	err := s.keychain.Delete(s.service, s.account)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
