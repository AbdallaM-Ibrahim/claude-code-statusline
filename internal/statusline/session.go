package statusline

import (
	"fmt"
	"strings"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/caveman"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/term"
)

// modelSegment renders "🤖 Opus 5 ⚡ xhigh 💭".
func modelSegment(in *payload.Input) string {
	name := term.Sanitize(in.Model.DisplayName)
	if name == "" {
		name = term.Sanitize(in.Model.ID)
	}
	if name == "" {
		name = "Claude"
	}

	bits := []string{name}
	if in.FastMode {
		bits = append(bits, "⚡")
	}
	if in.Effort.Level != "" {
		bits = append(bits, term.Dim(term.Sanitize(in.Effort.Level)))
	}
	if in.Thinking.Enabled {
		bits = append(bits, "💭")
	}
	return "🤖 " + strings.Join(bits, " ")
}

// buildSessionLine renders line 2: what the session costs.
//
// limits and cost are gathered by the caller so they can run concurrently with
// line 1's git read.
func buildSessionLine(in *payload.Input, limits string, cost []string) string {
	segments := []string{modelSegment(in)}

	if pct, ok := in.ContextPercent(); ok {
		segments = append(segments, fmt.Sprintf("🧠 %s", term.Heat(pct, fmt.Sprintf("%d%%", pct))))
	}

	segments = append(segments, cost...)

	if limits != "" {
		segments = append(segments, limits)
	}

	if cave := caveman.Segment(); cave != "" {
		segments = append(segments, cave)
	}

	return strings.Join(segments, term.Dim(" | "))
}
