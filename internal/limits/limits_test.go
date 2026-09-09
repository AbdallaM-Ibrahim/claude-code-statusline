package limits

import (
	"strings"
	"testing"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

func TestPickWindowPrefersHigherPercentInSameWindow(t *testing.T) {
	now := time.Now().Unix()
	reset := now + 3600

	session := &window{Percent: 40, ResetsAt: reset, Source: sourceSession, ObservedAt: now}
	global := &window{Percent: 72, ResetsAt: reset + 2, Source: sourceGlobal, ObservedAt: now - 300}

	got := pickWindow(session, global, fiveHours)
	if got.Percent != 72 {
		t.Errorf("percent = %v, want 72 (usage only grows within a window)", got.Percent)
	}
}

func TestPickWindowTieKeepsSessionSoNoAgeLabel(t *testing.T) {
	now := time.Now().Unix()
	reset := now + 3600

	session := &window{Percent: 50, ResetsAt: reset, Source: sourceSession, ObservedAt: now}
	global := &window{Percent: 50, ResetsAt: reset, Source: sourceGlobal, ObservedAt: now - 900}

	got := pickWindow(session, global, fiveHours)
	if got.Source != sourceSession {
		t.Error("a tie should keep the session reading so no staleness label appears")
	}
}

func TestPickWindowPrefersLaterResetAcrossDifferentWindows(t *testing.T) {
	now := time.Now().Unix()
	session := &window{Percent: 10, ResetsAt: now + 7200, Source: sourceSession, ObservedAt: now}
	global := &window{Percent: 90, ResetsAt: now + 600, Source: sourceGlobal, ObservedAt: now}

	got := pickWindow(session, global, fiveHours)
	if got.ResetsAt != now+7200 {
		t.Error("the later reset should win when the windows differ")
	}
}

// The live "5h 0%" currently on screen comes from this path: the stored record
// has expired, so the quota is known to have rolled over.
func TestPickWindowSynthesisesFreshWindowWhenAllExpired(t *testing.T) {
	now := time.Now().Unix()
	expired := &window{Percent: 72, ResetsAt: now - 60, Source: sourceGlobal, ObservedAt: now - 3600}

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

	fresh := &window{Percent: 42, ResetsAt: now + 3600, Source: sourceGlobal, ObservedAt: now - 10}
	if s := renderWindow("5h", fresh, true); strings.Contains(s, "·") {
		t.Errorf("a reading 10s old should carry no age label: %q", s)
	}

	stale := &window{Percent: 42, ResetsAt: now + 3600, Source: sourceGlobal, ObservedAt: now - 720}
	s := renderWindow("5h", stale, true)
	if !strings.Contains(s, "·12m") {
		t.Errorf("a 12m old reading should be labelled, got %q", s)
	}

	live := &window{Percent: 42, ResetsAt: now + 3600, Source: sourceSession, ObservedAt: now - 720}
	if s := renderWindow("5h", live, true); strings.Contains(s, "·") {
		t.Errorf("session readings are live by construction and need no label: %q", s)
	}
}

func TestRenderWindowClockOnlyWhenAsked(t *testing.T) {
	now := time.Now().Unix()
	w := &window{Percent: 18, ResetsAt: now + 3600, Source: sourceSession, ObservedAt: now}

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
func TestReadGlobalToleratesRealFile(t *testing.T) {
	five, week, scoped := readGlobal()
	t.Logf("five=%+v week=%+v scoped=%+v", five, week, scoped)
	if week != nil && week.Percent < 0 {
		t.Error("negative percent is not plausible")
	}
}

// A trimmed copy of the real record: the scoped row is listed before the
// unscoped one on purpose, so the account-wide week must not pick it up.
const scopedLimitsFixture = `{
  "cachedUsageUtilization": {
    "fetchedAtMs": 1788923483992,
    "utilization": {
      "five_hour": {"utilization": 9, "resets_at": "2026-09-09T05:49:59+00:00"},
      "seven_day": null,
      "limits": [
        {"kind": "session", "group": "session", "percent": 9, "resets_at": "2026-09-09T05:49:59+00:00", "scope": null},
        {"kind": "weekly_scoped", "group": "weekly", "percent": 30, "resets_at": "2026-09-10T05:59:59+00:00",
         "scope": {"model": {"id": null, "display_name": "Fable"}, "surface": null}},
        {"kind": "weekly_all", "group": "weekly", "percent": 16, "resets_at": "2026-09-10T05:59:59+00:00", "scope": null},
        {"kind": "weekly_scoped", "group": "weekly", "percent": null, "resets_at": null,
         "scope": {"model": {"id": "claude-opus-5", "display_name": "Opus"}}}
      ]
    }
  }
}`

func TestParseGlobalScopedWeek(t *testing.T) {
	five, week, scoped := parseGlobal([]byte(scopedLimitsFixture))
	if five == nil || five.Percent != 9 {
		t.Fatalf("five = %+v, want 9%%", five)
	}
	if week == nil || week.Percent != 16 {
		t.Fatalf("week = %+v, want the unscoped 16%% row, not the Fable one", week)
	}
	if len(scoped) != 1 {
		t.Fatalf("scoped = %+v, want exactly the Fable row (Opus has no percent)", scoped)
	}
	if scoped[0].Label != "Fable" || scoped[0].Window.Percent != 30 {
		t.Errorf("scoped[0] = %q %+v, want Fable 30%%", scoped[0].Label, scoped[0].Window)
	}
	if scoped[0].Window.ResetsAt != isoToEpoch("2026-09-10T05:59:59+00:00") {
		t.Errorf("scoped reset not carried through: %d", scoped[0].Window.ResetsAt)
	}
}

func TestParseGlobalScopedFallsBackToModelID(t *testing.T) {
	_, _, scoped := parseGlobal([]byte(`{"cachedUsageUtilization":{"fetchedAtMs":0,"utilization":{"limits":[
		{"kind":"weekly_scoped","group":"weekly","percent":5,"scope":{"model":{"id":"claude-fable-5-1","display_name":""}}}]}}}`))
	if len(scoped) != 1 || scoped[0].Label != "claude-fable-5-1" {
		t.Errorf("scoped = %+v, want the model id as the label", scoped)
	}
}

func TestRenderAppendsScopedWeek(t *testing.T) {
	now := time.Now().Unix()
	week := &window{Percent: 16, ResetsAt: now + 3600, Source: sourceSession, ObservedAt: now}
	fable := scopedWindow{Label: "Fable", Window: &window{Percent: 30, ResetsAt: now + 3600, Source: sourceGlobal, ObservedAt: now}}

	got := testutil.StripANSI(render(nil, week, []scopedWindow{fable}))
	if got != "⏳ 7d 16% · Fable 30%" {
		t.Errorf("render = %q", got)
	}
}

func TestRenderScopedAloneStillRenders(t *testing.T) {
	now := time.Now().Unix()
	fable := scopedWindow{Label: "Fable", Window: &window{Percent: 30, ResetsAt: now + 3600, Source: sourceGlobal, ObservedAt: now}}
	if got := testutil.StripANSI(render(nil, nil, []scopedWindow{fable})); got != "⏳ Fable 30%" {
		t.Errorf("render = %q", got)
	}
	if render(nil, nil, nil) != "" {
		t.Error("no windows should render nothing")
	}
}

// An expired scoped row is dropped, not rolled over to 0%: the payload has no
// per-model window to confirm the new week, and the account record is only
// refreshed by /usage, so on a machine where that has not been opened for a
// while every reset in it is in the past. Observed 2026-09-09 with a record
// from 2026-08-28: the 5h/7d windows came from the payload while "Fable 0%"
// was invented from a row whose reset passed six days earlier.
func TestRenderExpiredScopedIsDropped(t *testing.T) {
	now := time.Now().Unix()
	expired := scopedWindow{Label: "Fable", Window: &window{Percent: 30, ResetsAt: now - 60, Source: sourceGlobal, ObservedAt: now - 12*24*3600}}
	if got := testutil.StripANSI(render(nil, nil, []scopedWindow{expired})); got != "" {
		t.Errorf("render = %q, want nothing rather than a fabricated 0%%", got)
	}

	week := &window{Percent: 16, ResetsAt: now + 3600, Source: sourceSession, ObservedAt: now}
	if got := testutil.StripANSI(render(nil, week, []scopedWindow{expired})); got != "⏳ 7d 16%" {
		t.Errorf("render = %q, want the live week alone", got)
	}

	unknownReset := scopedWindow{Label: "Fable", Window: &window{Percent: 30, Source: sourceGlobal, ObservedAt: now}}
	if got := testutil.StripANSI(render(nil, nil, []scopedWindow{unknownReset})); got != "" {
		t.Errorf("render = %q, want nothing when the reset is unknown", got)
	}
}

// BenchmarkLimitsSegment reads and parses the account-wide record. Note that
// ~/.claude.json is the host's real file — its size varies per machine, so this
// row is comparable across runs on one box, not across boxes.
func BenchmarkLimitsSegment(b *testing.B) {
	in := &payload.Input{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Segment(in)
	}
}
