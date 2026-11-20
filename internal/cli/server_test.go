package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/cli"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

// steppingClock advances by step on every reading, so the response time of a call is the step.
func steppingClock(step time.Duration) testkit.Option {
	return func(d *cli.Deps) {
		now := time.Date(2026, 11, 14, 10, 21, 0, 0, time.UTC)
		reads := 0
		d.Now = func() time.Time {
			reads++
			return now.Add(time.Duration(reads-1) * step)
		}
	}
}

func runServer(t *testing.T, b *testkit.Backend, env map[string]string, args ...string) testkit.Result {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	if _, set := env[config.EnvAPIURL]; !set {
		env[config.EnvAPIURL] = b.URL()
	}
	if _, set := env[config.EnvHome]; !set {
		env[config.EnvHome] = filepath.Join(t.TempDir(), "state")
	}
	deps := testkit.NewDeps(testkit.WithEnv(env), testkit.WithHTTP(b.Client().Transport), steppingClock(123*time.Millisecond))
	return testkit.Run(t, deps, append([]string{"server"}, args...)...)
}

func hostOf(b *testkit.Backend) string { return strings.TrimPrefix(b.URL(), "http://") }

func TestServerReportOfAHealthyServer(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.HealthOK("5.41.0"))
	res := runServer(t, b, nil)
	want := "Server    " + hostOf(b) + "\nStatus    ok\nVersion   5.41.0\nResponse  123 ms\n"
	if res.Code != 0 || res.Stdout != want || res.Stderr != "" {
		t.Errorf("got %d\nstdout %q\nstderr %q\nwant stdout %q", res.Code, res.Stdout, res.Stderr, want)
	}
}

func TestServerShowsExtraScalarsAlphabeticallyAndIgnoresNestedValues(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.Response{Status: 200, Body: `{"status":"ok","version":"5.41.0","region":"eu","uptime_seconds":86400,"debug":false,"checks":{"db":"ok"},"tags":["a"]}`})
	res := runServer(t, b, nil)
	want := "Server          " + hostOf(b) + "\nStatus          ok\nVersion         5.41.0\n" +
		"debug           false\nregion          eu\nuptime_seconds  86400\nResponse        123 ms\n"
	if res.Code != 0 || res.Stdout != want {
		t.Errorf("got %d\n%s\nwant\n%s", res.Code, res.Stdout, want)
	}
}

func TestServerWithoutAVersionOmitsTheLine(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.JSON(200, map[string]any{"status": "ok"}))
	res := runServer(t, b, nil)
	if res.Code != 0 || strings.Contains(res.Stdout, "Version") {
		t.Errorf("got %d\n%s", res.Code, res.Stdout)
	}
}

func TestServerNeedsNoProfileAndCreatesNothing(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.HealthOK("1"))
	home := filepath.Join(t.TempDir(), "state")
	res := runServer(t, b, map[string]string{config.EnvHome: home})
	if res.Code != 0 {
		t.Fatalf("got %d %q", res.Code, res.Stderr)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Errorf("the home directory must not be created: %v", err)
	}
	if r := b.Last(t); r.Header.Get("Authorization") != "" {
		t.Error("no token may be sent")
	}
}

func TestServerNeverReadsTheTokenFromTheEnvironment(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.HealthOK("1"))
	res := runServer(t, b, map[string]string{config.EnvToken: "not-even-a-token"})
	if res.Code != 0 || res.Stderr != "" {
		t.Errorf("got %d %q", res.Code, res.Stderr)
	}
}

func TestServerTextIsNeutralized(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.Response{Status: 200, Body: `{"status":"ok","version":"1.0\u001b[31mred\u001b]0;title\u0007","evil\u001b[2J":"x\u202etxt"}`})
	res := runServer(t, b, nil)
	if res.Code != 0 || strings.ContainsAny(res.Stdout, "\x1b\x07\u202e") {
		t.Errorf("escape characters leaked: %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "Version   1.0red") {
		t.Errorf("got %q", res.Stdout)
	}
}

func TestServerWithAnUnhealthyStatusPrintsTheReportAndExits1(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.JSON(200, map[string]any{"status": "degraded", "version": "5.41.0"}))
	res := runServer(t, b, nil)
	if res.Code != 1 || res.Stderr != "" || !strings.Contains(res.Stdout, "Status    degraded") {
		t.Errorf("got %d\nstdout %q\nstderr %q", res.Code, res.Stdout, res.Stderr)
	}
}

func TestServerFailures(t *testing.T) {
	for _, c := range []struct {
		name string
		resp testkit.Response
		code int
		text string
	}{
		{"HTTP error", testkit.Text(503, "down"), 1, "error: the server at {url} answered HTTP 503"},
		{"unknown route", testkit.Detail(404, "Not Found"), 1, "error: the server at {url} answered HTTP 404"},
		{"web page", testkit.HTML(200), 1, "error: the address {url} did not answer like the agent's server"},
		{"plain text", testkit.Text(200, "OK"), 1, "error: the address {url} did not answer like the agent's server"},
		{"rate limited", testkit.Text(429, "Too Many Requests"), 6, "error: rate limited; wait a moment and try again"},
		{"redirect", testkit.Response{Status: 302, Header: map[string]string{"Location": "/x"}}, 1, "error: the server at {url} answered HTTP 302"},
	} {
		b := testkit.NewBackend(t)
		b.Handle("GET", "/health", c.resp)
		res := runServer(t, b, nil)
		want := strings.ReplaceAll(c.text, "{url}", b.URL()) + "\n"
		if res.Code != c.code || res.Stdout != "" || res.Stderr != want {
			t.Errorf("%s: got %d\nstdout %q\nstderr %q\nwant stderr %q", c.name, res.Code, res.Stdout, res.Stderr, want)
		}
		if b.Count() != 1 {
			t.Errorf("%s: %d requests", c.name, b.Count())
		}
	}
}

func TestServerUnreachable(t *testing.T) {
	b := testkit.NewBackend(t)
	res := runServer(t, b, map[string]string{config.EnvAPIURL: "http://127.0.0.1:1"})
	if res.Code != 1 || res.Stdout != "" || !strings.HasPrefix(res.Stderr, "error: could not reach the server at http://127.0.0.1:1: ") || strings.Count(res.Stderr, "\n") != 1 {
		t.Errorf("got %d\nstdout %q\nstderr %q", res.Code, res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stderr, `"`) || strings.Contains(res.Stderr, "/health") {
		t.Errorf("the reason must not repeat the request: %q", res.Stderr)
	}
}

func TestServerInvalidAPIURLSendsNothing(t *testing.T) {
	b := testkit.NewBackend(t)
	res := runServer(t, b, map[string]string{config.EnvAPIURL: "http://agent.example.test"})
	if res.Code != 2 || !strings.HasPrefix(res.Stderr, "error: invalid API URL") || b.Count() != 0 {
		t.Errorf("got %d %q, %d requests", res.Code, res.Stderr, b.Count())
	}
}

func TestServerRejectsArguments(t *testing.T) {
	b := testkit.NewBackend(t)
	res := runServer(t, b, nil, "now")
	if res.Code != 2 || b.Count() != 0 {
		t.Errorf("got %d %q", res.Code, res.Stderr)
	}
}

func TestServerAddressPrecedence(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.HealthOK("1"))
	home := filepath.Join(t.TempDir(), "state")
	writeConfig(t, home, "api_url = \""+b.URL()+"\"\n")
	deps := testkit.NewDeps(testkit.WithEnv(map[string]string{config.EnvHome: home}), testkit.WithHTTP(b.Client().Transport))
	if res := testkit.Run(t, deps, "server"); res.Code != 0 || !strings.Contains(res.Stdout, "Server    "+hostOf(b)) {
		t.Errorf("config address: got %d %q", res.Code, res.Stdout)
	}
	deps = testkit.NewDeps(testkit.WithEnv(map[string]string{config.EnvHome: home, config.EnvAPIURL: b.URL() + "/"}), testkit.WithHTTP(b.Client().Transport))
	if res := testkit.Run(t, deps, "server"); res.Code != 0 || !strings.Contains(res.Stdout, "Server    "+hostOf(b)+"\n") {
		t.Errorf("environment address with a trailing slash: got %d %q", res.Code, res.Stdout)
	}
	if got := b.Last(t).Path; got != "/health" {
		t.Errorf("path %q", got)
	}
}

func TestServerVerboseDiagnostics(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.Text(503, "down"))
	res := runServer(t, b, nil, "-v")
	lines := strings.Split(strings.TrimSpace(res.Stderr), "\n")
	if len(lines) != 3 || lines[0] != "Calling backend…" || lines[1] != "  (HTTP 503)" || !strings.HasPrefix(lines[2], "error: ") {
		t.Errorf("stderr %q", res.Stderr)
	}
	b.Handle("GET", "/health", testkit.HealthOK("1"))
	if res := runServer(t, b, nil); strings.Contains(res.Stderr, "Calling backend") {
		t.Error("the diagnostics are for --verbose only")
	}
}

func TestServerSendsTheBuildsUserAgent(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.HealthOK("1"))
	runServer(t, b, nil)
	if got := b.Last(t).Header.Get("User-Agent"); !strings.HasPrefix(got, "agent-cli/") || !strings.HasSuffix(got, " (linux/amd64)") {
		t.Errorf("User-Agent = %q", got)
	}
}
