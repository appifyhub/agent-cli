package credentials_test

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/appifyhub/agent-cli/internal/credentials"
)

func TestSystemKeychainRoundTripAndErrorMapping(t *testing.T) {
	keyring.MockInit()
	k := credentials.SystemKeychain{}
	if _, err := k.Get(service, deviceID); !errors.Is(err, credentials.ErrNotFound) {
		t.Errorf("missing entry: got %v, want ErrNotFound", err)
	}
	if err := k.Set(service, deviceID, token); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Get(service, deviceID); err != nil || got != token {
		t.Errorf("Get() = %q, %v", got, err)
	}
	if err := k.Delete(service, deviceID); err != nil {
		t.Fatal(err)
	}
	if err := k.Delete(service, deviceID); !errors.Is(err, credentials.ErrNotFound) {
		t.Errorf("second delete: got %v, want ErrNotFound", err)
	}

	keyring.MockInitWithError(keyring.ErrSetDataTooBig)
	if err := k.Set(service, deviceID, token); !errors.Is(err, credentials.ErrTooBig) {
		t.Errorf("oversized: got %v, want ErrTooBig", err)
	}
	locked := errors.New("keychain is locked")
	keyring.MockInitWithError(locked)
	if _, err := k.Get(service, deviceID); !errors.Is(err, locked) {
		t.Errorf("other failures keep their reason: got %v", err)
	}
}

func TestProbe(t *testing.T) {
	keyring.MockInit()
	if err := credentials.Probe(credentials.SystemKeychain{}, service); err != nil {
		t.Fatal(err)
	}
	if _, err := (credentials.SystemKeychain{}).Get(service, ".probe"); !errors.Is(err, credentials.ErrNotFound) {
		t.Errorf("probe left an entry behind: %v", err)
	}

	keyring.MockInitWithError(errors.New("no secret service"))
	if err := credentials.Probe(credentials.SystemKeychain{}, service); err == nil {
		t.Error("a failing keychain must not probe as usable")
	}
}

func TestProbeFailsWhenTheThrowawayEntryCannotBeRemoved(t *testing.T) {
	r := newRig(t)
	r.kc.DelErr = errors.New("access denied")
	if err := credentials.Probe(r.kc, service); err == nil {
		t.Error("probe must fail when the entry cannot be removed")
	}
}
