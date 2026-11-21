package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

func TestAcceptanceJourney(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			r := newConnectRig(t, goos)
			secret := testkit.NewToken(testkit.TokenClaims{Sub: "u1", Exp: connectExpiry, Hostname: "morpheus-mbp"})
			r.backend.Handle(http.MethodPost, "/cli/connect", testkit.ConnectOK(secret, connectExpiry, "u1", "Morpheus", 500))
			userHome, err := r.deps.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			cwd := filepath.Join(userHome, "project", "nested")
			if err := os.MkdirAll(cwd, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(userHome, "project", ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			r.deps.Getwd = func() (string, error) { return cwd, nil }
			run := func(code int, stdout, stderr string, args ...string) testkit.Result {
				t.Helper()
				got := testkit.Run(t, r.deps, args...)
				if got.Code != code || got.Stdout != stdout || got.Stderr != stderr {
					t.Fatalf("%v\n got: %#v\nwant: %#v", args, got, testkit.Result{Code: code, Stdout: stdout, Stderr: stderr})
				}
				testkit.AssertNoSecret(t, connectKey, got.Stdout, got.Stderr)
				testkit.AssertNoSecret(t, "secretphrase", got.Stdout, got.Stderr)
				if args[0] != "unlock" {
					testkit.AssertNoSecret(t, secret, got.Stdout, got.Stderr)
				}
				return got
			}
			lastChecked := statusNever
			checkStatus := func(code int, stdout string) {
				t.Helper()
				local := runOfflineStatus(t, r)
				want := testkit.Result{Code: code, Stdout: stdout + lastChecked}
				if local != want {
					t.Fatalf("offline status = %#v; want %#v", local, want)
				}
				online := runHealthyStatus(t, r, r.backend, local)
				lastChecked = "  Last checked      " + r.deps.Now().In(r.deps.Location).Format("2006-01-02 15:04") + "\n"
				if !strings.HasSuffix(online.Stdout, lastChecked) {
					t.Fatalf("online status = %+v; want completion %q", online, lastChecked)
				}
				testkit.AssertNoSecret(t, secret, local.Stdout, local.Stderr, online.Stdout, online.Stderr)
			}
			goldenOS := "unix"
			storagePath := "~/.agent-cli/credentials"
			unlockCommand := `eval "$(agent-cli unlock)"`
			keychainLocation := "system keyring (Secret Service)"
			if goos == "darwin" {
				keychainLocation = "macOS Keychain"
			}
			if goos == "windows" {
				goldenOS = "windows"
				storagePath = `~\.agent-cli\credentials`
				unlockCommand = "agent-cli unlock --shell pwsh | Invoke-Expression"
				keychainLocation = "Windows Credential Manager"
			}
			help, err := os.ReadFile(filepath.Join("testdata", "root_help_"+goldenOS+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			run(0, string(help), "", "--help")
			if r.backend.Count() != 0 {
				t.Fatal("help contacted the backend")
			}
			statusBackend := "  Backend           " + strings.TrimPrefix(r.backend.URL(), "http://") + "\n  Local CLI         " + r.deps.Build.Version + "\n"
			notConnected := "Not connected.\nGet a connect key from the agent's chat settings, then run `agent-cli connect <KEY>`.\n" + statusBackend
			checkStatus(3, notConnected)
			if _, err := os.Stat(config.Path(r.home)); !os.IsNotExist(err) {
				t.Fatalf("status created config.toml: %v", err)
			}

			var deviceID string
			for _, store := range []string{"unsafe", "keychain", "passphrase"} {
				location, statusLocation := storagePath+" (NOT encrypted)", storagePath+" (NOT encrypted)"
				hint, statusHint, statusCode := "", "", 0
				if store == "keychain" {
					location, statusLocation = keychainLocation, keychainLocation
				}
				if store == "passphrase" {
					r.terminal.TTYIn = true
					r.terminal.Secrets = []string{"secretphrase", "secretphrase"}
					location = storagePath + " (encrypted)"
					statusLocation = storagePath + " (encrypted, locked)"
					hint = "  To unlock once per terminal session: " + unlockCommand + "\n"
					statusHint, statusCode = "  To unlock: "+unlockCommand+"\n", 3
				}
				run(0, "✓ Connected as Morpheus (morpheus-mbp)\n  Token stored in: "+location+"\n  Expires: 2027-02-14 10:21 (in 3 months)\n  A new connect key was generated for your profile.\n"+hint, "", "connect", connectKey, "--ttl", "3mo", "--store", store)
				file := r.file(t)
				if deviceID == "" {
					deviceID = file.DeviceID
				}
				if file.DeviceID != deviceID || file.Token.Store != store {
					t.Fatal("reconnect changed the device or failed to select the store", file)
				}
				if store == "passphrase" && !reflect.DeepEqual(r.terminal.Prompts, []string{"Choose a passphrase: ", "Repeat passphrase: "}) {
					t.Fatalf("passphrase prompts: %q", r.terminal.Prompts)
				}
				if store == "keychain" {
					if _, err := os.Stat(filepath.Join(r.home, "credentials")); !os.IsNotExist(err) {
						t.Fatalf("reconnect kept the previous file secret: %v", err)
					}
				} else if r.keychain.Len() != 0 {
					t.Fatal("reconnect kept the previous keychain secret")
				}
				checkStatus(statusCode, "Connected as Morpheus (morpheus-mbp)\n  Device ID         "+deviceID+"\n  Token storage     "+statusLocation+"\n"+
					"  Token expiry      2027-02-14 10:21 (in 3 months)\n  Default chat      none (latest chat is used)\n"+statusBackend+statusHint)
			}
			r.terminal.TTYIn = false
			r.vars[config.EnvPassphrase] = "secretphrase"
			private, group := "Morpheus", "Dev Team"
			privateChat := testkit.ChatJSON(chatsSecondID, "whatsapp", &private, true)
			groupChat := testkit.ChatJSON(chatsGroupID, "telegram", &group, false)
			r.backend.Handle(http.MethodGet, "/cli/chats", testkit.JSON(200, []map[string]any{
				testkit.ChatJSON(chatsFirstID, "telegram", &private, true), privateChat, groupChat,
			}))
			table := "  ID                                PLATFORM  TITLE     PRIVATE   (* = default)\n" +
				"  " + chatsFirstID + "  telegram  Morpheus  yes\n" +
				"  " + chatsSecondID + "  whatsapp  Morpheus  yes\n" +
				"  " + chatsGroupID + "  telegram  Dev Team  no\n"
			run(0, table, "", "chats")
			run(0, "✓ Default chat set to "+chatsFirstID+" (telegram · Morpheus, private)\n", "", "chats", "default", "--set", strings.ToUpper(chatsFirstID))
			run(0, strings.Replace(table, "\n  "+chatsFirstID, "\n* "+chatsFirstID, 1), "", "chats")
			run(0, "Default chat: "+chatsFirstID+" (telegram · Morpheus, private)\nTip: change it with `agent-cli chats default --set <chat-id> | --platform telegram|whatsapp | --clear`.\n", "", "chats", "default")
			run(2, "", "error: chat \"nope\" is not one of your chats; run `agent-cli chats` to list them\n", "chats", "default", "--set", "nope")

			for _, target := range []struct {
				chat, platform string
				flags          []string
				path, body     string
			}{
				{"", "", nil, "/cli/notify/chat/" + chatsFirstID, `{"message":"Build finished"}`},
				{chatsSecondID, "telegram", nil, "/cli/notify/chat/" + chatsSecondID, `{"message":"Build finished"}`},
				{"", "whatsapp", nil, "/cli/notify/user/u1", `{"message":"Build finished","platform":"whatsapp"}`},
				{chatsFirstID, "telegram", []string{"--platform", "WhatsApp"}, "/cli/notify/user/u1", `{"message":"Build finished","platform":"whatsapp"}`},
				{chatsFirstID, "whatsapp", []string{"--chat", chatsGroupID}, "/cli/notify/chat/" + chatsGroupID, `{"message":"Build finished"}`},
			} {
				r.vars[config.EnvDefaultChat], r.vars[config.EnvDefaultPlatform] = target.chat, target.platform
				r.backend.Handle(http.MethodPost, target.path, testkit.NotifyOK(privateChat, 500))
				hits := r.backend.Count()
				run(0, "", "", append([]string{"notify", "Build finished"}, target.flags...)...)
				request := r.backend.Last(t)
				var got, want map[string]any
				if err := json.Unmarshal([]byte(request.Body), &got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(target.body), &want); err != nil {
					t.Fatal(err)
				}
				if r.backend.Count() != hits+1 || request.Method != http.MethodPost || request.Path != target.path || !reflect.DeepEqual(got, want) || request.Header.Get("Authorization") != "Bearer "+secret {
					t.Fatalf("wrong notification request: %+v", request)
				}
			}
			delete(r.vars, config.EnvDefaultChat)
			delete(r.vars, config.EnvDefaultPlatform)
			run(0, "✓ Default platform set to telegram\n", "", "chats", "default", "--platform", "telegram")
			r.backend.Handle(http.MethodPost, "/cli/notify/user/u1", testkit.NotifyOK(privateChat, 500))
			run(0, "", "", "notify", "Build finished")
			if r.backend.Last(t).Body != `{"message":"Build finished","platform":"telegram"}` {
				t.Fatal("stored platform was not used", r.backend.Last(t))
			}
			run(0, "✓ Default cleared\n", "", "chats", "default", "--clear")
			r.deps.Stdin = strings.NewReader("Build finished\n")
			run(0, "", "", "notify")
			if r.backend.Last(t).Body != `{"message":"Build finished","platform":null}` {
				t.Fatal("latest-chat pipe request was not used", r.backend.Last(t))
			}
			warning := "warning: low credit balance: 42.37 credits left; top up in the agent's chat settings\n"
			r.backend.Handle(http.MethodPost, "/cli/notify/user/u1", testkit.NotifyOK(privateChat, 42.37))
			run(0, "", warning, "notify", "Build finished")
			run(0, "✓ Sent to WhatsApp · Morpheus (private)\n", "Calling backend…\n"+warning, "notify", "--platform", "whatsapp", "Deploy done", "-v")
			r.backend.Handle(http.MethodPost, "/cli/notify/chat/"+chatsGroupID, testkit.NotifyOK(groupChat, 500))
			run(0, "✓ Sent to Telegram · Dev Team (group)\n", "Calling backend…\n", "notify", "--chat", chatsGroupID, "Deploy done", "-v")

			delete(r.vars, config.EnvPassphrase)
			r.terminal.TTYIn = true
			r.terminal.Secrets, r.terminal.Prompts = []string{"secretphrase"}, nil
			assignment := "export THE_AGENT_CLI_TOKEN='"
			if goos == "windows" {
				assignment = "$env:THE_AGENT_CLI_TOKEN = '"
			}
			hits := r.backend.Count()
			unlocked := run(0, assignment+secret+"'\n", "✓ Unlocked for this terminal session\n", "unlock")
			if r.backend.Count() != hits || !reflect.DeepEqual(r.terminal.Prompts, []string{"Passphrase: "}) {
				t.Fatal("unlock contacted the backend or used unexpected prompts", r.terminal.Prompts)
			}
			r.vars[config.EnvToken] = strings.TrimSuffix(strings.TrimPrefix(unlocked.Stdout, assignment), "'\n")
			r.terminal.TTYIn, r.terminal.Prompts = false, nil
			envTokenReport := "Connected using THE_AGENT_CLI_TOKEN\n  Token storage     THE_AGENT_CLI_TOKEN environment variable\n  Token expiry      "
			statusDetails := "\n  Default chat      none (latest chat is used)\n" + statusBackend
			checkStatus(0, envTokenReport+"2027-02-14 10:21 (in 3 months)"+statusDetails)
			now := r.deps.Now
			r.deps.Now = func() time.Time { return connectExpiry.Add(48 * time.Hour) }
			checkStatus(3, envTokenReport+"expired on 2027-02-14 10:21 (2 days ago)"+statusDetails+"  Run `agent-cli connect <KEY>` to connect again.\n")
			r.deps.Now = now
			run(0, table, "", "chats")
			if len(r.terminal.Prompts) != 0 || r.backend.Last(t).Header.Get("Authorization") != "Bearer "+secret {
				t.Fatal("unlocked session prompted or sent the wrong token")
			}
			run(0, "✓ Default chat set to "+chatsSecondID+" (whatsapp · Morpheus, private)\n", "", "chats", "default", "--set", chatsSecondID)
			beforeRevoke := r.file(t)
			r.backend.Handle(http.MethodDelete, "/cli/user/u1/devices", testkit.JSON(200, map[string]string{"status": "OK"}))
			run(0, "✓ Access revoked for all CLIs. Local token removed.\n", "", "revoke-all", "--yes")
			request := r.backend.Last(t)
			if request.Method != http.MethodDelete || request.Path != "/cli/user/u1/devices" || request.Header.Get("Authorization") != "Bearer "+secret {
				t.Fatal("wrong revoke request", request)
			}
			beforeRevoke.Token = config.Token{}
			if r.file(t) != beforeRevoke || r.keychain.Len() != 0 {
				t.Fatal("revocation failed to remove only the token")
			}
			if _, err := os.Stat(filepath.Join(r.home, "credentials")); !os.IsNotExist(err) {
				t.Fatalf("revocation left a credential file: %v", err)
			}
			delete(r.vars, config.EnvToken)
			checkStatus(3, notConnected)
			run(3, "", "This CLI is not connected. Run `agent-cli connect <KEY>`.\n", "notify", "Build finished")

			guide, err := os.ReadFile(filepath.Join("testdata", "skill_guide_"+goldenOS+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			beforeSkills, hits := statusSnapshot(t, r.home), r.backend.Count()
			run(0, string(guide), "", "skill", "show")
			for _, scope := range []string{"--user", "--project"} {
				root, display := userHome, "~/.agents/skills/agent-cli"
				if scope == "--project" {
					root, display = cwd, "~/project/nested/.agents/skills/agent-cli"
				}
				if goos == "windows" {
					display = strings.ReplaceAll(display, "/", `\`)
				}
				run(0, "Codex: missing — "+display+"\n", "", "skill", "status", "--agent", "codex", scope)
				run(0, "Codex: installed — "+display+"\n", "", "skill", "install", "codex", scope)
				run(0, "Codex: current — "+display+"\n", "", "skill", "status", "--agent", "codex", scope)
				stub, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "agent-cli", "SKILL.md"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(stub), "agent-cli skill show") {
					t.Fatal("installed skill does not point at the guide")
				}
				for _, value := range []string{secret, "secretphrase", connectKey, r.backend.URL()} {
					testkit.AssertNoSecret(t, value, string(stub))
				}
			}
			for _, scope := range []string{"--project", "--user"} {
				root, display := cwd, "~/project/nested/.agents/skills/agent-cli"
				if scope == "--user" {
					root, display = userHome, "~/.agents/skills/agent-cli"
				}
				if goos == "windows" {
					display = strings.ReplaceAll(display, "/", `\`)
				}
				run(0, "Codex: removed — "+display+"\n", "", "skill", "remove", "codex", scope)
				run(0, "Codex: missing — "+display+"\n", "", "skill", "status", "--agent", "codex", scope)
				if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "agent-cli")); !os.IsNotExist(err) {
					t.Fatalf("skill directory remains: %v", err)
				}
				if scope == "--project" {
					if _, err := os.Stat(filepath.Join(userHome, ".agents", "skills", "agent-cli", "SKILL.md")); err != nil {
						t.Fatalf("project removal affected the user skill: %v", err)
					}
				}
			}
			if _, err := os.Stat(filepath.Join(userHome, "project", ".agents")); !os.IsNotExist(err) {
				t.Fatalf("project skill used the repository root instead of cwd: %v", err)
			}
			run(0, string(help), "", "--help")
			if statusSnapshot(t, r.home) != beforeSkills || r.backend.Count() != hits || len(r.terminal.Prompts) != 0 {
				t.Fatal("skill commands or help touched CLI state, contacted the backend or prompted")
			}
		})
	}
}
