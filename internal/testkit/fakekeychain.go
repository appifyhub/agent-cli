package testkit

import (
	"github.com/appifyhub/agent-cli/internal/credentials"
)

// FakeKeychain is an in-memory credential store with injectable failures.
type FakeKeychain struct {
	Entries                map[string]string
	SetErr, GetErr, DelErr error
	Calls                  []string
}

func keychainKey(service, account string) string { return service + "\x00" + account }

func (k *FakeKeychain) Set(service, account, secret string) error {
	k.Calls = append(k.Calls, "set")
	if k.SetErr != nil {
		return k.SetErr
	}
	if k.Entries == nil {
		k.Entries = map[string]string{}
	}
	k.Entries[keychainKey(service, account)] = secret
	return nil
}

func (k *FakeKeychain) Get(service, account string) (string, error) {
	k.Calls = append(k.Calls, "get")
	if k.GetErr != nil {
		return "", k.GetErr
	}
	secret, ok := k.Entries[keychainKey(service, account)]
	if !ok {
		return "", credentials.ErrNotFound
	}
	return secret, nil
}

func (k *FakeKeychain) Delete(service, account string) error {
	k.Calls = append(k.Calls, "delete")
	if k.DelErr != nil {
		return k.DelErr
	}
	key := keychainKey(service, account)
	if _, ok := k.Entries[key]; !ok {
		return credentials.ErrNotFound
	}
	delete(k.Entries, key)
	return nil
}

// Secret returns the stored secret of an entry, and whether it exists.
func (k *FakeKeychain) Secret(service, account string) (string, bool) {
	secret, ok := k.Entries[keychainKey(service, account)]
	return secret, ok
}

// Len is the number of stored entries.
func (k *FakeKeychain) Len() int { return len(k.Entries) }
