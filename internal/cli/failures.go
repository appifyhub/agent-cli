package cli

import (
	"errors"

	"github.com/appifyhub/agent-cli/internal/config"
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
