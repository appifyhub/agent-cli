package credentials_test

import (
	"errors"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
)

func TestPlanSelectionMatrix(t *testing.T) {
	broken := errors.New("no secret service")
	cases := []struct {
		name         string
		flag         string
		keychainOK   bool
		tty          bool
		envPass      string
		typed        []string
		wantMode     credentials.Mode
		wantErr      error
		wantPrompts  int
		wantProbe    bool
		wantStoreErr bool
	}{
		{name: "auto keychain, no prompt even on a terminal", keychainOK: true, tty: true, envPass: "x", wantMode: "keychain", wantProbe: true},
		{name: "auto without keychain but a terminal", tty: true, typed: []string{"pw", "pw"}, wantMode: "passphrase", wantPrompts: 2, wantProbe: true},
		{name: "auto without keychain, no terminal, passphrase in the environment", envPass: "pw", wantMode: "passphrase", wantProbe: true},
		{name: "auto with nothing to use", wantErr: credentials.ErrNoStore, wantProbe: true},
		{name: "explicit keychain", flag: "keychain", keychainOK: true, wantMode: "keychain", wantProbe: true},
		{name: "explicit keychain unavailable never falls back", flag: "keychain", tty: true, envPass: "pw", wantErr: credentials.ErrNoKeychain, wantProbe: true},
		{name: "explicit passphrase on a terminal skips the probe", flag: "passphrase", keychainOK: true, tty: true, typed: []string{"pw", "pw"}, wantMode: "passphrase", wantPrompts: 2},
		{name: "explicit passphrase from the environment", flag: "passphrase", keychainOK: true, envPass: "pw", wantMode: "passphrase"},
		{name: "explicit passphrase without a way to ask", flag: "passphrase", keychainOK: true, wantErr: credentials.ErrNoPassphraseSource},
		{name: "explicit unsafe never probes and never prompts", flag: "unsafe", keychainOK: true, tty: true, envPass: "pw", wantMode: "unsafe"},
		{name: "unknown mode", flag: "vault", keychainOK: true, tty: true, wantStoreErr: true},
		{name: "empty passphrase", flag: "passphrase", tty: true, typed: []string{""}, wantErr: credentials.ErrPassphraseEmpty, wantPrompts: 1},
		{name: "passphrases differ", flag: "passphrase", tty: true, typed: []string{"one", "two"}, wantErr: credentials.ErrPassphraseMismatch, wantPrompts: 2},
		{name: "empty environment passphrase counts as unset", flag: "passphrase", envPass: "", wantErr: credentials.ErrNoPassphraseSource},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			if !c.keychainOK {
				r.kc.SetErr = broken
			}
			r.term.TTYIn = c.tty
			r.term.Secrets = c.typed
			r.vars[config.EnvPassphrase] = c.envPass

			plan, err := credentials.NewPlan(t.Context(), r.env, deviceID, c.flag)
			var flagErr *credentials.StoreFlagError
			switch {
			case c.wantStoreErr:
				if !errors.As(err, &flagErr) || flagErr.Value != c.flag {
					t.Fatalf("got %v, want a StoreFlagError for %q", err, c.flag)
				}
			case c.wantErr != nil:
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("got %v, want %v", err, c.wantErr)
				}
			default:
				if err != nil || plan.Mode() != c.wantMode {
					t.Fatalf("got %v, %v; want mode %s", plan, err, c.wantMode)
				}
			}
			if len(r.term.Prompts) != c.wantPrompts {
				t.Errorf("prompts %q, want %d", r.term.Prompts, c.wantPrompts)
			}
			if probed := len(r.kc.Calls) > 0; probed != c.wantProbe {
				t.Errorf("probed = %v, want %v", probed, c.wantProbe)
			}
		})
	}
}

func TestPassphrasePromptsAreTheConfiguredOnes(t *testing.T) {
	r := newRig(t)
	r.term.TTYIn = true
	r.term.Secrets = []string{"pw", "pw"}
	if _, err := credentials.NewPlan(t.Context(), r.env, deviceID, "passphrase"); err != nil {
		t.Fatal(err)
	}
	if got := r.term.Prompts; len(got) != 2 || got[0] != "Choose a passphrase: " || got[1] != "Repeat passphrase: " {
		t.Errorf("prompts %q", got)
	}
}

func TestAbortedPassphrasePromptIsReported(t *testing.T) {
	r := newRig(t)
	r.term.TTYIn = true
	if _, err := credentials.NewPlan(t.Context(), r.env, deviceID, "passphrase"); err == nil {
		t.Error("an aborted prompt must fail the plan")
	}
}

func TestPlanLeavesNoStateBehind(t *testing.T) {
	r := newRig(t)
	r.term.TTYIn = true
	r.term.Secrets = []string{"pw", "pw"}
	if _, err := credentials.NewPlan(t.Context(), r.env, deviceID, "passphrase"); err != nil {
		t.Fatal(err)
	}
	if r.fileExists(r.credentialsPath()) || r.fileExists(r.configPath()) {
		t.Error("planning must not write anything")
	}
	if r.kc.Len() != 0 {
		t.Error("planning must not leave a keychain entry")
	}
}
