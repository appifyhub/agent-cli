package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/cli"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

const (
	connectKey    = "K7Q2-M9XA-P3RT"
	connectDevice = "13892199-ba28-8c7e-a6ab-399e6bfddc6f"
	connectSecret = "header.payload.signature"
)

var connectExpiry = time.Date(2027, 2, 14, 9, 21, 0, 0, time.UTC)

type connectRig struct {
	deps     cli.Deps
	backend  *testkit.Backend
	keychain *testkit.FakeKeychain
	terminal *testkit.FakeTerm
	vars     map[string]string
	home     string
}

func newConnectRig(t *testing.T, goos string) *connectRig {
	t.Helper()
	userHome := t.TempDir()
	b := testkit.NewBackend(t)
	k := &testkit.FakeKeychain{}
	terminal := &testkit.FakeTerm{}
	vars := map[string]string{config.EnvHome: filepath.Join(userHome, ".agent-cli"), config.EnvAPIURL: b.URL()}
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	d := testkit.NewDeps(testkit.WithPlatform(goos, "amd64"), testkit.WithEnv(vars), testkit.WithTerm(terminal), testkit.WithHTTP(b.Client().Transport))
	d.UserHomeDir = func() (string, error) { return userHome, nil }
	d.Keychain = k
	d.Location = loc
	d.Now = func() time.Time { return time.Date(2026, 11, 14, 10, 21, 0, 0, loc) }
	b.Handle("POST", "/cli/connect", testkit.ConnectOK(connectSecret, connectExpiry, "u1", "Morpheus", 500))
	return &connectRig{deps: d, backend: b, keychain: k, terminal: terminal, vars: vars, home: vars[config.EnvHome]}
}

func (r *connectRig) run(t *testing.T, args ...string) testkit.Result {
	t.Helper()
	result := testkit.Run(t, r.deps, append([]string{"connect"}, args...)...)
	for _, secret := range []string{connectKey, connectSecret, "secretphrase"} {
		testkit.AssertNoSecret(t, secret, result.Stdout, result.Stderr)
	}
	return result
}

func (r *connectRig) file(t *testing.T) config.File {
	t.Helper()
	f, err := config.Load(config.Path(r.home))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (r *connectRig) save(t *testing.T, f config.File) {
	t.Helper()
	if err := config.EnsureHome(r.home); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(config.Path(r.home), f); err != nil {
		t.Fatal(err)
	}
}

func (r *connectRig) request(t *testing.T) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(r.backend.Last(t).Body), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestConnectKeychainOutputs(t *testing.T) {
	for goos, store := range map[string]string{"darwin": "macOS Keychain", "windows": "Windows Credential Manager", "linux": "system keyring (Secret Service)"} {
		t.Run(goos, func(t *testing.T) {
			r := newConnectRig(t, goos)
			result := r.run(t, connectKey, "--ttl", "3mo")
			want := "✓ Connected as Morpheus (morpheus-mbp)\n  Token stored in: " + store + "\n  Expires: 2027-02-14 10:21 (in 3 months)\n  A new connect key was generated for your profile.\n"
			if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
				t.Fatalf("got %+v; want %q", result, want)
			}
			f := r.file(t)
			if f.Token.UserID != "u1" || f.Token.Store != "keychain" || f.Token.Hostname != "morpheus-mbp" || f.APIURL != r.backend.URL() || !f.Token.ExpiresAt.Equal(connectExpiry) {
				t.Errorf("config %+v", f)
			}
			if !config.ValidDeviceID(f.DeviceID) {
				t.Errorf("invalid device id %q", f.DeviceID)
			}
			body := r.request(t)
			if body["device_id"] != f.DeviceID || body["os"] != goos || body["ttl_as_text"] != "3mo" {
				t.Errorf("body %v", body)
			}
			secret, ok := r.keychain.Secret(r.deps.Build.Name, f.DeviceID)
			if !ok || secret != connectSecret {
				t.Error("token not in expected keychain account")
			}
			if _, err := os.Stat(filepath.Join(r.home, "credentials")); !os.IsNotExist(err) {
				t.Errorf("unexpected credentials file: %v", err)
			}
		})
	}
}

func TestConnectFileStores(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, mode := range []string{"unsafe", "passphrase"} {
			t.Run(goos+"/"+mode, func(t *testing.T) {
				r := newConnectRig(t, goos)
				r.terminal.TTYIn = true
				r.terminal.Secrets = []string{"secretphrase", "secretphrase"}
				result := r.run(t, connectKey, "--store", mode)
				if result.Code != 0 || result.Stderr != "" {
					t.Fatalf("got %+v", result)
				}
				path := "~/.agent-cli/credentials"
				hint := "  To unlock once per terminal session: eval \"$(agent-cli unlock)\"\n"
				if goos == "windows" {
					path = `~\.agent-cli\credentials`
					hint = "  To unlock once per terminal session: agent-cli unlock --shell pwsh | Invoke-Expression\n"
				}
				suffix := " (NOT encrypted)"
				if mode == "passphrase" {
					suffix = " (encrypted)"
				}
				want := "✓ Connected as Morpheus (morpheus-mbp)\n  Token stored in: " + path + suffix + "\n  Expires: 2027-02-14 10:21 (in 3 months)\n  A new connect key was generated for your profile.\n"
				if mode == "passphrase" {
					want += hint
					if len(r.terminal.Prompts) != 2 || r.terminal.Prompts[0] != "Choose a passphrase: " || r.terminal.Prompts[1] != "Repeat passphrase: " {
						t.Errorf("prompts %q", r.terminal.Prompts)
					}
				} else if len(r.terminal.Prompts) != 0 {
					t.Error("unsafe prompted")
				}
				if result.Stdout != want {
					t.Errorf("got %q, want %q", result.Stdout, want)
				}
				raw, err := os.ReadFile(filepath.Join(r.home, "credentials"))
				if err != nil {
					t.Fatal(err)
				}
				if mode == "unsafe" && string(raw) != connectSecret+"\n" {
					t.Error("wrong plain credentials")
				}
				if mode == "passphrase" {
					testkit.AssertNoSecret(t, connectSecret, string(raw))
					if !strings.HasPrefix(string(raw), "-----BEGIN AGE ENCRYPTED FILE-----") {
						t.Error("not encrypted")
					}
				}
				configRaw, err := os.ReadFile(config.Path(r.home))
				if err != nil {
					t.Fatal(err)
				}
				testkit.AssertNoSecret(t, connectSecret, string(configRaw))
				testkit.AssertNoSecret(t, "secretphrase", string(configRaw))
			})
		}
	}
}

func TestConnectNormalizationAndTTL(t *testing.T) {
	for _, ttl := range []string{"absent", "1h30m", ""} {
		r := newConnectRig(t, "linux")
		args := []string{" k7q2-m9xa-p3rt ", "--store", "unsafe"}
		if ttl != "absent" {
			args = append(args, "--ttl", ttl)
		}
		result := r.run(t, args...)
		if result.Code != 0 {
			t.Fatal(result)
		}
		body := r.request(t)
		if body["connect_key"] != connectKey {
			t.Errorf("not normalized: %v", body)
		}
		value, present := body["ttl_as_text"]
		if ttl == "absent" && present || ttl != "absent" && (!present || value != ttl) {
			t.Errorf("ttl not forwarded: %v", body)
		}
		if r.backend.Last(t).Header.Get("Authorization") != "" {
			t.Error("connect sent a token")
		}
	}
}

func TestConnectHostnameFailureFallback(t *testing.T) {
	for _, hostname := range []string{"", "💻"} {
		r := newConnectRig(t, "linux")
		r.save(t, config.File{DeviceID: connectDevice})
		r.deps.Machine = func() config.Machine { return config.Machine{Hostname: hostname} }
		result := r.run(t, connectKey, "--store", "unsafe")
		if result.Code != 0 || r.request(t)["hostname"] != "device-13892199ba28" || !strings.Contains(result.Stdout, "(device-13892199ba28)") {
			t.Errorf("got %+v", result)
		}
	}
}

func TestConnectUsageAndPreflightFailures(t *testing.T) {
	for _, c := range []struct {
		name  string
		args  []string
		setup func(*connectRig)
		code  int
		want  string
	}{
		{"missing", nil, nil, 2, "missing connect key; usage:"},
		{"extra", []string{connectKey, "extra"}, nil, 2, "error:"},
		{"blank", []string{" "}, nil, 2, "missing connect key"},
		{"invalid store", []string{connectKey, "--store", "vault"}, nil, 2, "invalid --store"},
		{"no store", []string{connectKey}, func(r *connectRig) { r.keychain.SetErr = errors.New("unavailable") }, 2, "no keychain is available and no passphrase"},
		{"explicit keychain", []string{connectKey, "--store", "keychain"}, func(r *connectRig) { r.keychain.SetErr = errors.New("unavailable") }, 2, "no keychain is available on this machine"},
		{"explicit passphrase", []string{connectKey, "--store", "passphrase"}, nil, 2, "a passphrase is required"},
		{"mismatch", []string{connectKey, "--store", "passphrase"}, func(r *connectRig) { r.terminal.TTYIn = true; r.terminal.Secrets = []string{"a", "b"} }, 2, "passphrases do not match"},
		{"empty passphrase", []string{connectKey, "--store", "passphrase"}, func(r *connectRig) { r.terminal.TTYIn = true; r.terminal.Secrets = []string{""} }, 2, "passphrase cannot be empty"},
		{"unwritable home", []string{connectKey}, func(r *connectRig) {
			if err := os.WriteFile(r.home, []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, 1, "cannot use the home directory"},
		{"insecure URL", []string{connectKey}, func(r *connectRig) { r.vars[config.EnvAPIURL] = "http://remote.example.test" }, 2, "invalid API URL"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newConnectRig(t, "linux")
			if c.setup != nil {
				c.setup(r)
			}
			result := r.run(t, c.args...)
			if result.Code != c.code || result.Stdout != "" || !strings.Contains(result.Stderr, c.want) || r.backend.Count() != 0 {
				t.Errorf("got %+v (%d requests)", result, r.backend.Count())
			}
		})
	}
	for _, spelling := range [][]string{{"--store", ""}, {"--store="}} {
		for _, connected := range []bool{false, true} {
			t.Run(strings.Join(spelling, " ")+"/connected="+strconv.FormatBool(connected), func(t *testing.T) {
				r := newConnectRig(t, "linux")
				if connected {
					r = newChatsRig(t, "linux")
				}
				before := statusSnapshot(t, r.home)
				secrets := r.keychain.Len()
				got := r.run(t, append([]string{connectKey}, spelling...)...)
				if got.Code != 2 || got.Stdout != "" || got.Stderr != "error: invalid --store \"\": use keychain, passphrase, or unsafe\n" {
					t.Fatal(got)
				}
				if len(r.terminal.Prompts) != 0 || len(r.keychain.Calls) != 0 || r.backend.Count() != 0 || statusSnapshot(t, r.home) != before || r.keychain.Len() != secrets {
					t.Error("invalid empty store accessed or changed connection state")
				}
				if secret, exists := r.keychain.Secret(r.deps.Build.Name, connectDevice); exists != connected || connected && secret != connectSecret {
					t.Error("invalid empty store changed the stored credential")
				}
				if !connected {
					if _, err := os.Stat(r.home); !os.IsNotExist(err) {
						t.Error("invalid empty store created the state directory")
					}
				}
			})
		}
	}
}

func TestConnectBackendFailuresPreservePreviousConnection(t *testing.T) {
	for _, c := range []struct {
		resp    testkit.Response
		code    int
		message string
	}{
		{testkit.Err(404, 2015, "used"), 1, "invalid connect key"},
		{testkit.Err(401, 4004, "bad"), 1, "invalid connect key"},
		{testkit.Err(403, 3008, "policies"), 1, "invalid connect key"},
		{testkit.Err(400, 1043, "bad ttl"), 2, "invalid TTL \"1h30m\": use a single unit, e.g. 90m"},
		{testkit.Err(400, 1044, "long ttl"), 2, "TTL exceeds the server's maximum token lifetime"},
		{testkit.Text(429, "Too Many Requests"), 6, "rate limited; wait a moment and try again"},
		{testkit.HTML(500), 1, "the server had a problem; try again later"},
		{testkit.Err(422, 1045, "device"), 1, "the server rejected this machine's details; please report this"},
		{testkit.JSON(200, map[string]any{"token": "t"}), 1, "unexpected response from the server"},
		{testkit.Err(422, 1999, connectKey+" secretphrase"), 2, "*** ***"},
		{testkit.Err(422, 1999, strings.Repeat("x", 187)+connectKey), 2, strings.Repeat("x", 187) + "***"},
	} {
		for _, verbose := range []bool{false, true} {
			r := newConnectRig(t, "linux")
			old := config.File{DeviceID: connectDevice, APIURL: "https://previous.example.test", Token: config.Token{Store: "unsafe", UserID: "u1"}, Default: config.Default{Platform: "telegram"}}
			r.save(t, old)
			secretPath := filepath.Join(r.home, "credentials")
			if err := os.WriteFile(secretPath, []byte("old token\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			r.vars[config.EnvPassphrase] = "secretphrase"
			r.backend.Handle("POST", "/cli/connect", c.resp)
			args := []string{connectKey, "--store", "unsafe", "--ttl", "1h30m"}
			if verbose {
				args = append(args, "-v")
			}
			result := r.run(t, args...)
			want := "error: " + c.message + "\n"
			validStderr := result.Stderr == want
			if verbose {
				validStderr = strings.HasPrefix(result.Stderr, "Calling backend…\n") && strings.HasSuffix(result.Stderr, want) && strings.Count(result.Stderr, "error: ") == 1
			}
			if result.Code != c.code || result.Stdout != "" || !validStderr || r.backend.Count() != 1 {
				t.Errorf("response %+v verbose=%v: %+v", c.resp, verbose, result)
			}
			if got := r.file(t); got != old {
				t.Errorf("previous config changed: %+v", got)
			}
			raw, _ := os.ReadFile(secretPath)
			if string(raw) != "old token\n" {
				t.Error("old token was replaced on failure")
			}
			testkit.AssertNoSecret(t, connectKey[:len(connectKey)-1], result.Stdout, result.Stderr)
		}
	}
}

func TestConnectTransportFailure(t *testing.T) {
	r := newConnectRig(t, "linux")
	r.vars[config.EnvAPIURL] = "http://127.0.0.1:1"
	result := r.run(t, connectKey, "--store", "unsafe")
	if result.Code != 1 || result.Stderr != "error: could not reach the server\n" {
		t.Errorf("got %+v", result)
	}
	if r.file(t).Token.Store != "" {
		t.Error("unexpected connection metadata")
	}
}

func TestConnectReconnectAndSwitchStores(t *testing.T) {
	for _, sameUser := range []bool{true, false} {
		r := newConnectRig(t, "linux")
		r.save(t, config.File{DeviceID: connectDevice, Token: config.Token{UserID: "u1"}, Default: config.Default{Platform: "telegram"}})
		if !sameUser {
			r.backend.Handle("POST", "/cli/connect", testkit.ConnectOK(connectSecret, connectExpiry, "u2", "Other", 500))
		}
		result := r.run(t, connectKey, "--store", "unsafe")
		if result.Code != 0 {
			t.Fatal(result)
		}
		if (r.file(t).Default.Platform == "telegram") != sameUser {
			t.Error("wrong reconnect default")
		}
		if r.file(t).DeviceID != connectDevice {
			t.Error("device id changed")
		}
		if result = r.run(t, connectKey, "--store", "keychain"); result.Code != 0 {
			t.Fatal(result)
		}
		if _, err := os.Stat(filepath.Join(r.home, "credentials")); !os.IsNotExist(err) {
			t.Error("stale unsafe secret left")
		}
		r.vars[config.EnvPassphrase] = "secretphrase"
		if result = r.run(t, connectKey, "--store", "passphrase"); result.Code != 0 {
			t.Fatal(result)
		}
		if r.keychain.Len() != 0 {
			t.Error("stale keychain secret left")
		}
		if len(r.terminal.Prompts) != 0 {
			t.Error("environment passphrase prompted")
		}
	}
}

func TestConnectPersistenceFailures(t *testing.T) {
	for _, failConfig := range []bool{false, true} {
		t.Run(map[bool]string{false: "secret", true: "config"}[failConfig], func(t *testing.T) {
			r := newConnectRig(t, "linux")
			r.backend.HandleFunc("POST", "/cli/connect", func(testkit.Request) testkit.Response {
				if failConfig {
					if err := os.Mkdir(config.Path(r.home), 0o700); err != nil {
						t.Error(err)
					}
				} else {
					r.keychain.SetErr = errors.New("store unavailable")
				}
				return testkit.ConnectOK(connectSecret, connectExpiry, "u1", "Morpheus", 500)
			})
			result := r.run(t, connectKey, "--store", "keychain")
			if result.Code != 1 || result.Stdout != "" || !strings.HasPrefix(result.Stderr, "error: ") {
				t.Errorf("got %+v", result)
			}
			if !failConfig && result.Stderr != "error: connected, but the token could not be stored: store unavailable; the connect key was used, get a new one from the agent\n" {
				t.Errorf("got %q", result.Stderr)
			}
			if r.keychain.Len() != 0 {
				t.Error("new secret was not rolled back")
			}
		})
	}
}

func TestConnectLowCreditAndVerbose(t *testing.T) {
	for _, c := range []struct {
		balance any
		warning string
	}{
		{42.37, "42.37 credits left;"},
		{99.999, "99.99 credits left;"},
		{-3.5, "-3.50 credits left;"},
		{0, "0.00 credits left;"},
		{100, ""},
		{500, ""},
		{nil, ""},
		{"lots", ""},
	} {
		r := newConnectRig(t, "linux")
		r.backend.Handle("POST", "/cli/connect", testkit.ConnectOK(connectSecret, connectExpiry, "u1", "Morpheus", c.balance))
		result := r.run(t, connectKey, "--store", "unsafe", "-v")
		if result.Code != 0 || !strings.HasPrefix(result.Stderr, "Calling backend…\n") {
			t.Errorf("got %+v", result)
		}
		if c.warning == "" && result.Stderr != "Calling backend…\n" || c.warning != "" && !strings.Contains(result.Stderr, c.warning) {
			t.Errorf("wrong warning %q", result.Stderr)
		}
		if !strings.HasPrefix(result.Stdout, "✓ Connected as Morpheus") {
			t.Error("warning changed stdout")
		}
	}
}

func TestConnectCompletionAndHelp(t *testing.T) {
	r := newConnectRig(t, "linux")
	result := testkit.Run(t, r.deps, "__complete", "connect", "--store", "")
	for _, mode := range []string{"keychain", "passphrase", "unsafe"} {
		if !strings.Contains(result.Stdout, mode+"\n") {
			t.Errorf("completion %q lacks %s", result.Stdout, mode)
		}
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		r = newConnectRig(t, goos)
		result = testkit.Run(t, r.deps, "connect", "--help")
		golden := "unix"
		if goos == "windows" {
			golden = "windows"
		}
		testkit.Golden(t, "connect_help_"+golden, result.Stdout)
		if result.Code != 0 || result.Stderr != "" || r.backend.Count() != 0 {
			t.Error(result)
		}
		if _, err := os.Stat(r.home); !os.IsNotExist(err) {
			t.Error("help created home")
		}
	}
}

func newRevokeRig(t *testing.T, mode string) *connectRig {
	t.Helper()
	r := newConnectRig(t, "linux")
	r.vars[config.EnvPassphrase] = "secretphrase"
	if result := r.run(t, connectKey, "--store", mode); result.Code != 0 {
		t.Fatal(result)
	}
	delete(r.vars, config.EnvPassphrase)
	f := r.file(t)
	f.Default = config.Default{ChatID: "c9"}
	r.save(t, f)
	r.keychain.Calls = nil
	r.backend.Handle("DELETE", "/cli/user/u1/devices", testkit.JSON(200, map[string]string{"status": "OK"}))
	return r
}

func TestRevokeConfirmation(t *testing.T) {
	for _, answer := range []string{"y", "Y", "yes", "YeS", "", "n", "yes please", "eof"} {
		t.Run(answer, func(t *testing.T) {
			r := newRevokeRig(t, "keychain")
			before := r.file(t)
			r.terminal.TTYIn = true
			if answer != "eof" {
				r.terminal.Lines = []string{answer}
			}
			got := testkit.Run(t, r.deps, "revoke-all")
			accepted := strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
			if accepted {
				if got.Code != 0 || got.Stdout != "✓ Access revoked for all CLIs. Local token removed.\n" || r.backend.Count() != 2 {
					t.Fatal(got)
				}
			} else if got.Code != 1 || got.Stdout != "" || got.Stderr != "Aborted.\n" || r.backend.Count() != 1 || r.file(t) != before || len(r.keychain.Calls) != 0 {
				t.Fatalf("declined: %+v", got)
			}
			if len(r.terminal.Prompts) != 1 || r.terminal.Prompts[0] != "This revokes access for ALL CLIs on all your machines. Continue? [y/N] " {
				t.Errorf("prompts: %v", r.terminal.Prompts)
			}
		})
	}
}

func TestRevokePreconditionsAndUnlockOrder(t *testing.T) {
	t.Run("not connected before confirmation", func(t *testing.T) {
		r := newConnectRig(t, "linux")
		got := testkit.Run(t, r.deps, "revoke-all")
		if got.Code != 3 || got.Stderr != "This CLI is not connected. Run `agent-cli connect <KEY>`.\n" || got.Stdout != "" || len(r.terminal.Prompts) != 0 || r.backend.Count() != 0 {
			t.Fatal(got)
		}
		if _, err := os.Stat(r.home); !os.IsNotExist(err) {
			t.Error("created state without a connection")
		}
	})
	t.Run("nonterminal requires yes", func(t *testing.T) {
		r := newRevokeRig(t, "keychain")
		got := testkit.Run(t, r.deps, "revoke-all")
		if got.Code != 2 || got.Stderr != "error: refusing to revoke without confirmation; pass --yes\n" || got.Stdout != "" || len(r.keychain.Calls) != 0 || len(r.terminal.Prompts) != 0 || r.backend.Count() != 1 {
			t.Fatal(got)
		}
	})
	t.Run("locked without terminal", func(t *testing.T) {
		r := newRevokeRig(t, "passphrase")
		got := testkit.Run(t, r.deps, "revoke-all", "--yes")
		if got.Code != 3 || !strings.Contains(got.Stderr, "token is locked;") || got.Stdout != "" || len(r.terminal.Prompts) != 0 || r.backend.Count() != 1 {
			t.Fatal(got)
		}
	})
	t.Run("confirmation before passphrase", func(t *testing.T) {
		r := newRevokeRig(t, "passphrase")
		r.terminal.TTYIn = true
		r.terminal.Lines = []string{"yes"}
		r.terminal.Secrets = []string{"secretphrase"}
		got := testkit.Run(t, r.deps, "revoke-all")
		if got.Code != 0 || len(r.terminal.Prompts) != 2 || r.terminal.Prompts[1] != "Passphrase: " {
			t.Fatalf("%+v, prompts %v", got, r.terminal.Prompts)
		}
		if _, err := os.Stat(filepath.Join(r.home, credentials.FileName)); !os.IsNotExist(err) {
			t.Error("credentials survived revocation")
		}
	})
}

func TestRevokeOutcomes(t *testing.T) {
	for _, c := range []struct {
		name           string
		response       testkit.Response
		code           int
		stdout, stderr string
		remove         bool
	}{
		{"success", testkit.JSON(200, map[string]string{"status": "OK"}), 0, "✓ Access revoked for all CLIs. Local token removed.\n", "", true},
		{"revoked", testkit.Err(401, 4005, "revoked"), 0, "✓ Already revoked. Local token removed.\n", "", true},
		{"forbidden", testkit.Err(403, 3002, "forbidden"), 0, "✓ Already revoked. Local token removed.\n", "", true},
		{"merged", testkit.Err(404, 2001, "missing"), 0, "✓ Already revoked. Local token removed.\n", "", true},
		{"server", testkit.HTML(503), 1, "", "error: could not reach the server; local token kept\n", false},
		{"rate limit", testkit.Text(429, "limited"), 6, "", "error: rate limited; wait a moment and try again\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRevokeRig(t, "keychain")
			before := r.file(t)
			r.backend.Handle("DELETE", "/cli/user/u1/devices", c.response)
			got := testkit.Run(t, r.deps, "revoke-all", "--yes")
			if got.Code != c.code || got.Stdout != c.stdout || got.Stderr != c.stderr || len(r.terminal.Prompts) != 0 {
				t.Fatal(got)
			}
			request := r.backend.Last(t)
			if request.Method != http.MethodDelete || request.Path != "/cli/user/u1/devices" || request.Header.Get("Authorization") != "Bearer "+connectSecret {
				t.Errorf("wrong revoke request: %s %s", request.Method, request.Path)
			}
			want := before
			if c.remove {
				want.Token = config.Token{}
			}
			if r.file(t) != want || (r.keychain.Len() == 0) != c.remove {
				t.Error("incorrect local cleanup")
			}
			testkit.AssertNoSecret(t, connectSecret, got.Stdout, got.Stderr)
		})
	}
	t.Run("network", func(t *testing.T) {
		r := newRevokeRig(t, "keychain")
		before := r.file(t)
		r.vars[config.EnvAPIURL] = "http://127.0.0.1:1"
		got := testkit.Run(t, r.deps, "revoke-all", "--yes")
		if got.Code != 1 || got.Stderr != "error: could not reach the server; local token kept\n" || got.Stdout != "" || r.file(t) != before || r.keychain.Len() != 1 {
			t.Fatal(got)
		}
	})
	t.Run("cleanup failure", func(t *testing.T) {
		r := newRevokeRig(t, "keychain")
		before := r.file(t)
		r.keychain.DelErr = errors.New("access denied")
		got := testkit.Run(t, r.deps, "revoke-all", "--yes")
		if got.Code != 1 || got.Stdout != "" || !strings.Contains(got.Stderr, "could not remove the token") || r.file(t) != before {
			t.Fatal(got)
		}
	})
	for _, oversized := range []bool{false, true} {
		for _, status := range []int{200, 401, 429} {
			r := newChatsRig(t, "linux")
			before := r.file(t)
			body := setUnusableCommandResponse(r, status, oversized)
			got := testkit.Run(t, r.deps, "revoke-all", "--yes")
			code, out, stderr := 0, "✓ Access revoked for all CLIs. Local token removed.\n", ""
			switch status {
			case 401:
				out = "✓ Already revoked. Local token removed.\n"
			case 429:
				code, out, stderr = 6, "", "error: rate limited; wait a moment and try again\n"
			}
			remove := status != 429
			if remove {
				before.Token = config.Token{}
			}
			if got.Code != code || got.Stdout != out || got.Stderr != stderr || body.requests != 1 || body.closes != 1 || r.file(t) != before || (r.keychain.Len() == 0) != remove {
				t.Fatalf("status=%d oversized=%v: %+v", status, oversized, got)
			}
			testkit.AssertNoSecret(t, connectSecret, got.Stdout, got.Stderr)
		}
	}
}

func TestRevokeEnvironmentToken(t *testing.T) {
	for _, stored := range []bool{false, true} {
		for _, rejected := range []bool{false, true} {
			t.Run(strings.Join([]string{strconv.FormatBool(stored), strconv.FormatBool(rejected)}, "/"), func(t *testing.T) {
				r := newConnectRig(t, "linux")
				if stored {
					r = newRevokeRig(t, "keychain")
				}
				before := r.file(t)
				secret := testkit.NewToken(testkit.TokenClaims{Sub: "environment-user", Exp: connectExpiry})
				r.vars[config.EnvToken] = secret
				response := testkit.JSON(200, map[string]string{"status": "OK"})
				if rejected {
					response = testkit.Err(401, 4004, "rejected")
				}
				r.backend.Handle("DELETE", "/cli/user/environment-user/devices", response)
				got := testkit.Run(t, r.deps, "revoke-all", "--yes", "-v")
				request := r.backend.Last(t)
				if request.Path != "/cli/user/environment-user/devices" || request.Header.Get("Authorization") != "Bearer "+secret {
					t.Error("environment identity not used")
				}
				if rejected {
					if got.Code != 3 || !strings.HasSuffix(got.Stderr, "error: the token in THE_AGENT_CLI_TOKEN was rejected by the server (expired or revoked)\n") || got.Stdout != "" || r.file(t) != before {
						t.Fatal(got)
					}
				} else {
					want := "✓ Access revoked for all CLIs.\n"
					if stored {
						want = "✓ Access revoked for all CLIs. Local token removed.\n"
					}
					if got.Code != 0 || got.Stdout != want || r.keychain.Len() != 0 || r.file(t).Token != (config.Token{}) {
						t.Fatal(got)
					}
				}
				if !strings.HasPrefix(got.Stderr, "Calling backend…\n") || len(r.terminal.Prompts) != 0 {
					t.Fatal(got)
				}
				testkit.AssertNoSecret(t, secret, got.Stdout, got.Stderr)
			})
		}
	}
}

func statusRig(t *testing.T, goos, mode string) *connectRig {
	t.Helper()
	r := newConnectRig(t, goos)
	r.save(t, config.File{
		DeviceID: "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b",
		APIURL:   "https://stored.example",
		Token:    config.Token{Store: mode, UserID: "u1", UserName: "Example User", Hostname: "workstation", ExpiresAt: connectExpiry},
		Default:  config.Default{ChatID: "3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7", ChatPlatform: "telegram", ChatTitle: "Example Chat", ChatPrivate: true},
	})
	if mode != "keychain" {
		if err := os.WriteFile(filepath.Join(r.home, credentials.FileName), []byte("not-a-token\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r.keychain.GetErr = errors.New("status must not read keychain")
	return r
}

func statusSnapshot(t *testing.T, home string, names ...string) string {
	t.Helper()
	entries, err := os.ReadDir(home)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	var snapshot strings.Builder
	for _, entry := range entries {
		if len(names) != 0 && !slices.Contains(names, entry.Name()) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(home, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		snapshot.WriteString(entry.Name())
		snapshot.WriteByte(0)
		snapshot.Write(body)
	}
	return snapshot.String()
}

func readinessTransport(t *testing.T, backend *testkit.Backend) http.RoundTripper {
	t.Helper()
	return commandRoundTrip(func(request *http.Request) (*http.Response, error) {
		if backend == nil || request.URL.Scheme+"://"+request.URL.Host != backend.URL() {
			t.Error("status attempted an unexpected backend request")
			return nil, errors.New("unexpected status request")
		}
		return backend.Client().Transport.RoundTrip(request)
	})
}

func runStatus(t *testing.T, r *connectRig, backend *testkit.Backend, args ...string) testkit.Result {
	t.Helper()
	var names []string
	if backend != nil {
		names = []string{config.FileName, credentials.FileName}
	}
	before, calls, prompts := statusSnapshot(t, r.home, names...), len(r.keychain.Calls), len(r.terminal.Prompts)
	hits := 0
	if backend != nil {
		hits = backend.Count()
	}
	deps := r.deps
	deps.HTTP = readinessTransport(t, backend)
	got := testkit.Run(t, deps, append([]string{"status"}, args...)...)
	if statusSnapshot(t, r.home, names...) != before || len(r.keychain.Calls) != calls || len(r.terminal.Prompts) != prompts {
		t.Error("status accessed credentials, prompted or changed files")
	}
	if backend != nil {
		requests := backend.Requests()[hits:]
		if len(requests) != 1 {
			t.Errorf("status sent %d requests; want one", len(requests))
		} else {
			request := requests[0]
			if request.Method != http.MethodGet || request.Path != "/cli/status" || request.Header.Get("Authorization") != "" ||
				request.RawQuery != "" || request.Body != "" {
				t.Error("status did not send one public GET /cli/status without query or body")
			}
		}
	}
	for _, secret := range []string{connectKey, connectSecret, "not-a-token", "secretphrase", r.vars[config.EnvToken]} {
		if secret != "" {
			testkit.AssertNoSecret(t, secret, got.Stdout, got.Stderr)
		}
	}
	return got
}

func runOfflineStatus(t *testing.T, r *connectRig) testkit.Result {
	t.Helper()
	return runStatus(t, r, nil, "--offline", "--verbose")
}

func runHealthyStatus(t *testing.T, r *connectRig, backend *testkit.Backend, local testkit.Result) testkit.Result {
	t.Helper()
	latest := "2.0.0"
	if latest == r.deps.Build.Version {
		latest = "1.0.0"
	}
	backend.Handle(http.MethodGet, "/cli/status", testkit.StatusOK("9.8.7", latest))
	got := runStatus(t, r, backend)
	prefix, _, _ := strings.Cut(local.Stdout, "  Last checked")
	report, found := strings.CutPrefix(got.Stdout, prefix)
	if got.Code != local.Code || got.Stderr != "" || !found || !strings.HasPrefix(report, "\n  Backend status    ok\n") ||
		!strings.Contains(report, "  Latest CLI        "+latest+"\n"+statusUpdate) {
		t.Fatalf("online status = %#v; want the local report and exit %d, plus healthy backend and version comparison", got, local.Code)
	}
	return got
}

func TestStatusConnectedSample(t *testing.T) {
	r := statusRig(t, "darwin", "keychain")
	r.vars[config.EnvAPIURL] = "https://agent.cloud.appifyhub.com"
	r.deps.Build.APIURL = r.vars[config.EnvAPIURL]
	r.deps.Build.Version = "1.2.3"
	got := runOfflineStatus(t, r)
	want := "✓ Connected as Example User (workstation)\n" +
		"  Device ID         6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b\n" +
		"  Token storage     macOS Keychain\n  Token expiry      2027-02-14 10:21 (in 3 months)\n" +
		"  Default chat      3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7 (telegram · Example Chat, private)\n" +
		"  Backend           agent.cloud.appifyhub.com\n  Local CLI         1.2.3\n" + statusNever
	if got.Code != 0 || got.Stdout != want || got.Stderr != "" {
		t.Fatal(got)
	}
	testkit.Golden(t, "status_connected", got.Stdout)
	help := testkit.Run(t, r.deps, "status", "--help")
	if !strings.Contains(help.Stdout, strings.TrimSuffix(got.Stdout, "\n")) {
		t.Error("sample in help differs from actual status output")
	}
	r.deps.Term = &testkit.FakeTerm{TTYOut: true, ANSIOK: true}
	colored := runOfflineStatus(t, r)
	heading, rest, _ := strings.Cut(want, "\n")
	if colored.Code != 0 || colored.Stdout != term.Paint(heading, term.Success, true)+"\n"+rest || colored.Stderr != "" {
		t.Fatal(colored)
	}
}

func TestStatusFreshAndMissingCredentials(t *testing.T) {
	for _, mode := range []string{"", "unsafe", "passphrase"} {
		t.Run(mode, func(t *testing.T) {
			r := newConnectRig(t, "linux")
			if mode != "" {
				r = statusRig(t, "linux", mode)
				if err := os.Remove(filepath.Join(r.home, credentials.FileName)); err != nil {
					t.Fatal(err)
				}
			}
			got := runOfflineStatus(t, r)
			want := "Not connected.\nGet a connect key from the agent's chat settings, then run `agent-cli connect <KEY>`.\n" +
				"  Backend           " + strings.TrimPrefix(r.backend.URL(), "http://") + "\n  Local CLI         " + r.deps.Build.Version + "\n" + statusNever
			if got.Code != 3 || got.Stderr != "" || got.Stdout != want {
				t.Fatal(got)
			}
			runHealthyStatus(t, r, r.backend, got)
		})
	}
}

func TestStatusStorageAndExpiry(t *testing.T) {
	for _, c := range []struct {
		goos, mode, location string
		code                 int
	}{
		{"darwin", "keychain", "macOS Keychain", 0},
		{"windows", "keychain", "Windows Credential Manager", 0},
		{"linux", "keychain", "system keyring (Secret Service)", 0},
		{"linux", "unsafe", "~/.agent-cli/credentials (NOT encrypted)", 0},
		{"linux", "passphrase", "~/.agent-cli/credentials (encrypted, locked)", 3},
		{"windows", "passphrase", `~\.agent-cli\credentials (encrypted, locked)`, 3},
	} {
		t.Run(c.goos+"/"+c.mode, func(t *testing.T) {
			r := statusRig(t, c.goos, c.mode)
			r.vars[config.EnvPassphrase] = "secretphrase"
			header := "Connected as Example User (workstation)\n  Device ID         6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b\n" +
				"  Token storage     " + c.location + "\n"
			details := "\n  Default chat      3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7 (telegram · Example Chat, private)\n" +
				"  Backend           " + strings.TrimPrefix(r.backend.URL(), "http://") + "\n  Local CLI         " + r.deps.Build.Version + "\n"
			var hint string
			if c.mode == "passphrase" {
				hint = "  To unlock: eval \"$(agent-cli unlock)\"\n"
				if c.goos == "windows" {
					hint = "  To unlock: agent-cli unlock --shell pwsh | Invoke-Expression\n"
				}
			}
			readyHeader := header
			if c.code == 0 {
				readyHeader = "✓ " + header
			}
			got := runOfflineStatus(t, r)
			if want := readyHeader + "  Token expiry      2027-02-14 10:21 (in 3 months)" + details + hint + statusNever; got.Code != c.code || got.Stdout != want || got.Stderr != "" {
				t.Fatal(got)
			}
			runHealthyStatus(t, r, r.backend, got)

			r.deps.Now = func() time.Time { return connectExpiry.Add(48 * time.Hour) }
			got = runOfflineStatus(t, r)
			want := header + "  Token expiry      expired on 2027-02-14 10:21 (2 days ago)" + details +
				"  Run `agent-cli connect <KEY>` to connect again.\n" + statusChecked
			if got.Code != 3 || got.Stdout != want || got.Stderr != "" {
				t.Fatal(got)
			}
			runHealthyStatus(t, r, r.backend, got)
		})
	}
}

func TestStatusDefaultsAndBackend(t *testing.T) {
	for _, c := range []struct {
		name, chat, platform string
		stored               config.Default
		want                 string
	}{
		{"group", "", "", config.Default{ChatID: "c9", ChatPlatform: "telegram", ChatTitle: "Dev Team"}, "  Default chat      c9 (telegram · Dev Team, group)"},
		{"platform", "", "", config.Default{Platform: "whatsapp"}, "  Default platform  whatsapp"},
		{"none", "", "", config.Default{}, "  Default chat      none (latest chat is used)"},
		{"env chat", "c7", "telegram", config.Default{ChatID: "c9"}, "  Default chat      c7 (from THE_AGENT_CLI_DEFAULT_CHAT)"},
		{"env platform", "", "WhatsApp", config.Default{ChatID: "c9"}, "  Default platform  whatsapp (from THE_AGENT_CLI_DEFAULT_PLATFORM)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := statusRig(t, "linux", "keychain")
			f := r.file(t)
			f.Default = c.stored
			r.save(t, f)
			r.vars[config.EnvDefaultChat], r.vars[config.EnvDefaultPlatform] = c.chat, c.platform
			got := runOfflineStatus(t, r)
			if got.Code != 0 || got.Stderr != "" || !strings.Contains(got.Stdout, c.want+"\n") {
				t.Fatal(got)
			}
		})
	}
	r := statusRig(t, "linux", "keychain")
	delete(r.vars, config.EnvAPIURL)
	if got := runOfflineStatus(t, r); !strings.Contains(got.Stdout, "  Backend           stored.example\n") {
		t.Fatal(got)
	}
	f := r.file(t)
	f.APIURL = ""
	r.save(t, f)
	if got := runOfflineStatus(t, r); !strings.Contains(got.Stdout, "  Backend           localhost\n") {
		t.Fatal(got)
	}
	r.deps.Build.APIURL = ""
	if got := runOfflineStatus(t, r); !strings.Contains(got.Stdout, "  Backend           not set\n") {
		t.Fatal(got)
	}
	f.Token.UserName, f.Token.Hostname, f.Default.ChatID = "\x1b[31mMorpheus", "host\x1b[0m", "c\x00"
	f.Default.ChatTitle = "\x1b]0;bad\x07Title"
	r.save(t, f)
	if got := runOfflineStatus(t, r); strings.ContainsAny(got.Stdout, "\x1b\x00\x07") {
		t.Fatal(got)
	}
}

func TestStatusBackendPrecedence(t *testing.T) {
	env, stored, build := testkit.NewBackend(t), testkit.NewBackend(t), testkit.NewBackend(t)
	for _, c := range []struct {
		name, env, stored, build, displayed string
		selected                            *testkit.Backend
	}{
		{"environment wins", env.URL(), stored.URL(), build.URL(), env.URL(), env},
		{"config wins", "", stored.URL(), build.URL(), stored.URL(), stored},
		{"build default", "", "", build.URL(), build.URL(), build},
		{"invalid environment", "not-a-url", stored.URL(), build.URL(), "invalid address", nil},
		{"invalid config", "", "http://insecure.example", build.URL(), "invalid address", nil},
		{"invalid build default", "", "", "not-a-url", "invalid address", nil},
		{"URL credentials", "https://user:secret@agent.example.test", stored.URL(), build.URL(), "invalid address", nil},
		{"empty fragment", env.URL() + "#", stored.URL(), build.URL(), "invalid address", nil},
		{"no address", "", "", "", "not set", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := statusRig(t, "linux", "keychain")
			r.vars[config.EnvAPIURL], r.deps.Build.APIURL = c.env, c.build
			f := r.file(t)
			f.APIURL = c.stored
			r.save(t, f)
			local := runOfflineStatus(t, r)
			if local.Code != 0 || local.Stderr != "" ||
				!strings.Contains(local.Stdout, "  Backend           "+strings.TrimPrefix(c.displayed, "http://")+"\n") ||
				!strings.HasSuffix(local.Stdout, statusNever) || strings.Contains(local.Stdout, "secret@") {
				t.Fatal(local)
			}
			if c.selected != nil {
				runHealthyStatus(t, r, c.selected, local)
				return
			}
			got := runStatus(t, r, nil, "--verbose")
			prefix := strings.TrimSuffix(local.Stdout, statusNever)
			if got.Code != local.Code || got.Stdout != prefix+statusUnavailable+statusNever || got.Stderr != "" {
				t.Fatal(got)
			}
		})
	}
}

func TestStatusEnvironmentToken(t *testing.T) {
	for _, c := range []struct {
		name   string
		exp    any
		code   int
		expiry string
	}{
		{"future", connectExpiry.Unix(), 0, "2027-02-14 10:21 (in 3 months)"},
		{"past", time.Date(2026, 11, 12, 9, 21, 0, 0, time.UTC).Unix(), 3, "expired on 2026-11-12 10:21 (2 days ago)"},
		{"absent", nil, 0, "unknown"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := statusRig(t, "linux", "passphrase")
			if err := os.Remove(filepath.Join(r.home, credentials.FileName)); err != nil {
				t.Fatal(err)
			}
			payload := map[string]any{"sub": "u1"}
			if c.exp != nil {
				payload["exp"] = c.exp
			}
			raw := testkit.RawToken(map[string]any{"alg": "HS256"}, payload)
			r.vars[config.EnvToken] = raw
			want := "Connected using THE_AGENT_CLI_TOKEN\n  Token storage     THE_AGENT_CLI_TOKEN environment variable\n" +
				"  Token expiry      " + c.expiry + "\n  Default chat      3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7 (telegram · Example Chat, private)\n" +
				"  Backend           " + strings.TrimPrefix(r.backend.URL(), "http://") + "\n  Local CLI         " + r.deps.Build.Version + "\n"
			if c.code == 3 {
				want += "  Run `agent-cli connect <KEY>` to connect again.\n"
			}
			want += statusNever
			got := runOfflineStatus(t, r)
			if got.Code != c.code || got.Stdout != want || got.Stderr != "" {
				t.Fatal(got)
			}
			runHealthyStatus(t, r, r.backend, got)
		})
	}
}

func TestStatusLocalErrorsSkipRequests(t *testing.T) {
	t.Run("malformed config", func(t *testing.T) {
		r := statusRig(t, "linux", "keychain")
		path := config.Path(r.home)
		if err := os.WriteFile(path, []byte("invalid toml ["), 0o600); err != nil {
			t.Fatal(err)
		}
		local := runOfflineStatus(t, r)
		if local.Code != 1 || local.Stdout != "" || !strings.HasPrefix(local.Stderr, "error: could not read "+path+": ") || !strings.HasSuffix(local.Stderr, "; fix or delete the file\n") {
			t.Fatal(local)
		}
		if got := runStatus(t, r, nil, "--verbose"); got != local {
			t.Fatal(got)
		}
	})
	for _, c := range []struct {
		name, env, value, stderr string
		code                     int
	}{
		{"invalid token", config.EnvToken, "garbage", "error: THE_AGENT_CLI_TOKEN is not a valid CLI token\n", 3},
		{"invalid default", config.EnvDefaultPlatform, "slack", "error: invalid THE_AGENT_CLI_DEFAULT_PLATFORM \"slack\": use telegram or whatsapp\n", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := statusRig(t, "linux", "keychain")
			r.vars[c.env] = c.value
			want := testkit.Result{Code: c.code, Stderr: c.stderr}
			if got := runOfflineStatus(t, r); got != want {
				t.Fatal(got)
			}
			if got := runStatus(t, r, nil, "--verbose"); got != want {
				t.Fatal(got)
			}
		})
	}
}

func TestStatusLocalWriteFailureSkipsRequests(t *testing.T) {
	t.Run("disconnected", func(t *testing.T) {
		checkStatusLocalWriteFailure(t, newConnectRig(t, "linux"))
	})
	t.Run("connected", func(t *testing.T) {
		checkStatusLocalWriteFailure(t, statusRig(t, "linux", "keychain"))
	})
}

func checkStatusLocalWriteFailure(t *testing.T, r *connectRig) {
	t.Helper()
	before := statusSnapshot(t, r.home)
	var stderr bytes.Buffer
	deps := r.deps
	deps.Stdout, deps.Stderr = commandWriteFailure{}, &stderr
	deps.HTTP = readinessTransport(t, nil)
	code := cli.Run(t.Context(), deps, []string{"status", "--verbose"})
	if code != 1 || stderr.String() != "error: operation failed\n" {
		t.Fatalf("code %d, stderr %q", code, stderr.String())
	}
	if statusSnapshot(t, r.home) != before || len(r.keychain.Calls) != 0 || len(r.terminal.Prompts) != 0 {
		t.Error("status accessed credentials, prompted or changed files")
	}
}

const (
	chatsFirstID  = "3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7"
	chatsSecondID = "9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e"
	chatsGroupID  = "c0de1234c0de1234c0de1234c0de1234"
)

func newChatsRig(t *testing.T, goos string) *connectRig {
	t.Helper()
	r := newConnectRig(t, goos)
	r.save(t, config.File{
		DeviceID: connectDevice, APIURL: r.backend.URL(),
		Token:   config.Token{Store: "keychain", UserID: "u1", UserName: "Morpheus", Hostname: "workstation", ExpiresAt: connectExpiry},
		Default: config.Default{ChatID: chatsFirstID, ChatPlatform: "telegram", ChatTitle: "Morpheus", ChatPrivate: true},
	})
	if err := r.keychain.Set(r.deps.Build.Name, connectDevice, connectSecret); err != nil {
		t.Fatal(err)
	}
	r.keychain.Calls = nil
	private, group := "Morpheus", "Dev Team"
	r.backend.Handle(http.MethodGet, "/cli/chats", testkit.JSON(200, []map[string]any{
		testkit.ChatJSON(chatsFirstID, "telegram", &private, true),
		testkit.ChatJSON(chatsSecondID, "whatsapp", &private, true),
		testkit.ChatJSON(chatsGroupID, "telegram", &group, false),
	}))
	return r
}

func runChats(t *testing.T, r *connectRig, args ...string) testkit.Result {
	t.Helper()
	got := testkit.Run(t, r.deps, append([]string{"chats"}, args...)...)
	for _, secret := range []string{connectSecret, "secretphrase", r.vars[config.EnvToken]} {
		if secret != "" {
			testkit.AssertNoSecret(t, secret, got.Stdout, got.Stderr)
		}
	}
	return got
}

func TestChatsTableAndHelpSample(t *testing.T) {
	r := newChatsRig(t, "linux")
	before := statusSnapshot(t, r.home)
	got := runChats(t, r)
	want := "  ID                                PLATFORM  TITLE     PRIVATE   (* = default)\n" +
		"* " + chatsFirstID + "  telegram  Morpheus  yes\n" +
		"  " + chatsSecondID + "  whatsapp  Morpheus  yes\n" +
		"  " + chatsGroupID + "  telegram  Dev Team  no\n"
	if got.Code != 0 || got.Stdout != want || got.Stderr != "" {
		t.Fatalf("got %+v; want %q", got, want)
	}
	testkit.Golden(t, "chats_table", got.Stdout)
	request := r.backend.Last(t)
	if r.backend.Count() != 1 || request.Method != http.MethodGet || request.Path != "/cli/chats" || request.Header.Get("Authorization") != "Bearer "+connectSecret {
		t.Error("wrong authenticated chat listing request")
	}
	if statusSnapshot(t, r.home) != before {
		t.Error("listing changed local state")
	}
	help := runChats(t, r, "--help")
	if help.Code != 0 || help.Stderr != "" || !strings.Contains(help.Stdout, strings.TrimSuffix(got.Stdout, "\n")) || r.backend.Count() != 1 {
		t.Error("help sample differs from the actual table or contacted the backend")
	}
}

func TestChatsEffectiveDefaultMarker(t *testing.T) {
	for _, c := range []struct {
		name, chat, platform string
		stored               config.Default
		marked               string
	}{
		{"stored chat", "", "", config.Default{ChatID: chatsFirstID}, chatsFirstID},
		{"environment chat", chatsSecondID, "telegram", config.Default{ChatID: chatsFirstID}, chatsSecondID},
		{"case insensitive environment chat", strings.ToUpper(chatsSecondID), "", config.Default{ChatID: chatsFirstID}, chatsSecondID},
		{"stored platform", "", "", config.Default{Platform: "whatsapp"}, ""},
		{"environment platform", "", "WhatsApp", config.Default{ChatID: chatsFirstID}, ""},
		{"no default", "", "", config.Default{}, ""},
		{"unknown default", "missing", "", config.Default{ChatID: chatsFirstID}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			f := r.file(t)
			f.Default = c.stored
			r.save(t, f)
			r.vars[config.EnvDefaultChat], r.vars[config.EnvDefaultPlatform] = c.chat, c.platform
			got := runChats(t, r)
			if got.Code != 0 || got.Stderr != "" {
				t.Fatal(got)
			}
			var marked []string
			for _, line := range strings.Split(got.Stdout, "\n") {
				if strings.HasPrefix(line, "* ") {
					marked = append(marked, strings.Fields(line)[1])
				}
			}
			if c.marked == "" && len(marked) != 0 || c.marked != "" && (len(marked) != 1 || marked[0] != c.marked) {
				t.Errorf("marked %v, want %q", marked, c.marked)
			}
		})
	}
}

func TestChatsTitlesPlatformsAndEmptyList(t *testing.T) {
	longTitle, unsafeTitle := strings.Repeat("界", 80), "\x1b[31mNeo\x1b[0m\n\tTeam\x00"
	for _, c := range []struct {
		name, platform, want string
		title                *string
	}{
		{"long title", "telegram", longTitle, &longTitle},
		{"null title", "whatsapp", "untitled", nil},
		{"unknown platform", "matrix", "matrix", nil},
		{"untrusted text", "\x1b[32mmatrix\x1b[0m", "Neo Team", &unsafeTitle},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			r.backend.Handle(http.MethodGet, "/cli/chats", testkit.JSON(200, []map[string]any{testkit.ChatJSON("chat", c.platform, c.title, false)}))
			got := runChats(t, r)
			if got.Code != 0 || got.Stderr != "" || !strings.Contains(got.Stdout, c.want) || strings.ContainsAny(got.Stdout, "\x1b\x00\t") || strings.Count(got.Stdout, "\n") != 2 {
				t.Fatal(got)
			}
		})
	}
	r := newChatsRig(t, "linux")
	r.backend.Handle(http.MethodGet, "/cli/chats", testkit.JSON(200, []any{}))
	got := runChats(t, r)
	if got.Code != 0 || got.Stderr != "" || got.Stdout != "No chats found. Start a conversation with the agent on Telegram or WhatsApp, then try again.\n" {
		t.Fatal(got)
	}
}

func TestChatsDefaultOffline(t *testing.T) {
	for _, c := range []struct {
		name, chat, platform string
		stored               config.Default
		line                 string
	}{
		{"stored chat", "", "", config.Default{ChatID: chatsFirstID, ChatPlatform: "telegram", ChatTitle: "Morpheus", ChatPrivate: true}, "Default chat: " + chatsFirstID + " (telegram · Morpheus, private)"},
		{"stored group", "", "", config.Default{ChatID: chatsGroupID, ChatPlatform: "telegram", ChatTitle: "Dev Team"}, "Default chat: " + chatsGroupID + " (telegram · Dev Team, group)"},
		{"untitled", "", "", config.Default{ChatID: "c1", ChatPlatform: "matrix"}, "Default chat: c1 (matrix · untitled, group)"},
		{"stored platform", "", "", config.Default{Platform: "whatsapp"}, "Default platform: whatsapp"},
		{"none", "", "", config.Default{}, "No default chat is set; notifications go to your latest chat."},
		{"environment chat", "c7", "telegram", config.Default{ChatID: "c1"}, "Default chat: c7 (from THE_AGENT_CLI_DEFAULT_CHAT)"},
		{"environment platform", "", "Telegram", config.Default{ChatID: "c1"}, "Default platform: telegram (from THE_AGENT_CLI_DEFAULT_PLATFORM)"},
		{"untrusted cached text", "", "", config.Default{ChatID: "c\x001", ChatPlatform: "\x1b[31mtelegram", ChatTitle: "Neo\n Team\x1b[0m"}, "Default chat: c1 (telegram · Neo Team, group)"},
		{"untrusted environment chat", "\x1b[31mc7\x00", "", config.Default{}, "Default chat: c7 (from THE_AGENT_CLI_DEFAULT_CHAT)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			f := r.file(t)
			f.Default = c.stored
			r.save(t, f)
			r.vars[config.EnvDefaultChat], r.vars[config.EnvDefaultPlatform] = c.chat, c.platform
			r.vars[config.EnvAPIURL], r.vars[config.EnvToken] = "invalid URL", "invalid-token"
			before := statusSnapshot(t, r.home)
			got := runChats(t, r, "default", "-v")
			want := c.line + "\nTip: change it with `agent-cli chats default --set <chat-id> | --platform telegram|whatsapp | --clear`.\n"
			if got.Code != 0 || got.Stdout != want || got.Stderr != "" {
				t.Fatalf("got %+v, want %q", got, want)
			}
			if r.backend.Count() != 0 || len(r.keychain.Calls) != 0 || len(r.terminal.Prompts) != 0 || statusSnapshot(t, r.home) != before {
				t.Error("offline default accessed credentials, prompted, called the backend or changed state")
			}
		})
	}
	r := newConnectRig(t, "linux")
	if got := runChats(t, r, "default"); got.Code != 0 || got.Stderr != "" || !strings.HasPrefix(got.Stdout, "No default chat is set;") {
		t.Fatal(got)
	}
	if _, err := os.Stat(r.home); !os.IsNotExist(err) || r.backend.Count() != 0 {
		t.Error("fresh offline default created state or contacted the backend")
	}
}

func TestChatsSetDefaultChat(t *testing.T) {
	for _, c := range []struct {
		name, id, platform, title, description string
		private                                bool
	}{
		{"canonical private", chatsSecondID, "whatsapp", "Morpheus", "whatsapp · Morpheus, private", true},
		{"group", chatsGroupID, "telegram", "Dev Team", "telegram · Dev Team, group", false},
		{"unknown platform and untitled", "c123", "matrix", "", "matrix · untitled, group", false},
		{"untrusted title", "c456", "telegram", "\x1b[31mNeo\n Team\x1b[0m", "telegram · Neo Team, private", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			before := r.file(t)
			before.Default = config.Default{Platform: "telegram"}
			r.save(t, before)
			r.backend.Handle(http.MethodGet, "/cli/chats", testkit.JSON(200, []map[string]any{testkit.ChatJSON(c.id, c.platform, &c.title, c.private)}))
			got := runChats(t, r, "default", "--set", strings.ToUpper(c.id))
			if got.Code != 0 || got.Stdout != "✓ Default chat set to "+c.id+" ("+c.description+")\n" || got.Stderr != "" || r.backend.Count() != 1 {
				t.Fatal(got)
			}
			want := before
			want.Default = config.Default{ChatID: c.id, ChatPlatform: c.platform, ChatTitle: c.title, ChatPrivate: c.private}
			if r.file(t) != want {
				t.Errorf("config %+v, want %+v", r.file(t), want)
			}
			secret, present := r.keychain.Secret(r.deps.Build.Name, connectDevice)
			if !present || secret != connectSecret {
				t.Error("setting the default changed credentials")
			}
		})
	}
}

func TestChatsUnknownDefaultPreservesConfig(t *testing.T) {
	for _, id := range []string{"nope", chatsFirstID[:12], " " + chatsFirstID} {
		t.Run(id, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			before := statusSnapshot(t, r.home)
			got := runChats(t, r, "default", "--set", id)
			want := "error: chat \"" + id + "\" is not one of your chats; run `agent-cli chats` to list them\n"
			if got.Code != 2 || got.Stdout != "" || got.Stderr != want || r.backend.Count() != 1 || statusSnapshot(t, r.home) != before {
				t.Fatal(got)
			}
		})
	}
}

func TestChatsOfflineDefaultWrites(t *testing.T) {
	for _, c := range []struct {
		name   string
		args   []string
		stored config.Default
		want   config.Default
		line   string
	}{
		{"platform replaces chat", []string{"--platform", "WhatsApp"}, config.Default{ChatID: chatsFirstID, ChatPlatform: "telegram", ChatTitle: "Morpheus", ChatPrivate: true}, config.Default{Platform: "whatsapp"}, "✓ Default platform set to whatsapp"},
		{"telegram replaces platform", []string{"--platform", "TELEGRAM"}, config.Default{Platform: "whatsapp"}, config.Default{Platform: "telegram"}, "✓ Default platform set to telegram"},
		{"clear chat", []string{"--clear"}, config.Default{ChatID: chatsFirstID}, config.Default{}, "✓ Default cleared"},
		{"clear platform", []string{"--clear"}, config.Default{Platform: "telegram"}, config.Default{}, "✓ Default cleared"},
		{"clear nothing", []string{"--clear"}, config.Default{}, config.Default{}, "✓ Default cleared"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			before := r.file(t)
			before.Default = c.stored
			r.save(t, before)
			r.vars[config.EnvAPIURL], r.vars[config.EnvToken] = "invalid URL", "invalid-token"
			got := runChats(t, r, append([]string{"default"}, c.args...)...)
			if got.Code != 0 || got.Stdout != c.line+"\n" || got.Stderr != "" {
				t.Fatal(got)
			}
			want := before
			want.Default = c.want
			if r.file(t) != want || r.backend.Count() != 0 || len(r.keychain.Calls) != 0 || len(r.terminal.Prompts) != 0 {
				t.Error("offline default write changed unrelated state or accessed credentials/network")
			}
		})
	}
	for _, args := range [][]string{{"--platform", "telegram"}, {"--clear"}} {
		r := newConnectRig(t, "linux")
		got := runChats(t, r, append([]string{"default"}, args...)...)
		if got.Code != 0 || got.Stderr != "" || r.backend.Count() != 0 || r.file(t).DeviceID != "" || r.file(t).Token != (config.Token{}) {
			t.Fatalf("fresh offline write: %+v", got)
		}
	}
}

func TestChatsSetDefaultValidationBeforeEffects(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--set", ""}, "choose one of --set <chat-id>, --platform, or --clear, or omit flags to show the default"},
		{[]string{"--clear", "--platform", "telegram"}, "choose one of --set <chat-id>, --platform, or --clear, or omit flags to show the default"},
		{[]string{"--set", chatsFirstID, "--clear"}, "choose one of --set <chat-id>, --platform, or --clear, or omit flags to show the default"},
		{[]string{"--set", chatsFirstID, "--platform", "telegram"}, "choose one of --set <chat-id>, --platform, or --clear, or omit flags to show the default"},
		{[]string{"--set", chatsFirstID, "--clear", "--platform", "telegram"}, "choose one of --set <chat-id>, --platform, or --clear, or omit flags to show the default"},
		{[]string{"--clear=false"}, "choose one of --set <chat-id>, --platform, or --clear, or omit flags to show the default"},
		{[]string{"--platform", "slack"}, "invalid --platform \"slack\": use telegram or whatsapp"},
		{[]string{"--platform="}, "invalid --platform \"\": use telegram or whatsapp"},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			r := newConnectRig(t, "linux")
			if err := config.EnsureHome(r.home); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config.Path(r.home), []byte("invalid toml ["), 0o600); err != nil {
				t.Fatal(err)
			}
			before := statusSnapshot(t, r.home)
			got := runChats(t, r, append([]string{"default"}, c.args...)...)
			if got.Code != 2 || got.Stdout != "" || got.Stderr != "error: "+c.want+"\n" || statusSnapshot(t, r.home) != before || r.backend.Count() != 0 || len(r.keychain.Calls) != 0 || len(r.terminal.Prompts) != 0 {
				t.Fatal(got)
			}
		})
	}
	for _, args := range [][]string{{"extra"}, {"default", "extra"}, {"set-default", chatsFirstID}} {
		r := newConnectRig(t, "linux")
		got := runChats(t, r, args...)
		if got.Code != 2 || got.Stdout != "" || !strings.HasPrefix(got.Stderr, "error: ") || r.backend.Count() != 0 {
			t.Fatal(got)
		}
		if _, err := os.Stat(r.home); !os.IsNotExist(err) {
			t.Error("invalid arguments created state")
		}
	}
}

func TestChatsDefaultOverrideWarning(t *testing.T) {
	for _, c := range []struct{ chat, platform, variable string }{
		{"c7", "", config.EnvDefaultChat},
		{"", "whatsapp", config.EnvDefaultPlatform},
		{"c7", "telegram", config.EnvDefaultChat},
		{"", "", ""},
	} {
		for _, args := range [][]string{{"--platform", "telegram"}, {"--clear"}, {"--set", chatsSecondID}} {
			t.Run(c.variable+"/"+strings.Join(args, " "), func(t *testing.T) {
				r := newChatsRig(t, "linux")
				r.vars[config.EnvDefaultChat], r.vars[config.EnvDefaultPlatform] = c.chat, c.platform
				got := runChats(t, r, append([]string{"default"}, args...)...)
				warning := ""
				if c.variable != "" {
					warning = "warning: " + c.variable + " is set and overrides the stored default\n"
				}
				if got.Code != 0 || got.Stderr != warning {
					t.Fatal(got)
				}
				want := config.Default{}
				switch args[0] {
				case "--platform":
					want.Platform = "telegram"
				case "--set":
					want = config.Default{ChatID: chatsSecondID, ChatPlatform: "whatsapp", ChatTitle: "Morpheus", ChatPrivate: true}
				}
				if r.file(t).Default != want {
					t.Error("environment override prevented storing the requested default")
				}
			})
		}
	}
}

func TestChatsSharedBackendFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"default", "--set", chatsSecondID}} {
		for _, c := range []struct {
			name     string
			response testkit.Response
			code     int
			message  string
			removed  bool
		}{
			{"revoked", testkit.Err(401, 4005, "revoked"), 3, "This CLI is not connected. Run `agent-cli connect <KEY>`.\n", true},
			{"unauthorized", testkit.Detail(401, "unauthorized"), 3, "This CLI is not connected. Run `agent-cli connect <KEY>`.\n", true},
			{"forbidden", testkit.Err(403, 3002, "forbidden"), 3, "This CLI is not connected. Run `agent-cli connect <KEY>`.\n", true},
			{"merged", testkit.Err(404, 2001, "missing"), 3, "This CLI is not connected. Run `agent-cli connect <KEY>`.\n", true},
			{"rate limited", testkit.Text(429, "limited"), 6, "error: rate limited; wait a moment and try again\n", false},
			{"server", testkit.HTML(503), 1, "error: the server had a problem; try again later\n", false},
			{"malformed list", testkit.JSON(200, map[string]any{"chats": []any{}}), 1, "error: unexpected response from the server\n", false},
			{"chat target", testkit.Err(404, 2002, "missing"), 4, "error: chat unreachable: no such chat; run `agent-cli chats` to list yours\n", false},
			{"policies", testkit.Err(403, 3008, "policies"), 1, "error: accept the agent's policies in chat settings, then try again\n", false},
		} {
			t.Run(strings.Join(args, " ")+"/"+c.name, func(t *testing.T) {
				r := newChatsRig(t, "linux")
				before := r.file(t)
				r.backend.Handle(http.MethodGet, "/cli/chats", c.response)
				got := runChats(t, r, args...)
				if got.Code != c.code || got.Stdout != "" || got.Stderr != c.message || r.backend.Count() != 1 {
					t.Fatal(got)
				}
				want := before
				if c.removed {
					want.Token = config.Token{}
				}
				if r.file(t) != want || (r.keychain.Len() == 0) != c.removed {
					t.Error("incorrect shared authentication cleanup")
				}
			})
		}
	}
	for _, oversized := range []bool{false, true} {
		for _, environment := range []bool{false, true} {
			for _, status := range []int{401, 429} {
				for _, command := range [][]string{{"chats"}, {"notify", "hi"}} {
					r := newChatsRig(t, "linux")
					before := r.file(t)
					secret := connectSecret
					if environment {
						secret = testkit.NewToken(testkit.TokenClaims{Sub: "neo", Exp: connectExpiry})
						r.vars[config.EnvToken] = secret
					}
					body := setUnusableCommandResponse(r, status, oversized)
					got := testkit.Run(t, r.deps, command...)
					code, message := 6, "error: rate limited; wait a moment and try again\n"
					remove := status == 401 && !environment
					if status == 401 {
						code = 3
						message = "This CLI is not connected. Run `agent-cli connect <KEY>`.\n"
						if environment {
							message = "error: the token in THE_AGENT_CLI_TOKEN was rejected by the server (expired or revoked)\n"
						}
					}
					if got.Code != code || got.Stdout != "" || got.Stderr != message || body.requests != 1 || body.closes != 1 || body.authorization != "Bearer "+secret {
						t.Fatalf("%v status=%d oversized=%v environment=%v: %+v", command, status, oversized, environment, got)
					}
					if remove {
						before.Token = config.Token{}
					}
					if r.file(t) != before || (r.keychain.Len() == 0) != remove {
						t.Error("unusable error body changed authentication cleanup")
					}
					if stored, exists := r.keychain.Secret(r.deps.Build.Name, connectDevice); exists != !remove || !remove && stored != connectSecret {
						t.Error("unusable response changed the stored secret")
					}
					testkit.AssertNoSecret(t, secret, got.Stdout, got.Stderr)
				}
			}
		}
	}
}

type chatsOfflineTransport struct{}

func (chatsOfflineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("offline")
}

func TestChatsConnectionPreconditionsAndNetworkFailure(t *testing.T) {
	for _, args := range [][]string{nil, {"default", "--set", chatsSecondID}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := newConnectRig(t, "linux")
			got := runChats(t, r, args...)
			if got.Code != 3 || got.Stdout != "" || got.Stderr != "This CLI is not connected. Run `agent-cli connect <KEY>`.\n" || r.backend.Count() != 0 || len(r.terminal.Prompts) != 0 {
				t.Fatal(got)
			}
			if _, err := os.Stat(r.home); !os.IsNotExist(err) {
				t.Error("not-connected command created state")
			}
			r = newChatsRig(t, "linux")
			before := statusSnapshot(t, r.home)
			r.deps.HTTP = chatsOfflineTransport{}
			got = runChats(t, r, append(args, "-v")...)
			if got.Code != 1 || got.Stdout != "" || got.Stderr != "Calling backend…\nerror: could not reach the server\n" || statusSnapshot(t, r.home) != before || r.keychain.Len() != 1 {
				t.Fatal(got)
			}
		})
	}
}

func TestChatsEnvironmentAuthentication(t *testing.T) {
	for _, args := range [][]string{nil, {"default", "--set", chatsSecondID}} {
		for _, rejected := range []bool{false, true} {
			t.Run(strings.Join(args, " ")+"/"+strconv.FormatBool(rejected), func(t *testing.T) {
				r := newChatsRig(t, "linux")
				before := r.file(t)
				secret := testkit.NewToken(testkit.TokenClaims{Sub: "neo", Exp: connectExpiry})
				r.vars[config.EnvToken] = secret
				if rejected {
					r.backend.Handle(http.MethodGet, "/cli/chats", testkit.Err(401, 4004, "rejected"))
				}
				got := runChats(t, r, append(args, "-v")...)
				if rejected {
					if got.Code != 3 || got.Stdout != "" || got.Stderr != "Calling backend…\n  (HTTP 401, code 4004)\nerror: the token in THE_AGENT_CLI_TOKEN was rejected by the server (expired or revoked)\n" || r.file(t) != before {
						t.Fatal(got)
					}
				} else if got.Code != 0 || got.Stdout == "" || got.Stderr != "Calling backend…\n" {
					t.Fatal(got)
				}
				if len(r.keychain.Calls) != 0 || r.keychain.Len() != 1 || r.backend.Last(t).Header.Get("Authorization") != "Bearer "+secret {
					t.Error("environment token did not bypass stored credentials")
				}
			})
		}
	}
}

func TestChatsLockedAndUnlockedCredentials(t *testing.T) {
	for _, args := range [][]string{nil, {"default", "--set", chatsSecondID}} {
		for _, unlock := range []string{"locked", "environment", "terminal"} {
			t.Run(strings.Join(args, " ")+"/"+unlock, func(t *testing.T) {
				r := newRevokeRig(t, "passphrase")
				title := "Morpheus"
				r.backend.Handle(http.MethodGet, "/cli/chats", testkit.JSON(200, []map[string]any{testkit.ChatJSON(chatsSecondID, "whatsapp", &title, true)}))
				r.terminal.Prompts = nil
				before := r.backend.Count()
				switch unlock {
				case "environment":
					r.vars[config.EnvPassphrase] = "secretphrase"
				case "terminal":
					r.terminal.TTYIn = true
					r.terminal.Secrets = []string{"secretphrase"}
				}
				got := runChats(t, r, args...)
				if unlock == "locked" {
					if got.Code != 3 || got.Stdout != "" || !strings.HasPrefix(got.Stderr, "error: token is locked;") || r.backend.Count() != before || len(r.terminal.Prompts) != 0 {
						t.Fatal(got)
					}
				} else if got.Code != 0 || got.Stderr != "" || r.backend.Count() != before+1 {
					t.Fatal(got)
				}
				if unlock == "terminal" && (len(r.terminal.Prompts) != 1 || r.terminal.Prompts[0] != "Passphrase: ") {
					t.Errorf("prompts %v", r.terminal.Prompts)
				}
				if unlock == "environment" && len(r.terminal.Prompts) != 0 {
					t.Error("environment passphrase prompted")
				}
			})
		}
	}
}

func TestChatsInvalidEnvironmentDefault(t *testing.T) {
	for _, args := range [][]string{nil, {"default"}} {
		r := newChatsRig(t, "linux")
		r.vars[config.EnvDefaultPlatform] = "slack"
		before := statusSnapshot(t, r.home)
		got := runChats(t, r, args...)
		if got.Code != 2 || got.Stdout != "" || got.Stderr != "error: invalid THE_AGENT_CLI_DEFAULT_PLATFORM \"slack\": use telegram or whatsapp\n" || r.backend.Count() != 0 || len(r.keychain.Calls) != 0 || statusSnapshot(t, r.home) != before {
			t.Fatal(got)
		}
	}
}

func runNotifyTest(t *testing.T, r *connectRig, args ...string) testkit.Result {
	t.Helper()
	got := testkit.Run(t, r.deps, append([]string{"notify"}, args...)...)
	testkit.AssertNoSecret(t, connectSecret, got.Stdout, got.Stderr)
	return got
}

func TestNotifyTargetPrecedence(t *testing.T) {
	for _, c := range []struct {
		name                 string
		flags                []string
		envChat, envPlatform string
		stored               config.Default
		path, platform       string
	}{
		{"explicit chat", []string{"--chat", "c1"}, "c7", "telegram", config.Default{Platform: "whatsapp"}, "/cli/notify/chat/c1", ""},
		{"explicit platform", []string{"--platform", "WhatsApp"}, "c7", "telegram", config.Default{ChatID: "c9"}, "/cli/notify/user/u1", "whatsapp"},
		{"environment chat", nil, "c7", "telegram", config.Default{ChatID: "c9"}, "/cli/notify/chat/c7", ""},
		{"environment platform", nil, "", "whatsapp", config.Default{ChatID: "c9"}, "/cli/notify/user/u1", "whatsapp"},
		{"stored chat", nil, "", "", config.Default{ChatID: "c9"}, "/cli/notify/chat/c9", ""},
		{"stored platform", nil, "", "", config.Default{Platform: "telegram"}, "/cli/notify/user/u1", "telegram"},
		{"no default", nil, "", "", config.Default{}, "/cli/notify/user/u1", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			file := r.file(t)
			file.Default = c.stored
			r.save(t, file)
			r.vars[config.EnvDefaultChat], r.vars[config.EnvDefaultPlatform] = c.envChat, c.envPlatform
			r.backend.Handle("POST", c.path, testkit.JSON(200, map[string]any{"credit_balance": 500}))
			got := runNotifyTest(t, r, append(c.flags, "Dinner's ready")...)
			if got.Code != 0 || got.Stdout != "" || got.Stderr != "" {
				t.Fatal(got)
			}
			request := r.backend.Last(t)
			var body map[string]any
			if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
				t.Fatal(err)
			}
			if body["message"] != "Dinner's ready" {
				t.Fatal(body)
			}
			if strings.Contains(c.path, "/chat/") {
				if _, ok := body["platform"]; ok {
					t.Fatal(body)
				}
			} else if c.platform == "" {
				if body["platform"] != nil {
					t.Fatal(body)
				}
			} else if body["platform"] != c.platform {
				t.Fatal(body)
			}
			if r.file(t) != file {
				t.Fatal("notify changed config")
			}
		})
	}
}

func TestNotifyMessages(t *testing.T) {
	for _, c := range []struct {
		name                 string
		args                 []string
		input, want, warning string
		tty                  bool
	}{
		{"argument", []string{"done"}, "ignored", "done", "", false},
		{"pipe", nil, "\n\ndone\n\n", "done", "", false},
		{"multiline", []string{"one\ntwo"}, "", "one\ntwo", "", false},
		{"bom", nil, "\uFEFF hello \n", "hello", "", false},
		{"invalid utf8", nil, string([]byte{0xff, 'x'}), "\uFFFDx", "", false},
		{"truncated", nil, strings.Repeat("a", 5000), strings.Repeat("a", 4096), "warning: input truncated to 4096 characters\n", false},
		{"unicode", nil, strings.Repeat("界", 5000), strings.Repeat("界", 4096), "warning: input truncated to 4096 characters\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			r.deps.Stdin = strings.NewReader(c.input)
			r.terminal.TTYIn = c.tty
			r.backend.Handle("POST", "/cli/notify/chat/"+chatsFirstID, testkit.Text(204, ""))
			got := runNotifyTest(t, r, c.args...)
			if got.Code != 0 || got.Stderr != c.warning || got.Stdout != "" {
				t.Fatal(got)
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(r.backend.Last(t).Body), &body); err != nil {
				t.Fatal(err)
			}
			if body["message"] != c.want {
				t.Fatalf("message %q, want %q", body["message"], c.want)
			}
		})
	}
}

func TestNotifyValidationBeforeAuthentication(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "no message given; pass it as an argument or pipe it on stdin"},
		{[]string{"Build", "finished"}, "pass the message as a single quoted argument"},
		{[]string{"   "}, "message is empty"},
		{[]string{strings.Repeat("a", 5000)}, "message is too long (max 4096 characters)"},
		{[]string{"--chat", "c1", "--platform", "telegram", "hi"}, "use either --chat or --platform, not both"},
		{[]string{"--chat", "", "hi"}, "--chat needs a chat id"},
		{[]string{"--platform", "slack", "hi"}, "invalid --platform \"slack\": use telegram or whatsapp"},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			r := newConnectRig(t, "linux")
			r.terminal.TTYIn = true
			got := runNotifyTest(t, r, c.args...)
			if got.Code != 2 || got.Stdout != "" || got.Stderr != "error: "+c.want+"\n" || r.backend.Count() != 0 || len(r.keychain.Calls) != 0 || len(r.terminal.Prompts) != 0 {
				t.Fatal(got)
			}
		})
	}
}

func TestNotifySuccessResponses(t *testing.T) {
	for _, c := range []struct {
		name     string
		response testkit.Response
		verbose  bool
		out, err string
	}{
		{"empty", testkit.Text(204, ""), false, "", ""},
		{"unreadable", testkit.Text(202, "not json"), true, "✓ Sent\n", "Calling backend…\n"},
		{"no details", testkit.JSON(200, map[string]any{}), true, "✓ Sent\n", "Calling backend…\n"},
		{"private", testkit.NotifyOK(testkit.ChatJSON("c1", "whatsapp", stringPointer("Morpheus"), true), 500), true, "✓ Sent to WhatsApp · Morpheus (private)\n", "Calling backend…\n"},
		{"group low", testkit.NotifyOK(testkit.ChatJSON("c1", "telegram", stringPointer("Dev Team"), false), 42.37), true, "✓ Sent to Telegram · Dev Team (group)\n", "Calling backend…\nwarning: low credit balance: 42.37 credits left; top up in the agent's chat settings\n"},
		{"silent low", testkit.JSON(200, map[string]any{"credit_balance": 42.37}), false, "", "warning: low credit balance: 42.37 credits left; top up in the agent's chat settings\n"},
		{"threshold", testkit.JSON(200, map[string]any{"credit_balance": 100}), false, "", ""},
		{"null balance", testkit.JSON(200, map[string]any{"credit_balance": nil}), false, "", ""},
		{"string balance", testkit.JSON(200, map[string]any{"credit_balance": "42"}), false, "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			r.backend.Handle("POST", "/cli/notify/chat/"+chatsFirstID, c.response)
			args := []string{"hi"}
			if c.verbose {
				args = append(args, "-v")
			}
			got := runNotifyTest(t, r, args...)
			if got.Code != 0 || got.Stdout != c.out || got.Stderr != c.err {
				t.Fatal(got)
			}
		})
	}
	for _, oversized := range []bool{false, true} {
		for _, verbose := range []bool{false, true} {
			for _, target := range [][]string{nil, {"--platform", "telegram"}} {
				r := newChatsRig(t, "linux")
				before := r.file(t)
				body := setUnusableCommandResponse(r, 202, oversized)
				args := append([]string{"hi"}, target...)
				wantOut, wantErr := "", ""
				if verbose {
					args = append(args, "-v")
					wantOut, wantErr = "✓ Sent\n", "Calling backend…\n"
				}
				got := runNotifyTest(t, r, args...)
				if got.Code != 0 || got.Stdout != wantOut || got.Stderr != wantErr || body.requests != 1 || body.closes != 1 || r.file(t) != before || r.keychain.Len() != 1 {
					t.Fatalf("oversized=%v verbose=%v target=%v: %+v", oversized, verbose, target, got)
				}
			}
		}
	}
}

func stringPointer(s string) *string { return &s }

func TestNotifyFailures(t *testing.T) {
	for _, c := range []struct {
		status, backendCode, exit int
		message                   string
		removed                   bool
	}{
		{401, 4005, 3, "This CLI is not connected. Run \x60agent-cli connect <KEY>\x60.\n", true},
		{403, 3010, 4, "error: chat unreachable: the platform refused the message; try another chat or platform\n", false},
		{403, 3004, 4, "error: chat unreachable: no private chat found; start a conversation with the agent on Telegram or WhatsApp first\n", false},
		{403, 3001, 4, "error: chat unreachable: you are not an admin of that chat\n", false},
		{403, 3009, 4, "error: chat unreachable: you are not an admin of that chat\n", false},
		{404, 2002, 4, "error: chat unreachable: no such chat; run \x60agent-cli chats\x60 to list yours\n", false},
		{422, 1034, 5, "error: insufficient credits; top up in the agent's chat settings\n", false},
		{429, 6004, 6, "error: rate limited; wait a moment and try again\n", false},
		{422, 1046, 2, "error: the message was rejected: it must be between 1 and 4096 characters\n", false},
		{502, 5018, 1, "error: the platform could not deliver the message; try again later\n", false},
		{403, 3008, 1, "error: accept the agent's policies in chat settings, then try again\n", false},
		{403, 3007, 1, "error: your account is still on the waitlist; try again once the agent has activated it\n", false},
	} {
		t.Run(strconv.Itoa(c.backendCode), func(t *testing.T) {
			r := newChatsRig(t, "linux")
			before := r.file(t)
			r.backend.Handle("POST", "/cli/notify/chat/"+chatsFirstID, testkit.Err(c.status, c.backendCode, "failure"))
			got := runNotifyTest(t, r, "hi")
			if got.Code != c.exit || got.Stdout != "" || got.Stderr != c.message {
				t.Fatal(got)
			}
			if c.removed {
				before.Token = config.Token{}
			}
			if r.file(t) != before || (r.keychain.Len() == 0) != c.removed {
				t.Fatal("incorrect token removal")
			}
		})
	}
	r := newConnectRig(t, "linux")
	got := runNotifyTest(t, r, "hi")
	if got.Code != 3 || r.backend.Count() != 0 {
		t.Fatal(got)
	}
	r = newChatsRig(t, "linux")
	r.deps.HTTP = chatsOfflineTransport{}
	got = runNotifyTest(t, r, "hi")
	if got.Code != 1 || got.Stderr != "error: could not reach the server\n" {
		t.Fatal(got)
	}
}

type notifyCountingReader struct{ bytes int }

func (r *notifyCountingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.bytes += len(p)
	return len(p), nil
}

func TestNotifyBoundedInputAndArgumentPrecedence(t *testing.T) {
	for _, argument := range []bool{false, true} {
		r := newChatsRig(t, "linux")
		reader := &notifyCountingReader{}
		r.deps.Stdin = reader
		r.backend.Handle("POST", "/cli/notify/chat/"+chatsFirstID, testkit.Text(204, ""))
		var args []string
		if argument {
			args = []string{"hi"}
		}
		got := runNotifyTest(t, r, args...)
		if got.Code != 0 {
			t.Fatal(got)
		}
		if argument && reader.bytes != 0 || !argument && reader.bytes != 16384 {
			t.Fatal(reader.bytes)
		}
	}
}

func TestNotifyLockedCredentials(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			r := newRevokeRig(t, "passphrase")
			r.deps.GOOS = goos
			r.deps.Text = testkit.NewDeps(testkit.WithPlatform(goos, "amd64")).Text
			r.terminal.Prompts = nil
			before := r.backend.Count()
			got := runNotifyTest(t, r, "hi")
			hint := "eval \"$(agent-cli unlock)\""
			if goos == "windows" {
				hint = "agent-cli unlock --shell pwsh | Invoke-Expression"
			}
			if got.Code != 3 || got.Stdout != "" || !strings.Contains(got.Stderr, hint) || r.backend.Count() != before || len(r.terminal.Prompts) != 0 {
				t.Fatal(got)
			}
		})
	}
}

func TestUnlockShells(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		for _, shell := range []string{"", "sh", "bash", "zsh", "fish", "pwsh", "powershell"} {
			t.Run(goos+shell, func(t *testing.T) {
				r := newRevokeRig(t, "passphrase")
				r.deps.GOOS = goos
				r.deps.Text = testkit.NewDeps(testkit.WithPlatform(goos, "amd64")).Text
				r.vars[config.EnvPassphrase] = "secretphrase"
				before := statusSnapshot(t, r.home)
				hits := r.backend.Count()
				r.terminal.Prompts = nil
				args := []string{"unlock"}
				if shell != "" {
					args = append(args, "--shell", shell)
				}
				got := testkit.Run(t, r.deps, args...)
				want := "export THE_AGENT_CLI_TOKEN='" + connectSecret + "'\n"
				if shell == "fish" {
					want = "set -gx THE_AGENT_CLI_TOKEN '" + connectSecret + "'\n"
				}
				if shell == "pwsh" || shell == "powershell" || shell == "" && goos == "windows" {
					want = "$env:THE_AGENT_CLI_TOKEN = '" + connectSecret + "'\n"
				}
				if got.Code != 0 || got.Stdout != want || got.Stderr != "✓ Unlocked for this terminal session\n" || r.backend.Count() != hits || statusSnapshot(t, r.home) != before || len(r.terminal.Prompts) != 0 {
					t.Fatal(got)
				}
			})
		}
	}
}

func TestUnlockFailuresAndNotes(t *testing.T) {
	for _, c := range []struct {
		name, mode, passphrase, envToken string
		args                             []string
		code                             int
		message                          string
	}{
		{"missing", "", "", "", nil, 3, "This CLI is not connected. Run \x60agent-cli connect <KEY>\x60.\n"},
		{"keychain", "keychain", "", "", nil, 0, "Nothing to unlock: your token is not passphrase-protected.\n"},
		{"unsafe", "unsafe", "", "", nil, 0, "Nothing to unlock: your token is not passphrase-protected.\n"},
		{"already", "", "", "set", nil, 0, "Already unlocked: THE_AGENT_CLI_TOKEN is set in this environment.\n"},
		{"wrong", "passphrase", "wrong", "", nil, 3, "error: incorrect passphrase\n"},
		{"shell", "", "", "", []string{"--shell", "csh"}, 2, "error: invalid --shell \"csh\": use sh, bash, zsh, fish, pwsh, or powershell\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var r *connectRig
			if c.mode == "" {
				r = newConnectRig(t, "linux")
			} else {
				r = newRevokeRig(t, c.mode)
			}
			r.vars[config.EnvPassphrase] = c.passphrase
			r.vars[config.EnvToken] = c.envToken
			r.terminal.Prompts = nil
			hits := r.backend.Count()
			got := testkit.Run(t, r.deps, append([]string{"unlock"}, c.args...)...)
			if got.Code != c.code || got.Stdout != "" || got.Stderr != c.message || r.backend.Count() != hits || len(r.terminal.Prompts) != 0 {
				t.Fatal(got)
			}
		})
	}
}

func TestUnlockPromptAndHint(t *testing.T) {
	r := newRevokeRig(t, "passphrase")
	r.terminal.TTYIn = true
	r.terminal.TTYOut = true
	r.terminal.Secrets = []string{"secretphrase"}
	r.terminal.Prompts = nil
	got := testkit.Run(t, r.deps, "unlock")
	if got.Code != 0 || got.Stdout != "export THE_AGENT_CLI_TOKEN='"+connectSecret+"'\n" || !strings.Contains(got.Stderr, "eval \"$(agent-cli unlock)\"") || len(r.terminal.Prompts) != 1 || r.terminal.Prompts[0] != "Passphrase: " {
		t.Fatal(got, r.terminal.Prompts)
	}
	r = newRevokeRig(t, "passphrase")
	r.terminal.Prompts = nil
	got = testkit.Run(t, r.deps, "unlock")
	if got.Code != 3 || got.Stdout != "" || !strings.Contains(got.Stderr, "token is locked;") || len(r.terminal.Prompts) != 0 {
		t.Fatal(got)
	}
}

func TestTerminalColorsRespectEachStream(t *testing.T) {
	for _, c := range []struct {
		name                 string
		outTTY, errTTY, ansi bool
		noColor, terminal    string
		outColor, errColor   bool
	}{
		{"terminal", true, true, true, "", "xterm", true, true},
		{"no color", true, true, true, "1", "xterm", false, false},
		{"empty no color", true, true, true, "", "xterm", true, true},
		{"dumb", true, true, true, "", "dumb", false, false},
		{"no ANSI", true, true, false, "", "xterm", false, false},
		{"pipe stdout", false, true, true, "", "xterm", false, true},
		{"pipe stderr", true, false, true, "", "xterm", true, false},
		{"both piped", false, false, true, "", "xterm", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newChatsRig(t, "linux")
			r.terminal.TTYOut = c.outTTY
			r.terminal.TTYErr = c.errTTY
			r.terminal.ANSIOK = c.ansi
			r.vars["NO_COLOR"] = c.noColor
			r.vars["TERM"] = c.terminal
			help := testkit.Run(t, r.deps, "notify", "--help")
			if help.Code != 0 || strings.Contains(help.Stdout, "\x1b[") != c.outColor {
				t.Fatal(help)
			}
			if c.outColor && !strings.Contains(help.Stdout, "\x1b[1;36mUsage:\x1b[0m") {
				t.Fatal(help)
			}
			r.vars[config.EnvDefaultPlatform] = "telegram"
			result := testkit.Run(t, r.deps, "chats", "default", "--clear")
			if result.Code != 0 || strings.Contains(result.Stdout, "\x1b[32m") != c.outColor || strings.Contains(result.Stderr, "\x1b[33m") != c.errColor {
				t.Fatal(result)
			}
			plain := strings.NewReplacer("\x1b[32m", "", "\x1b[33m", "", "\x1b[0m", "")
			if plain.Replace(result.Stdout) != "✓ Default cleared\n" || plain.Replace(result.Stderr) != "warning: THE_AGENT_CLI_DEFAULT_PLATFORM is set and overrides the stored default\n" {
				t.Fatal(result)
			}
			failure := testkit.Run(t, r.deps, "notify", "--chat", "", "hi")
			if failure.Code != 2 || strings.Contains(failure.Stderr, "\x1b[31m") != c.errColor || failure.Stdout != "" {
				t.Fatal(failure)
			}
		})
	}
}

func TestUnlockAssignmentNeverColored(t *testing.T) {
	r := newRevokeRig(t, "passphrase")
	r.terminal.TTYOut = true
	r.terminal.TTYErr = true
	r.terminal.ANSIOK = true
	r.vars[config.EnvPassphrase] = "secretphrase"
	got := testkit.Run(t, r.deps, "unlock")
	if got.Code != 0 || got.Stdout != "export THE_AGENT_CLI_TOKEN='"+connectSecret+"'\n" || !strings.Contains(got.Stderr, "\x1b[32m✓ Unlocked for this terminal session\x1b[0m") {
		t.Fatal(got)
	}
}

type commandRoundTrip func(*http.Request) (*http.Response, error)

func (f commandRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type commandReadFailure struct{}

func (commandReadFailure) Read([]byte) (int, error) { return 0, errors.New("input sentinel") }

type commandResponseBody struct {
	io.Reader
	requests, closes int
	authorization    string
}

func (b *commandResponseBody) Close() error {
	b.closes++
	return nil
}

func setUnusableCommandResponse(r *connectRig, status int, oversized bool) *commandResponseBody {
	var tail io.Reader = commandReadFailure{}
	if oversized {
		tail = strings.NewReader(strings.Repeat(" ", (4<<20)+1))
	}
	body := &commandResponseBody{Reader: io.MultiReader(strings.NewReader(`{"error_code":1034,"message":"discard this","chat_id":"c1","platform":"telegram","is_private":true,"credit_balance":1}`), tail)}
	r.deps.HTTP = commandRoundTrip(func(request *http.Request) (*http.Response, error) {
		body.requests++
		body.authorization = request.Header.Get("Authorization")
		return &http.Response{StatusCode: status, Body: body}, nil
	})
	return body
}

func TestAuthenticatedValidationRedactsTokens(t *testing.T) {
	for _, environment := range []bool{false, true} {
		for _, verbose := range []bool{false, true} {
			for _, boundary := range []bool{false, true} {
				r := newChatsRig(t, "linux")
				before := statusSnapshot(t, r.home)
				secret := connectSecret
				if environment {
					secret = testkit.NewToken(testkit.TokenClaims{Sub: "neo", Exp: connectExpiry})
					r.vars[config.EnvToken] = secret
				}
				prefix := "Invalid token "
				if boundary {
					prefix = strings.Repeat("x", 187)
				}
				r.backend.Handle("GET", "/cli/chats", testkit.JSON(422, map[string]any{"error_code": 1999, "message": prefix + secret + " rejected"}))
				args := []string{"chats"}
				wantErr := "error: " + prefix + "*** rejected\n"
				if verbose {
					args = append(args, "-v")
					wantErr = "Calling backend…\n  (HTTP 422, code 1999)\n" + wantErr
				}
				got := testkit.Run(t, r.deps, args...)
				if got.Code != 2 || got.Stdout != "" || got.Stderr != wantErr || r.backend.Count() != 1 || statusSnapshot(t, r.home) != before || r.keychain.Len() != 1 {
					t.Fatalf("environment=%v verbose=%v boundary=%v: %+v", environment, verbose, boundary, got)
				}
				if stored, exists := r.keychain.Secret(r.deps.Build.Name, connectDevice); !exists || stored != connectSecret {
					t.Error("validation failure changed the stored secret")
				}
				if r.backend.Last(t).Header.Get("Authorization") != "Bearer "+secret {
					t.Error("redaction altered the request token")
				}
				testkit.AssertNoSecret(t, secret, got.Stdout, got.Stderr)
				testkit.AssertNoSecret(t, secret[:13], got.Stdout, got.Stderr)
			}
		}
	}
}

type commandWriteFailure struct{}

func (commandWriteFailure) Write([]byte) (int, error) { return 0, errors.New("output sentinel") }

func TestOperationalFailuresUseCatalog(t *testing.T) {
	t.Run("stdin", func(t *testing.T) {
		r := newChatsRig(t, "linux")
		before := statusSnapshot(t, r.home)
		r.deps.Stdin = commandReadFailure{}
		got := testkit.Run(t, r.deps, "notify")
		if got.Code != 1 || got.Stdout != "" || got.Stderr != "error: operation failed\n" || r.backend.Count() != 0 || len(r.keychain.Calls) != 0 || statusSnapshot(t, r.home) != before {
			t.Fatal(got)
		}
	})
	t.Run("required output", func(t *testing.T) {
		r := newChatsRig(t, "linux")
		before := statusSnapshot(t, r.home)
		var stderr bytes.Buffer
		r.deps.Stdout, r.deps.Stderr = commandWriteFailure{}, &stderr
		code := cli.Run(t.Context(), r.deps, []string{"chats"})
		if code != 1 || stderr.String() != "error: operation failed\n" || r.backend.Count() != 1 || statusSnapshot(t, r.home) != before || r.keychain.Len() != 1 {
			t.Fatalf("code %d, stderr %q", code, stderr.String())
		}
	})
	t.Run("in-flight cancellation", func(t *testing.T) {
		r := newChatsRig(t, "linux")
		before := statusSnapshot(t, r.home)
		entered := make(chan struct{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		requests := 0
		r.deps.HTTP = commandRoundTrip(func(request *http.Request) (*http.Response, error) {
			requests++
			close(entered)
			<-request.Context().Done()
			return nil, request.Context().Err()
		})
		var stdout, stderr bytes.Buffer
		r.deps.Stdout, r.deps.Stderr = &stdout, &stderr
		done := make(chan int, 1)
		go func() { done <- cli.Run(ctx, r.deps, []string{"chats", "-v"}) }()
		<-entered
		cancel()
		code := <-done
		if code != 1 || stdout.Len() != 0 || stderr.String() != "Calling backend…\nerror: operation cancelled\n" || requests != 1 || statusSnapshot(t, r.home) != before || r.keychain.Len() != 1 {
			t.Fatalf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
		}
		testkit.AssertNoSecret(t, connectSecret, stdout.String(), stderr.String())
	})
}
