package usage

import (
	"os"
	"strings"
	"time"
)

// EnvSwitch is the environment variable that turns the fetch on. It is read
// from the status line process's own environment, so the natural place to set
// it is a prefix on the command in settings.json:
//
//	"command": "STATUSLINE_USAGE_REFRESH=1 /Users/you/.claude/statusline"
const EnvSwitch = "STATUSLINE_USAGE_REFRESH"

const (
	// DefaultInterval matches how often Claude Code itself is willing to persist
	// a fresh usage record (its /usage fetch has a five-minute write throttle).
	DefaultInterval = 5 * time.Minute
	// MinInterval floors a user-supplied interval. The endpoint is cheap, but a
	// status line has no business polling an account API every few seconds.
	MinInterval = 2 * time.Minute
)

// ConfigFromEnv reads the switch from the process environment.
func ConfigFromEnv() Config { return ParseConfig(os.Getenv(EnvSwitch)) }

// ParseConfig turns the switch's value into a Config.
//
// Unset, empty, "0", "false", "off" and "no" are off. "1", "true", "on" and
// "yes" are on at DefaultInterval. A Go duration ("10m", "1h") is on at that
// interval, floored at MinInterval. Anything else is off: a typo must never turn
// into a network call.
func ParseConfig(v string) Config {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "", "0", "false", "off", "no":
		return Config{}
	case "1", "true", "on", "yes":
		return Config{Enabled: true, Interval: DefaultInterval}
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return Config{}
	}
	if d < MinInterval {
		d = MinInterval
	}
	return Config{Enabled: true, Interval: d}
}
