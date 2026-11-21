package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/render"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/text"
	"github.com/appifyhub/agent-cli/internal/token"
)

func (s *rootState) statusCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	backend, _ := statusAddress(cat, strings.TrimRight(s.deps.Build.APIURL, "/"))
	var offline bool
	cmd := withHelp(&cobra.Command{
		Use: "status", Short: cat.T("command.status.short"),
		Long: cat.T("command.status.long", "name", name, "backend", backend),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return s.runStatus(cmd.Context(), offline) },
	}, cat.T("command.status.examples", "name", name), cat.T("command.status.environment"), cat.T("command.status.exit_codes"))
	cmd.Flags().BoolVar(&offline, "offline", false, cat.T("command.status.offline"))
	return cmd
}

type statusLocal struct {
	home, address string
	ready         bool
}

func (s *rootState) runStatus(ctx context.Context, offline bool) error {
	local, err := s.localStatus()
	if err != nil {
		return err
	}
	if err := s.statusCheck(ctx, local, offline); err != nil {
		return err
	}
	if !local.ready {
		return &Failure{Code: ExitNotConnected, Silent: true}
	}
	return nil
}

func (s *rootState) localStatus() (statusLocal, error) {
	d, cat := s.deps, s.deps.Text
	home, err := config.ResolveHome(d.Getenv, d.UserHomeDir, d.Build.Name)
	if err != nil && !errors.Is(err, config.ErrHomeUnavailable) {
		return statusLocal{}, s.failure(err)
	}
	var file config.File
	if err == nil {
		file, err = config.Load(config.Path(home))
		if err != nil {
			return statusLocal{}, s.failure(err)
		}
	}
	raw := d.Getenv(config.EnvToken)
	var claims token.Claims
	if raw != "" {
		claims, err = token.Parse(raw)
		if err != nil {
			return statusLocal{}, s.failure(credentials.ErrInvalidEnvToken)
		}
	}
	mode := credentials.Mode(file.Token.Store)
	connected := mode.Valid()
	path := filepath.Join(home, credentials.FileName)
	if raw == "" && connected && mode != credentials.ModeKeychain {
		if _, err := os.Stat(path); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return statusLocal{}, s.failure(&credentials.StoreError{Op: credentials.OpRead, Mode: mode, Err: err})
			}
			connected = false
		}
	}
	address, _ := config.APIURL(d.Getenv, file, d.Build.APIURL)
	apiURL, valid := statusAddress(cat, address)
	if !valid {
		address = ""
	}
	local := statusLocal{home: home, address: address}
	var b strings.Builder
	var hint string
	if raw == "" && !connected {
		fmt.Fprintln(&b, cat.T("status.not_connected", "name", d.Build.Name))
	} else {
		target, err := config.DefaultTarget(d.Getenv, file)
		if err != nil {
			return statusLocal{}, s.failure(err)
		}
		location, expires := cat.T("status.location_environment"), claims.ExpiresAt
		locked := raw == "" && mode == credentials.ModePassphrase
		if raw == "" {
			expires = file.Token.ExpiresAt
		}
		now := d.Now().In(d.Location)
		expired := !expires.IsZero() && !expires.After(now)
		local.ready = !expired && !locked
		if raw != "" {
			fmt.Fprintln(&b, cat.T("status.connected_environment"))
		} else {
			user := term.Clean(file.Token.UserName, 0)
			hostname := term.Clean(file.Token.Hostname, 0)
			if local.ready {
				heading := cat.T("status.connected_ready", "user", user, "hostname", hostname)
				fmt.Fprintln(&b, d.styled(term.Out, term.Success, heading))
			} else {
				fmt.Fprintln(&b, cat.T("status.connected", "user", user, "hostname", hostname))
			}
			statusRow(&b, cat.T("status.label_device"), term.Clean(file.DeviceID, 0))
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
		statusRow(&b, cat.T("status.label_storage"), location)
		statusRow(&b, cat.T("status.label_expiry"), s.statusExpiry(expires, now))
		label, value := s.statusDefault(target, file.Default)
		statusRow(&b, label, value)
		if expired {
			hint = cat.T("status.reconnect", "name", d.Build.Name)
		} else if locked {
			hint = cat.T("status.unlock", "name", d.Build.Name)
		}
	}
	statusRow(&b, cat.T("status.label_backend"), apiURL)
	statusRow(&b, cat.T("status.label_cli_local"), term.Clean(d.Build.Version, 0))
	if hint != "" {
		fmt.Fprintln(&b, hint)
	}
	_, err = io.WriteString(d.Stdout, b.String())
	return local, err
}

func (s *rootState) statusCheck(ctx context.Context, local statusLocal, offline bool) error {
	d, cat := s.deps, s.deps.Text
	var b, diagnostics strings.Builder
	var checked time.Time
	if offline {
		if local.home != "" && local.address != "" {
			var err error
			checked, err = config.LoadStatusCheck(local.home, local.address)
			if err != nil {
				return s.failure(err)
			}
		}
	} else {
		if err := ctx.Err(); err != nil {
			return err
		}
		var result api.Status
		var elapsed time.Duration
		if local.address != "" {
			client := s.clientWithDiagnostics(local.address, &diagnostics)
			lookup, cancel := context.WithTimeout(ctx, 3*time.Second)
			start := d.Now()
			result, _ = client.Status(lookup)
			checked = d.Now()
			cancel()
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := config.SaveStatusCheck(local.home, local.address, checked); err != nil {
				return s.failure(err)
			}
			elapsed = checked.Sub(start)
		}
		b.WriteByte('\n')
		unavailable := cat.T("status.unavailable")
		healthStyle := term.Warning
		if result.Status == "ok" {
			healthStyle = term.Success
		}
		s.statusStyledRow(&b, cat.T("status.label_status"), cmp.Or(term.Clean(result.Status, 200), unavailable), healthStyle)
		statusRow(&b, cat.T("status.label_version"), cmp.Or(term.Clean(result.Version, 200), unavailable))
		latest := term.Clean(result.CLIVersion, 200)
		statusRow(&b, cat.T("status.label_cli_latest"), cmp.Or(latest, unavailable))
		localVersion := "v" + strings.TrimPrefix(d.Build.Version, "v")
		remoteVersion := "v" + strings.TrimPrefix(result.CLIVersion, "v")
		if semver.IsValid(localVersion) && semver.IsValid(remoteVersion) {
			label := cat.T("status.label_update")
			switch semver.Compare(remoteVersion, localVersion) {
			case 0:
				s.statusStyledRow(&b, label, cat.T("status.latest"), term.Success)
			case 1:
				s.statusStyledRow(&b, label, cat.T("status.update_available", "url", "https://github.com/appifyhub/agent-cli/releases"), term.Warning)
			}
		}
		if result.Status != "" {
			statusRow(&b, cat.T("status.label_response"), cat.T("status.response", "ms", strconv.FormatInt(elapsed.Milliseconds(), 10)))
		}
	}
	when := cat.T("status.never_checked")
	if !checked.IsZero() {
		when = render.LocalTime(checked, d.Location)
	}
	statusRow(&b, cat.T("status.label_checked"), when)
	if _, err := io.WriteString(d.Stdout, b.String()); err != nil {
		return err
	}
	if diagnostics.Len() != 0 {
		_, err := io.WriteString(d.Stderr, diagnostics.String())
		return err
	}
	return nil
}

func (s *rootState) statusExpiry(expires, now time.Time) string {
	cat := s.deps.Text
	if expires.IsZero() {
		return cat.T("status.expiry_unknown")
	}
	when, relative := render.LocalTime(expires, s.deps.Location), render.RelativeTime(expires, now, cat)
	if !expires.After(now) {
		return cat.T("status.token_expired", "time", when, "relative", relative)
	}
	return cat.T("status.token_expires", "time", when, "relative", relative)
}

func (s *rootState) statusDefault(target config.Target, stored config.Default) (label, value string) {
	d, cat := s.deps, s.deps.Text
	chat, platform := cat.T("status.label_default_chat"), cat.T("status.label_default_platform")
	if d.Getenv(config.EnvDefaultChat) != "" {
		return chat, cat.T("status.default_environment_chat", "id", term.Clean(target.ChatID, 0))
	}
	if d.Getenv(config.EnvDefaultPlatform) != "" {
		return platform, cat.T("status.default_environment_platform", "platform", term.Clean(target.Platform, 0))
	}
	if target.ChatID != "" {
		kind := cat.T("status.group")
		if stored.ChatPrivate {
			kind = cat.T("status.private")
		}
		return chat, cat.T("status.default_chat", "id", term.Clean(target.ChatID, 0), "platform", term.Clean(stored.ChatPlatform, 0), "title", term.Clean(stored.ChatTitle, 0), "kind", kind)
	}
	if target.Platform != "" {
		return platform, term.Clean(target.Platform, 0)
	}
	return chat, cat.T("status.default_none")
}

func statusRow(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "  %-16s  %s\n", label, value)
}

func (s *rootState) statusStyledRow(b *strings.Builder, label, value string, style term.Style) {
	b.WriteString(s.deps.styled(term.Out, style, fmt.Sprintf("  %-16s  %s", label, value)))
	b.WriteByte('\n')
}

func statusAddress(cat *text.Catalog, address string) (display string, valid bool) {
	if address == "" {
		return cat.T("env.not_set"), false
	}
	if config.ValidateAPIURL(address) != nil {
		return cat.T("status.invalid_address"), false
	}
	_, host, _ := strings.Cut(address, "://")
	return term.Clean(host, 0), true
}
