package credentials

import (
	"context"
	"errors"
	"os"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/token"
)

// Resolved is the token to use and where it came from.
type Resolved struct {
	Token string
	// UserID is the owner of the token: its subject claim for THE_AGENT_CLI_TOKEN, the id saved at connect otherwise.
	UserID string
	// FromEnv is true when the token came from THE_AGENT_CLI_TOKEN and nothing was read from the stores.
	FromEnv bool
}

// Resolve finds the token: THE_AGENT_CLI_TOKEN when set and non-empty, else the stored one. The
// stored token of the passphrase mode is unlocked with THE_AGENT_CLI_PASSPHRASE or a prompt on a
// terminal; without either the result is ErrLocked and stdin is never read. A secret that is gone
// from its store clears the token metadata and is ErrNotConnected; any other store failure keeps
// the token and is a *StoreError.
func Resolve(ctx context.Context, env Env) (Resolved, error) {
	if raw := env.Getenv(config.EnvToken); raw != "" {
		claims, err := token.Parse(raw)
		if err != nil {
			return Resolved{}, ErrInvalidEnvToken
		}
		return Resolved{Token: raw, UserID: claims.Subject, FromEnv: true}, nil
	}
	configPath := config.Path(env.Home)
	file, err := config.Load(configPath)
	if err != nil {
		return Resolved{}, err
	}
	mode := Mode(file.Token.Store)
	if !mode.Valid() {
		return Resolved{}, ErrNotConnected
	}
	passphrase := ""
	if mode == ModePassphrase {
		if _, err := os.Stat(env.filePath()); err != nil && errors.Is(err, os.ErrNotExist) {
			return Resolved{}, gone(configPath, file)
		}
		if passphrase, err = unlockPassphrase(ctx, env); err != nil {
			return Resolved{}, err
		}
	}
	secret, err := env.store(mode, file.DeviceID, passphrase).Load()
	switch {
	case err == nil:
		return Resolved{Token: secret, UserID: file.Token.UserID}, nil
	case errors.Is(err, ErrNotFound):
		return Resolved{}, gone(configPath, file)
	case errors.Is(err, ErrIncorrectPassphrase), errors.Is(err, ErrDamaged):
		return Resolved{}, err
	}
	return Resolved{}, &StoreError{Op: OpRead, Mode: mode, Err: err}
}

func unlockPassphrase(ctx context.Context, env Env) (string, error) {
	if fromEnv := env.Getenv(config.EnvPassphrase); fromEnv != "" {
		return fromEnv, nil
	}
	if !env.Term.IsTTY(term.In) {
		return "", ErrLocked
	}
	return env.Term.ReadSecret(ctx, env.Prompts.Passphrase)
}

// gone clears the token metadata of a secret that no longer exists and reports not connected.
func gone(configPath string, file config.File) error {
	file.Token = config.Token{}
	if err := config.Save(configPath, file); err != nil {
		return errors.Join(ErrNotConnected, err)
	}
	return ErrNotConnected
}

// Forget removes the local token: the secret from its store and the token metadata from the config
// file, keeping the device id, the API URL and the default target. It succeeds when nothing is stored.
func Forget(env Env) error {
	configPath := config.Path(env.Home)
	file, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if mode := Mode(file.Token.Store); mode.Valid() {
		if err := env.store(mode, file.DeviceID, "").Delete(); err != nil {
			return &StoreError{Op: OpRemove, Mode: mode, Err: err}
		}
	}
	if file.Token == (config.Token{}) {
		return nil
	}
	file.Token = config.Token{}
	return config.Save(configPath, file)
}
