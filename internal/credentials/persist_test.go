package credentials_test

import (
	"errors"
	"os"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
)

func TestConnectThenResolveInEveryMode(t *testing.T) {
	for _, mode := range []string{"keychain", "passphrase", "unsafe"} {
		t.Run(mode, func(t *testing.T) {
			r := newRig(t)
			r.vars[config.EnvPassphrase] = "correct horse"
			if got := r.connect(mode); string(got) != mode {
				t.Fatalf("mode %s", got)
			}
			if r.readConfig().Token.Store != mode {
				t.Errorf("config records %q", r.readConfig().Token.Store)
			}
			got, err := credentials.Resolve(t.Context(), r.env)
			if err != nil || got.Token != token || got.FromEnv {
				t.Fatalf("Resolve() = %+v, %v", got, err)
			}
			inKeychain := r.kc.Len() == 1
			if inKeychain != (mode == "keychain") || r.fileExists(r.credentialsPath()) != (mode != "keychain") {
				t.Errorf("token is in the wrong place: keychain=%v file=%v", inKeychain, r.fileExists(r.credentialsPath()))
			}
		})
	}
}

func TestSwitchingStoresRemovesTheStaleSecret(t *testing.T) {
	t.Run("unsafe to keychain", func(t *testing.T) {
		r := newRig(t)
		r.connect("unsafe")
		r.connect("keychain")
		if r.fileExists(r.credentialsPath()) || r.kc.Len() != 1 {
			t.Errorf("file=%v keychain entries=%d", r.fileExists(r.credentialsPath()), r.kc.Len())
		}
	})
	t.Run("keychain to passphrase", func(t *testing.T) {
		r := newRig(t)
		r.vars[config.EnvPassphrase] = "pw"
		r.connect("keychain")
		r.connect("passphrase")
		if r.kc.Len() != 0 || !r.fileExists(r.credentialsPath()) {
			t.Errorf("keychain entries=%d file=%v", r.kc.Len(), r.fileExists(r.credentialsPath()))
		}
		if _, err := credentials.Resolve(t.Context(), r.env); err != nil {
			t.Errorf("the new secret must resolve: %v", err)
		}
	})
	t.Run("unsafe to passphrase keeps the one credentials file", func(t *testing.T) {
		r := newRig(t)
		r.vars[config.EnvPassphrase] = "pw"
		r.connect("unsafe")
		r.connect("passphrase")
		if !r.fileExists(r.credentialsPath()) {
			t.Fatal("the new encrypted file was removed as if it were the old plain one")
		}
		if _, err := credentials.Resolve(t.Context(), r.env); err != nil {
			t.Errorf("the new secret must resolve: %v", err)
		}
	})
	t.Run("passphrase to keychain", func(t *testing.T) {
		r := newRig(t)
		r.vars[config.EnvPassphrase] = "pw"
		r.connect("passphrase")
		r.connect("keychain")
		if r.fileExists(r.credentialsPath()) || r.kc.Len() != 1 {
			t.Errorf("file=%v keychain entries=%d", r.fileExists(r.credentialsPath()), r.kc.Len())
		}
	})
}

func TestConfigFailureRollsTheNewSecretBack(t *testing.T) {
	r := newRig(t)
	r.writeConfig(config.File{DeviceID: deviceID, Token: config.Token{Store: "unsafe"}})
	plan, err := credentials.NewPlan(t.Context(), r.env, deviceID, "keychain")
	if err != nil {
		t.Fatal(err)
	}
	r.kc.Calls = nil
	boom := errors.New("disk full")
	stale, err := plan.Persist(token, credentials.ModeUnsafe, func() error { return boom })
	if !errors.Is(err, boom) || stale != nil {
		t.Fatalf("got %v, %v", stale, err)
	}
	if r.kc.Len() != 0 {
		t.Error("the new keychain entry must be removed again")
	}
}

func TestSameLocationConfigFailureRemovesTheNewSecret(t *testing.T) {
	for _, mode := range []string{"keychain", "unsafe", "passphrase"} {
		t.Run(mode, func(t *testing.T) {
			r := newRig(t)
			r.vars[config.EnvPassphrase] = "pw"
			r.connect(mode)
			plan, err := credentials.NewPlan(t.Context(), r.env, deviceID, mode)
			if err != nil {
				t.Fatal(err)
			}
			boom := errors.New("disk full")
			_, err = plan.Persist("new-secret", credentials.Mode(mode), func() error { return boom })
			if !errors.Is(err, boom) {
				t.Fatalf("got %v", err)
			}
			if r.kc.Len() != 0 || r.fileExists(r.credentialsPath()) {
				t.Error("the newly saved secret survived a failed config write")
			}
		})
	}
}

func TestSecretFailureNeverReachesTheConfig(t *testing.T) {
	r := newRig(t)
	plan, err := credentials.NewPlan(t.Context(), r.env, deviceID, "keychain")
	if err != nil {
		t.Fatal(err)
	}
	r.kc.SetErr = errors.New("keychain is locked")
	called := false
	_, err = plan.Persist(token, "", func() error { called = true; return nil })
	var storeErr *credentials.StoreError
	if !errors.As(err, &storeErr) || storeErr.Mode != credentials.ModeKeychain {
		t.Fatalf("got %v, want a StoreError for the keychain", err)
	}
	if called || r.fileExists(r.configPath()) {
		t.Error("the config must not be written when the secret could not be saved")
	}
}

func TestFailedCleanupOfThePreviousStoreIsReportedAsStale(t *testing.T) {
	r := newRig(t)
	r.connect("keychain")
	plan, err := credentials.NewPlan(t.Context(), r.env, deviceID, "unsafe")
	if err != nil {
		t.Fatal(err)
	}
	r.kc.DelErr = errors.New("access denied")
	stale, err := plan.Persist(token, credentials.ModeKeychain, func() error { return nil })
	var storeErr *credentials.StoreError
	if err != nil || !errors.As(stale, &storeErr) || storeErr.Mode != credentials.ModeKeychain {
		t.Fatalf("got stale=%v err=%v", stale, err)
	}
	if !r.fileExists(r.credentialsPath()) {
		t.Error("the connection itself must stand")
	}
}

func TestSameModeReconnectReplacesInPlace(t *testing.T) {
	r := newRig(t)
	r.connect("keychain")
	r.connect("keychain")
	if r.kc.Len() != 1 {
		t.Errorf("keychain entries=%d, want 1", r.kc.Len())
	}
}

func TestForgetKeepsDeviceAPIURLAndDefault(t *testing.T) {
	for _, mode := range []string{"keychain", "passphrase", "unsafe"} {
		t.Run(mode, func(t *testing.T) {
			r := newRig(t)
			r.vars[config.EnvPassphrase] = "pw"
			r.writeConfig(config.File{APIURL: "https://agent.example.com"})
			r.connect(mode)
			if err := credentials.Forget(r.env); err != nil {
				t.Fatal(err)
			}
			if r.kc.Len() != 0 || r.fileExists(r.credentialsPath()) {
				t.Error("the secret must be gone")
			}
			f := r.readConfig()
			if f.Token != (config.Token{}) || f.DeviceID != deviceID || f.APIURL != "https://agent.example.com" || f.Default.ChatID != "3f2a" {
				t.Errorf("config after removal: %+v", f)
			}
			if err := credentials.Forget(r.env); err != nil {
				t.Errorf("removal must be idempotent: %v", err)
			}
		})
	}
}

func TestForgetWithNothingStoredCreatesNothing(t *testing.T) {
	r := newRig(t)
	if err := credentials.Forget(r.env); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.configPath()); err == nil {
		t.Error("removal must not create a config file")
	}
}

func TestForgetReportsStoreFailuresAndKeepsMetadata(t *testing.T) {
	r := newRig(t)
	r.connect("keychain")
	r.kc.DelErr = errors.New("access denied")
	var storeErr *credentials.StoreError
	if err := credentials.Forget(r.env); !errors.As(err, &storeErr) {
		t.Fatalf("got %v", err)
	}
	if r.readConfig().Token.Store != "keychain" {
		t.Error("metadata must stay while the secret is still there")
	}
}
