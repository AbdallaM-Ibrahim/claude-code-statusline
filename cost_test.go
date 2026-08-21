package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPricingTableHasModelsInUse(t *testing.T) {
	p := loadPricing()
	for _, m := range []string{"claude-opus-5", "claude-opus-4-8", "claude-haiku-4-5-20251001", "<synthetic>"} {
		if _, ok := p[m]; !ok {
			t.Errorf("pricing table missing %q, which appears in real transcripts", m)
		}
	}
}

func TestCostOfKnownModel(t *testing.T) {
	// opus-5: input $5/M, output $25/M, cacheRead $0.50/M, 5m write $6.25/M, 1h write $10/M
	usd, ok := costOf("claude-opus-5", TokenCounts{
		Input: 1_000_000, Output: 1_000_000, CacheRead: 1_000_000,
		CacheWrite5m: 1_000_000, CacheWrite1h: 1_000_000,
	})
	if !ok {
		t.Fatal("known model reported as unknown")
	}
	want := 5.0 + 25.0 + 0.5 + 6.25 + 10.0
	if diff := usd - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("cost = %v, want %v", usd, want)
	}
}

// The two cache-write tiers are priced differently; collapsing them would
// under-count every long-lived cache.
func TestCacheWriteTiersArePricedDifferently(t *testing.T) {
	fiveMin, _ := costOf("claude-opus-5", TokenCounts{CacheWrite5m: 1_000_000})
	oneHour, _ := costOf("claude-opus-5", TokenCounts{CacheWrite1h: 1_000_000})
	if oneHour <= fiveMin {
		t.Errorf("1h write (%v) should cost more than 5m write (%v)", oneHour, fiveMin)
	}
}

func TestCostOfUnknownModelContributesNothing(t *testing.T) {
	usd, ok := costOf("some-model-that-does-not-exist", TokenCounts{Input: 1_000_000})
	if ok {
		t.Error("unknown model should report ok=false")
	}
	if usd != 0 {
		t.Errorf("unknown model should contribute 0, got %v", usd)
	}
}

// Writing a transcript and scanning it is the closest thing to an end-to-end
// test of the money path.
func writeTranscript(t *testing.T, path string, entries []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(entries, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func entry(id, reqID, model string, ts time.Time, in, out, cr, cw5, cw1 int64) string {
	m := map[string]any{
		"type":      "assistant",
		"timestamp": ts.UTC().Format(time.RFC3339Nano),
		"requestId": reqID,
		"message": map[string]any{
			"id":    id,
			"model": model,
			"usage": map[string]any{
				"input_tokens":                in,
				"output_tokens":               out,
				"cache_read_input_tokens":     cr,
				"cache_creation_input_tokens": cw5 + cw1,
				"cache_creation": map[string]any{
					"ephemeral_5m_input_tokens": cw5,
					"ephemeral_1h_input_tokens": cw1,
				},
			},
		},
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestScanDeduplicatesRepeatedResponses(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	e := entry("msg_1", "req_1", "claude-opus-5", now.Add(-10*time.Minute), 1_000_000, 0, 0, 0, 0)

	// The same response three times, as really happens in these transcripts.
	writeTranscript(t, filepath.Join(root, "p", "a.jsonl"), []string{e, e, e})

	st := newCostState()
	st.scan(root, now)

	rep := &CostReport{}
	aggregate(st, rep, now)

	if rep.TodayUSD != 5.0 {
		t.Errorf("today = %v, want 5.00 — duplicates were counted %vx", rep.TodayUSD, rep.TodayUSD/5.0)
	}
}

func TestScanIsIncrementalAcrossAppends(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	path := filepath.Join(root, "p", "a.jsonl")

	writeTranscript(t, path, []string{
		entry("m1", "r1", "claude-opus-5", now.Add(-30*time.Minute), 1_000_000, 0, 0, 0, 0),
	})

	st := newCostState()
	st.scan(root, now)
	rep := &CostReport{}
	aggregate(st, rep, now)
	if rep.TodayUSD != 5.0 {
		t.Fatalf("first pass today = %v, want 5.00", rep.TodayUSD)
	}

	// Append a second response; the cursor must pick up only the new bytes and
	// must not re-count the first.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, entry("m2", "r2", "claude-opus-5", now.Add(-5*time.Minute), 1_000_000, 0, 0, 0, 0))
	f.Close()

	st.scan(root, now)
	rep = &CostReport{}
	aggregate(st, rep, now)
	if rep.TodayUSD != 10.0 {
		t.Errorf("after append today = %v, want 10.00", rep.TodayUSD)
	}
}

// A render can land between a record and its newline. The half-written line must
// be picked up once it completes, not skipped.
func TestScanRecoversFromPartialTrailingLine(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	path := filepath.Join(root, "p", "a.jsonl")
	complete := entry("m1", "r1", "claude-opus-5", now.Add(-20*time.Minute), 1_000_000, 0, 0, 0, 0)
	second := entry("m2", "r2", "claude-opus-5", now.Add(-10*time.Minute), 1_000_000, 0, 0, 0, 0)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// First line complete, second line truncated mid-write (no newline).
	partial := second[:len(second)/2]
	if err := os.WriteFile(path, []byte(complete+"\n"+partial), 0o644); err != nil {
		t.Fatal(err)
	}

	st := newCostState()
	st.scan(root, now)
	rep := &CostReport{}
	aggregate(st, rep, now)
	if rep.TodayUSD != 5.0 {
		t.Fatalf("with a partial tail today = %v, want 5.00", rep.TodayUSD)
	}

	// The writer finishes the record.
	if err := os.WriteFile(path, []byte(complete+"\n"+second+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st.scan(root, now)
	rep = &CostReport{}
	aggregate(st, rep, now)
	if rep.TodayUSD != 10.0 {
		t.Errorf("after completion today = %v, want 10.00 — the partial line was lost", rep.TodayUSD)
	}
}

func TestUnknownModelSetsApproximateFlag(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	writeTranscript(t, filepath.Join(root, "p", "a.jsonl"), []string{
		entry("m1", "r1", "claude-opus-5", now.Add(-10*time.Minute), 1_000_000, 0, 0, 0, 0),
		entry("m2", "r2", "model-from-the-future", now.Add(-9*time.Minute), 1_000_000, 0, 0, 0, 0),
	})

	st := newCostState()
	st.scan(root, now)
	rep := &CostReport{}
	aggregate(st, rep, now)

	if !rep.Approximate {
		t.Error("an unpriced model must flag the totals as approximate")
	}
	if rep.TodayUSD != 5.0 {
		t.Errorf("today = %v, want 5.00 (unknown model contributes nothing)", rep.TodayUSD)
	}
	segs := costSegments(rep, true)
	if len(segs) == 0 || !strings.Contains(segs[0], "~") {
		t.Errorf("approximate totals must render a ~ prefix, got %q", segs)
	}
}

func TestEntriesOlderThanHorizonAreIgnored(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	writeTranscript(t, filepath.Join(root, "p", "a.jsonl"), []string{
		entry("old", "r0", "claude-opus-5", now.Add(-72*time.Hour), 1_000_000, 0, 0, 0, 0),
		entry("new", "r1", "claude-opus-5", now.Add(-1*time.Hour), 1_000_000, 0, 0, 0, 0),
	})

	st := newCostState()
	st.scan(root, now)
	rep := &CostReport{}
	aggregate(st, rep, now)

	if rep.TodayUSD != 5.0 {
		t.Errorf("today = %v, want 5.00 — the 72h-old entry should be outside the horizon", rep.TodayUSD)
	}
}

func TestBlockSplitsAfterFiveHourGap(t *testing.T) {
	now := time.Now()
	st := newCostState()

	// Two hours of activity, then a gap well past 5h, then activity now.
	add := func(ts time.Time, usd6 int64) {
		h := ts.Unix() / 3600 * 3600
		st.Hours[strconv.FormatInt(h, 10)] = hourBucket{
			"claude-opus-5": TokenCounts{Input: usd6},
		}
	}
	add(now.Add(-40*time.Hour), 1_000_000)
	add(now.Add(-30*time.Minute), 1_000_000)

	rep := &CostReport{}
	aggregate(st, rep, now)

	if rep.BlockUSD != 5.0 {
		t.Errorf("active block = %v, want 5.00 (the 40h-old hour belongs to an earlier block)", rep.BlockUSD)
	}
	if rep.BurnPerHour <= 0 {
		t.Error("an active block should produce a burn rate")
	}
}

func TestCostSegmentsDropBlockWhenLimitsPresent(t *testing.T) {
	rep := &CostReport{
		SessionUSD: 0.42, HasSession: true,
		TodayUSD: 1.0, BlockUSD: 3.46,
		BlockStart:  time.Now().Unix() - 600,
		BlockEnd:    time.Now().Unix() + 3600,
		BurnPerHour: 2.0,
	}

	with := costSegments(rep, false)
	if !strings.Contains(with[0], "block") {
		t.Errorf("block estimate should appear when it is not dropped: %q", with[0])
	}

	without := costSegments(rep, true)
	if strings.Contains(without[0], "block") {
		t.Errorf("block estimate should be dropped alongside an authoritative limit: %q", without[0])
	}
	if !strings.Contains(without[0], "$0.42 session") || !strings.Contains(without[0], "$1.00 today") {
		t.Errorf("session and today must survive: %q", without[0])
	}
}

func TestSessionCostIsPassedThroughNotComputed(t *testing.T) {
	v := 3.39
	in := &StatusLineInput{}
	in.Cost.TotalCostUSD = &v

	rep := buildCostReport(in, time.Now())
	if !rep.HasSession || rep.SessionUSD != 3.39 {
		t.Errorf("session cost = %v (has=%v), want exactly the payload value 3.39",
			rep.SessionUSD, rep.HasSession)
	}
}
