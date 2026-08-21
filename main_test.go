package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderMalformedStdinFallsBack(t *testing.T) {
	got := render([]byte("not json"))
	if got != "🤖 Claude" {
		t.Errorf("render = %q, want the single-line fallback", got)
	}
	if strings.Contains(got, "\n") {
		t.Error("the fallback must be a single line")
	}
}

// Truncated JSON cannot yield a model name: the only route to the fallback is a
// parse failure, so any retry would fail the same way.
func TestRenderTruncatedJSONFallsBack(t *testing.T) {
	got := render([]byte(`{"model":{"display_name":"Opus 5"},`))
	if got != "🤖 Claude" {
		t.Errorf("render = %q, want the bare fallback", got)
	}
}

// A valid JSON object always decodes, because unknown fields are ignored — so a
// well-formed payload never reaches the fallback no matter what it contains.
func TestRenderUnknownFieldsStillDecode(t *testing.T) {
	got := render([]byte(`{"totally":"unexpected","cwd":"."}`))
	if got == "🤖 Claude" {
		t.Error("a valid object with unknown fields should still render fully")
	}
}

func TestRenderEmptyStdinDoesNotPanic(t *testing.T) {
	if got := render(nil); got != "🤖 Claude" {
		t.Errorf("render(nil) = %q", got)
	}
}

// Anything piping the payload through a Windows shell can prepend a BOM, which
// Go's JSON decoder rejects outright.
func TestRenderToleratesUTF8BOM(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"model":          map[string]string{"display_name": "Opus 5"},
		"cwd":            t.TempDir(),
		"context_window": map[string]any{"used_percentage": 34},
	})
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, payload...)

	got := render(withBOM)
	if got == "🤖 Claude" {
		t.Fatal("a BOM-prefixed payload fell through to the fallback")
	}
	if !strings.Contains(got, "🧠") {
		t.Errorf("expected a full render, got %q", got)
	}
}

func TestRenderAlwaysProducesTwoLines(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"model": map[string]string{"display_name": "Opus 5"},
		"cwd":   t.TempDir(), // not a repository
	})
	got := render(payload)
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("expected exactly two lines, got %d newlines: %q", n, got)
	}
}

func TestModelSegmentVariants(t *testing.T) {
	in := &StatusLineInput{}
	if got := modelSegment(in); got != "🤖 Claude" {
		t.Errorf("empty model = %q, want the Claude default", got)
	}

	in.Model.DisplayName = "Opus 5"
	in.FastMode = true
	in.Effort.Level = "xhigh"
	in.Thinking.Enabled = true

	got := modelSegment(in)
	for _, want := range []string{"Opus 5", "⚡", "💭", dim("xhigh")} {
		if !strings.Contains(got, want) {
			t.Errorf("model segment %q missing %q", got, want)
		}
	}
}

func TestModelSegmentFallsBackToID(t *testing.T) {
	in := &StatusLineInput{}
	in.Model.ID = "claude-opus-5"
	if got := modelSegment(in); !strings.Contains(got, "claude-opus-5") {
		t.Errorf("model segment = %q, want the id when no display name is set", got)
	}
}

// A payload carrying no cost must not invent one.
func TestSessionCostAbsentIsNotRenderedAsZero(t *testing.T) {
	rep := &CostReport{HasSession: false, TodayUSD: 1.5}
	segs := costSegments(rep, true)
	if len(segs) == 0 {
		t.Fatal("expected a today segment")
	}
	if strings.Contains(segs[0], "session") {
		t.Errorf("no session cost in the payload should render no session figure: %q", segs[0])
	}
}

// Claude Code passes no arguments, so this exists purely so a binary someone
// downloaded can say what it is.
func TestVersionRequest(t *testing.T) {
	for _, args := range [][]string{
		{"statusline", "--version"},
		{"statusline", "-version"},
		{"statusline", "version"},
		{"statusline", "-V"},
	} {
		if !versionRequest(args) {
			t.Errorf("%v should ask for the version", args[1:])
		}
	}
	for _, args := range [][]string{
		{"statusline"},
		{"statusline", "--padding"},
		{"statusline", ""},
	} {
		if versionRequest(args) {
			t.Errorf("%v should render, not print a version", args[1:])
		}
	}
}

// An unstamped build must still answer, rather than printing an empty line.
func TestVersionDefaultIsNotEmpty(t *testing.T) {
	if version == "" {
		t.Error("version must never be empty")
	}
}
