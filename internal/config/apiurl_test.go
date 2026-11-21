package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
)

func TestAPIURLPrecedence(t *testing.T) {
	const buildDefault = "http://localhost"
	for _, c := range []struct {
		name   string
		env    map[string]string
		file   config.File
		build  string
		want   string
		wanted error
	}{
		{"environment wins", map[string]string{config.EnvAPIURL: "https://b.test"}, config.File{APIURL: "https://a.test"}, buildDefault, "https://b.test", nil},
		{"config beats build default", nil, config.File{APIURL: "https://a.test"}, buildDefault, "https://a.test", nil},
		{"build default", nil, config.File{}, buildDefault, buildDefault, nil},
		{"empty variable counts as unset", map[string]string{config.EnvAPIURL: ""}, config.File{APIURL: "https://a.test"}, buildDefault, "https://a.test", nil},
		{"trailing slash is ignored", map[string]string{config.EnvAPIURL: "https://a.test/"}, config.File{}, buildDefault, "https://a.test", nil},
		{"trailing slashes in the config", nil, config.File{APIURL: "https://a.test/base//"}, buildDefault, "https://a.test/base", nil},
		{"trailing slash in the build default", nil, config.File{}, "https://agent.cloud.appifyhub.com/", "https://agent.cloud.appifyhub.com", nil},
		{"nothing anywhere", nil, config.File{}, "", "", config.ErrNoAPIURL},
	} {
		got, err := config.APIURL(envOf(c.env), c.file, c.build)
		if got != c.want || !errors.Is(err, c.wanted) {
			t.Errorf("%s: got %q, %v; want %q, %v", c.name, got, err, c.want, c.wanted)
		}
	}
}

func TestAPIURLDoesNotValidate(t *testing.T) {
	got, err := config.APIURL(envOf(map[string]string{config.EnvAPIURL: "http://agent.example.test"}), config.File{}, "http://localhost")
	if err != nil || got != "http://agent.example.test" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestValidateAPIURLAccepts(t *testing.T) {
	for _, raw := range []string{
		"https://agent.cloud.appifyhub.com",
		"https://agent.example.test:8443/base",
		"http://localhost",
		"http://LOCALHOST:3000",
		"http://127.0.0.1:8080",
		"http://127.0.0.2",
		"http://[::1]:8080",
	} {
		if err := config.ValidateAPIURL(raw); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
}

func TestValidateAPIURLRejects(t *testing.T) {
	for _, c := range []struct {
		raw     string
		problem config.URLProblem
		shown   string
	}{
		{"http://agent.example.test", config.URLInsecure, "http://agent.example.test"},
		{"http://127.0.0.1.evil.test", config.URLInsecure, "http://127.0.0.1.evil.test"},
		{"http://localhost.evil.test", config.URLInsecure, "http://localhost.evil.test"},
		{"ftp://agent.example.test", config.URLInsecure, "ftp://agent.example.test"},
		{"https://user:secret@agent.example.test", config.URLCredentials, "https://agent.example.test"},
		{"https://user@agent.example.test/path", config.URLCredentials, "https://agent.example.test/path"},
		{"http://user:secret@localhost", config.URLCredentials, "http://localhost"},
		{"https://agent.example.test?x=1", config.URLQueryOrFragment, "https://agent.example.test?x=1"},
		{"https://agent.example.test/#top", config.URLQueryOrFragment, "https://agent.example.test/#top"},
		{"https://agent.example.test/#", config.URLQueryOrFragment, "https://agent.example.test/#"},
		{"https://agent.example.test?", config.URLQueryOrFragment, "https://agent.example.test?"},
		{"agent.example.test", config.URLMalformed, "agent.example.test"},
		{"https://", config.URLMalformed, "https://"},
		{"", config.URLMalformed, ""},
		{"https://user:secret@agent.example.test:port", config.URLMalformed, "https://agent.example.test:port"},
	} {
		err := config.ValidateAPIURL(c.raw)
		var urlErr *config.InvalidURLError
		if !errors.As(err, &urlErr) {
			t.Errorf("%q: got %v, want *InvalidURLError", c.raw, err)
			continue
		}
		if urlErr.Problem != c.problem || urlErr.URL != c.shown {
			t.Errorf("%q: problem %v, URL %q; want %v, %q", c.raw, urlErr.Problem, urlErr.URL, c.problem, c.shown)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(urlErr.URL, "secret") {
			t.Errorf("%q: the error echoes the password: %v", c.raw, err)
		}
	}
}
