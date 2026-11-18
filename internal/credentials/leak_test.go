package credentials_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

const leakyPassphrase = "hunter2-correct-horse"

// secrets are every value that must never show up in a config file or an error message.
func secrets() []string {
	return []string{token, leakyPassphrase, strings.Split(token, ".")[1]}
}

func assertClean(t *testing.T, text string) {
	t.Helper()
	for _, secret := range secrets() {
		testkit.AssertNoSecret(t, secret, text)
	}
}

func TestConfigFileNeverHoldsASecret(t *testing.T) {
	for _, mode := range []string{"keychain", "passphrase", "unsafe"} {
		r := newRig(t)
		r.vars[config.EnvPassphrase] = leakyPassphrase
		r.connect(mode)
		raw, err := os.ReadFile(r.configPath())
		if err != nil {
			t.Fatal(err)
		}
		assertClean(t, string(raw))
	}
}

func TestEncryptedFileHoldsNeitherTokenNorPassphrase(t *testing.T) {
	r := newRig(t)
	r.vars[config.EnvPassphrase] = leakyPassphrase
	r.connect("passphrase")
	raw, err := os.ReadFile(r.credentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	assertClean(t, string(raw))
}

func TestErrorsNeverCarrySecrets(t *testing.T) {
	var errs []error
	collect := func(err error) {
		if err == nil {
			t.Error("an error path unexpectedly succeeded")
			return
		}
		errs = append(errs, err)
	}

	// invalid environment token that looks like a secret
	r := newRig(t)
	r.vars[config.EnvToken] = leakyPassphrase
	_, err := resolve(r)
	collect(err)

	// every plan failure with a passphrase in play
	for _, c := range []struct {
		flag  string
		typed []string
		tty   bool
		env   string
		bad   bool
	}{
		{flag: "vault"},
		{flag: "keychain", bad: true, env: leakyPassphrase},
		{flag: "passphrase"},
		{flag: "passphrase", tty: true, typed: []string{leakyPassphrase, leakyPassphrase + "x"}},
		{flag: "passphrase", tty: true, typed: []string{""}},
		{bad: true},
	} {
		r := newRig(t)
		if c.bad {
			r.kc.SetErr = errors.New("keychain unavailable")
		}
		r.term.TTYIn, r.term.Secrets = c.tty, c.typed
		r.vars[config.EnvPassphrase] = c.env
		_, err := credentials.NewPlan(t.Context(), r.env, deviceID, c.flag)
		collect(err)
	}

	// store failures while saving and reading
	r = newRig(t)
	r.kc.SetErr = errors.New("keychain is locked")
	plan, planErr := credentials.NewPlan(t.Context(), r.env, deviceID, "unsafe")
	if planErr != nil {
		t.Fatal(planErr)
	}
	_, err = plan.Persist(token, "", func() error { return errors.New("disk full") })
	collect(err)

	r = newRig(t)
	r.connect("keychain")
	r.kc.GetErr = errors.New("access denied")
	_, err = resolve(r)
	collect(err)

	// passphrase failures: wrong, locked, damaged, missing
	r = newRig(t)
	r.vars[config.EnvPassphrase] = leakyPassphrase
	r.connect("passphrase")
	r.vars[config.EnvPassphrase] = "wrong"
	_, err = resolve(r)
	collect(err)
	r.vars[config.EnvPassphrase] = ""
	_, err = resolve(r)
	collect(err)
	raw, _ := os.ReadFile(r.credentialsPath())
	_ = os.WriteFile(r.credentialsPath(), raw[:len(raw)/3], 0o600)
	r.vars[config.EnvPassphrase] = leakyPassphrase
	_, err = resolve(r)
	collect(err)
	_ = os.Remove(r.credentialsPath())
	_, err = resolve(r)
	collect(err)

	for _, err := range errs {
		assertClean(t, err.Error())
		var storeErr *credentials.StoreError
		if errors.As(err, &storeErr) {
			assertClean(t, storeErr.Reason())
		}
	}
}
