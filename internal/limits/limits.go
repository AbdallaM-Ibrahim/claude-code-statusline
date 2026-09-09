// Package limits renders the rate-limit segment: the 5-hour and 7-day windows
// and any per-model weekly caps.
//
// Two sources describe the same windows — the payload's rate_limits, which is
// live for this session, and the account-wide record in ~/.claude.json, which
// Claude refreshes every few minutes. pickWindow reconciles them.
package limits

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/term"
)

const (
	fiveHours = 5 * 60 * 60
	sevenDays = 7 * 24 * 60 * 60
	// ageLabelMin: below this a global reading is current enough not to bother
	// labelling how old it is.
	ageLabelMin = 60
)

type windowSource int

const (
	sourceSession windowSource = iota
	sourceGlobal
	sourceReset
)

type window struct {
	Percent    float64
	ResetsAt   int64 // 0 means unknown
	Source     windowSource
	ObservedAt int64
}

// globalConfig is decoded with a narrow struct rather than a generic map: the
// file is ~53KB and contains per-project keys that differ only in path case,
// which has already broken at least one other JSON parser. Only this subtree
// matters.
type globalConfig struct {
	CachedUsageUtilization *struct {
		FetchedAtMs float64 `json:"fetchedAtMs"`
		Utilization struct {
			FiveHour *globalWindowEntry `json:"five_hour"`
			SevenDay *globalWindowEntry `json:"seven_day"`
			Limits   []globalLimitRow   `json:"limits"`
		} `json:"utilization"`
	} `json:"cachedUsageUtilization"`
}

// globalLimitRow is one entry of the flat "limits" list. Unscoped rows repeat
// the five_hour / seven_day windows; a row with a model scope is a per-model
// weekly cap (the "Current week (Fable)" line in /usage) that exists nowhere
// else in the record.
type globalLimitRow struct {
	Kind     string   `json:"kind"`
	Group    string   `json:"group"`
	Percent  *float64 `json:"percent"`
	ResetsAt string   `json:"resets_at"`
	Scope    *struct {
		Model *struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

// modelName is the display name of the model this row is scoped to, or "" for
// an account-wide row.
func (r globalLimitRow) modelName() string {
	if r.Scope == nil || r.Scope.Model == nil {
		return ""
	}
	if r.Scope.Model.DisplayName != "" {
		return r.Scope.Model.DisplayName
	}
	return r.Scope.Model.ID
}

// scopedWindow is a per-model weekly limit, labelled by the model it caps.
type scopedWindow struct {
	Label  string
	Window *window
}

type globalWindowEntry struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

func isoToEpoch(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// readGlobal pulls the account-wide windows and any per-model weekly caps out
// of the account record. Any failure — missing file, or a torn read while
// Claude rewrites it — yields nothing and the segment simply falls back to the
// payload.
func readGlobal() (five, week *window, scoped []scopedWindow) {
	data, err := os.ReadFile(paths.GlobalConfig())
	if err != nil {
		return nil, nil, nil
	}
	return parseGlobal(data)
}

func parseGlobal(data []byte) (five, week *window, scoped []scopedWindow) {
	var cfg globalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, nil
	}
	if cfg.CachedUsageUtilization == nil {
		return nil, nil, nil
	}

	observedAt := int64(cfg.CachedUsageUtilization.FetchedAtMs / 1000)
	u := cfg.CachedUsageUtilization.Utilization

	fromRow := func(row globalLimitRow) *window {
		return &window{
			Percent:    *row.Percent,
			ResetsAt:   isoToEpoch(row.ResetsAt),
			Source:     sourceGlobal,
			ObservedAt: observedAt,
		}
	}

	// pick prefers the dedicated window entry and falls back to the matching
	// unscoped row. Scoped rows are skipped here: a per-model cap must never be
	// mistaken for the account-wide week.
	pick := func(entry *globalWindowEntry, kinds ...string) *window {
		if entry != nil && entry.Utilization != nil {
			return &window{
				Percent:    *entry.Utilization,
				ResetsAt:   isoToEpoch(entry.ResetsAt),
				Source:     sourceGlobal,
				ObservedAt: observedAt,
			}
		}
		for _, row := range u.Limits {
			if row.Percent == nil || row.modelName() != "" {
				continue
			}
			for _, k := range kinds {
				if row.Kind == k || row.Group == k {
					return fromRow(row)
				}
			}
		}
		return nil
	}

	for _, row := range u.Limits {
		name := term.Sanitize(row.modelName())
		if name == "" || row.Percent == nil || row.Group != "weekly" {
			continue
		}
		scoped = append(scoped, scopedWindow{Label: name, Window: fromRow(row)})
	}

	return pick(u.FiveHour, "session", "five_hour"),
		pick(u.SevenDay, "weekly", "week", "seven_day"),
		scoped
}

// pickWindow reconciles this session's headers against the account-wide record.
//
//	same window      -> the higher percent (usage only grows within a window)
//	different windows -> the later reset
//	all expired      -> a fresh window, because the quota has rolled over
//
// That last case is why an expired record still renders 0% rather than nothing.
func pickWindow(session, global *window, windowSeconds int64) *window {
	now := time.Now().Unix()
	live := func(w *window) *window {
		if w != nil && w.ResetsAt != 0 && w.ResetsAt > now {
			return w
		}
		return nil
	}

	s, g := live(session), live(global)

	switch {
	case s != nil && g != nil:
		// Resets landing within a few seconds of each other are the same window;
		// the two clocks differ by a hair.
		if abs64(s.ResetsAt-g.ResetsAt) <= 5 {
			if g.Percent > s.Percent {
				return g
			}
			return s // a tie keeps the session reading, so no age label appears
		}
		if s.ResetsAt > g.ResetsAt {
			return s
		}
		return g
	case s != nil:
		return s
	case g != nil:
		return g
	}

	if session != nil || global != nil {
		return &window{
			Percent:    0,
			ResetsAt:   now + windowSeconds,
			Source:     sourceReset,
			ObservedAt: now,
		}
	}
	return nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func renderWindow(label string, w *window, withClock bool) string {
	pct := int(math.Round(w.Percent))
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", label, term.Heat(pct, fmt.Sprintf("%d%%", pct)))

	if withClock && w.ResetsAt != 0 {
		b.WriteString(term.Dim(" resets " + term.Clock(w.ResetsAt)))
	}

	// Only a global reading can be meaningfully stale; session values are live
	// by construction.
	if w.Source == sourceGlobal {
		if age := time.Now().Unix() - w.ObservedAt; age >= ageLabelMin {
			b.WriteString(term.Dim(" ·" + strings.TrimSuffix(term.CompactAge(w.ObservedAt), " ago")))
		}
	}
	return b.String()
}

// Segment renders "⏳ 5h 42% resets 3:15pm · 7d 18% · Fable 30%", or "" when no
// window is known. Per-model weekly caps follow the account-wide week and
// appear only while the account record carries a live one.
func Segment(in *payload.Input) string {
	now := time.Now().Unix()
	fromPayload := func(w *payload.Window) *window {
		if w == nil || w.UsedPercentage == nil {
			return nil
		}
		lw := &window{
			Percent:    *w.UsedPercentage,
			Source:     sourceSession,
			ObservedAt: now,
		}
		if w.ResetsAt != nil {
			lw.ResetsAt = int64(*w.ResetsAt)
		}
		return lw
	}

	globalFive, globalWeek, scoped := readGlobal()
	five := pickWindow(fromPayload(in.RateLimits.FiveHour), globalFive, fiveHours)
	week := pickWindow(fromPayload(in.RateLimits.SevenDay), globalWeek, sevenDays)
	return render(five, week, scoped)
}

// render joins the reconciled windows.
//
// A scoped cap is never reconciled or rolled over. The payload carries no
// per-model window, so nothing live can confirm that an expired reading has
// rolled into a fresh week — and the account record is refreshed only when
// /usage is opened, so it can sit for weeks with every reset in the past. An
// expired scoped row therefore says nothing about the current week and is
// dropped rather than rendered as a fabricated 0%. The 5h/7d windows keep their
// rollover because the payload backs them from the first API response on.
func render(five, week *window, scoped []scopedWindow) string {
	var bits []string
	if five != nil {
		bits = append(bits, renderWindow("5h", five, true))
	}
	if week != nil {
		bits = append(bits, renderWindow("7d", week, false))
	}
	now := time.Now().Unix()
	for _, sw := range scoped {
		if w := sw.Window; w != nil && w.ResetsAt > now {
			bits = append(bits, renderWindow(sw.Label, w, false))
		}
	}
	if len(bits) == 0 {
		return ""
	}
	return "⏳ " + strings.Join(bits, term.Dim(" · "))
}
