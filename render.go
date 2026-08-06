package main

import (
	"fmt"
	"strings"
	"time"
)

// Colour codes are reproduced exactly from the TypeScript version this replaces:
// line 1 must stay byte-for-byte identical so the two can be diffed during
// rollout.

const reset = "\x1b[0m"

func paint(code, s string) string { return "\x1b[" + code + "m" + s + reset }

func dim(s string) string     { return paint("2", s) }
func cyan(s string) string    { return paint("1;36", s) }
func magenta(s string) string { return paint("35", s) }
func yellow(s string) string  { return paint("33", s) }
func green(s string) string   { return paint("32", s) }
func red(s string) string     { return paint("31", s) }
func blue(s string) string    { return paint("34", s) }
func orange(s string) string  { return paint("38;5;172", s) }

// heat is the shared threshold for every percentage on the line: context usage
// and both rate-limit windows.
func heat(pct int, s string) string {
	switch {
	case pct >= 75:
		return red(s)
	case pct >= 50:
		return yellow(s)
	default:
		return green(s)
	}
}

// compactAge renders an age the way the status line has room for. Git's own
// `%cr` spells out "15 hours ago"; this does not.
//
// Each unit is derived from the total, not from the previous unit, matching the
// original: weeks come from days/7, months from days/30, years from days/365.
func compactAge(epochSeconds int64) string {
	s := time.Now().Unix() - epochSeconds
	if s < 0 {
		s = 0
	}
	if s < 60 {
		return fmt.Sprintf("%ds ago", s)
	}
	m := s / 60
	if m < 60 {
		return fmt.Sprintf("%dm ago", m)
	}
	h := m / 60
	if h < 24 {
		return fmt.Sprintf("%dh ago", h)
	}
	d := h / 24
	if d < 14 {
		return fmt.Sprintf("%dd ago", d)
	}
	if w := d / 7; w < 9 {
		return fmt.Sprintf("%dw ago", w)
	}
	if mo := d / 30; mo < 12 {
		return fmt.Sprintf("%dmo ago", mo)
	}
	return fmt.Sprintf("%dy ago", d/365)
}

// clock formats a reset time as "3:15pm". Pinned to a 12-hour layout so it looks
// the same regardless of the host locale — the original had to strip a narrow
// no-break space that some locales insert before am/pm.
func clock(epochSeconds int64) string {
	return time.Unix(epochSeconds, 0).Format("3:04pm")
}

// basename splits on both separators so a Windows path and a POSIX path behave
// the same.
func basename(p string) string {
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return p
	}
	return parts[len(parts)-1]
}

// truncate counts runes, not bytes, so a multi-byte commit subject cannot be cut
// mid-character.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
