package credentials_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

const (
	deviceID = "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b"
	service  = "agent-cli"
)

var token = testkit.NewToken(testkit.TokenClaims{
	Sub: "9f1c2d3e4a5b6c7d8e9f0a1b2c3d4e5f",
	Exp: time.Date(2027, 2, 14, 9, 21, 0, 0, time.UTC),
})

type rig struct {
	t    *testing.T
	env  credentials.Env
	term *testkit.FakeTerm
	kc   *testkit.FakeKeychain
	vars map[string]string
	home string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, term: &testkit.FakeTerm{}, kc: &testkit.FakeKeychain{}, vars: map[string]string{}, home: t.TempDir()}
	r.env = credentials.Env{
		Getenv:     func(k string) string { return r.vars[k] },
		Term:       r.term,
		Keychain:   r.kc,
		Service:    service,
		Home:       r.home,
		WorkFactor: 8,
		Prompts:    credentials.Prompts{Choose: "Choose a passphrase: ", Repeat: "Repeat passphrase: ", Passphrase: "Passphrase: "},
	}
	return r
}

func (r *rig) configPath() string { return config.Path(r.home) }

func (r *rig) credentialsPath() string { return filepath.Join(r.home, credentials.FileName) }

func (r *rig) writeConfig(f config.File) {
	r.t.Helper()
	if err := config.Save(r.configPath(), f); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) readConfig() config.File {
	r.t.Helper()
	f, err := config.Load(r.configPath())
	if err != nil {
		r.t.Fatal(err)
	}
	return f
}

func (r *rig) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// connect runs a plan and persists the token the way the connect command will, returning the plan's mode.
func (r *rig) connect(requested string) credentials.Mode {
	r.t.Helper()
	plan, err := credentials.NewPlan(r.t.Context(), r.env, deviceID, requested)
	if err != nil {
		r.t.Fatal(err)
	}
	previous := credentials.Mode(r.readConfig().Token.Store)
	_, err = plan.Persist(token, previous, func() error {
		f := r.readConfig()
		f.DeviceID = deviceID
		f.Token = config.Token{Store: string(plan.Mode()), UserID: "9f1c", UserName: "Milos", ExpiresAt: time.Date(2027, 2, 14, 9, 21, 0, 0, time.UTC)}
		f.Default = config.Default{ChatID: "3f2a", ChatPlatform: "telegram", ChatTitle: "Milos", ChatPrivate: true}
		return config.Save(r.configPath(), f)
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return plan.Mode()
}
