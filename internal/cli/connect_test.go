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
	b.Handle("POST", "/cli/connect", testkit.ConnectOK(connectSecret, connectExpiry, "u1", "Milos", 500))
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
			want := "✓ Connected as Milos (milos-mbp)\n  Token stored in: " + store + "\n  Expires: 2027-02-14 10:21 (in 3 months)\n  A new connect key was generated for your profile.\n"
			if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
				t.Fatalf("got %+v; want %q", result, want)
			}
			f := r.file(t)
			if f.Token.UserID != "u1" || f.Token.Store != "keychain" || f.Token.Hostname != "milos-mbp" || f.APIURL != r.backend.URL() || !f.Token.ExpiresAt.Equal(connectExpiry) {
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
				want := "✓ Connected as Milos (milos-mbp)\n  Token stored in: " + path + suffix + "\n  Expires: 2027-02-14 10:21 (in 3 months)\n  A new connect key was generated for your profile.\n"
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
				return testkit.ConnectOK(connectSecret, connectExpiry, "u1", "Milos", 500)
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
		r.backend.Handle("POST", "/cli/connect", testkit.ConnectOK(connectSecret, connectExpiry, "u1", "Milos", c.balance))
		result := r.run(t, connectKey, "--store", "unsafe", "-v")
		if result.Code != 0 || !strings.HasPrefix(result.Stderr, "Calling backend…\n") {
			t.Errorf("got %+v", result)
		}
		if c.warning == "" && result.Stderr != "Calling backend…\n" || c.warning != "" && !strings.Contains(result.Stderr, c.warning) {
			t.Errorf("wrong warning %q", result.Stderr)
		}
		if !strings.HasPrefix(result.Stdout, "✓ Connected as Milos") {
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
