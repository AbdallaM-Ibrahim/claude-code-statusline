package term

import (
	"testing"
	"time"
)

func TestCompactAge(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		agoSeconds int64
		want       string
	}{
		{0, "0s ago"},
		{59, "59s ago"},
		{60, "1m ago"},
		{59 * 60, "59m ago"},
		{60 * 60, "1h ago"},
		{23 * 3600, "23h ago"},
		{24 * 3600, "1d ago"},
		{13 * 24 * 3600, "13d ago"},
		{14 * 24 * 3600, "2w ago"},     // 14d: first day that reports in weeks
		{62 * 24 * 3600, "8w ago"},     // 62d/7 = 8w, still < 9 so weeks
		{9 * 7 * 24 * 3600, "2mo ago"}, // 63d = 9w, no longer < 9, so 63/30 = 2mo
		{400 * 24 * 3600, "1y ago"},
	}
	for _, c := range cases {
		if got := CompactAge(now - c.agoSeconds); got != c.want {
			t.Errorf("CompactAge(%ds ago) = %q, want %q", c.agoSeconds, got, c.want)
		}
	}
}

func TestCompactAgeFutureClamps(t *testing.T) {
	if got := CompactAge(time.Now().Unix() + 500); got != "0s ago" {
		t.Errorf("future timestamp = %q, want %q", got, "0s ago")
	}
}

func TestTruncateCountsRunes(t *testing.T) {
	if got := Truncate("hello", 10); got != "hello" {
		t.Errorf("short string was altered: %q", got)
	}
	if got := Truncate("abcdefghij", 5); got != "abcd…" {
		t.Errorf("Truncate = %q, want %q", got, "abcd…")
	}
	// A multi-byte subject must not be cut mid-character.
	if got := Truncate("héllo wörld ünicode", 8); got != "héllo w…" {
		t.Errorf("multibyte Truncate = %q, want %q", got, "héllo w…")
	}
}

func TestBasename(t *testing.T) {
	cases := map[string]string{
		"C:/Users/abdo/workspace/site": "site",
		`C:\Users\abdo\workspace\site`: "site",
		"/home/abdo/site/":             "site",
		"site":                         "site",
	}
	for in, want := range cases {
		if got := Basename(in); got != want {
			t.Errorf("Basename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeatThresholds(t *testing.T) {
	if Heat(49, "x") != Green("x") {
		t.Error("49 should be green")
	}
	if Heat(50, "x") != Yellow("x") {
		t.Error("50 should be yellow")
	}
	if Heat(74, "x") != Yellow("x") {
		t.Error("74 should be yellow")
	}
	if Heat(75, "x") != Red("x") {
		t.Error("75 should be red")
	}
}

func TestClockIsLowercase12Hour(t *testing.T) {
	// 15:15 local -> "3:15pm"
	ts := time.Date(2026, 8, 7, 15, 15, 0, 0, time.Local).Unix()
	if got := Clock(ts); got != "3:15pm" {
		t.Errorf("Clock = %q, want %q", got, "3:15pm")
	}
	ts = time.Date(2026, 8, 7, 5, 2, 0, 0, time.Local).Unix()
	if got := Clock(ts); got != "5:02am" {
		t.Errorf("Clock = %q, want %q", got, "5:02am")
	}
}
