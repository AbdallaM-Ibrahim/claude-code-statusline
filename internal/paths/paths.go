// Package paths resolves where Claude Code keeps its state.
//
// Path resolution is centralised so the cross-platform rules live in one place:
// always filepath.Join, always os.UserHomeDir, and always honour
// CLAUDE_CONFIG_DIR where Claude Code itself does. Every file the status line
// reads or writes under ~/.claude is named here and nowhere else.
package paths

import (
	"os"
	"path/filepath"
)

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// ClaudeDir is where Claude Code keeps its state. CLAUDE_CONFIG_DIR overrides
// it, matching the caveman plugin's own hook.
func ClaudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(homeDir(), ".claude")
}

// GlobalConfig is the account-wide record Claude refreshes from the API on a
// ~5 minute throttle. The status line only ever reads it.
func GlobalConfig() string {
	return filepath.Join(homeDir(), ".claude.json")
}

// ProjectsDir holds one subdirectory per project, each full of .jsonl
// transcripts.
func ProjectsDir() string {
	return filepath.Join(ClaudeDir(), "projects")
}

// CavemanFlag is the caveman plugin's activation file.
func CavemanFlag() string {
	return filepath.Join(ClaudeDir(), ".caveman-active")
}

// CavemanSuffix is the savings suffix written by /caveman-stats.
func CavemanSuffix() string {
	return filepath.Join(ClaudeDir(), ".caveman-statusline-suffix")
}

// Credentials is Claude Code's credential store on Windows and Linux (macOS keeps
// it in the Keychain). Read only when STATUSLINE_USAGE_REFRESH is on.
func Credentials() string {
	return filepath.Join(ClaudeDir(), ".credentials.json")
}

// UsageCache holds the usage record this program fetched itself, in the shape of
// ~/.claude.json's cachedUsageUtilization subtree.
func UsageCache() string {
	return filepath.Join(ClaudeDir(), "statusline-usage.json")
}

// UsageLock serialises usage fetches across concurrent sessions.
func UsageLock() string {
	return filepath.Join(ClaudeDir(), "statusline-usage.lock")
}

// CostState holds the incremental transcript scan offsets.
func CostState() string {
	return filepath.Join(ClaudeDir(), "statusline-cost-state.json")
}

// GitCache holds the ahead/behind counts keyed on commit hash pairs.
func GitCache() string {
	return filepath.Join(ClaudeDir(), "statusline-git-cache.json")
}
