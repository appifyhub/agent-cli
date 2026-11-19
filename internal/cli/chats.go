package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/render"
	"github.com/appifyhub/agent-cli/internal/session"
	"github.com/appifyhub/agent-cli/internal/term"
)

func (s *rootState) chatsCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	const sampleDefault = "3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7"
	sample := render.ChatTable([]render.ChatRow{
		{ID: sampleDefault, Platform: "telegram", Title: cat.T("chats.sample_private_title"), Private: true},
		{ID: "9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e", Platform: "whatsapp", Title: cat.T("chats.sample_private_title"), Private: true},
		{ID: "c0de1234c0de1234c0de1234c0de1234", Platform: "telegram", Title: cat.T("chats.sample_group_title")},
	}, sampleDefault, cat)
	command := withHelp(&cobra.Command{
		Use: "chats", Short: cat.T("command.chats.short"),
		Long: cat.T("command.chats.long", "name", name, "sample", strings.TrimSuffix(sample, "\n")),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return s.runChats(cmd.Context()) },
	}, cat.T("command.chats.examples", "name", name), cat.T("command.chats.environment"), cat.T("command.chats.exit_codes"))
	command.AddCommand(s.chatsDefaultCommand())
	return command
}

func (s *rootState) runChats(ctx context.Context) error {
	d, cat := s.deps, s.deps.Text
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil {
		return s.failure(err)
	}
	file, err := config.Load(config.Path(home))
	if err != nil {
		return s.failure(err)
	}
	target, err := config.DefaultTarget(d.Getenv, file)
	if err != nil {
		return s.failure(err)
	}
	chats, err := s.fetchChats(ctx, home, file)
	if err != nil {
		return s.failure(err)
	}
	if len(chats) == 0 {
		_, err = fmt.Fprintln(d.Stdout, cat.T("chats.empty"))
		return err
	}
	rows := make([]render.ChatRow, len(chats))
	for i := range chats {
		rows[i] = render.ChatRow(chats[i])
	}
	_, err = fmt.Fprint(d.Stdout, render.ChatTable(rows, target.ChatID, cat))
	return err
}

func (s *rootState) fetchChats(ctx context.Context, home string, file config.File) ([]api.Chat, error) {
	d := s.deps
	auth := session.Session{Credentials: credentials.Env{
		Getenv: d.Getenv, Term: d.Term, Keychain: d.Keychain, Service: d.Build.Name,
		Home: home, WorkFactor: d.ScryptWorkFactor,
		Prompts: credentials.Prompts{Passphrase: d.Text.T("connect.passphrase")},
	}}
	var chats []api.Chat
	err := auth.Call(ctx, func(identity session.Identity) error {
		apiURL, err := config.APIURL(d.Getenv, file, d.Build.APIURL)
		if err != nil {
			return err
		}
		if err := config.ValidateAPIURL(apiURL); err != nil {
			return err
		}
		chats, err = s.newClient(apiURL).Chats(ctx, identity.Token)
		return err
	})
	return chats, err
}

func (s *rootState) runChatsDefault() error {
	d, cat := s.deps, s.deps.Text
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil {
		return s.failure(err)
	}
	file, err := config.Load(config.Path(home))
	if err != nil {
		return s.failure(err)
	}
	target, err := config.DefaultTarget(d.Getenv, file)
	if err != nil {
		return s.failure(err)
	}
	var line string
	switch {
	case d.Getenv(config.EnvDefaultChat) != "":
		line = cat.T("chats.default_environment_chat", "id", term.Clean(target.ChatID, 0))
	case d.Getenv(config.EnvDefaultPlatform) != "":
		line = cat.T("chats.default_environment_platform", "platform", term.Clean(target.Platform, 0))
	case target.ChatID != "":
		line = cat.T("chats.default_chat", "chat", s.chatsChatDescription(file.Default))
	case target.Platform != "":
		line = cat.T("chats.default_platform", "platform", term.Clean(target.Platform, 0))
	default:
		line = cat.T("chats.default_none")
	}
	_, err = fmt.Fprintf(d.Stdout, "%s\n%s\n", line, cat.T("chats.default_tip", "name", d.Build.Name))
	return err
}

func (s *rootState) chatsDefaultCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	var chatID, platform string
	var clear bool
	command := withHelp(&cobra.Command{
		Use:   "default",
		Short: cat.T("command.chats_default.short"), Long: cat.T("command.chats_default.long", "name", name),
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			forms := 0
			if cmd.Flags().Changed("set") {
				forms++
			}
			if cmd.Flags().Changed("platform") {
				forms++
			}
			if cmd.Flags().Changed("clear") {
				forms++
			}
			if forms > 1 || cmd.Flags().Changed("clear") && !clear || cmd.Flags().Changed("set") && strings.TrimSpace(chatID) == "" {
				return usageError(cat.T("chats.choose_form"))
			}
			if cmd.Flags().Changed("platform") {
				normalized := strings.ToLower(platform)
				if normalized != "telegram" && normalized != "whatsapp" {
					return usageError(cat.T("chats.invalid_platform", "value", platform))
				}
				platform = normalized
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("set") {
				return s.runChatsSetDefault(cmd.Context(), []string{chatID}, "")
			}
			if cmd.Flags().Changed("platform") || cmd.Flags().Changed("clear") {
				return s.runChatsSetDefault(cmd.Context(), nil, platform)
			}
			return s.runChatsDefault()
		},
	}, cat.T("command.chats_default.examples", "name", name), cat.T("command.chats_default.environment"), cat.T("command.chats_default.exit_codes"))
	command.Flags().StringVar(&chatID, "set", "", cat.T("command.chats_default.flag_set"))
	command.Flags().StringVar(&platform, "platform", "", cat.T("command.chats_default.flag_platform"))
	command.Flags().BoolVar(&clear, "clear", false, cat.T("command.chats_default.flag_clear"))
	_ = command.RegisterFlagCompletionFunc("platform", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"telegram", "whatsapp"}, cobra.ShellCompDirectiveNoFileComp
	})
	return command
}

func (s *rootState) runChatsSetDefault(ctx context.Context, args []string, platform string) error {
	d, cat := s.deps, s.deps.Text
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil {
		return s.failure(err)
	}
	file, err := config.Load(config.Path(home))
	if err != nil {
		return s.failure(err)
	}
	var target config.Default
	var message string
	switch {
	case len(args) == 1:
		chats, err := s.fetchChats(ctx, home, file)
		if err != nil {
			return s.failure(err)
		}
		for _, chat := range chats {
			if strings.EqualFold(chat.ID, args[0]) {
				target = config.Default{ChatID: chat.ID, ChatPlatform: chat.Platform, ChatTitle: chat.Title, ChatPrivate: chat.Private}
				break
			}
		}
		if target.ChatID == "" {
			return usageError(cat.T("chats.unknown_chat", "id", args[0], "name", d.Build.Name))
		}
		message = cat.T("chats.set_chat", "chat", s.chatsChatDescription(target))
	case platform != "":
		target.Platform = platform
		message = cat.T("chats.set_platform", "platform", platform)
	default:
		message = cat.T("chats.cleared")
	}
	file.Default = target
	if err := config.EnsureHome(home); err != nil {
		return s.failure(err)
	}
	if err := config.Save(config.Path(home), file); err != nil {
		return s.failure(err)
	}
	if _, err := fmt.Fprintln(d.Stdout, d.styled(term.Out, term.Success, message)); err != nil {
		return err
	}
	variable := ""
	if d.Getenv(config.EnvDefaultChat) != "" {
		variable = config.EnvDefaultChat
	} else if d.Getenv(config.EnvDefaultPlatform) != "" {
		variable = config.EnvDefaultPlatform
	}
	if variable != "" {
		_, _ = fmt.Fprintln(d.Stderr, d.styled(term.Err, term.Warning, cat.T("chats.override_warning", "variable", variable)))
	}
	return nil
}

func (s *rootState) chatsChatDescription(chat config.Default) string {
	cat := s.deps.Text
	kind := cat.T("status.group")
	if chat.ChatPrivate {
		kind = cat.T("status.private")
	}
	title := term.Clean(chat.ChatTitle, 0)
	if title == "" {
		title = cat.T("chats.untitled")
	}
	return cat.T("chats.chat_description", "id", term.Clean(chat.ChatID, 0), "platform", term.Clean(chat.ChatPlatform, 0), "title", title, "kind", kind)
}
