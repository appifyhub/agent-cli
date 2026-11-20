package cli

import (
	"strings"

	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/text"
)

func notifyTarget(chat, platform string, chatSet, platformSet bool, getenv func(string) string, file config.File, cat *text.Catalog) (config.Target, error) {
	if chatSet && platformSet {
		return config.Target{}, usageError(cat.T("notify.conflicting_targets"))
	}
	if chatSet {
		if strings.TrimSpace(chat) == "" {
			return config.Target{}, usageError(cat.T("notify.empty_chat"))
		}
		return config.Target{ChatID: chat}, nil
	}
	if platformSet {
		platform = strings.ToLower(platform)
		if platform != "telegram" && platform != "whatsapp" {
			return config.Target{}, usageError(cat.T("chats.invalid_platform", "value", platform))
		}
		return config.Target{Platform: platform}, nil
	}
	return config.DefaultTarget(getenv, file)
}
