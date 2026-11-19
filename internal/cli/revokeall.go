package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/session"
	"github.com/appifyhub/agent-cli/internal/term"
)

func (s *rootState) revokeAllCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	var yes bool
	command := withHelp(&cobra.Command{
		Use: "revoke-all", Short: cat.T("command.revoke_all.short"), Long: cat.T("command.revoke_all.long"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return s.runRevokeAll(cmd.Context(), yes) },
	}, cat.T("command.revoke_all.examples", "name", name), cat.T("command.revoke_all.environment"), cat.T("command.revoke_all.exit_codes"))
	command.Flags().BoolVar(&yes, "yes", false, cat.T("command.revoke_all.flag_yes"))
	return command
}

func (s *rootState) runRevokeAll(ctx context.Context, yes bool) error {
	d, cat := s.deps, s.deps.Text
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil {
		return s.failure(err)
	}
	file, err := config.Load(config.Path(home))
	if err != nil {
		return s.failure(err)
	}
	stored := credentials.Mode(file.Token.Store).Valid()
	if !stored && d.Getenv(config.EnvToken) == "" {
		return s.failure(credentials.ErrNotConnected)
	}
	if !yes {
		if !d.Term.IsTTY(term.In) {
			return usageError(cat.T("revoke_all.confirmation_required"))
		}
		if err := s.confirm(ctx, cat.T("revoke_all.confirm")); err != nil {
			return err
		}
	}
	environment := credentials.Env{
		Getenv: d.Getenv, Term: d.Term, Keychain: d.Keychain, Service: d.Build.Name,
		Home: home, WorkFactor: d.ScryptWorkFactor,
		Prompts: credentials.Prompts{Passphrase: cat.T("connect.passphrase")},
	}
	resolved, err := credentials.Resolve(ctx, environment)
	if err != nil {
		return s.failure(err)
	}
	apiURL, err := config.APIURL(d.Getenv, file, d.Build.APIURL)
	if err != nil {
		return s.failure(err)
	}
	if err := config.ValidateAPIURL(apiURL); err != nil {
		return s.failure(err)
	}
	err = s.newClient(apiURL).RevokeAll(ctx, resolved.Token, resolved.UserID)
	already := api.IsKind(err, api.KindTokenRejected)
	if already && resolved.FromEnv {
		return s.failure(session.ErrEnvTokenRejected)
	}
	if err != nil && !already {
		if api.IsKind(err, api.KindUnreachable) || api.IsKind(err, api.KindServerFault) {
			return &Failure{Code: ExitFailure, Message: cat.T("revoke_all.unreachable")}
		}
		return s.failure(err)
	}
	if err := credentials.Forget(environment); err != nil {
		return s.failure(err)
	}
	message := cat.T("revoke_all.success")
	if already {
		message = cat.T("revoke_all.already_revoked")
	} else if !stored {
		message = cat.T("revoke_all.success_no_local")
	}
	_, err = fmt.Fprintln(d.Stdout, message)
	return err
}
