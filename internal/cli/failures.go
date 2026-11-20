package cli

import (
	"errors"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/credentials"
	"github.com/appifyhub/agent-cli/internal/session"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/text"
)

// configFailure gives the typed errors of the config package their catalog text and the exit code the
// specs name; any other error is returned unchanged.
func configFailure(cat *text.Catalog, err error) error {
	var (
		home     *config.HomeError
		read     *config.ReadError
		write    *config.WriteError
		url      *config.InvalidURLError
		platform *config.InvalidPlatformError
	)
	switch {
	case errors.Is(err, config.ErrHomeUnavailable):
		return &Failure{Code: ExitFailure, Message: cat.T("error.home_unavailable")}
	case errors.As(err, &home):
		return &Failure{Code: ExitFailure, Message: cat.T("error.home_unusable", "path", home.Path, "reason", home.Reason())}
	case errors.As(err, &read):
		return &Failure{Code: ExitFailure, Message: cat.T("error.config_unreadable", "path", read.Path, "reason", read.Reason())}
	case errors.As(err, &write):
		return &Failure{Code: ExitFailure, Message: cat.T("error.config_unwritable", "path", write.Path, "reason", write.Reason())}
	case errors.Is(err, config.ErrNoAPIURL):
		return &Failure{Code: ExitUsage, Message: cat.T("error.no_api_url")}
	case errors.As(err, &url):
		return &Failure{Code: ExitUsage, Message: invalidURLMessage(cat, url)}
	case errors.As(err, &platform):
		return &Failure{Code: ExitUsage, Message: cat.T("error.invalid_platform", "value", platform.Value)}
	}
	return err
}

func invalidURLMessage(cat *text.Catalog, err *config.InvalidURLError) string {
	switch err.Problem {
	case config.URLInsecure:
		return cat.T("error.api_url_insecure", "url", err.URL)
	case config.URLCredentials:
		return cat.T("error.api_url_credentials", "url", err.URL)
	case config.URLQueryOrFragment:
		return cat.T("error.api_url_extras", "url", err.URL)
	default:
		return cat.T("error.api_url_malformed", "url", err.URL)
	}
}

// credentialFailure gives the typed errors of the credentials package their catalog text and the
// exit code the specs name; any other error is returned unchanged. name is the command name of the build.
func credentialFailure(cat *text.Catalog, name string, err error) error {
	var (
		flag  *credentials.StoreFlagError
		store *credentials.StoreError
	)
	switch {
	case errors.As(err, &flag):
		return &Failure{Code: ExitUsage, Message: cat.T("error.invalid_store", "value", flag.Value)}
	case errors.Is(err, credentials.ErrNoKeychain):
		return &Failure{Code: ExitUsage, Message: cat.T("error.no_keychain")}
	case errors.Is(err, credentials.ErrNoPassphraseSource):
		return &Failure{Code: ExitUsage, Message: cat.T("error.passphrase_required")}
	case errors.Is(err, credentials.ErrNoStore):
		return &Failure{Code: ExitUsage, Message: cat.T("error.no_store")}
	case errors.Is(err, credentials.ErrPassphraseMismatch):
		return &Failure{Code: ExitUsage, Message: cat.T("error.passphrase_mismatch")}
	case errors.Is(err, credentials.ErrPassphraseEmpty):
		return &Failure{Code: ExitUsage, Message: cat.T("error.passphrase_empty")}
	case errors.Is(err, credentials.ErrIncorrectPassphrase):
		return &Failure{Code: ExitNotConnected, Message: cat.T("error.incorrect_passphrase")}
	case errors.Is(err, credentials.ErrDamaged):
		return &Failure{Code: ExitNotConnected, Message: cat.T("error.credentials_damaged", "name", name)}
	case errors.Is(err, credentials.ErrLocked):
		return &Failure{Code: ExitNotConnected, Message: cat.T("error.token_locked", "name", name)}
	case errors.Is(err, credentials.ErrInvalidEnvToken):
		return &Failure{Code: ExitNotConnected, Message: cat.T("error.invalid_env_token")}
	case errors.Is(err, credentials.ErrNotConnected):
		return &Failure{Code: ExitNotConnected, Message: cat.T("error.not_connected", "name", name), Bare: true}
	case errors.As(err, &store):
		return &Failure{Code: ExitFailure, Message: storeMessage(cat, store)}
	}
	return err
}

func storeMessage(cat *text.Catalog, err *credentials.StoreError) string {
	where := cat.T("store." + string(err.Mode))
	reason := err.Reason()
	if errors.Is(err.Err, credentials.ErrTooBig) {
		reason = cat.T("error.token_too_big")
	}
	switch err.Op {
	case credentials.OpRead:
		return cat.T("error.store_read", "store", where, "reason", reason)
	case credentials.OpRemove:
		return cat.T("error.store_remove", "store", where, "reason", reason)
	}
	return cat.T("error.store_write", "store", where, "reason", reason)
}

// translate is the one table from domain and API errors to a Failure: the exit code and the catalog
// text of every error a command can return. Errors it does not know are returned unchanged, and
// Present reports them as failures. name is the command name of the build.
func translate(cat *text.Catalog, name string, err error) error {
	if err == nil {
		return nil
	}
	var known *Failure
	if errors.As(err, &known) {
		return err
	}
	if errors.Is(err, session.ErrEnvTokenRejected) {
		return &Failure{Code: ExitNotConnected, Message: cat.T("error.env_token_rejected")}
	}
	for _, convert := range []func(error) error{
		func(e error) error { return credentialFailure(cat, name, e) },
		func(e error) error { return apiFailure(cat, name, e) },
		func(e error) error { return configFailure(cat, e) },
	} {
		var converted *Failure
		if errors.As(convert(err), &converted) {
			return converted
		}
	}
	return err
}

// apiFailure gives the outcomes of backend calls their catalog text and exit code.
func apiFailure(cat *text.Catalog, name string, err error) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	fail := func(code int, message string) error { return &Failure{Code: code, Message: message} }
	switch apiErr.Kind {
	case api.KindUnreachable:
		return fail(ExitFailure, cat.T("error.unreachable"))
	case api.KindRateLimited:
		return fail(ExitRateLimited, cat.T("error.rate_limited"))
	case api.KindServerFault:
		if apiErr.Code == api.CodeNotificationUndelivered {
			return fail(ExitFailure, cat.T("error.delivery_failed"))
		}
		return fail(ExitFailure, cat.T("error.server_fault"))
	case api.KindAccountGate:
		if apiErr.Code == api.CodeWaitlistNotActive {
			return fail(ExitFailure, cat.T("error.waitlist"))
		}
		return fail(ExitFailure, cat.T("error.policies_required"))
	case api.KindChatUnreachable:
		return fail(ExitChatUnreachable, chatUnreachableMessage(cat, name, apiErr.Code))
	case api.KindInsufficientCredits:
		return fail(ExitInsufficientCredits, cat.T("error.insufficient_credits"))
	case api.KindInvalidConnectKey:
		return fail(ExitFailure, cat.T("error.invalid_connect_key"))
	case api.KindDeviceRejected:
		return fail(ExitFailure, cat.T("error.device_rejected"))
	case api.KindValidation, api.KindInvalidTTL, api.KindTTLTooLong:
		return fail(ExitUsage, validationMessage(cat, name, apiErr))
	}
	return fail(ExitFailure, cat.T("error.unexpected_response"))
}

func chatUnreachableMessage(cat *text.Catalog, name string, code int) string {
	switch code {
	case api.CodeNoPrivateChat:
		return cat.T("error.chat_no_private")
	case api.CodeDeliveryRefused:
		return cat.T("error.chat_delivery_refused")
	case api.CodeChatNotFound:
		return cat.T("error.chat_unknown", "name", name)
	}
	return cat.T("error.chat_not_admin")
}

// validationMessage is the catalog text of a known validation code, else the cleaned server text
// cut to 200 characters, else a generic one for framework bodies and empty messages.
func validationMessage(cat *text.Catalog, name string, err *api.Error) string {
	switch err.Code {
	case api.CodeInvalidPlatform:
		return cat.T("error.server_invalid_platform")
	case api.CodeMalformedChatID:
		return cat.T("error.malformed_chat_id", "name", name)
	case api.CodeInvalidNotification:
		return cat.T("error.message_rejected")
	}
	if message := term.Clean(err.Message, 200); message != "" {
		return message
	}
	return cat.T("error.request_rejected")
}
