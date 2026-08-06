package main

import (
	"strings"
	"testing"
	"time"
)

func TestPickWindowPrefersHigherPercentInSameWindow(t *testing.T) {
	now := time.Now().Unix()
	reset := now + 3600

	session := &limitWindow{Percent: 40, ResetsAt: reset, Source: sourceSession, ObservedAt: now}
	global := &limitWindow{Percent: 72, ResetsAt: reset + 2, Source: sourceGlobal, ObservedAt: now - 300}

	got := pickWindow(session, global, fiveHours)
	if got.Percent != 72 {
		t.Errorf("percent = %v, want 72 (usage only grows within a window)", got.Percent)
	}
}

func TestPickWindowTieKeepsSessionSoNoAgeLabel(t *testing.T) {
	now := time.Now().Unix()
	reset := now + 3600

	session := &limitWindow{Percent: 50, ResetsAt: reset, Source: sourceSession, ObservedAt: now}
	global := &limitWindow{Percent: 50, ResetsAt: reset, Source: sourceGlobal, ObservedAt: now - 900}

	got := pickWindow(session, global, fiveHours)
	if got.Source != sourceSession {
		t.Error("a tie should keep the session reading so no staleness label appears")
	}
}

func TestPickWindowPrefersLaterResetAcrossDifferentWindows(t *testing.T) {
	now := time.Now().Unix()
	session := &limitWindow{Percent: 10, ResetsAt: now + 7200, Source: sourceSession, ObservedAt: now}
	global := &limitWindow{Percent: 90, ResetsAt: now + 600, Source: sourceGlobal, ObservedAt: now}

	got := pickWindow(session, global, fiveHours)
	if got.ResetsAt != now+7200 {
		t.Error("the later reset should win when the windows differ")
	}
}

// The live "5h 0%" currently on screen comes from this path: the stored record
// has expired, so the quota is known to have rolled over.
func TestPickWindowSynthesisesFreshWindowWhenAllExpired(t *testing.T) {
	now := time.Now().Unix()
	expired := &limitWindow{Percent: 72, ResetsAt: now - 60, Source: sourceGlobal, ObservedAt: now - 3600}

	got := pickWindow(nil, expired, fiveHours)
	if got == nil {
		t.Fatal("expected a synthesised window, got nil")
	}
	if got.Percent != 0 || got.Source != sourceReset {
		t.Errorf("expected a fresh 0%% window, got percent=%v source=%v", got.Percent, got.Source)
	}
	if got.ResetsAt < now+fiveHours-5 {
		t.Error("synthesised reset should be a full window away")
	}
}

func TestPickWindowNothingKnownReturnsNil(t *testing.T) {
	if got := pickWindow(nil, nil, fiveHours); got != nil {
		t.Errorf("no readings should yield nil, got %+v", got)
	}
}

func TestRenderWindowLabelsStaleGlobalReadings(t *testing.T) {
	now := time.Now().Unix()

	fresh := &limitWindow{Percent: 42, ResetsAt: now + 3600, Source: sourceGlobal, ObservedAt: now - 10}
	if s := renderWindow("5h", fresh, true); strings.Contains(s, "·") {
		t.Errorf("a reading 10s old should carry no age label: %q", s)
	}

	stale := &limitWindow{Percent: 42, ResetsAt: now + 3600, Source: sourceGlobal, ObservedAt: now - 720}
	s := renderWindow("5h", stale, true)
	if !strings.Contains(s, "·12m") {
		t.Errorf("a 12m old reading should be labelled, got %q", s)
	}

	live := &limitWindow{Percent: 42, ResetsAt: now + 3600, Source: sourceSession, ObservedAt: now - 720}
	if s := renderWindow("5h", live, true); strings.Contains(s, "·") {
		t.Errorf("session readings are live by construction and need no label: %q", s)
	}
}

func TestRenderWindowClockOnlyWhenAsked(t *testing.T) {
	now := time.Now().Unix()
	w := &limitWindow{Percent: 18, ResetsAt: now + 3600, Source: sourceSession, ObservedAt: now}

	if !strings.Contains(renderWindow("5h", w, true), "resets") {
		t.Error("5h window should show its reset clock")
	}
	if strings.Contains(renderWindow("7d", w, false), "resets") {
		t.Error("7d window should not show a reset clock")
	}
}

func TestIsoToEpoch(t *testing.T) {
	if got := isoToEpoch("2026-07-23T08:09:59.860620+00:00"); got != 1784794199 {
		t.Errorf("isoToEpoch = %d, want 1784794199", got)
	}
	if got := isoToEpoch(""); got != 0 {
		t.Errorf("empty string should yield 0, got %d", got)
	}
	if got := isoToEpoch("not a date"); got != 0 {
		t.Errorf("garbage should yield 0, got %d", got)
	}
}

// The real file on this machine has seven_day: null and an expired five_hour;
// reading it must not panic or error out.
func TestReadGlobalLimitsToleratesRealFile(t *testing.T) {
	five, week := readGlobalLimits()
	t.Logf("five=%+v week=%+v", five, week)
	if week != nil && week.Percent < 0 {
		t.Error("negative percent is not plausible")
	}
}
