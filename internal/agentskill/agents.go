// Package agentskill manages the CLI's small owned skills without agent settings or hooks.
package agentskill

import "path/filepath"

type Agent struct{ Name, DisplayName string }

func Supported() [9]Agent {
	return [9]Agent{
		{"claude", "Claude Code"},
		{"codex", "Codex"},
		{"opencode", "OpenCode"},
		{"pi", "Pi"},
		{"omp", "Oh-My-Pi (OMP)"},
		{"windsurf", "Windsurf"},
		{"cursor", "Cursor"},
		{"gemini", "Gemini CLI"},
		{"antigravity", "Antigravity"},
	}
}

func Lookup(name string) (Agent, bool) {
	for _, a := range Supported() {
		if a.Name == name {
			return a, true
		}
	}
	return Agent{}, false
}

func Directory(agent Agent, root, claudeConfig, buildName string, project bool) string {
	if agent.Name == "claude" {
		if !project && claudeConfig != "" {
			return filepath.Join(claudeConfig, "skills", buildName)
		}
		return filepath.Join(root, ".claude", "skills", buildName)
	}
	if agent.Name == "antigravity" && !project {
		return filepath.Join(root, ".gemini", "config", "skills", buildName)
	}
	return filepath.Join(root, ".agents", "skills", buildName)
}
