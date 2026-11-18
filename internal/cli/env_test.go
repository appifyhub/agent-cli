package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

func runEnv(t *testing.T, env map[string]string, opts ...testkit.Option) testkit.Result {
	t.Helper()
	opts = append([]testkit.Option{testkit.WithEnv(env)}, opts...)
	return testkit.Run(t, testkit.NewDeps(opts...), "env")
}

// stateOf returns what the report says about one variable, after the colon.
func stateOf(t *testing.T, stdout, name string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if state, ok := strings.CutPrefix(line, name+": "); ok {
			return state
		}
	}
	t.Fatalf("no line for %s in\n%s", name, stdout)
	return ""
}

func writeConfig(t *testing.T, home, content string) {
	t.Helper()
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.Path(home), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnvGolden(t *testing.T) {
	for _, c := range []struct{ goos, golden string }{{"darwin", "unix"}, {"linux", "unix"}, {"windows", "windows"}} {
		res := runEnv(t, nil, testkit.WithPlatform(c.goos, "amd64"))
		if res.Code != 0 || res.Stderr != "" {
			t.Fatalf("%s: code %d, stderr %q", c.goos, res.Code, res.Stderr)
		}
		testkit.Golden(t, "env_"+c.golden, res.Stdout)
	}
}

func TestEnvListsEveryVariableInOrder(t *testing.T) {
	res := runEnv(t, nil)
	var names []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		if name, _, ok := strings.Cut(line, ": "); ok && !strings.HasPrefix(line, " ") {
			names = append(names, name)
		}
	}
	want := []string{config.EnvAPIURL, config.EnvHome, config.EnvToken, config.EnvPassphrase, config.EnvDefaultChat, config.EnvDefaultPlatform, "NO_COLOR"}
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestEnvDefaultsFollowTheBuild(t *testing.T) {
	res := runEnv(t, nil, testkit.WithBuild(buildinfo.Info{Name: "abot-dev", APIURL: "https://staging.agent.cloud.appifyhub.com"}))
	if got := stateOf(t, res.Stdout, config.EnvAPIURL); got != "not set, using https://staging.agent.cloud.appifyhub.com" {
		t.Errorf("API URL: %q", got)
	}
	if got := stateOf(t, res.Stdout, config.EnvHome); got != "not set, using ~/.abot-dev" {
		t.Errorf("home: %q", got)
	}
}

func TestEnvShowsValuesInEffect(t *testing.T) {
	home := t.TempDir()
	res := runEnv(t, map[string]string{
		config.EnvAPIURL:          "https://agent.example.com/",
		config.EnvHome:            home,
		config.EnvDefaultPlatform: "WhatsApp",
		config.EnvDefaultChat:     "c1",
	})
	if res.Code != 0 || res.Stderr != "" {
		t.Fatalf("code %d, stderr %q", res.Code, res.Stderr)
	}
	for name, want := range map[string]string{
		config.EnvAPIURL:          "set to https://agent.example.com",
		config.EnvHome:            "set to " + home,
		config.EnvDefaultChat:     "set to c1",
		config.EnvDefaultPlatform: "set to whatsapp",
	} {
		if got := stateOf(t, res.Stdout, name); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestEnvShowsTheStoredValuesInEffect(t *testing.T) {
	home := t.TempDir()
	env := map[string]string{config.EnvHome: home}
	writeConfig(t, home, "api_url = \"https://a.test\"\n[default]\nchat_id = \"c2\"\n")
	res := runEnv(t, env)
	if got := stateOf(t, res.Stdout, config.EnvAPIURL); got != "not set, using https://a.test" {
		t.Errorf("API URL: %q", got)
	}
	if got := stateOf(t, res.Stdout, config.EnvDefaultChat); got != "not set, using c2" {
		t.Errorf("chat: %q", got)
	}
	if got := stateOf(t, res.Stdout, config.EnvDefaultPlatform); got != "not set" {
		t.Errorf("platform: %q", got)
	}

	writeConfig(t, home, "[default]\nplatform = \"telegram\"\n")
	res = runEnv(t, env)
	if got := stateOf(t, res.Stdout, config.EnvDefaultPlatform); got != "not set, using telegram" {
		t.Errorf("stored platform: %q", got)
	}

	env[config.EnvAPIURL] = "https://b.test"
	writeConfig(t, home, "api_url = \"https://a.test\"\n")
	res = runEnv(t, env)
	if got := stateOf(t, res.Stdout, config.EnvAPIURL); got != "set to https://b.test" {
		t.Errorf("environment must beat the stored URL: %q", got)
	}
	if got := stateOf(t, res.Stdout, config.EnvHome); got != "set to "+home {
		t.Errorf("home: %q", got)
	}
}

func TestEnvSecretsStayHidden(t *testing.T) {
	const token, passphrase = "tok-s3cr3t-value", "pass-s3cr3t-value"
	res := runEnv(t, map[string]string{config.EnvToken: token, config.EnvPassphrase: passphrase})
	testkit.AssertNoSecret(t, token, res.Stdout, res.Stderr)
	testkit.AssertNoSecret(t, passphrase, res.Stdout, res.Stderr)
	for _, name := range []string{config.EnvToken, config.EnvPassphrase} {
		if got := stateOf(t, res.Stdout, name); got != "set, value hidden" {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestEnvNeutralizesUntrustedValues(t *testing.T) {
	res := runEnv(t, map[string]string{
		config.EnvDefaultChat: "c1\x1b[31m-red\x1b]0;title\x07-end",
		config.EnvAPIURL:      "https://a.test\x1b[2J",
	})
	if strings.ContainsAny(res.Stdout, "\x1b\x07") {
		t.Errorf("control characters in %q", res.Stdout)
	}
	if got := stateOf(t, res.Stdout, config.EnvDefaultChat); got != "set to c1-red-end" {
		t.Errorf("chat: %q", got)
	}
}

func TestEnvNoColor(t *testing.T) {
	for value, want := range map[string]string{"1": "set, colors are off", "": "not set, colors follow the terminal"} {
		res := runEnv(t, map[string]string{"NO_COLOR": value})
		if got := stateOf(t, res.Stdout, "NO_COLOR"); got != want {
			t.Errorf("NO_COLOR=%q: got %q, want %q", value, got, want)
		}
	}
}

func TestEnvDoesNotJudgeValues(t *testing.T) {
	res := runEnv(t, map[string]string{config.EnvAPIURL: "http://agent.example.test", config.EnvDefaultPlatform: "slack"})
	if res.Code != 0 || res.Stderr != "" {
		t.Fatalf("code %d, stderr %q", res.Code, res.Stderr)
	}
	if got := stateOf(t, res.Stdout, config.EnvAPIURL); got != "set to http://agent.example.test" {
		t.Errorf("API URL: %q", got)
	}
	if got := stateOf(t, res.Stdout, config.EnvDefaultPlatform); got != "set to slack" {
		t.Errorf("platform: %q", got)
	}
}

func TestEnvWithoutAUserHome(t *testing.T) {
	deps := testkit.NewDeps()
	deps.UserHomeDir = func() (string, error) { return "", errors.New("$HOME is not defined") }
	res := testkit.Run(t, deps, "env")
	if res.Code != 0 || res.Stderr != "" {
		t.Fatalf("code %d, stderr %q", res.Code, res.Stderr)
	}
	if got := stateOf(t, res.Stdout, config.EnvHome); got != "not set, and your home directory could not be determined" {
		t.Errorf("home: %q", got)
	}
}

func TestEnvCreatesNoFiles(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "state")
	res := runEnv(t, map[string]string{config.EnvHome: missing})
	if res.Code != 0 {
		t.Fatalf("code %d, stderr %q", res.Code, res.Stderr)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the home directory was created (%v)", err)
	}

	existing := filepath.Join(root, "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	runEnv(t, map[string]string{config.EnvHome: existing})
	if entries, err := os.ReadDir(existing); err != nil || len(entries) != 0 {
		t.Errorf("files appeared: %v, %v", entries, err)
	}
}

func TestEnvRejectsArguments(t *testing.T) {
	res := testkit.Run(t, testkit.NewDeps(), "env", "extra")
	if res.Code != 2 || res.Stdout != "" || !strings.HasPrefix(res.Stderr, "error: ") || strings.Count(res.Stderr, "\n") != 1 {
		t.Errorf("code %d, stdout %q, stderr %q", res.Code, res.Stdout, res.Stderr)
	}
}

func TestEnvStopsOnAMalformedConfig(t *testing.T) {
	home := t.TempDir()
	writeConfig(t, home, "device_id = = broken")
	res := runEnv(t, map[string]string{config.EnvHome: home})
	path := config.Path(home)
	if res.Code != 1 || res.Stdout != "" {
		t.Fatalf("code %d, stdout %q", res.Code, res.Stdout)
	}
	if !strings.HasPrefix(res.Stderr, "error: could not read "+path+": ") || !strings.HasSuffix(res.Stderr, "; fix or delete the file\n") || strings.Count(res.Stderr, "\n") != 1 {
		t.Errorf("stderr %q", res.Stderr)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "device_id = = broken" {
		t.Errorf("the file changed: %q, %v", raw, err)
	}
}

func TestEnvHelp(t *testing.T) {
	viaHelp, viaFlag := runOn(t, "linux", "help", "env"), runOn(t, "linux", "env", "-h")
	if viaHelp.Code != 0 || viaHelp.Stdout != viaFlag.Stdout || viaHelp.Stderr != "" {
		t.Fatalf("help env %+v, env -h %+v", viaHelp, viaFlag)
	}
	for _, want := range []string{"Usage: agent-cli env\n", "Secrets are reported as set, never shown."} {
		if !strings.Contains(viaHelp.Stdout, want) {
			t.Errorf("env help lacks %q:\n%s", want, viaHelp.Stdout)
		}
	}
}
