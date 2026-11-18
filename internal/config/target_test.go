package config_test

import (
	"errors"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
)

func TestDefaultTarget(t *testing.T) {
	chat := func(id string) config.File { return config.File{Default: config.Default{ChatID: id}} }
	platform := func(p string) config.File { return config.File{Default: config.Default{Platform: p}} }
	for _, c := range []struct {
		name string
		env  map[string]string
		file config.File
		want config.Target
	}{
		{"environment chat beats everything", map[string]string{config.EnvDefaultChat: "c1", config.EnvDefaultPlatform: "whatsapp"}, chat("c2"), config.Target{ChatID: "c1"}},
		{"environment platform beats config", map[string]string{config.EnvDefaultPlatform: "whatsapp"}, chat("c2"), config.Target{Platform: "whatsapp"}},
		{"config chat", nil, chat("c2"), config.Target{ChatID: "c2"}},
		{"config platform", nil, platform("telegram"), config.Target{Platform: "telegram"}},
		{"config chat beats config platform", nil, config.File{Default: config.Default{ChatID: "c2", Platform: "telegram"}}, config.Target{ChatID: "c2"}},
		{"case-insensitive platform", map[string]string{config.EnvDefaultPlatform: "WhatsApp"}, config.File{}, config.Target{Platform: "whatsapp"}},
		{"empty variables count as unset", map[string]string{config.EnvDefaultChat: "", config.EnvDefaultPlatform: ""}, platform("telegram"), config.Target{Platform: "telegram"}},
		{"no default anywhere", nil, config.File{}, config.Target{}},
	} {
		got, err := config.DefaultTarget(envOf(c.env), c.file)
		if err != nil || got != c.want {
			t.Errorf("%s: got %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
}

func TestDefaultTargetRejectsAnInvalidEnvironmentPlatform(t *testing.T) {
	for _, env := range []map[string]string{
		{config.EnvDefaultPlatform: "slack"},
		{config.EnvDefaultPlatform: "slack", config.EnvDefaultChat: "c1"},
	} {
		_, err := config.DefaultTarget(envOf(env), config.File{})
		var platformErr *config.InvalidPlatformError
		if !errors.As(err, &platformErr) || platformErr.Value != "slack" {
			t.Errorf("%v: got %v, want *InvalidPlatformError for slack", env, err)
		}
	}
}
