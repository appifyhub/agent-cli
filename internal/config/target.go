package config

import "strings"

// Target is where a notification goes when the command names none: a chat, a platform, or neither.
type Target struct {
	ChatID   string
	Platform string
}

// DefaultTarget is THE_AGENT_CLI_DEFAULT_CHAT, else THE_AGENT_CLI_DEFAULT_PLATFORM, else the default in
// the config file. Empty variables count as unset. A platform in the environment must be telegram or
// whatsapp (any case) even when a chat takes precedence, otherwise the error is an *InvalidPlatformError.
func DefaultTarget(getenv func(string) string, f File) (Target, error) {
	chat, platform := getenv(EnvDefaultChat), getenv(EnvDefaultPlatform)
	if platform != "" {
		normalized := strings.ToLower(platform)
		if normalized != "telegram" && normalized != "whatsapp" {
			return Target{}, &InvalidPlatformError{Value: platform}
		}
		platform = normalized
	}
	switch {
	case chat != "":
		return Target{ChatID: chat}, nil
	case platform != "":
		return Target{Platform: platform}, nil
	case f.Default.ChatID != "":
		return Target{ChatID: f.Default.ChatID}, nil
	}
	return Target{Platform: f.Default.Platform}, nil
}
