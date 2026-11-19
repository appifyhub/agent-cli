package cli

import "strings"

func (s *rootState) unlockShell(shell string) (string, error) {
	if shell == "" {
		if s.deps.GOOS == "windows" {
			return "pwsh", nil
		}
		return "sh", nil
	}
	switch shell {
	case "sh", "bash", "zsh", "fish", "pwsh", "powershell":
		return shell, nil
	default:
		return "", usageError(s.deps.Text.T("unlock.invalid_shell", "shell", shell))
	}
}

func shellAssignment(shell, value string) string {
	if shell == "pwsh" || shell == "powershell" {
		return "$env:THE_AGENT_CLI_TOKEN = '" + strings.ReplaceAll(value, "'", "''") + "'\n"
	}
	if shell == "fish" {
		value = strings.ReplaceAll(value, "\\", "\\\\")
	}
	quoted := "'" + strings.ReplaceAll(value, "'", "'\\''") + "'\n"
	if shell == "fish" {
		return "set -gx THE_AGENT_CLI_TOKEN " + quoted
	}
	return "export THE_AGENT_CLI_TOKEN=" + quoted
}
