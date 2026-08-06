package main

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
		if got := compactAge(now - c.agoSeconds); got != c.want {
			t.Errorf("compactAge(%ds ago) = %q, want %q", c.agoSeconds, got, c.want)
		}
	}
}

func TestCompactAgeFutureClamps(t *testing.T) {
	if got := compactAge(time.Now().Unix() + 500); got != "0s ago" {
		t.Errorf("future timestamp = %q, want %q", got, "0s ago")
	}
}

func TestTruncateCountsRunes(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short string was altered: %q", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcd…" {
		t.Errorf("truncate = %q, want %q", got, "abcd…")
	}
	// A multi-byte subject must not be cut mid-character.
	if got := truncate("héllo wörld ünicode", 8); got != "héllo w…" {
		t.Errorf("multibyte truncate = %q, want %q", got, "héllo w…")
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
		if got := basename(in); got != want {
			t.Errorf("basename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeatThresholds(t *testing.T) {
	if heat(49, "x") != green("x") {
		t.Error("49 should be green")
	}
	if heat(50, "x") != yellow("x") {
		t.Error("50 should be yellow")
	}
	if heat(74, "x") != yellow("x") {
		t.Error("74 should be yellow")
	}
	if heat(75, "x") != red("x") {
		t.Error("75 should be red")
	}
}

func TestClockIsLowercase12Hour(t *testing.T) {
	// 15:15 local -> "3:15pm"
	ts := time.Date(2026, 8, 7, 15, 15, 0, 0, time.Local).Unix()
	if got := clock(ts); got != "3:15pm" {
		t.Errorf("clock = %q, want %q", got, "3:15pm")
	}
	ts = time.Date(2026, 8, 7, 5, 2, 0, 0, time.Local).Unix()
	if got := clock(ts); got != "5:02am" {
		t.Errorf("clock = %q, want %q", got, "5:02am")
	}
}

func TestContextPercentFallback(t *testing.T) {
	in := &StatusLineInput{}
	if _, ok := in.contextPercent(); ok {
		t.Error("empty context window should report nothing")
	}

	in.ContextWindow.ContextWindowSize = 200000
	in.ContextWindow.CurrentUsage = &struct {
		InputTokens              int `json:"input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	}{InputTokens: 1000, CacheCreationInputTokens: 2000, CacheReadInputTokens: 47000}
	got, ok := in.contextPercent()
	if !ok || got != 25 {
		t.Errorf("derived percent = %d (ok=%v), want 25", got, ok)
	}

	// An explicit percentage always wins over the derived one.
	p := 34.4
	in.ContextWindow.UsedPercentage = &p
	if got, _ := in.contextPercent(); got != 34 {
		t.Errorf("explicit percent = %d, want 34", got)
	}
}
