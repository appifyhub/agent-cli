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

func (s *rootState) notifyCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	var chat, platform string
	command := withHelp(&cobra.Command{
		Use: "notify [<message>]", Short: cat.T("command.notify.short"), Long: cat.T("command.notify.long"),
		Args: func(cmd *cobra.Command, args []string) error {
			_, err := notifyTarget(chat, platform, cmd.Flags().Changed("chat"), cmd.Flags().Changed("platform"), s.deps.Getenv, config.File{}, cat)
			if err != nil {
				return s.failure(err)
			}
			if len(args) > 1 {
				return usageError(cat.T("notify.single_argument"))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			message, err := s.notifyMessage(args)
			if err != nil {
				return err
			}
			return s.runNotify(cmd.Context(), message, chat, platform, cmd.Flags().Changed("chat"), cmd.Flags().Changed("platform"))
		},
	}, cat.T("command.notify.examples", "name", name), cat.T("command.notify.environment"), cat.T("command.notify.exit_codes"))
	command.Flags().StringVar(&chat, "chat", "", cat.T("command.notify.flag_chat"))
	command.Flags().StringVar(&platform, "platform", "", cat.T("command.notify.flag_platform"))
	_ = command.RegisterFlagCompletionFunc("platform", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"telegram", "whatsapp"}, cobra.ShellCompDirectiveNoFileComp
	})
	return command
}

func (s *rootState) runNotify(ctx context.Context, message, chat, platform string, chatSet, platformSet bool) error {
	d := s.deps
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil {
		return s.failure(err)
	}
	file, err := config.Load(config.Path(home))
	if err != nil {
		return s.failure(err)
	}
	target, err := notifyTarget(chat, platform, chatSet, platformSet, d.Getenv, file, d.Text)
	if err != nil {
		return s.failure(err)
	}
	auth := session.Session{Credentials: credentials.Env{
		Getenv: d.Getenv, Term: d.Term, Keychain: d.Keychain, Service: d.Build.Name,
		Home: home, WorkFactor: d.ScryptWorkFactor,
		Prompts: credentials.Prompts{Passphrase: d.Text.T("connect.passphrase")},
	}}
	var delivery api.Delivery
	err = auth.Call(ctx, func(identity session.Identity) error {
		apiURL, urlErr := config.APIURL(d.Getenv, file, d.Build.APIURL)
		if urlErr != nil {
			return urlErr
		}
		if urlErr := config.ValidateAPIURL(apiURL); urlErr != nil {
			return urlErr
		}
		client := s.newClient(apiURL)
		var callErr error
		if target.ChatID != "" {
			delivery, callErr = client.NotifyChat(ctx, identity.Token, target.ChatID, message)
		} else {
			delivery, callErr = client.NotifyUser(ctx, identity.Token, identity.UserID, message, target.Platform)
		}
		return callErr
	})
	if err != nil {
		return s.failure(err)
	}
	if s.verbose {
		line := d.Text.T("notify.sent")
		if chat := delivery.Chat; chat != nil {
			platform := term.Clean(chat.Platform, 0)
			switch platform {
			case "telegram":
				platform = d.Text.T("notify.telegram")
			case "whatsapp":
				platform = d.Text.T("notify.whatsapp")
			}
			title := term.Clean(chat.Title, 0)
			if title == "" {
				title = d.Text.T("chats.untitled")
			}
			kind := d.Text.T("status.group")
			if chat.Private {
				kind = d.Text.T("status.private")
			}
			line = d.Text.T("notify.sent_to", "platform", platform, "title", title, "kind", kind)
		}
		if _, err := fmt.Fprintln(d.Stdout, d.styled(term.Out, term.Success, line)); err != nil {
			return err
		}
	}
	s.warnLowCredit(delivery.CreditBalance)
	return nil
}
