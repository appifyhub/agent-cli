package credentials

import (
	"context"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/term"
)

// Plan is the decision, taken before the backend is contacted, of where a new token will be stored.
type Plan struct {
	env      Env
	deviceID string
	mode     Mode
	store    Store
}

// NewPlan picks the store and collects the passphrase it needs. An explicit mode is honored or
// fails; it never falls back. Without one the keychain is tried, then the passphrase mode when a
// passphrase can be obtained, then it fails. Only the passphrase mode prompts, and the keychain is
// probed only when its result matters.
func NewPlan(ctx context.Context, env Env, deviceID, requested string) (*Plan, error) {
	mode, err := selectMode(env, deviceID, requested)
	if err != nil {
		return nil, err
	}
	plan := &Plan{env: env, deviceID: deviceID, mode: mode}
	passphrase := ""
	if mode == ModePassphrase {
		if passphrase, err = collectPassphrase(ctx, env); err != nil {
			return nil, err
		}
	}
	plan.store = env.store(mode, deviceID, passphrase)
	return plan, nil
}

// Mode is the chosen mode, as recorded in the config file.
func (p *Plan) Mode() Mode { return p.mode }

func selectMode(env Env, deviceID, requested string) (Mode, error) {
	if requested != "" {
		mode := Mode(requested)
		if !mode.Valid() {
			return "", &StoreFlagError{Value: requested}
		}
		switch {
		case mode == ModeKeychain && Probe(env.Keychain, env.Service) != nil:
			return "", ErrNoKeychain
		case mode == ModePassphrase && !canAskPassphrase(env):
			return "", ErrNoPassphraseSource
		}
		return mode, nil
	}
	if Probe(env.Keychain, env.Service) == nil {
		return ModeKeychain, nil
	}
	if canAskPassphrase(env) {
		return ModePassphrase, nil
	}
	return "", ErrNoStore
}

func canAskPassphrase(env Env) bool {
	return env.Getenv(config.EnvPassphrase) != "" || env.Term.IsTTY(term.In)
}

func collectPassphrase(ctx context.Context, env Env) (string, error) {
	if fromEnv := env.Getenv(config.EnvPassphrase); fromEnv != "" {
		return fromEnv, nil
	}
	first, err := env.Term.ReadSecret(ctx, env.Prompts.Choose)
	if err != nil {
		return "", err
	}
	if first == "" {
		return "", ErrPassphraseEmpty
	}
	second, err := env.Term.ReadSecret(ctx, env.Prompts.Repeat)
	if err != nil {
		return "", err
	}
	if first != second {
		return "", ErrPassphraseMismatch
	}
	return first, nil
}

// Persist stores the token and then runs writeConfig, which records the metadata. The order is
// fixed: secret first, then config, then removal of the secret left in the previous store. A config
// failure removes the new secret again and a secret failure never reaches the config.
// Failing to clean up the previous store
// is reported as stale and does not undo the connection.
func (p *Plan) Persist(token string, previous Mode, writeConfig func() error) (stale, err error) {
	if err := p.store.Save(token); err != nil {
		return nil, &StoreError{Op: OpWrite, Mode: p.mode, Err: err}
	}
	replaced := previous.Valid() && sameLocation(previous, p.mode)
	if err := writeConfig(); err != nil {
		_ = p.store.Delete()
		return nil, err
	}
	if previous.Valid() && !replaced {
		if err := p.env.store(previous, p.deviceID, "").Delete(); err != nil {
			return &StoreError{Op: OpRemove, Mode: previous, Err: err}, nil
		}
	}
	return nil, nil
}
