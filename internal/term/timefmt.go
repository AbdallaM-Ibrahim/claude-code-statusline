package term

import (
	"fmt"
	"time"
)

// CompactAge renders an age the way the status line has room for. Git's own
// `%cr` spells out "15 hours ago"; this does not.
//
// Each unit is derived from the total, not from the previous unit, matching the
// original: weeks come from days/7, months from days/30, years from days/365.
func CompactAge(epochSeconds int64) string {
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

// Clock formats a reset time as "3:15pm". Pinned to a 12-hour layout so it looks
// the same regardless of the host locale — the original had to strip a narrow
// no-break space that some locales insert before am/pm.
func Clock(epochSeconds int64) string {
	return time.Unix(epochSeconds, 0).Format("3:04pm")
}
