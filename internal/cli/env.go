package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/render"
	"github.com/appifyhub/agent-cli/internal/term"
)

// envVariables are the variables the CLI reads, in the order of the report; id selects the catalog text.
var envVariables = []struct{ name, id string }{
	{config.EnvAPIURL, "api_url"},
	{config.EnvHome, "home"},
	{config.EnvToken, "token"},
	{config.EnvPassphrase, "passphrase"},
	{config.EnvDefaultChat, "default_chat"},
	{config.EnvDefaultPlatform, "default_platform"},
	{"NO_COLOR", "no_color"},
}

func (s *rootState) envCommand() *cobra.Command {
	cat := s.deps.Text
	return withHelp(&cobra.Command{
		Use:   "env",
		Short: cat.T("command.env.short"),
		Long:  cat.T("command.env.long"),
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return s.runEnv() },
	}, cat.T("command.env.examples", "name", s.deps.Build.Name), cat.T("command.env.environment"), cat.T("command.env.exit_codes"))
}

// runEnv prints every variable with its purpose, whether it is set and the value in effect. It reads the
// config file, because the stored values are part of what is in effect, and writes nothing.
func (s *rootState) runEnv() error {
	d, cat := s.deps, s.deps.Text
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	homeKnown := err == nil
	if err != nil && !errors.Is(err, config.ErrHomeUnavailable) {
		return configFailure(cat, err)
	}
	var file config.File
	if homeKnown {
		if file, err = config.Load(config.Path(home)); err != nil {
			return configFailure(cat, err)
		}
	}
	userHome, _ := d.UserHomeDir()
	apiURL, _ := config.APIURL(d.Getenv, file, d.Build.APIURL)
	target, targetErr := config.DefaultTarget(d.Getenv, file)

	secret := func(name string) string {
		if d.Getenv(name) != "" {
			return cat.T("env.set_hidden")
		}
		return cat.T("env.not_set")
	}
	value := func(name, effective string) string {
		switch {
		case d.Getenv(name) != "":
			return cat.T("env.set_to", "value", term.Clean(effective, 0))
		case effective != "":
			return cat.T("env.not_set_using", "value", term.Clean(effective, 0))
		}
		return cat.T("env.not_set")
	}
	platform := target.Platform
	if raw := d.Getenv(config.EnvDefaultPlatform); raw != "" {
		platform = raw
		if targetErr == nil {
			platform = strings.ToLower(raw)
		}
	}

	states := map[string]string{
		config.EnvAPIURL:          value(config.EnvAPIURL, apiURL),
		config.EnvToken:           secret(config.EnvToken),
		config.EnvPassphrase:      secret(config.EnvPassphrase),
		config.EnvDefaultChat:     value(config.EnvDefaultChat, target.ChatID),
		config.EnvDefaultPlatform: value(config.EnvDefaultPlatform, platform),
		"NO_COLOR":                cat.T("env.colors_follow"),
	}
	if d.Getenv("NO_COLOR") != "" {
		states["NO_COLOR"] = cat.T("env.colors_off")
	}
	if homeKnown {
		states[config.EnvHome] = value(config.EnvHome, render.DisplayPath(home, userHome, d.GOOS))
	} else {
		states[config.EnvHome] = cat.T("env.home_unavailable")
	}

	var b strings.Builder
	for i, v := range envVariables {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s: %s\n  %s\n", v.name, states[v.name], cat.T("env.purpose."+v.id))
	}
	_, err = fmt.Fprint(d.Stdout, b.String())
	return err
}
