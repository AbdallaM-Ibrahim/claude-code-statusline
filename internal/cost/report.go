// Package cost turns Claude Code transcripts into the money segments on line 2:
// today's spend, the active 5-hour billing block, and a burn rate.
//
// Transcripts record token counts and a model name, never a cost, so the scan
// (state.go) buckets tokens by hour and model, and the pricing table
// (pricing.go) converts them to dollars at aggregation time. The scan is
// incremental and remembers its offsets between renders in a small state file.
package cost

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
)

// blockSeconds is Anthropic's billing block length. Blocks are aligned to the
// hour of the first activity, which is why the scan buckets by hour.
const blockSeconds int64 = 5 * 3600

// Report is everything line 2's money segments need.
type Report struct {
	SessionUSD    float64 // straight from the payload, never computed
	HasSession    bool
	TodayUSD      float64
	BlockUSD      float64
	BlockStart    int64
	BlockEnd      int64
	BurnPerHour   float64
	Approximate   bool // at least one model was missing from the pricing table
	UnknownModels []string
}

// Build brings the incremental scan up to date and aggregates it.
func Build(in *payload.Input, now time.Time) *Report {
	rep := &Report{}
	if in.Cost.TotalCostUSD != nil {
		rep.SessionUSD = *in.Cost.TotalCostUSD
		rep.HasSession = true
	}

	statePath := paths.CostState()
	st := loadState(statePath)
	st.scan(paths.ProjectsDir(), now)
	st.save(statePath)

	aggregate(st, rep, now)
	return rep
}

// aggregate turns hour buckets into today, the active block, and a burn rate.
func aggregate(st *state, rep *Report, now time.Time) {
	hours := make([]int64, 0, len(st.Hours))
	for k := range st.Hours {
		if h, err := strconv.ParseInt(k, 10, 64); err == nil {
			hours = append(hours, h)
		}
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i] < hours[j] })
	if len(hours) == 0 {
		return
	}

	unknown := map[string]bool{}
	costAt := func(h int64) float64 {
		var sum float64
		for model, tc := range st.Hours[strconv.FormatInt(h, 10)] {
			usd, ok := costOf(model, tc)
			if !ok {
				unknown[model] = true
				continue
			}
			sum += usd
		}
		return sum
	}

	// Today, in local time — the same day boundary the user sees on a clock.
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()

	// Blocks: walk forward, starting a new one whenever an hour falls outside
	// the current block's 5h span.
	blockStart := hours[0]
	var blockCost float64
	activeStart, activeCost := blockStart, 0.0

	for _, h := range hours {
		if h >= blockStart+blockSeconds {
			blockStart = h
			blockCost = 0
		}
		c := costAt(h)
		blockCost += c
		if h >= midnight {
			rep.TodayUSD += c
		}
		activeStart, activeCost = blockStart, blockCost
	}

	nowUnix := now.Unix()
	// Only report a block that has not already elapsed.
	if activeStart+blockSeconds > nowUnix {
		rep.BlockStart = activeStart
		rep.BlockEnd = activeStart + blockSeconds
		rep.BlockUSD = activeCost

		elapsed := float64(nowUnix-activeStart) / 3600.0
		if elapsed < 1.0/60.0 {
			elapsed = 1.0 / 60.0 // a fresh block must not divide by ~zero
		}
		rep.BurnPerHour = activeCost / elapsed
	}

	if len(unknown) > 0 {
		rep.Approximate = true
		for m := range unknown {
			rep.UnknownModels = append(rep.UnknownModels, m)
		}
		sort.Strings(rep.UnknownModels)
	}
}

func money(v float64) string {
	return fmt.Sprintf("$%.2f", v)
}

// burnEmoji is our own threshold, not a reproduction of ccusage's.
//
// ccusage's marker did not track the rate monotonically in observed output
// ($5.27/hr rendered green while $4.32/hr rendered warning), so it keys off
// something not derivable from the transcript. Rather than guess at that, this
// is a plain, documented rate threshold.
func burnEmoji(perHour float64) string {
	switch {
	case perHour >= 15:
		return "🔴"
	case perHour >= 5:
		return "⚠️"
	default:
		return "🟢"
	}
}

// Segments renders the 💰 and 🔥 segments.
//
// dropBlock removes the block estimate when the payload already carries an
// authoritative rate-limit window, matching the previous implementation: two
// competing "how much is left" readings on one line is one too many.
func Segments(rep *Report, dropBlock bool) []string {
	if rep == nil {
		return nil
	}

	approx := ""
	if rep.Approximate {
		approx = "~"
	}

	var money1 []string
	if rep.HasSession {
		// Session cost comes from Claude Code itself, so it is exact even when
		// the derived figures are not.
		money1 = append(money1, money(rep.SessionUSD)+" session")
	}
	money1 = append(money1, approx+money(rep.TodayUSD)+" today")

	if !dropBlock && rep.BlockEnd > 0 {
		left := time.Duration(rep.BlockEnd-time.Now().Unix()) * time.Second
		money1 = append(money1, fmt.Sprintf("%s%s block (%s left)",
			approx, money(rep.BlockUSD), compactDuration(left)))
	}

	segments := []string{"💰 " + strings.Join(money1, " / ")}

	if rep.BurnPerHour > 0 {
		segments = append(segments, fmt.Sprintf("🔥 %s%s/hr %s",
			approx, money(rep.BurnPerHour), burnEmoji(rep.BurnPerHour)))
	}

	return segments
}

// compactDuration renders "3h 42m", the form the block countdown used.
func compactDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
