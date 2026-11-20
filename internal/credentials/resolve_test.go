package credentials_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
)

func resolve(r *rig) (credentials.Resolved, error) {
	return credentials.Resolve(r.t.Context(), r.env)
}

func TestEnvironmentTokenWinsAndTouchesNoStore(t *testing.T) {
	r := newRig(t)
	r.vars[config.EnvPassphrase] = "pw"
	r.connect("passphrase")
	r.vars[config.EnvPassphrase] = ""
	r.kc.Calls = nil
	r.vars[config.EnvToken] = token

	got, err := resolve(r)
	if err != nil || got.Token != token || !got.FromEnv || got.UserID != "9f1c2d3e4a5b6c7d8e9f0a1b2c3d4e5f" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(r.term.Prompts) != 0 || len(r.kc.Calls) != 0 {
		t.Errorf("prompts=%q keychain calls=%v", r.term.Prompts, r.kc.Calls)
	}
}

func TestEnvironmentTokenNeedsNoMachineState(t *testing.T) {
	r := newRig(t)
	r.vars[config.EnvToken] = token
	if got, err := resolve(r); err != nil || got.Token != token {
		t.Fatalf("got %+v, %v", got, err)
	}
	if r.fileExists(r.configPath()) {
		t.Error("nothing may be written")
	}
}

func TestEmptyEnvironmentTokenFallsBackToTheStoredOne(t *testing.T) {
	r := newRig(t)
	r.connect("unsafe")
	r.vars[config.EnvToken] = ""
	if got, err := resolve(r); err != nil || got.Token != token || got.FromEnv || got.UserID != "9f1c" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestMalformedEnvironmentToken(t *testing.T) {
	r := newRig(t)
	r.connect("unsafe")
	r.vars[config.EnvToken] = "not-a-token"
	_, err := resolve(r)
	if !errors.Is(err, credentials.ErrInvalidEnvToken) {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "not-a-token") {
		t.Error("the error must not echo the value")
	}
}

func TestNotConnectedWhenNothingIsRecorded(t *testing.T) {
	r := newRig(t)
	if _, err := resolve(r); !errors.Is(err, credentials.ErrNotConnected) {
		t.Fatalf("got %v", err)
	}
	if r.fileExists(r.configPath()) {
		t.Error("resolving must not create state")
	}
}

func TestMissingSecretClearsMetadataAndReportsNotConnected(t *testing.T) {
	cases := map[string]func(*rig){
		"keychain entry deleted":   func(r *rig) { r.kc.Entries = nil },
		"credentials file deleted": func(r *rig) { _ = os.Remove(r.credentialsPath()) },
	}
	for name, remove := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			mode := "keychain"
			if strings.Contains(name, "file") {
				mode = "unsafe"
			}
			r.connect(mode)
			remove(r)
			if _, err := resolve(r); !errors.Is(err, credentials.ErrNotConnected) {
				t.Fatalf("got %v", err)
			}
			f := r.readConfig()
			if f.Token != (config.Token{}) || f.DeviceID != deviceID || f.Default.ChatID == "" {
				t.Errorf("config after clearing: %+v", f)
			}
		})
	}
}

func TestMissingPassphraseFileIsNotConnectedEvenWithoutATerminal(t *testing.T) {
	r := newRig(t)
	r.vars[config.EnvPassphrase] = "pw"
	r.connect("passphrase")
	r.vars[config.EnvPassphrase] = ""
	_ = os.Remove(r.credentialsPath())
	if _, err := resolve(r); !errors.Is(err, credentials.ErrNotConnected) {
		t.Fatalf("got %v, want not connected rather than locked", err)
	}
}

func TestKeychainReadFailureKeepsTheToken(t *testing.T) {
	r := newRig(t)
	r.connect("keychain")
	r.kc.GetErr = errors.New("access denied by the user")
	before := r.readConfig()
	_, err := resolve(r)
	var storeErr *credentials.StoreError
	if !errors.As(err, &storeErr) || storeErr.Mode != credentials.ModeKeychain || storeErr.Reason() != "access denied by the user" {
		t.Fatalf("got %v", err)
	}
	if r.readConfig() != before {
		t.Error("metadata must be unchanged")
	}
}

func passphraseMachine(t *testing.T) *rig {
	r := newRig(t)
	r.vars[config.EnvPassphrase] = "correct horse"
	r.connect("passphrase")
	r.vars[config.EnvPassphrase] = ""
	return r
}

func TestUnlockWithThePassphraseFromTheEnvironment(t *testing.T) {
	r := passphraseMachine(t)
	r.vars[config.EnvPassphrase] = "correct horse"
	if got, err := resolve(r); err != nil || got.Token != token {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(r.term.Prompts) != 0 {
		t.Errorf("prompts %q, want none", r.term.Prompts)
	}
}

func TestUnlockWithAPromptOnATerminal(t *testing.T) {
	r := passphraseMachine(t)
	r.term.TTYIn = true
	r.term.Secrets = []string{"correct horse"}
	if got, err := resolve(r); err != nil || got.Token != token {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(r.term.Prompts) != 1 || r.term.Prompts[0] != "Passphrase: " {
		t.Errorf("prompts %q", r.term.Prompts)
	}
}

func TestLockedWithoutATerminalNeverAsks(t *testing.T) {
	r := passphraseMachine(t)
	if _, err := resolve(r); !errors.Is(err, credentials.ErrLocked) {
		t.Fatalf("got %v", err)
	}
	if len(r.term.Prompts) != 0 {
		t.Errorf("a non-terminal run must not prompt: %q", r.term.Prompts)
	}
}

func TestWrongPassphrase(t *testing.T) {
	r := passphraseMachine(t)
	r.vars[config.EnvPassphrase] = "battery staple"
	if _, err := resolve(r); !errors.Is(err, credentials.ErrIncorrectPassphrase) {
		t.Fatalf("got %v", err)
	}
	if r.readConfig().Token.Store != "passphrase" || !r.fileExists(r.credentialsPath()) {
		t.Error("a wrong passphrase must keep the file and the metadata")
	}
}

func TestDamagedCredentialsFileIsKept(t *testing.T) {
	r := passphraseMachine(t)
	raw, _ := os.ReadFile(r.credentialsPath())
	if err := os.WriteFile(r.credentialsPath(), raw[:len(raw)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	r.vars[config.EnvPassphrase] = "correct horse"
	if _, err := resolve(r); !errors.Is(err, credentials.ErrDamaged) {
		t.Fatalf("got %v", err)
	}
	if !r.fileExists(r.credentialsPath()) || r.readConfig().Token.Store != "passphrase" {
		t.Error("a damaged file must be kept")
	}
}

func TestPlainTokenFileReadInPassphraseModeIsDamaged(t *testing.T) {
	r := passphraseMachine(t)
	if err := os.WriteFile(r.credentialsPath(), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.vars[config.EnvPassphrase] = "correct horse"
	if _, err := resolve(r); !errors.Is(err, credentials.ErrDamaged) {
		t.Fatalf("got %v", err)
	}
}

func TestKeychainAndUnsafeNeverPromptNorUsePassphrases(t *testing.T) {
	for _, mode := range []string{"keychain", "unsafe"} {
		r := newRig(t)
		r.connect(mode)
		r.vars[config.EnvPassphrase] = "ignored"
		if got, err := resolve(r); err != nil || got.Token != token {
			t.Fatalf("%s: got %+v, %v", mode, got, err)
		}
		if len(r.term.Prompts) != 0 {
			t.Errorf("%s: prompts %q", mode, r.term.Prompts)
		}
	}
}

func TestMalformedConfigIsReported(t *testing.T) {
	r := newRig(t)
	if err := os.WriteFile(r.configPath(), []byte("x = = y"), 0o600); err != nil {
		t.Fatal(err)
	}
	var readErr *config.ReadError
	if _, err := resolve(r); !errors.As(err, &readErr) {
		t.Fatalf("got %v", err)
	}
}
