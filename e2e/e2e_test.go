// Package e2e_test runs the binaries built by tools/build, one per build variant, on the current OS.
// scripts/e2e.sh builds them and points the tests at them through binDirEnv; without it the tests skip.
package e2e_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

const binDirEnv = "THE_AGENT_CLI_E2E_BIN_DIR"

type result struct {
	code           int
	stdout, stderr string
}

// variants reads the build list from tools/variants, the one place the builds are defined.
func variants(t *testing.T) []string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "tools", "variants"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^VARIANTS="([^"]+)"$`).FindStringSubmatch(string(content))
	if match == nil {
		t.Fatal("tools/variants defines no VARIANTS")
	}
	return strings.Fields(match[1])
}

func version(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(content))
}

func binary(t *testing.T, name string) string {
	t.Helper()
	dir := os.Getenv(binDirEnv)
	if dir == "" {
		t.Skip(binDirEnv + " is not set; run scripts/e2e.sh")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("..", dir)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing binary: %v", err)
	}
	return path
}

// execute runs the binary with piped streams and the given extra variables, without any THE_AGENT_CLI_*
// variable from the caller, and checks that the output is plain UTF-8 text.
func execute(t *testing.T, bin string, env []string, args ...string) result {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bin, args...)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "THE_AGENT_CLI_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, env...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	r := result{}
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		r.code = exitErr.ExitCode()
	default:
		t.Fatal(err)
	}
	r.stdout, r.stderr = stdout.String(), stderr.String()
	for _, out := range []string{r.stdout, r.stderr} {
		if strings.ContainsRune(out, '\x1b') {
			t.Errorf("%v: escape sequence in piped output %q", args, out)
		}
		if !utf8.ValidString(out) {
			t.Errorf("%v: output is not valid UTF-8: %q", args, out)
		}
	}
	return r
}

// run executes the binary with a home directory that must stay absent: informational commands write nothing.
func run(t *testing.T, bin string, args ...string) result {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	r := execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home}, args...)
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%v: created the home directory", args)
	}
	return r
}

// builtInURL is the backend address tools/build bakes into a variant, read from tools/variants.
func builtInURL(t *testing.T, name string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `. ./tools/variants && load_variant "$1" && printf %s "$VARIANT_API_URL"`, "sh", name)
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		t.Fatalf("tools/variants gives no backend URL for %s: %v", name, err)
	}
	return string(out)
}

func TestBinaries(t *testing.T) {
	ver := version(t)
	for _, name := range variants(t) {
		t.Run(name, func(t *testing.T) {
			bin := binary(t, name)
			quoted := regexp.QuoteMeta(name)

			t.Run("version", func(t *testing.T) {
				r := run(t, bin, "--version")
				want := regexp.MustCompile(`^` + quoted + ` ` + regexp.QuoteMeta(ver) + ` \(` + runtime.GOOS + `/` + runtime.GOARCH + `, \d{4}-\d{2}-\d{2}\)\n$`)
				if r.code != 0 || r.stderr != "" || !want.MatchString(r.stdout) {
					t.Errorf("code %d, stdout %q, stderr %q; want %s", r.code, r.stdout, r.stderr, want)
				}
				r = run(t, bin, "--version", "-v")
				want = regexp.MustCompile(`\n  commit: [0-9a-f]{7,}\n$`)
				if r.code != 0 || !want.MatchString(r.stdout) {
					t.Errorf("verbose: code %d, stdout %q; want the build commit", r.code, r.stdout)
				}
			})

			t.Run("root help", func(t *testing.T) {
				r := run(t, bin)
				for _, part := range []string{name + " – ", "\nUsage: " + name + " <command> [flags]\n", "\nCommands:\n", "  completion  ", "  help  ", "\nExit codes: "} {
					if !strings.Contains(r.stdout, part) {
						t.Errorf("root help lacks %q:\n%s", part, r.stdout)
					}
				}
				if r.code != 0 || r.stderr != "" {
					t.Errorf("code %d, stderr %q", r.code, r.stderr)
				}
				if h := run(t, bin, "--help"); h != r {
					t.Errorf("--help differs from the bare command: %+v", h)
				}
			})

			t.Run("command help", func(t *testing.T) {
				viaHelp, viaFlag := run(t, bin, "help", "completion"), run(t, bin, "completion", "-h")
				if viaHelp.code != 0 || viaHelp != viaFlag || !strings.Contains(viaHelp.stdout, "Usage: "+name+" completion") {
					t.Errorf("help completion %+v, completion -h %+v", viaHelp, viaFlag)
				}
			})

			t.Run("completion", func(t *testing.T) {
				for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
					r := run(t, bin, "completion", shell)
					if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, name) {
						t.Errorf("%s: code %d, stderr %q, script mentions %s: %t", shell, r.code, r.stderr, name, strings.Contains(r.stdout, name))
					}
				}
			})

			t.Run("env", func(t *testing.T) {
				userHome := t.TempDir()
				base := []string{"HOME=" + userHome, "USERPROFILE=" + userHome}
				r := execute(t, bin, base, "env")
				want := []string{
					"THE_AGENT_CLI_API_URL: not set, using " + builtInURL(t, name) + "\n",
					"THE_AGENT_CLI_HOME: not set, using ~" + string(filepath.Separator) + "." + name + "\n",
				}
				for _, line := range want {
					if !strings.Contains(r.stdout, "\n"+line) && !strings.HasPrefix(r.stdout, line) {
						t.Errorf("env lacks the line %q:\n%s", line, r.stdout)
					}
				}
				if r.code != 0 || r.stderr != "" {
					t.Errorf("code %d, stderr %q", r.code, r.stderr)
				}

				r = execute(t, bin, append(base, "THE_AGENT_CLI_API_URL=https://override.example.test/"), "env")
				if line := "THE_AGENT_CLI_API_URL: set to https://override.example.test\n"; !strings.HasPrefix(r.stdout, line) {
					t.Errorf("env does not show the override %q:\n%s", line, r.stdout)
				}
				if entries, err := os.ReadDir(userHome); err != nil || len(entries) != 0 {
					t.Errorf("env created files in the user home: %v, %v", entries, err)
				}
			})

			t.Run("connect", func(t *testing.T) {
				for _, args := range [][]string{{"status", "--help"}, {"help", "status"}} {
					r := run(t, bin, args...)
					if r.code != 0 || !strings.Contains(r.stdout, "Usage: "+name+" status") || r.stderr != "" {
						t.Errorf("status help: %+v", r)
					}
				}
				if r := run(t, bin, "status"); r.code != 3 || r.stderr != "" || !strings.Contains(r.stdout, "`"+name+" connect <KEY>`") {
					t.Errorf("fresh status: %+v", r)
				}
				for _, args := range [][]string{{"revoke-all", "--help"}, {"help", "revoke-all"}} {
					r := run(t, bin, args...)
					if r.code != 0 || !strings.Contains(r.stdout, "Usage: "+name+" revoke-all") || r.stderr != "" {
						t.Errorf("revoke help: %+v", r)
					}
				}
				for _, args := range [][]string{{"connect", "--help"}, {"help", "connect"}} {
					r := run(t, bin, args...)
					if r.code != 0 || !strings.Contains(r.stdout, "Usage: "+name+" connect <KEY>") || r.stderr != "" {
						t.Errorf("help: %+v", r)
					}
				}
				if r := run(t, bin, "connect"); r.code != 2 || !strings.HasPrefix(r.stderr, "error: missing connect key;") {
					t.Errorf("missing key: %+v", r)
				}
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete && r.URL.Path == "/cli/user/u1/devices" && r.Header.Get("Authorization") == "Bearer test-token" {
						w.WriteHeader(http.StatusNoContent)
						return
					}
					var request map[string]any
					if r.Method != http.MethodPost || r.URL.Path != "/cli/connect" || r.Header.Get("Authorization") != "" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request["connect_key"] != "TEST-KEY" || request["device_id"] == "" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"token":"test-token","expires_at":"2027-02-14T09:21:00Z","user_id":"u1","user_display_name":"Tester","credit_balance":500}`)
				}))
				defer backend.Close()
				home := filepath.Join(t.TempDir(), "state")
				r := execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home, "THE_AGENT_CLI_API_URL=" + backend.URL}, "connect", "test-key", "--store", "unsafe")
				if r.code != 0 || r.stderr != "" || !strings.HasPrefix(r.stdout, "✓ Connected as Tester (") {
					t.Fatalf("connect: %+v", r)
				}
				for _, secret := range []string{"test-token", "TEST-KEY", "test-key"} {
					if strings.Contains(r.stdout+r.stderr, secret) {
						t.Error("secret in command output")
					}
				}
				secret, err := os.ReadFile(filepath.Join(home, "credentials"))
				if err != nil || string(secret) != "test-token\n" {
					t.Errorf("stored credentials: %v", err)
				}
				metadata, err := os.ReadFile(filepath.Join(home, "config.toml"))
				if err != nil || !strings.Contains(string(metadata), `user_id = 'u1'`) || strings.Contains(string(metadata), "test-token") {
					t.Errorf("connection metadata: %q, %v", metadata, err)
				}
				r = execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home}, "status", "-v")
				if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, "Connected as Tester (") || !strings.Contains(r.stdout, "(NOT encrypted)") || !strings.Contains(r.stdout, "  Backend: "+backend.URL+"\n") || strings.Contains(r.stdout, "test-token") {
					t.Fatalf("offline status: %+v", r)
				}
				afterStatus, err := os.ReadFile(filepath.Join(home, "config.toml"))
				if err != nil || string(afterStatus) != string(metadata) {
					t.Error("status changed config")
				}
				r = execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home}, "revoke-all")
				if r.code != 2 || r.stdout != "" || r.stderr != "error: refusing to revoke without confirmation; pass --yes\n" {
					t.Fatalf("noninteractive revoke: %+v", r)
				}
				r = execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home}, "revoke-all", "--yes")
				if r.code != 0 || r.stdout != "✓ Access revoked for all CLIs. Local token removed.\n" || r.stderr != "" {
					t.Fatalf("revoke: %+v", r)
				}
				if _, err := os.Stat(filepath.Join(home, "credentials")); !os.IsNotExist(err) {
					t.Error("revoke kept credentials")
				}
				metadata, err = os.ReadFile(filepath.Join(home, "config.toml"))
				if err != nil || strings.Contains(string(metadata), "[token]") || !strings.Contains(string(metadata), "device_id") {
					t.Error("incorrect metadata after revoke")
				}
				r = execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home}, "status")
				if r.code != 3 || !strings.HasPrefix(r.stdout, "Not connected.\n") || r.stderr != "" {
					t.Fatalf("status after revoke: %+v", r)
				}
			})

			t.Run("chats", func(t *testing.T) {
				for _, path := range [][]string{{"chats"}, {"chats", "default"}} {
					viaHelp := run(t, bin, append([]string{"help"}, path...)...)
					viaFlag := run(t, bin, append(append([]string{}, path...), "--help")...)
					if viaHelp.code != 0 || viaHelp != viaFlag || viaHelp.stderr != "" || !strings.Contains(viaHelp.stdout, "Usage: "+name+" "+strings.Join(path, " ")) {
						t.Fatalf("chats help: %+v / %+v", viaHelp, viaFlag)
					}
					for _, variable := range []string{"THE_AGENT_CLI_DEFAULT_CHAT", "THE_AGENT_CLI_DEFAULT_PLATFORM"} {
						if !strings.Contains(viaHelp.stdout, variable) {
							t.Errorf("%v help omits %s", path, variable)
						}
					}
				}
				for _, args := range [][]string{{"chats"}, {"chats", "default", "--set", "c1"}} {
					got := run(t, bin, args...)
					if got.code != 3 || got.stdout != "" || got.stderr != "This CLI is not connected. Run `"+name+" connect <KEY>`.\n" {
						t.Fatalf("not connected: %+v", got)
					}
				}
				for _, args := range [][]string{
					{"chats", "stray"},
					{"chats", "default", "stray"},
					{"chats", "default", "--set", ""},
					{"chats", "default", "--clear", "--platform", "telegram"},
					{"chats", "default", "--platform", "slack"},
				} {
					got := run(t, bin, args...)
					if got.code != 2 || got.stdout != "" || !strings.HasPrefix(got.stderr, "error: ") {
						t.Fatalf("usage validation: %+v", got)
					}
				}
				tip := "Tip: change it with `" + name + " chats default --set <chat-id> | --platform telegram|whatsapp | --clear`.\n"
				if got := run(t, bin, "chats", "default"); got.code != 0 || got.stderr != "" || got.stdout != "No default chat is set; notifications go to your latest chat.\n"+tip {
					t.Fatalf("fresh default: %+v", got)
				}
				const chatID = "3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7"
				secret := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"neo"}`)) + ".signature"
				var hits atomic.Int32
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					hits.Add(1)
					if request.Method != http.MethodGet || request.URL.Path != "/cli/chats" || request.Header.Get("Authorization") != "Bearer "+secret || !strings.HasPrefix(request.Header.Get("User-Agent"), name+"/") {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `[{"chat_id":"`+chatID+`","platform":"telegram","title":"Morpheus","is_private":true},{"chat_id":"c2","platform":"matrix","title":null,"is_private":false}]`)
				}))
				defer backend.Close()
				home := filepath.Join(t.TempDir(), "state")
				env := []string{"THE_AGENT_CLI_HOME=" + home, "THE_AGENT_CLI_API_URL=" + backend.URL, "THE_AGENT_CLI_TOKEN=" + secret}
				got := execute(t, bin, env, "chats")
				if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, "Morpheus") || !strings.Contains(got.stdout, "matrix") || !strings.Contains(got.stdout, "untitled") || hits.Load() != 1 {
					t.Fatalf("listing: %+v", got)
				}
				if _, err := os.Stat(home); !os.IsNotExist(err) {
					t.Error("environment-token listing created state")
				}
				got = execute(t, bin, env, "chats", "default", "--set", strings.ToUpper(chatID))
				if got.code != 0 || got.stderr != "" || got.stdout != "✓ Default chat set to "+chatID+" (telegram · Morpheus, private)\n" || hits.Load() != 2 {
					t.Fatalf("set chat default: %+v", got)
				}
				metadata, err := os.ReadFile(filepath.Join(home, "config.toml"))
				if err != nil || !strings.Contains(string(metadata), chatID) || !strings.Contains(string(metadata), "Morpheus") || strings.Contains(string(metadata), secret) {
					t.Fatalf("cached chat: %q, %v", metadata, err)
				}
				got = execute(t, bin, env, "chats")
				if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, "\n* "+chatID+" ") || hits.Load() != 3 {
					t.Fatalf("default marker: %+v", got)
				}
				got = execute(t, bin, env, "chats", "default", "--set", "nope")
				if got.code != 2 || got.stdout != "" || got.stderr != "error: chat \"nope\" is not one of your chats; run `"+name+" chats` to list them\n" || hits.Load() != 4 {
					t.Fatalf("unknown chat: %+v", got)
				}
				after, err := os.ReadFile(filepath.Join(home, "config.toml"))
				if err != nil || string(after) != string(metadata) {
					t.Error("unknown chat changed the config")
				}
				backend.Close()
				offline := []string{"THE_AGENT_CLI_HOME=" + home, "THE_AGENT_CLI_API_URL=invalid", "THE_AGENT_CLI_TOKEN=invalid"}
				got = execute(t, bin, offline, "chats", "default", "-v")
				if got.code != 0 || got.stderr != "" || got.stdout != "Default chat: "+chatID+" (telegram · Morpheus, private)\n"+tip {
					t.Fatalf("offline cached default: %+v", got)
				}
				got = execute(t, bin, offline, "chats", "default", "--platform", "WhatsApp")
				if got.code != 0 || got.stderr != "" || got.stdout != "✓ Default platform set to whatsapp\n" {
					t.Fatalf("offline set platform: %+v", got)
				}
				got = execute(t, bin, offline, "chats", "default")
				if got.code != 0 || got.stderr != "" || got.stdout != "Default platform: whatsapp\n"+tip {
					t.Fatalf("offline platform: %+v", got)
				}
				got = execute(t, bin, append(offline, "THE_AGENT_CLI_DEFAULT_CHAT=c7"), "chats", "default", "--clear")
				if got.code != 0 || got.stdout != "✓ Default cleared\n" || got.stderr != "warning: THE_AGENT_CLI_DEFAULT_CHAT is set and overrides the stored default\n" {
					t.Fatalf("offline clear with override: %+v", got)
				}
				got = execute(t, bin, offline, "chats", "default")
				if got.code != 0 || got.stderr != "" || got.stdout != "No default chat is set; notifications go to your latest chat.\n"+tip {
					t.Fatalf("cleared default: %+v", got)
				}
				got = execute(t, bin, offline, "chats", "default", "--clear")
				if got.code != 0 || got.stderr != "" || got.stdout != "✓ Default cleared\n" || hits.Load() != 4 {
					t.Fatalf("idempotent offline clear: %+v", got)
				}
				metadata, err = os.ReadFile(filepath.Join(home, "config.toml"))
				if err != nil || strings.Contains(string(metadata), "[default]") || strings.Contains(string(metadata), secret) {
					t.Fatalf("cleared config: %q, %v", metadata, err)
				}
			})

			t.Run("server", func(t *testing.T) {
				var hits atomic.Int32
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits.Add(1)
					if r.URL.Path != "/health" || r.Header.Get("Authorization") != "" || !strings.HasPrefix(r.Header.Get("User-Agent"), name+"/") {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					_, _ = io.WriteString(w, `{"status":"ok","version":"9.9.9","region":"eu"}`)
				}))
				defer backend.Close()

				home := filepath.Join(t.TempDir(), "home")
				r := execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home, "THE_AGENT_CLI_API_URL=" + backend.URL}, "server")
				host := strings.TrimPrefix(backend.URL, "http://")
				report := regexp.MustCompile(`^Server    ` + regexp.QuoteMeta(host) + `\nStatus    ok\nVersion   9\.9\.9\nregion    eu\nResponse  \d+ ms\n$`)
				if r.code != 0 || r.stderr != "" || !report.MatchString(r.stdout) {
					t.Errorf("code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
				}
				if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
					t.Error("server created the home directory")
				}

				backend.Close()
				r = execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home, "THE_AGENT_CLI_API_URL=" + backend.URL}, "server")
				if r.code != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, "error: could not reach the server at "+backend.URL+": ") {
					t.Errorf("closed port: code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
				}

				before := hits.Load()
				r = execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + home, "THE_AGENT_CLI_API_URL=http://agent.example.test"}, "server")
				if r.code != 2 || r.stdout != "" || !strings.HasPrefix(r.stderr, "error: invalid API URL") || hits.Load() != before {
					t.Errorf("remote http: code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
				}
			})

			t.Run("usage errors exit 2", func(t *testing.T) {
				cases := []struct {
					args   []string
					stderr string
				}{
					{[]string{"nope"}, "error: unknown command \"nope\" for \"" + name + "\"\n"},
					{[]string{"completion", "tcsh"}, "error: unsupported shell \"tcsh\"; use bash, zsh, fish or powershell\n"},
					{[]string{"--nope"}, ""},
					{[]string{"env", "extra"}, ""},
				}
				for _, c := range cases {
					r := run(t, bin, c.args...)
					if r.code != 2 || r.stdout != "" || !strings.HasPrefix(r.stderr, "error: ") || (c.stderr != "" && r.stderr != c.stderr) {
						t.Errorf("%v: code %d, stdout %q, stderr %q", c.args, r.code, r.stdout, r.stderr)
					}
				}
			})
		})
	}
}

func TestNotifyBuilds(t *testing.T) {
	for _, name := range variants(t) {
		t.Run(name, func(t *testing.T) {
			bin := binary(t, name)
			help := run(t, bin, "notify", "--help")
			if help.code != 0 || !strings.Contains(help.stdout, "Usage: "+name+" notify") || !strings.Contains(help.stdout, "THE_AGENT_CLI_DEFAULT_CHAT") {
				t.Fatal(help)
			}
			for _, args := range [][]string{{"notify", "hi"}, {"notify", "--chat", "c1", "hi"}} {
				got := run(t, bin, args...)
				if got.code != 3 || got.stderr != "This CLI is not connected. Run \x60"+name+" connect <KEY>\x60.\n" {
					t.Fatal(got)
				}
			}
			got := run(t, bin, "notify", "--chat", "c1", "--platform", "telegram", "hi")
			if got.code != 2 {
				t.Fatal(got)
			}
			secret := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"neo"}`)) + ".signature"
			var hits atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || r.URL.Path != "/cli/notify/user/neo" || r.Header.Get("Authorization") != "Bearer "+secret || body["message"] != "Backup finished" || body["platform"] != "whatsapp" {
					t.Errorf("request %s %s %+v", r.Method, r.URL.Path, body)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_, _ = io.WriteString(w, `{"chat_id":"c1","platform":"whatsapp","title":"Morpheus","is_private":true,"credit_balance":42.37}`)
			}))
			defer backend.Close()
			home := filepath.Join(t.TempDir(), "absent")
			env := []string{"THE_AGENT_CLI_HOME=" + home, "THE_AGENT_CLI_API_URL=" + backend.URL, "THE_AGENT_CLI_TOKEN=" + secret}
			got = execute(t, bin, env, "notify", "--platform", "whatsapp", "Backup finished", "-v")
			warning := "warning: low credit balance: 42.37 credits left; top up in the agent's chat settings\n"
			if got.code != 0 || got.stdout != "✓ Sent to WhatsApp · Morpheus (private)\n" || got.stderr != "Calling backend…\n"+warning {
				t.Fatal(got)
			}
			cmd := exec.CommandContext(t.Context(), bin, "notify", "--platform", "whatsapp")
			for _, kv := range os.Environ() {
				if !strings.HasPrefix(kv, "THE_AGENT_CLI_") {
					cmd.Env = append(cmd.Env, kv)
				}
			}
			cmd.Env = append(cmd.Env, env...)
			cmd.Stdin = strings.NewReader("Backup finished\n")
			var out, errOut strings.Builder
			cmd.Stdout = &out
			cmd.Stderr = &errOut
			if err := cmd.Run(); err != nil || out.String() != "" || errOut.String() != warning {
				t.Fatalf("pipe: %v %q %q", err, out.String(), errOut.String())
			}
			if hits.Load() != 2 {
				t.Fatal(hits.Load())
			}
			if _, err := os.Stat(home); !os.IsNotExist(err) {
				t.Fatal("notify created state", err)
			}
		})
	}
}

func TestUnlockBuilds(t *testing.T) {
	for _, name := range variants(t) {
		t.Run(name, func(t *testing.T) {
			bin := binary(t, name)
			help := run(t, bin, "unlock", "--help")
			if help.code != 0 || !strings.Contains(help.stdout, "Usage: "+name+" unlock") || !strings.Contains(help.stdout, "THE_AGENT_CLI_PASSPHRASE") {
				t.Fatal(help)
			}
			got := run(t, bin, "unlock")
			if got.code != 3 || got.stdout != "" || got.stderr != "This CLI is not connected. Run \x60"+name+" connect <KEY>\x60.\n" {
				t.Fatal(got)
			}
			got = run(t, bin, "unlock", "--shell", "csh")
			if got.code != 2 || got.stdout != "" {
				t.Fatal(got)
			}
			got = execute(t, bin, []string{"THE_AGENT_CLI_HOME=" + filepath.Join(t.TempDir(), "absent"), "THE_AGENT_CLI_TOKEN=already-set"}, "unlock")
			if got.code != 0 || got.stdout != "" || got.stderr != "Already unlocked: THE_AGENT_CLI_TOKEN is set in this environment.\n" {
				t.Fatal(got)
			}
			completion := run(t, bin, "__complete", "unlock", "--shell", "")
			if completion.code != 0 || !strings.Contains(completion.stdout, "fish\npwsh\npowershell\n") {
				t.Fatal(completion)
			}
		})
	}
}
