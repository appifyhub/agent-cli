package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/render"
	"github.com/appifyhub/agent-cli/internal/term"
)

func (s *rootState) connectCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	var mode, ttl string
	command := withHelp(&cobra.Command{
		Use:   "connect <KEY>",
		Short: cat.T("command.connect.short"),
		Long:  cat.T("command.connect.long"),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 || len(args) == 1 && strings.TrimSpace(args[0]) == "" {
				return usageError(cat.T("connect.missing_key", "name", name))
			}
			return cobra.ExactArgs(1)(nil, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.runConnect(cmd.Context(), strings.ToUpper(strings.TrimSpace(args[0])), mode, ttl, cmd.Flags().Changed("ttl"))
		},
	}, cat.T("command.connect.examples", "name", name), cat.T("command.connect.environment"), cat.T("command.connect.exit_codes"))
	command.Flags().StringVar(&mode, "store", "", cat.T("command.connect.flag_store"))
	command.Flags().StringVar(&ttl, "ttl", "", cat.T("command.connect.flag_ttl"))
	_ = command.RegisterFlagCompletionFunc("store", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{string(credentials.ModeKeychain), string(credentials.ModePassphrase), string(credentials.ModeUnsafe)}, cobra.ShellCompDirectiveNoFileComp
	})
	return command
}

func (s *rootState) runConnect(ctx context.Context, key, mode, ttl string, ttlSet bool) error {
	d, cat := s.deps, s.deps.Text
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil {
		return s.failure(err)
	}
	if err := config.EnsureHome(home); err != nil {
		return s.failure(err)
	}
	path := config.Path(home)
	file, err := config.Load(path)
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
	if err := config.CheckWritable(home); err != nil {
		return s.failure(err)
	}
	machine := d.Machine()
	if file.DeviceID == "" {
		file.EnsureDeviceID(machine, home)
	}
	host := machineHostname(machine.Hostname, file.DeviceID)
	environment := credentials.Env{
		Getenv: d.Getenv, Term: d.Term, Keychain: d.Keychain, Service: d.Build.Name,
		Home: home, WorkFactor: d.ScryptWorkFactor,
		Prompts: credentials.Prompts{
			Choose: cat.T("connect.choose_passphrase"), Repeat: cat.T("connect.repeat_passphrase"),
			Passphrase: cat.T("connect.passphrase"),
		},
	}
	plan, err := credentials.NewPlan(ctx, environment, file.DeviceID, mode)
	if err != nil {
		return s.failure(err)
	}
	connection, err := s.newClient(apiURL).Connect(ctx, api.ConnectRequest{
		ConnectKey: key, DeviceID: file.DeviceID, Hostname: host, OS: d.GOOS, TTL: ttl, TTLSet: ttlSet,
	})
	if err != nil {
		return s.connectFailure(err, ttl, key, environment.Getenv(config.EnvPassphrase))
	}
	previous := credentials.Mode(file.Token.Store)
	if file.Token.UserID != connection.UserID {
		file.Default = config.Default{}
	}
	file.APIURL = apiURL
	file.Token = config.Token{
		Store: string(plan.Mode()), UserID: connection.UserID, UserName: connection.UserDisplayName,
		Hostname: host, ExpiresAt: connection.ExpiresAt,
	}
	stale, err := plan.Persist(connection.Token, previous, func() error { return config.Save(path, file) })
	if err != nil {
		var storeError *credentials.StoreError
		if errors.As(err, &storeError) {
			reason := redactSecrets(cat.T("error.connected_store_failed", "reason", storeError.Reason()), key, connection.Token, d.Getenv(config.EnvPassphrase))
			return &Failure{Code: ExitFailure, Message: reason}
		}
		return s.failure(err)
	}
	if stale != nil {
		return s.failure(stale)
	}
	userHome, _ := d.UserHomeDir()
	location := cat.T("connect.location_keychain")
	switch plan.Mode() {
	case credentials.ModePassphrase:
		location = cat.T("connect.location_encrypted", "path", render.DisplayPath(filepath.Join(home, credentials.FileName), userHome, d.GOOS))
	case credentials.ModeUnsafe:
		location = cat.T("connect.location_unsafe", "path", render.DisplayPath(filepath.Join(home, credentials.FileName), userHome, d.GOOS))
	}
	output := cat.T("connect.success", "user", term.Clean(connection.UserDisplayName, 0), "hostname", host,
		"store", term.Clean(location, 0), "expires", render.LocalTime(connection.ExpiresAt, d.Location),
		"relative", render.RelativeTime(connection.ExpiresAt, d.Now().In(d.Location), cat))
	if plan.Mode() == credentials.ModePassphrase {
		output += "\n" + cat.T("connect.unlock_hint", "name", d.Build.Name)
	}
	if _, err := fmt.Fprintln(d.Stdout, d.styledFirstLine(term.Out, term.Success, output)); err != nil {
		return err
	}
	s.warnLowCredit(connection.CreditBalance)
	return nil
}

func (s *rootState) connectFailure(err error, ttl string, secrets ...string) error {
	cat := s.deps.Text
	var backend *api.Error
	if errors.As(err, &backend) {
		switch {
		case backend.Status == 401 || backend.Status == 403:
			return &Failure{Code: ExitFailure, Message: cat.T("error.invalid_connect_key")}
		case backend.Kind == api.KindInvalidTTL:
			return &Failure{Code: ExitUsage, Message: cat.T("error.invalid_ttl", "value", ttl)}
		case backend.Kind == api.KindTTLTooLong:
			return &Failure{Code: ExitUsage, Message: cat.T("error.ttl_too_long")}
		}
	}
	translated := s.failure(err)
	var failure *Failure
	if errors.As(translated, &failure) {
		failure.Message = redactSecrets(failure.Message, secrets...)
	}
	return translated
}

func redactSecrets(message string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "***")
		}
	}
	return message
}
