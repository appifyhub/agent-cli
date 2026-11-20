package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/term"
)

func (s *rootState) unlockCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	var shell string
	command := withHelp(&cobra.Command{
		Use: "unlock", Short: cat.T("command.unlock.short"), Long: cat.T("command.unlock.long", "name", name),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			selected, err := s.unlockShell(shell)
			if err != nil {
				return err
			}
			return s.runUnlock(cmd.Context(), selected)
		},
	}, cat.T("command.unlock.examples", "name", name), cat.T("command.unlock.environment"), cat.T("command.unlock.exit_codes"))
	command.Flags().StringVar(&shell, "shell", "", cat.T("command.unlock.flag_shell"))
	_ = command.RegisterFlagCompletionFunc("shell", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"sh", "bash", "zsh", "fish", "pwsh", "powershell"}, cobra.ShellCompDirectiveNoFileComp
	})
	return command
}

func (s *rootState) runUnlock(ctx context.Context, shell string) error {
	d := s.deps
	if d.Getenv(config.EnvToken) != "" {
		_, err := fmt.Fprintln(d.Stderr, d.Text.T("unlock.already"))
		return err
	}
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil {
		return s.failure(err)
	}
	file, err := config.Load(config.Path(home))
	if err != nil {
		return s.failure(err)
	}
	mode := credentials.Mode(file.Token.Store)
	if !mode.Valid() {
		return s.failure(credentials.ErrNotConnected)
	}
	if mode != credentials.ModePassphrase {
		_, err := fmt.Fprintln(d.Stderr, d.Text.T("unlock.nothing"))
		return err
	}
	resolved, err := credentials.Resolve(ctx, credentials.Env{
		Getenv: d.Getenv, Term: d.Term, Keychain: d.Keychain, Service: d.Build.Name,
		Home: home, WorkFactor: d.ScryptWorkFactor,
		Prompts: credentials.Prompts{Passphrase: d.Text.T("connect.passphrase")},
	})
	if err != nil {
		return s.failure(err)
	}
	if _, err := fmt.Fprintln(d.Stderr, d.styled(term.Err, term.Success, d.Text.T("unlock.success"))); err != nil {
		return err
	}
	if d.Term.IsTTY(term.Out) {
		if _, err := fmt.Fprintln(d.Stderr, d.Text.T("unlock.hint", "name", d.Build.Name)); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(d.Stdout, shellAssignment(shell, resolved.Token))
	return err
}
