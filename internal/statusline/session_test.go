package statusline

import (
	"strings"
	"testing"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/term"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

func TestModelSegmentVariants(t *testing.T) {
	in := &payload.Input{}
	if got := modelSegment(in); got != "🤖 Claude" {
		t.Errorf("empty model = %q, want the Claude default", got)
	}

	in.Model.DisplayName = "Opus 5"
	in.FastMode = true
	in.Effort.Level = "xhigh"
	in.Thinking.Enabled = true

	got := modelSegment(in)
	for _, want := range []string{"Opus 5", "⚡", "💭", term.Dim("xhigh")} {
		if !strings.Contains(got, want) {
			t.Errorf("model segment %q missing %q", got, want)
		}
	}
}

func TestModelSegmentFallsBackToID(t *testing.T) {
	in := &payload.Input{}
	in.Model.ID = "claude-opus-5"
	if got := modelSegment(in); !strings.Contains(got, "claude-opus-5") {
		t.Errorf("model segment = %q, want the id when no display name is set", got)
	}
}

func TestSessionLineOrdersSegments(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir()) // do not read the real caveman flag

	in := &payload.Input{}
	in.Model.DisplayName = "Opus 5"
	pct := 34.0
	in.ContextWindow.UsedPercentage = &pct

	got := testutil.StripANSI(buildSessionLine(in, "⏳ 5h 42%", []string{"💰 $1.42 session", "🔥 $2.10/hr 🟢"}))
	want := "🤖 Opus 5 | 🧠 34% | 💰 $1.42 session | 🔥 $2.10/hr 🟢 | ⏳ 5h 42%"
	if got != want {
		t.Errorf("session line = %q, want %q", got, want)
	}
}

func TestSessionLineNeutralisesHostilePayloadStrings(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir()) // do not read the real caveman flag

	in := &payload.Input{}
	in.Model.DisplayName = "Opus 5\x1b[2J"
	in.Effort.Level = "xhigh\u009b0m"

	got := buildSessionLine(in, "", nil)

	testutil.AssertNoInjection(t, got)
	if !strings.Contains(got, "Opus 5") || !strings.Contains(got, "xhigh") {
		t.Errorf("session line %q lost its text", got)
	}
}
