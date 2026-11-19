package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/render"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/token"
)

func (s *rootState) statusCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	backend := s.deps.Build.APIURL
	if backend == "" {
		backend = cat.T("env.not_set")
	}
	return withHelp(&cobra.Command{
		Use: "status", Short: cat.T("command.status.short"), Long: cat.T("command.status.long", "name", name, "backend", term.Clean(backend, 0)),
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return s.runStatus() },
	}, cat.T("command.status.examples", "name", name), cat.T("command.status.environment"), cat.T("command.status.exit_codes"))
}

func (s *rootState) runStatus() error {
	d, cat := s.deps, s.deps.Text
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil && !errors.Is(err, config.ErrHomeUnavailable) {
		return s.failure(err)
	}
	var file config.File
	if err == nil {
		file, err = config.Load(config.Path(home))
		if err != nil {
			return s.failure(err)
		}
	}
	raw := d.Getenv(config.EnvToken)
	var claims token.Claims
	if raw != "" {
		claims, err = token.Parse(raw)
		if err != nil {
			return s.failure(credentials.ErrInvalidEnvToken)
		}
	}
	mode := credentials.Mode(file.Token.Store)
	connected := mode.Valid()
	path := filepath.Join(home, credentials.FileName)
	if raw == "" && connected && mode != credentials.ModeKeychain {
		if _, err := os.Stat(path); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return s.failure(&credentials.StoreError{Op: credentials.OpRead, Mode: mode, Err: err})
			}
			connected = false
		}
	}
	if raw == "" && !connected {
		if _, err := fmt.Fprintln(d.Stdout, cat.T("status.not_connected", "name", d.Build.Name)); err != nil {
			return err
		}
		return &Failure{Code: ExitNotConnected, Silent: true}
	}
	target, err := config.DefaultTarget(d.Getenv, file)
	if err != nil {
		return s.failure(err)
	}
	apiURL, _ := config.APIURL(d.Getenv, file, d.Build.APIURL)
	if apiURL == "" {
		apiURL = cat.T("env.not_set")
	}
	var b strings.Builder
	location, expires := cat.T("status.location_environment"), claims.ExpiresAt
	locked := raw == "" && mode == credentials.ModePassphrase
	if raw != "" {
		fmt.Fprintln(&b, cat.T("status.connected_environment"))
	} else {
		fmt.Fprintln(&b, cat.T("status.connected", "user", term.Clean(file.Token.UserName, 0), "hostname", term.Clean(file.Token.Hostname, 0)))
		fmt.Fprintln(&b, cat.T("status.device", "id", term.Clean(file.DeviceID, 0)))
		expires = file.Token.ExpiresAt
		location = cat.T("connect.location_keychain")
		if mode != credentials.ModeKeychain {
			userHome, _ := d.UserHomeDir()
			displayPath := term.Clean(render.DisplayPath(path, userHome, d.GOOS), 0)
			if locked {
				location = cat.T("status.location_locked", "path", displayPath)
			} else {
				location = cat.T("connect.location_unsafe", "path", displayPath)
			}
		}
	}
	now := d.Now().In(d.Location)
	expired := !expires.IsZero() && !expires.After(now)
	fmt.Fprintln(&b, s.statusTokenLine(location, expires, now))
	fmt.Fprintln(&b, s.statusDefaultLine(target, file.Default))
	fmt.Fprintln(&b, cat.T("status.backend", "url", term.Clean(apiURL, 0)))
	if expired {
		fmt.Fprintln(&b, cat.T("status.reconnect", "name", d.Build.Name))
	} else if locked {
		fmt.Fprintln(&b, cat.T("status.unlock", "name", d.Build.Name))
	}
	if _, err := fmt.Fprint(d.Stdout, b.String()); err != nil {
		return err
	}
	if expired || locked {
		return &Failure{Code: ExitNotConnected, Silent: true}
	}
	return nil
}

func (s *rootState) statusTokenLine(location string, expires, now time.Time) string {
	cat := s.deps.Text
	if expires.IsZero() {
		return cat.T("status.token", "location", location)
	}
	when, relative := render.LocalTime(expires, s.deps.Location), render.RelativeTime(expires, now, cat)
	if !expires.After(now) {
		return cat.T("status.token_expired", "location", location, "time", when, "relative", relative)
	}
	return cat.T("status.token_expires", "location", location, "time", when, "relative", relative)
}

func (s *rootState) statusDefaultLine(target config.Target, stored config.Default) string {
	d, cat := s.deps, s.deps.Text
	if d.Getenv(config.EnvDefaultChat) != "" {
		return cat.T("status.default_environment_chat", "id", term.Clean(target.ChatID, 0))
	}
	if d.Getenv(config.EnvDefaultPlatform) != "" {
		return cat.T("status.default_environment_platform", "platform", term.Clean(target.Platform, 0))
	}
	if target.ChatID != "" {
		kind := cat.T("status.group")
		if stored.ChatPrivate {
			kind = cat.T("status.private")
		}
		return cat.T("status.default_chat", "id", term.Clean(target.ChatID, 0), "platform", term.Clean(stored.ChatPlatform, 0), "title", term.Clean(stored.ChatTitle, 0), "kind", kind)
	}
	if target.Platform != "" {
		return cat.T("status.default_platform", "platform", term.Clean(target.Platform, 0))
	}
	return cat.T("status.default_none")
}
