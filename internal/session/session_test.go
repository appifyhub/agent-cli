package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/session"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

const deviceID = "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b"

var token = testkit.NewToken(testkit.TokenClaims{Sub: "u1", Exp: time.Date(2027, 2, 14, 9, 21, 0, 0, time.UTC)})

type rig struct {
	t       *testing.T
	backend *testkit.Backend
	kc      *testkit.FakeKeychain
	term    *testkit.FakeTerm
	vars    map[string]string
	home    string
	session session.Session
	client  *api.Client
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, backend: testkit.NewBackend(t), kc: &testkit.FakeKeychain{}, term: &testkit.FakeTerm{}, vars: map[string]string{}, home: t.TempDir()}
	r.session = session.Session{Credentials: credentials.Env{
		Getenv: func(k string) string { return r.vars[k] }, Term: r.term, Keychain: r.kc,
		Service: "agent-cli", Home: r.home, WorkFactor: 8,
		Prompts: credentials.Prompts{Choose: "Choose: ", Repeat: "Repeat: ", Passphrase: "Passphrase: "},
	}}
	r.client = api.New(api.Options{
		BaseURL: r.backend.URL(), Build: buildinfo.Resolve(buildinfo.Info{Version: "0.1.2"}), GOOS: "linux", GOARCH: "amd64",
		Transport: r.backend.Client().Transport,
	})
	return r
}

func (r *rig) connect(mode string) {
	r.t.Helper()
	r.session.Credentials.Getenv = func(k string) string {
		if k == config.EnvPassphrase {
			return "pw"
		}
		return r.vars[k]
	}
	plan, err := credentials.NewPlan(r.t.Context(), r.session.Credentials, deviceID, mode)
	if err != nil {
		r.t.Fatal(err)
	}
	_, err = plan.Persist(token, "", func() error {
		return config.Save(config.Path(r.home), config.File{
			DeviceID: deviceID, Token: config.Token{Store: string(plan.Mode()), UserID: "u1"},
			Default: config.Default{ChatID: "c1"},
		})
	})
	if err != nil {
		r.t.Fatal(err)
	}
	r.session.Credentials.Getenv = func(k string) string { return r.vars[k] }
}

func (r *rig) chats() error {
	return r.session.Call(r.t.Context(), func(id session.Identity) error {
		_, err := r.client.Chats(r.t.Context(), id.Token)
		return err
	})
}

func (r *rig) stored() bool {
	f, err := config.Load(config.Path(r.home))
	if err != nil {
		r.t.Fatal(err)
	}
	_, inFile := os.Stat(filepath.Join(r.home, credentials.FileName))
	return f.Token.Store != "" && (r.kc.Len() == 1 || inFile == nil)
}

func TestRejectedStoredTokenIsRemoved(t *testing.T) {
	for name, resp := range map[string]testkit.Response{
		"401 invalid":      testkit.Err(401, 4004, "Invalid token"),
		"401 revoked":      testkit.Err(401, 4005, "Revoked"),
		"401 framework":    testkit.Detail(401, "Unauthorized"),
		"403 other user":   testkit.Err(403, 3002, "Not you"),
		"403 without code": testkit.Text(403, "Forbidden"),
		"404 user gone":    testkit.Err(404, 2001, "No such user"),
	} {
		for _, mode := range []string{"keychain", "unsafe"} {
			r := newRig(t)
			r.connect(mode)
			r.backend.Handle("GET", "/cli/chats", resp)
			if err := r.chats(); !errors.Is(err, credentials.ErrNotConnected) {
				t.Errorf("%s/%s: got %v, want not connected", name, mode, err)
			}
			f, _ := config.Load(config.Path(r.home))
			if r.kc.Len() != 0 || r.stored() || f.Token != (config.Token{}) || f.DeviceID != deviceID || f.Default.ChatID != "c1" {
				t.Errorf("%s/%s: the stored token must be gone and the rest kept: %+v", name, mode, f)
			}
			if r.backend.Count() != 1 {
				t.Errorf("%s/%s: %d requests, want 1", name, mode, r.backend.Count())
			}
		}
	}
}

func TestOtherFailuresKeepTheStoredToken(t *testing.T) {
	for name, resp := range map[string]testkit.Response{
		"403 not admin":     testkit.Err(403, 3001, "x"),
		"403 no private":    testkit.Err(403, 3004, "x"),
		"403 waitlist":      testkit.Err(403, 3007, "x"),
		"403 policies":      testkit.Err(403, 3008, "x"),
		"403 not member":    testkit.Err(403, 3009, "x"),
		"403 refused":       testkit.Err(403, 3010, "x"),
		"404 chat":          testkit.Err(404, 2002, "x"),
		"404 unknown route": testkit.Detail(404, "Not Found"),
		"422 credits":       testkit.Err(422, 1034, "x"),
		"429":               testkit.Text(429, "slow down"),
		"500":               testkit.Text(500, "boom"),
		"502":               testkit.HTML(502),
		"302":               {Status: 302, Header: map[string]string{"Location": "/x"}},
	} {
		r := newRig(t)
		r.connect("keychain")
		r.backend.Handle("GET", "/cli/chats", resp)
		err := r.chats()
		var apiErr *api.Error
		if !errors.As(err, &apiErr) || errors.Is(err, credentials.ErrNotConnected) {
			t.Errorf("%s: got %v, want the API error", name, err)
		}
		if !r.stored() {
			t.Errorf("%s: the stored token must be kept", name)
		}
	}
}

func TestSuccessReturnsNothingAndKeepsTheToken(t *testing.T) {
	r := newRig(t)
	r.connect("keychain")
	r.backend.Handle("GET", "/cli/chats", testkit.JSON(200, []any{}))
	if err := r.chats(); err != nil || !r.stored() {
		t.Fatalf("got %v", err)
	}
	if got := r.backend.Last(t).Header.Get("Authorization"); got != "Bearer "+token {
		t.Errorf("Authorization = %q", got)
	}
}

func TestEnvironmentTokenRejectedTouchesNothingStored(t *testing.T) {
	r := newRig(t)
	r.connect("keychain")
	r.vars[config.EnvToken] = testkit.NewToken(testkit.TokenClaims{Sub: "someone", Exp: time.Now().Add(time.Hour)})
	r.backend.Handle("GET", "/cli/chats", testkit.Err(401, 4005, "Revoked"))
	err := r.chats()
	if !errors.Is(err, session.ErrEnvTokenRejected) || errors.Is(err, credentials.ErrNotConnected) {
		t.Fatalf("got %v", err)
	}
	if !r.stored() || r.kc.Len() != 1 {
		t.Error("stored credentials must be untouched")
	}
	if got := r.backend.Last(t).Header.Get("Authorization"); got != "Bearer "+r.vars[config.EnvToken] {
		t.Errorf("the environment token must be the one sent: %q", got)
	}
}

func TestEnvironmentTokenNeedsNoConnection(t *testing.T) {
	r := newRig(t)
	r.vars[config.EnvToken] = token
	r.backend.Handle("GET", "/cli/chats", testkit.JSON(200, []any{}))
	var seen session.Identity
	err := r.session.Call(t.Context(), func(id session.Identity) error {
		seen = id
		_, err := r.client.Chats(t.Context(), id.Token)
		return err
	})
	if err != nil || seen.UserID != "u1" || seen.Token != token {
		t.Fatalf("got %v, %+v", err, seen)
	}
}

func TestNotConnectedAndLockedNeverReachTheBackend(t *testing.T) {
	r := newRig(t)
	if err := r.chats(); !errors.Is(err, credentials.ErrNotConnected) {
		t.Errorf("got %v", err)
	}
	r.connect("passphrase")
	if err := r.chats(); !errors.Is(err, credentials.ErrLocked) {
		t.Errorf("got %v", err)
	}
	if r.backend.Count() != 0 {
		t.Errorf("%d requests, want none", r.backend.Count())
	}
}

func TestCallRunsTheFunctionOnce(t *testing.T) {
	r := newRig(t)
	r.connect("keychain")
	calls := 0
	boom := errors.New("boom")
	err := r.session.Call(t.Context(), func(session.Identity) error { calls++; return boom })
	if !errors.Is(err, boom) || calls != 1 {
		t.Errorf("got %v after %d calls", err, calls)
	}
}
