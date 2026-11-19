package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/session"
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

func TestCredentialFailureTextsAndExitCodes(t *testing.T) {
	for _, c := range []struct {
		name string
		goos string
		err  error
		code int
		text string
		bare bool
	}{
		{"unknown mode", "linux", &credentials.StoreFlagError{Value: "vault"}, ExitUsage, `invalid --store "vault": use keychain, passphrase, or unsafe`, false},
		{"explicit keychain unavailable", "linux", credentials.ErrNoKeychain, ExitUsage, "no keychain is available on this machine; use --store passphrase or --store unsafe", false},
		{"explicit passphrase without a way to ask", "linux", credentials.ErrNoPassphraseSource, ExitUsage, "a passphrase is required; run in a terminal or set THE_AGENT_CLI_PASSPHRASE", false},
		{"nothing to store with", "linux", credentials.ErrNoStore, ExitUsage, "no keychain is available and no passphrase can be requested here; use --store unsafe or provide a token with THE_AGENT_CLI_TOKEN", false},
		{"passphrases differ", "linux", credentials.ErrPassphraseMismatch, ExitUsage, "passphrases do not match", false},
		{"empty passphrase", "linux", credentials.ErrPassphraseEmpty, ExitUsage, "passphrase cannot be empty", false},
		{"wrong passphrase", "linux", credentials.ErrIncorrectPassphrase, ExitNotConnected, "incorrect passphrase", false},
		{"damaged file", "linux", credentials.ErrDamaged, ExitNotConnected, "the credentials file is damaged; run `agent-cli connect <KEY>` to connect again", false},
		{"locked on linux", "linux", credentials.ErrLocked, ExitNotConnected, "token is locked; set THE_AGENT_CLI_PASSPHRASE or run `eval \"$(agent-cli unlock)\"`", false},
		{"locked on macOS", "darwin", credentials.ErrLocked, ExitNotConnected, "token is locked; set THE_AGENT_CLI_PASSPHRASE or run `eval \"$(agent-cli unlock)\"`", false},
		{"locked on windows", "windows", credentials.ErrLocked, ExitNotConnected, "token is locked; set THE_AGENT_CLI_PASSPHRASE or run `agent-cli unlock --shell pwsh | Invoke-Expression`", false},
		{"malformed environment token", "linux", credentials.ErrInvalidEnvToken, ExitNotConnected, "THE_AGENT_CLI_TOKEN is not a valid CLI token", false},
		{"not connected", "linux", credentials.ErrNotConnected, ExitNotConnected, "This CLI is not connected. Run `agent-cli connect <KEY>`.", true},
		{"not connected and metadata not cleared", "linux", errors.Join(credentials.ErrNotConnected, errors.New("disk full")), ExitNotConnected, "This CLI is not connected. Run `agent-cli connect <KEY>`.", true},
		{"keychain read failure", "linux", &credentials.StoreError{Op: credentials.OpRead, Mode: credentials.ModeKeychain, Err: errors.New("access denied")}, ExitFailure, "could not read the token from the system keychain: access denied", false},
		{"file write failure", "linux", &credentials.StoreError{Op: credentials.OpWrite, Mode: credentials.ModeUnsafe, Err: &fs.PathError{Op: "open", Path: "/h/credentials", Err: errors.New("permission denied")}}, ExitFailure, "could not store the token in the credentials file: permission denied", false},
		{"token too big", "linux", &credentials.StoreError{Op: credentials.OpWrite, Mode: credentials.ModeKeychain, Err: credentials.ErrTooBig}, ExitFailure, "could not store the token in the system keychain: the token is too big for the credential store", false},
		{"removal failure", "linux", &credentials.StoreError{Op: credentials.OpRemove, Mode: credentials.ModePassphrase, Err: errors.New("busy")}, ExitFailure, "could not remove the token from the encrypted credentials file: busy", false},
	} {
		var failure *Failure
		err := credentialFailure(text.MustNew(c.goos), "agent-cli", c.err)
		if !errors.As(err, &failure) || failure.Code != c.code || failure.Message != c.text || failure.Bare != c.bare {
			t.Errorf("%s: got %#v, want code %d, %q", c.name, err, c.code, c.text)
		}
	}
}

func TestCredentialFailureLeavesOtherErrorsAlone(t *testing.T) {
	other := errors.New("something else")
	if got := credentialFailure(text.MustNew("linux"), "agent-cli", other); !errors.Is(got, other) {
		t.Errorf("got %v", got)
	}
}

func TestPresentPrintsTheNotConnectedGuidanceWithoutThePrefix(t *testing.T) {
	var out bytes.Buffer
	err := credentialFailure(text.MustNew("linux"), "agent-cli", credentials.ErrNotConnected)
	if code := Present(&out, text.MustNew("linux"), err); code != ExitNotConnected {
		t.Errorf("exit code %d", code)
	}
	if want := "This CLI is not connected. Run `agent-cli connect <KEY>`.\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestTranslateAPIOutcomes(t *testing.T) {
	long := strings.Repeat("word ", 80)
	for _, c := range []struct {
		name string
		err  *api.Error
		code int
		text string
	}{
		{"unreachable", &api.Error{Kind: api.KindUnreachable}, ExitFailure, "could not reach the server"},
		{"rate limited", &api.Error{Kind: api.KindRateLimited, Status: 429}, ExitRateLimited, "rate limited; wait a moment and try again"},
		{"server fault", &api.Error{Kind: api.KindServerFault, Status: 500}, ExitFailure, "the server had a problem; try again later"},
		{"delivery failed", &api.Error{Kind: api.KindServerFault, Status: 502, Code: 5018}, ExitFailure, "the platform could not deliver the message; try again later"},
		{"policies", &api.Error{Kind: api.KindAccountGate, Status: 403, Code: 3008}, ExitFailure, "accept the agent's policies in chat settings, then try again"},
		{"waitlist", &api.Error{Kind: api.KindAccountGate, Status: 403, Code: 3007}, ExitFailure, "your account is still on the waitlist; try again once the agent has activated it"},
		{"no private chat", &api.Error{Kind: api.KindChatUnreachable, Status: 403, Code: 3004}, ExitChatUnreachable, "chat unreachable: no private chat found; start a conversation with the agent on Telegram or WhatsApp first"},
		{"delivery refused", &api.Error{Kind: api.KindChatUnreachable, Status: 403, Code: 3010}, ExitChatUnreachable, "chat unreachable: the platform refused the message; try another chat or platform"},
		{"not admin", &api.Error{Kind: api.KindChatUnreachable, Status: 403, Code: 3001}, ExitChatUnreachable, "chat unreachable: you are not an admin of that chat"},
		{"not member", &api.Error{Kind: api.KindChatUnreachable, Status: 403, Code: 3009}, ExitChatUnreachable, "chat unreachable: you are not an admin of that chat"},
		{"unknown chat", &api.Error{Kind: api.KindChatUnreachable, Status: 404, Code: 2002}, ExitChatUnreachable, "chat unreachable: no such chat; run `agent-cli chats` to list yours"},
		{"insufficient credits", &api.Error{Kind: api.KindInsufficientCredits, Status: 422, Code: 1034}, ExitInsufficientCredits, "insufficient credits; top up in the agent's chat settings"},
		{"invalid connect key", &api.Error{Kind: api.KindInvalidConnectKey, Status: 404, Code: 2015}, ExitFailure, "invalid connect key"},
		{"device rejected", &api.Error{Kind: api.KindDeviceRejected, Status: 422, Code: 1045}, ExitFailure, "the server rejected this machine's details; please report this"},
		{"invalid platform", &api.Error{Kind: api.KindValidation, Status: 422, Code: 1019, Message: "ignored"}, ExitUsage, "invalid platform; use telegram or whatsapp"},
		{"malformed chat id", &api.Error{Kind: api.KindValidation, Status: 422, Code: 1030}, ExitUsage, "malformed chat id; run `agent-cli chats` to list yours"},
		{"message rejected", &api.Error{Kind: api.KindValidation, Status: 422, Code: 1046}, ExitUsage, "the message was rejected: it must be between 1 and 4096 characters"},
		{"unknown validation code", &api.Error{Kind: api.KindValidation, Status: 422, Code: 1999, Message: "Something is off"}, ExitUsage, "Something is off"},
		{"validation text is neutralized", &api.Error{Kind: api.KindValidation, Status: 400, Message: "bad \x1b[31mred\x1b[0m\x07 input"}, ExitUsage, "bad red input"},
		{"validation text is cut", &api.Error{Kind: api.KindValidation, Status: 400, Message: long}, ExitUsage, strings.TrimSpace(long[:200])},
		{"framework validation body", &api.Error{Kind: api.KindValidation, Status: 422, Detail: true}, ExitUsage, "the server rejected the request"},
		{"validation without text", &api.Error{Kind: api.KindValidation, Status: 400}, ExitUsage, "the server rejected the request"},
		{"unexpected", &api.Error{Kind: api.KindUnexpected, Status: 302}, ExitFailure, "unexpected response from the server"},
		{"unknown route", &api.Error{Kind: api.KindFailure, Status: 404, Detail: true}, ExitFailure, "unexpected response from the server"},
		{"conflict", &api.Error{Kind: api.KindFailure, Status: 409, Code: 1999}, ExitFailure, "unexpected response from the server"},
	} {
		var failure *Failure
		err := translate(text.MustNew("linux"), "agent-cli", c.err)
		if !errors.As(err, &failure) || failure.Code != c.code || strings.TrimSpace(failure.Message) != c.text {
			t.Errorf("%s: got %#v, want code %d, %q", c.name, err, c.code, c.text)
		}
	}
}

func TestTranslateSessionOutcomes(t *testing.T) {
	cat := text.MustNew("linux")
	var failure *Failure
	err := translate(cat, "agent-cli", errors.Join(session.ErrEnvTokenRejected, &api.Error{Kind: api.KindTokenRejected, Status: 401}))
	if !errors.As(err, &failure) || failure.Code != ExitNotConnected || failure.Bare ||
		failure.Message != "the token in THE_AGENT_CLI_TOKEN was rejected by the server (expired or revoked)" {
		t.Errorf("rejected environment token: %#v", err)
	}

	cleared := translate(cat, "agent-cli", errors.Join(credentials.ErrNotConnected, &config.WriteError{Path: "/h/config.toml", Err: errors.New("disk full")}))
	if !errors.As(cleared, &failure) || failure.Code != ExitNotConnected || !failure.Bare {
		t.Errorf("not connected must win over the config write error: %#v", cleared)
	}
}

func TestTranslateKeepsExistingFailuresAndUnknownErrors(t *testing.T) {
	cat := text.MustNew("linux")
	own := &Failure{Code: ExitUsage, Message: "mine"}
	if got := translate(cat, "agent-cli", own); !errors.Is(got, own) {
		t.Errorf("got %v", got)
	}
	other := errors.New("something else")
	if got := translate(cat, "agent-cli", other); !errors.Is(got, other) {
		t.Errorf("got %v", got)
	}
	if translate(cat, "agent-cli", nil) != nil {
		t.Error("nil stays nil")
	}
	var failure *Failure
	if got := translate(cat, "agent-cli", config.ErrNoAPIURL); !errors.As(got, &failure) || failure.Code != ExitUsage {
		t.Errorf("config errors are translated too: %v", got)
	}
}

func TestPresentSilentFailurePrintsNothing(t *testing.T) {
	var out bytes.Buffer
	if code := Present(&out, text.MustNew("linux"), &Failure{Code: ExitFailure, Silent: true}); code != ExitFailure || out.Len() != 0 {
		t.Errorf("code %d, output %q", code, out.String())
	}
}
