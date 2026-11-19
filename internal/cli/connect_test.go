package cli_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/cli"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
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
		{testkit.Err(400, 1044, "long ttl"), 2, "TTL exceeds the maximum of 2 years"},
		{testkit.Text(429, "Too Many Requests"), 6, "rate limited; wait a moment and try again"},
		{testkit.HTML(500), 1, "the server had a problem; try again later"},
		{testkit.Err(422, 1045, "device"), 1, "the server rejected this machine's details; please report this"},
		{testkit.JSON(200, map[string]any{"token": "t"}), 1, "unexpected response from the server"},
		{testkit.Err(422, 1999, connectKey+" secretphrase"), 2, "*** ***"},
	} {
		r := newConnectRig(t, "linux")
		old := config.File{DeviceID: connectDevice, APIURL: "https://previous.example.test", Token: config.Token{Store: "unsafe", UserID: "u1"}, Default: config.Default{Platform: "telegram"}}
		r.save(t, old)
		secretPath := filepath.Join(r.home, "credentials")
		if err := os.WriteFile(secretPath, []byte("old token\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		r.vars[config.EnvPassphrase] = "secretphrase"
		r.backend.Handle("POST", "/cli/connect", c.resp)
		result := r.run(t, connectKey, "--store", "unsafe", "--ttl", "1h30m")
		if result.Code != c.code || result.Stdout != "" || result.Stderr != "error: "+c.message+"\n" {
			t.Errorf("response %+v: %+v", c.resp, result)
		}
		if got := r.file(t); got != old {
			t.Errorf("previous config changed: %+v", got)
		}
		raw, _ := os.ReadFile(secretPath)
		if string(raw) != "old token\n" {
			t.Error("old token was replaced on failure")
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
	r.vars[config.EnvAPIURL] = "https://agent.cloud.appifyhub.com"
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

func statusSnapshot(t *testing.T, home string) string {
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

func runStatus(t *testing.T, r *connectRig) testkit.Result {
	t.Helper()
	before := statusSnapshot(t, r.home)
	got := testkit.Run(t, r.deps, "status", "-v")
	if statusSnapshot(t, r.home) != before || len(r.keychain.Calls) != 0 || len(r.terminal.Prompts) != 0 || r.backend.Count() != 0 {
		t.Error("status accessed credentials, prompted, contacted network or changed files")
	}
	testkit.AssertNoSecret(t, "not-a-token", got.Stdout, got.Stderr)
	testkit.AssertNoSecret(t, "secretphrase", got.Stdout, got.Stderr)
	return got
}

func TestStatusConnectedSample(t *testing.T) {
	r := statusRig(t, "darwin", "keychain")
	r.deps.Build.APIURL = r.vars[config.EnvAPIURL]
	got := runStatus(t, r)
	want := "Connected as Example User (workstation)\n  Device ID: 6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b\n  Token: macOS Keychain, expires 2027-02-14 10:21 (in 3 months)\n  Default chat: 3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7 (telegram · Example Chat, private)\n  Backend: https://agent.cloud.appifyhub.com\n"
	if got.Code != 0 || got.Stdout != want || got.Stderr != "" {
		t.Fatal(got)
	}
	testkit.Golden(t, "status_connected", got.Stdout)
	help := testkit.Run(t, r.deps, "status", "--help")
	if !strings.Contains(help.Stdout, strings.TrimSuffix(got.Stdout, "\n")) {
		t.Error("sample in help differs from actual status output")
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
			got := runStatus(t, r)
			if got.Code != 3 || got.Stderr != "" || got.Stdout != "Not connected.\nGet a connect key from the agent (send /settings, open Command Line), then run `agent-cli connect <KEY>`.\n" {
				t.Fatal(got)
			}
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
			got := runStatus(t, r)
			if got.Code != c.code || got.Stderr != "" || !strings.Contains(got.Stdout, "  Token: "+c.location+", expires 2027-02-14 10:21 (in 3 months)\n") {
				t.Fatal(got)
			}
			if c.mode == "passphrase" {
				hint := `  To unlock: eval "$(agent-cli unlock)"`
				if c.goos == "windows" {
					hint = "  To unlock: agent-cli unlock --shell pwsh | Invoke-Expression"
				}
				if !strings.HasSuffix(got.Stdout, hint+"\n") {
					t.Fatal(got)
				}
			}
			r.deps.Now = func() time.Time { return connectExpiry.Add(48 * time.Hour) }
			got = runStatus(t, r)
			if got.Code != 3 || !strings.Contains(got.Stdout, "expired on 2027-02-14 10:21 (2 days ago)") || !strings.HasSuffix(got.Stdout, "  Run `agent-cli connect <KEY>` to connect again.\n") {
				t.Fatal(got)
			}
		})
	}
}

func TestStatusDefaultsAndBackend(t *testing.T) {
	for _, c := range []struct {
		name, chat, platform string
		stored               config.Default
		want                 string
	}{
		{"group", "", "", config.Default{ChatID: "c9", ChatPlatform: "telegram", ChatTitle: "Dev Team"}, "  Default chat: c9 (telegram · Dev Team, group)"},
		{"platform", "", "", config.Default{Platform: "whatsapp"}, "  Default platform: whatsapp"},
		{"none", "", "", config.Default{}, "  Default chat: none (latest chat is used)"},
		{"env chat", "c7", "telegram", config.Default{ChatID: "c9"}, "  Default chat: c7 (from THE_AGENT_CLI_DEFAULT_CHAT)"},
		{"env platform", "", "WhatsApp", config.Default{ChatID: "c9"}, "  Default platform: whatsapp (from THE_AGENT_CLI_DEFAULT_PLATFORM)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := statusRig(t, "linux", "keychain")
			f := r.file(t)
			f.Default = c.stored
			r.save(t, f)
			r.vars[config.EnvDefaultChat], r.vars[config.EnvDefaultPlatform] = c.chat, c.platform
			got := runStatus(t, r)
			if got.Code != 0 || got.Stderr != "" || !strings.Contains(got.Stdout, c.want+"\n") {
				t.Fatal(got)
			}
		})
	}
	r := statusRig(t, "linux", "keychain")
	delete(r.vars, config.EnvAPIURL)
	if got := runStatus(t, r); !strings.Contains(got.Stdout, "  Backend: https://stored.example\n") {
		t.Fatal(got)
	}
	f := r.file(t)
	f.APIURL = ""
	r.save(t, f)
	if got := runStatus(t, r); !strings.Contains(got.Stdout, "  Backend: http://localhost\n") {
		t.Fatal(got)
	}
	r.deps.Build.APIURL = ""
	if got := runStatus(t, r); !strings.Contains(got.Stdout, "  Backend: not set\n") {
		t.Fatal(got)
	}
	f.Token.UserName, f.Token.Hostname, f.Default.ChatID = "\x1b[31mMorpheus", "host\x1b[0m", "c\x00"
	f.Default.ChatTitle = "\x1b]0;bad\x07Title"
	r.save(t, f)
	if got := runStatus(t, r); strings.ContainsAny(got.Stdout, "\x1b\x00\x07") {
		t.Fatal(got)
	}
}

func TestStatusEnvironmentToken(t *testing.T) {
	for _, c := range []struct {
		name string
		exp  any
		code int
		want string
	}{
		{"future", connectExpiry.Unix(), 0, "expires 2027-02-14 10:21 (in 3 months)"},
		{"past", time.Date(2026, 11, 12, 9, 21, 0, 0, time.UTC).Unix(), 3, "expired on 2026-11-12 10:21 (2 days ago)"},
		{"absent", nil, 0, "  Token: THE_AGENT_CLI_TOKEN environment variable\n"},
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
			got := runStatus(t, r)
			if got.Code != c.code || got.Stderr != "" || !strings.HasPrefix(got.Stdout, "Connected using THE_AGENT_CLI_TOKEN\n") || !strings.Contains(got.Stdout, c.want) || strings.Contains(got.Stdout, "To unlock") {
				t.Fatal(got)
			}
			testkit.AssertNoSecret(t, raw, got.Stdout, got.Stderr)
		})
	}
	r := newConnectRig(t, "linux")
	r.vars[config.EnvToken] = "garbage"
	got := runStatus(t, r)
	if got.Code != 3 || got.Stdout != "" || got.Stderr != "error: THE_AGENT_CLI_TOKEN is not a valid CLI token\n" {
		t.Fatal(got)
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
			{"policies", testkit.Err(403, 3008, "policies"), 1, "error: accept the agent's policies first; send /settings to the agent, then try again\n", false},
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
