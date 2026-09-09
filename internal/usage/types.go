package usage

import (
	"encoding/json"
	"time"
)

// Config is the parsed STATUSLINE_USAGE_REFRESH switch.
type Config struct {
	// Enabled is false unless the user opted in; every code path in this package
	// that touches a credential or the network is behind it.
	Enabled bool
	// Interval is how old the newest known record may be before a fetch.
	Interval time.Duration
}

// record is the on-disk cache. It is deliberately the same subtree Claude Code
// keeps in ~/.claude.json, so the limits segment reads both files with one
// parser and simply takes whichever was fetched last.
type record struct {
	CachedUsageUtilization struct {
		FetchedAtMs int64 `json:"fetchedAtMs"`
		// Utilization is the usage endpoint's response body, stored verbatim.
		Utilization json.RawMessage `json:"utilization"`
	} `json:"cachedUsageUtilization"`
}

// credentials is the slice of Claude Code's credential store this program
// reads. Every other field — the refresh token above all — is never decoded.
type credentials struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		// ExpiresAt is Unix milliseconds; 0 when the store does not say.
		ExpiresAt int64 `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

// responseProbe is the one field of the usage response that must be present for
// the body to be worth caching: the limits list the segment is built from.
type responseProbe struct {
	Limits json.RawMessage `json:"limits"`
}
