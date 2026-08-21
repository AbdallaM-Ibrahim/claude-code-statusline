package main

import (
	"fmt"
	"strings"
)

// modelSegment renders "🤖 Opus 5 ⚡ xhigh 💭".
func modelSegment(in *StatusLineInput) string {
	name := safeTerminal(in.Model.DisplayName)
	if name == "" {
		name = safeTerminal(in.Model.ID)
	}
	if name == "" {
		name = "Claude"
	}

	bits := []string{name}
	if in.FastMode {
		bits = append(bits, "⚡")
	}
	if in.Effort.Level != "" {
		bits = append(bits, dim(safeTerminal(in.Effort.Level)))
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
func buildSessionLine(in *StatusLineInput, limits string, cost []string) string {
	segments := []string{modelSegment(in)}

	if pct, ok := in.contextPercent(); ok {
		segments = append(segments, fmt.Sprintf("🧠 %s", heat(pct, fmt.Sprintf("%d%%", pct))))
	}

	segments = append(segments, cost...)

	if limits != "" {
		segments = append(segments, limits)
	}

	if cave := cavemanSegment(); cave != "" {
		segments = append(segments, cave)
	}

	return strings.Join(segments, dim(" | "))
}
