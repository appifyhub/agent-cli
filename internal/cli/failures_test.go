package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/text"
)

func TestConfigFailureTextsAndExitCodes(t *testing.T) {
	notADir := &config.HomeError{Path: "/work/state", Err: errors.New("not a directory")}
	for _, c := range []struct {
		name string
		err  error
		code int
		text string
	}{
		{"home unavailable", config.ErrHomeUnavailable, ExitFailure, "could not determine your home directory; set THE_AGENT_CLI_HOME"},
		{"home unusable", notADir, ExitFailure, "cannot use the home directory /work/state: not a directory"},
		{"config unreadable", &config.ReadError{Path: "/h/config.toml", Err: errors.New("toml: invalid key")}, ExitFailure, "could not read /h/config.toml: invalid key; fix or delete the file"},
		{"invalid device id", &config.ReadError{Path: "/h/config.toml", Err: config.ErrInvalidDeviceID}, ExitFailure, "could not read /h/config.toml: device_id is not a valid UUID; fix or delete the file"},
		{"config unwritable", &config.WriteError{Path: "/h/config.toml", Err: errors.New("disk full")}, ExitFailure, "could not write /h/config.toml: disk full"},
		{"no backend URL", config.ErrNoAPIURL, ExitUsage, "no backend URL is configured; set THE_AGENT_CLI_API_URL"},
		{"cleartext remote URL", config.ValidateAPIURL("http://agent.example.test"), ExitUsage, `invalid API URL "http://agent.example.test": use https (http is allowed only for localhost)`},
		{"credentials in URL", config.ValidateAPIURL("https://user:secret@agent.example.test"), ExitUsage, `invalid API URL "https://agent.example.test": remove the user name and password from the URL`},
		{"query in URL", config.ValidateAPIURL("https://agent.example.test?a=b"), ExitUsage, `invalid API URL "https://agent.example.test?a=b": remove the query string and the fragment`},
		{"malformed URL", config.ValidateAPIURL("agent.example.test"), ExitUsage, `invalid API URL "agent.example.test": use a web address like https://agent.example.com`},
		{"invalid platform", &config.InvalidPlatformError{Value: "slack"}, ExitUsage, `invalid THE_AGENT_CLI_DEFAULT_PLATFORM "slack": use telegram or whatsapp`},
		{"wrapped error", fmt.Errorf("while connecting: %w", config.ErrNoAPIURL), ExitUsage, "no backend URL is configured; set THE_AGENT_CLI_API_URL"},
	} {
		var failure *Failure
		if err := configFailure(text.MustNew("linux"), c.err); !errors.As(err, &failure) || failure.Code != c.code || failure.Message != c.text {
			t.Errorf("%s: got %#v, want code %d, %q", c.name, err, c.code, c.text)
		}
	}
}

func TestConfigFailureLeavesOtherErrorsAlone(t *testing.T) {
	other := errors.New("something else")
	if got := configFailure(text.MustNew("linux"), other); !errors.Is(got, other) {
		t.Errorf("got %v", got)
	}
}
