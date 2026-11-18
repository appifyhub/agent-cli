// Package session makes authenticated calls: it finds the token, hands it to the call and reacts
// to a rejected token. It writes nothing itself; diagnostics are the API client's hooks.
package session

import (
	"context"
	"errors"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/credentials"
)

// ErrEnvTokenRejected means the server refused the token in THE_AGENT_CLI_TOKEN. Nothing stored is touched.
var ErrEnvTokenRejected = errors.New("the token in THE_AGENT_CLI_TOKEN was rejected")

// Session is the token source and the way to reach the backend.
type Session struct {
	Credentials credentials.Env
}

// Identity is the token and its owner, handed to every authenticated call.
type Identity struct {
	Token  string
	UserID string
}

// Call resolves the token and runs fn once with it. When the server rejects the token, a stored
// token is removed locally and the result is credentials.ErrNotConnected; a token from the
// environment is left alone and the result is ErrEnvTokenRejected. Every other outcome, including
// the codes that describe a chat or the account rather than the token, keeps the stored token.
func (s Session) Call(ctx context.Context, fn func(Identity) error) error {
	resolved, err := credentials.Resolve(ctx, s.Credentials)
	if err != nil {
		return err
	}
	err = fn(Identity{Token: resolved.Token, UserID: resolved.UserID})
	if !api.IsKind(err, api.KindTokenRejected) {
		return err
	}
	if resolved.FromEnv {
		return errors.Join(ErrEnvTokenRejected, err)
	}
	if forgetErr := credentials.Forget(s.Credentials); forgetErr != nil {
		return errors.Join(credentials.ErrNotConnected, forgetErr)
	}
	return credentials.ErrNotConnected
}
