// Package payload is the stdin contract: the JSON Claude Code writes to the
// status line command, and the few derived readings taken straight from it.
package payload

import (
	"bytes"
	"encoding/json"
	"math"
)

// Input is the JSON Claude Code writes to our stdin.
//
// Optional numbers are pointers on purpose: a rate-limit window reporting 0% is
// meaningfully different from one reporting nothing at all, and treating the
// absent case as zero would render a confident "0%" for a window we know
// nothing about. Everything else is tolerant — unknown fields are ignored, so a
// new Claude Code release cannot break the render by adding to the payload.
type Input struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`

	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`

	Workspace struct {
		CurrentDir string `json:"current_dir"`
		ProjectDir string `json:"project_dir"`
	} `json:"workspace"`

	Cost struct {
		TotalCostUSD *float64 `json:"total_cost_usd"`
	} `json:"cost"`

	ContextWindow struct {
		ContextWindowSize int           `json:"context_window_size"`
		TotalInputTokens  int           `json:"total_input_tokens"`
		UsedPercentage    *float64      `json:"used_percentage"`
		CurrentUsage      *ContextUsage `json:"current_usage"`
	} `json:"context_window"`

	FastMode bool `json:"fast_mode"`

	Effort struct {
		Level string `json:"level"`
	} `json:"effort"`

	Thinking struct {
		Enabled bool `json:"enabled"`
	} `json:"thinking"`

	RateLimits struct {
		FiveHour *Window `json:"five_hour"`
		SevenDay *Window `json:"seven_day"`
	} `json:"rate_limits"`

	Agent struct {
		Name string `json:"name"`
	} `json:"agent"`

	PR struct {
		Number int `json:"number"`
	} `json:"pr"`

	Worktree struct {
		Name   string `json:"name"`
		Branch string `json:"branch"`
	} `json:"worktree"`
}

// ContextUsage is the token breakdown behind the context percentage.
type ContextUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// Window is one rate-limit window as the payload reports it.
type Window struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

// utf8BOM is stripped before decoding. Go's JSON decoder rejects a leading BOM
// outright, and anything that pipes the payload through a Windows shell can add
// one — which turns a perfectly good payload into the bare "🤖 Claude" fallback.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Decode parses the raw stdin bytes.
func Decode(raw []byte) (*Input, error) {
	raw = bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(raw), utf8BOM))

	var in Input
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	return &in, nil
}

// Dir is the directory line 1 describes, preferring the workspace over the
// bare cwd exactly as the previous implementation did.
func (in *Input) Dir(fallback string) string {
	if in.Workspace.CurrentDir != "" {
		return in.Workspace.CurrentDir
	}
	if in.CWD != "" {
		return in.CWD
	}
	return fallback
}

// ContextPercent prefers the pre-computed percentage and falls back to deriving
// one from the token breakdown.
func (in *Input) ContextPercent() (int, bool) {
	cw := in.ContextWindow
	if cw.UsedPercentage != nil {
		return int(math.Round(*cw.UsedPercentage)), true
	}
	if cw.CurrentUsage == nil || cw.ContextWindowSize <= 0 {
		return 0, false
	}
	used := cw.CurrentUsage.InputTokens +
		cw.CurrentUsage.CacheCreationInputTokens +
		cw.CurrentUsage.CacheReadInputTokens
	return int(math.Round(float64(used) / float64(cw.ContextWindowSize) * 100)), true
}
