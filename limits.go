package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
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

type limitWindow struct {
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
			Limits   []struct {
				Kind     string   `json:"kind"`
				Group    string   `json:"group"`
				Percent  *float64 `json:"percent"`
				ResetsAt string   `json:"resets_at"`
			} `json:"limits"`
		} `json:"utilization"`
	} `json:"cachedUsageUtilization"`
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

// readGlobalLimits pulls both windows out of the account-wide record. Any
// failure — missing file, or a torn read while Claude rewrites it — yields two
// nils and the segment simply falls back to the payload.
func readGlobalLimits() (five, week *limitWindow) {
	data, err := os.ReadFile(globalConfigPath())
	if err != nil {
		return nil, nil
	}
	var cfg globalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil
	}
	if cfg.CachedUsageUtilization == nil {
		return nil, nil
	}

	observedAt := int64(cfg.CachedUsageUtilization.FetchedAtMs / 1000)
	u := cfg.CachedUsageUtilization.Utilization

	pick := func(entry *globalWindowEntry, kinds ...string) *limitWindow {
		if entry != nil && entry.Utilization != nil {
			return &limitWindow{
				Percent:    *entry.Utilization,
				ResetsAt:   isoToEpoch(entry.ResetsAt),
				Source:     sourceGlobal,
				ObservedAt: observedAt,
			}
		}
		for _, row := range u.Limits {
			for _, k := range kinds {
				if (row.Kind == k || row.Group == k) && row.Percent != nil {
					return &limitWindow{
						Percent:    *row.Percent,
						ResetsAt:   isoToEpoch(row.ResetsAt),
						Source:     sourceGlobal,
						ObservedAt: observedAt,
					}
				}
			}
		}
		return nil
	}

	return pick(u.FiveHour, "session", "five_hour"),
		pick(u.SevenDay, "weekly", "week", "seven_day")
}

// pickWindow reconciles this session's headers against the account-wide record.
//
//	same window      -> the higher percent (usage only grows within a window)
//	different windows -> the later reset
//	all expired      -> a fresh window, because the quota has rolled over
//
// That last case is why an expired record still renders 0% rather than nothing.
func pickWindow(session, global *limitWindow, windowSeconds int64) *limitWindow {
	now := time.Now().Unix()
	live := func(w *limitWindow) *limitWindow {
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
		return &limitWindow{
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

func renderWindow(label string, w *limitWindow, withClock bool) string {
	pct := int(math.Round(w.Percent))
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", label, heat(pct, fmt.Sprintf("%d%%", pct)))

	if withClock && w.ResetsAt != 0 {
		b.WriteString(dim(" resets " + clock(w.ResetsAt)))
	}

	// Only a global reading can be meaningfully stale; session values are live
	// by construction.
	if w.Source == sourceGlobal {
		if age := time.Now().Unix() - w.ObservedAt; age >= ageLabelMin {
			b.WriteString(dim(" ·" + strings.TrimSuffix(compactAge(w.ObservedAt), " ago")))
		}
	}
	return b.String()
}

// limitsSegment renders "⏳ 5h 42% resets 3:15pm · 7d 18%", or "" when neither
// window is known.
func limitsSegment(in *StatusLineInput) string {
	now := time.Now().Unix()
	fromPayload := func(w *PayloadWindow) *limitWindow {
		if w == nil || w.UsedPercentage == nil {
			return nil
		}
		lw := &limitWindow{
			Percent:    *w.UsedPercentage,
			Source:     sourceSession,
			ObservedAt: now,
		}
		if w.ResetsAt != nil {
			lw.ResetsAt = int64(*w.ResetsAt)
		}
		return lw
	}

	globalFive, globalWeek := readGlobalLimits()
	five := pickWindow(fromPayload(in.RateLimits.FiveHour), globalFive, fiveHours)
	week := pickWindow(fromPayload(in.RateLimits.SevenDay), globalWeek, sevenDays)

	var bits []string
	if five != nil {
		bits = append(bits, renderWindow("5h", five, true))
	}
	if week != nil {
		bits = append(bits, renderWindow("7d", week, false))
	}
	if len(bits) == 0 {
		return ""
	}
	return "⏳ " + strings.Join(bits, dim(" · "))
}
