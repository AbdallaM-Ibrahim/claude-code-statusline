package main

import (
	"os"
	"path/filepath"
)

// Path resolution is centralised so the cross-platform rules live in one place:
// always filepath.Join, always os.UserHomeDir, and always honour
// CLAUDE_CONFIG_DIR where Claude Code itself does.

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// claudeDir is where Claude Code keeps its state. CLAUDE_CONFIG_DIR overrides
// it, matching the caveman plugin's own hook.
func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(homeDir(), ".claude")
}

// globalConfigPath is the account-wide record Claude refreshes from the API on a
// ~5 minute throttle. The status line only ever reads it.
func globalConfigPath() string {
	return filepath.Join(homeDir(), ".claude.json")
}

func projectsDir() string {
	return filepath.Join(claudeDir(), "projects")
}

func cavemanFlagPath() string {
	return filepath.Join(claudeDir(), ".caveman-active")
}

func cavemanSuffixPath() string {
	return filepath.Join(claudeDir(), ".caveman-statusline-suffix")
}

// costStatePath holds the incremental transcript scan offsets.
func costStatePath() string {
	return filepath.Join(claudeDir(), "statusline-cost-state.json")
}
