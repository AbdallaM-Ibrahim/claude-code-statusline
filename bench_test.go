package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
)

// These measure in-process render cost only. The wall-clock a user sees also
// includes process creation, which is not something this program can influence —
// separating the two is the only way to know which half to attack. bench/e2e.ps1
// measures the other half.
//
// Everything that touches ~/.claude is pointed at a synthesized fixture instead:
// otherwise the numbers describe one machine's accumulated transcript history and
// cannot be compared with anybody else's — and the benchmark mutates real state
// while it runs.

// benchTranscripts is the fixture size: enough entries to make the parse
// measurable, few enough that the fixture builds instantly.
const (
	benchFiles           = 4
	benchEntriesPerFile  = 250
	benchDuplicateEvery  = 5 // transcripts really do repeat responses
	benchFixtureHourSpan = 6
)

// benchFixture points CLAUDE_CONFIG_DIR at a fresh directory holding a synthetic
// transcript tree, and returns it.
func benchFixture(b *testing.B) string {
	b.Helper()

	dir := b.TempDir()
	b.Setenv("CLAUDE_CONFIG_DIR", dir)

	now := time.Now()
	for f := 0; f < benchFiles; f++ {
		lines := make([]string, 0, benchEntriesPerFile)
		for i := 0; i < benchEntriesPerFile; i++ {
			// Spread entries over a few hours so the hour bucketing, the block
			// walk and the "today" boundary all do real work.
			ts := now.Add(-time.Duration(i%(benchFixtureHourSpan*60)) * time.Minute)
			model := "claude-opus-5"
			if i%3 == 0 {
				model = "claude-haiku-4-5-20251001"
			}
			id := fmt.Sprintf("msg_%d_%d", f, i/benchDuplicateEvery*benchDuplicateEvery)
			req := fmt.Sprintf("req_%d_%d", f, i/benchDuplicateEvery*benchDuplicateEvery)
			lines = append(lines, entryForBench(id, req, model, ts))
		}
		path := filepath.Join(dir, "projects", fmt.Sprintf("proj-%d", f), "session.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(joinLines(lines)), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	return dir
}

// entryForBench mirrors the test helper but takes a *testing.B-free path so both
// suites can share the fixture shape.
func entryForBench(id, reqID, model string, ts time.Time) string {
	m := map[string]any{
		"type":      "assistant",
		"timestamp": ts.UTC().Format(time.RFC3339Nano),
		"requestId": reqID,
		"message": map[string]any{
			"id":    id,
			"model": model,
			"usage": map[string]any{
				"input_tokens":                120,
				"output_tokens":               840,
				"cache_read_input_tokens":     48_000,
				"cache_creation_input_tokens": 2_400,
				"cache_creation": map[string]any{
					"ephemeral_5m_input_tokens": 2_000,
					"ephemeral_1h_input_tokens": 400,
				},
			},
		},
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

func benchPayload(b *testing.B, repo string) []byte {
	b.Helper()
	p, err := json.Marshal(map[string]any{
		"session_id": "741a0639-b2eb-42f0-8f7e-5827c5932a1f",
		"cwd":        repo,
		"model":      map[string]string{"display_name": "Opus 5"},
		"cost":       map[string]float64{"total_cost_usd": 0.42},
		"context_window": map[string]any{
			"context_window_size": 200000,
			"total_input_tokens":  68000,
			"used_percentage":     34,
		},
		"effort":   map[string]string{"level": "xhigh"},
		"thinking": map[string]bool{"enabled": true},
	})
	if err != nil {
		b.Fatal(err)
	}
	return p
}

// BenchmarkRenderFull is the whole render: both lines, both goroutines, warm
// caches. This is what one status line tick costs once the process exists.
func BenchmarkRenderFull(b *testing.B) {
	repo := repoForTest(b)
	benchFixture(b)
	p := benchPayload(b, repo)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = render(p)
	}
}

func BenchmarkDecodePayload(b *testing.B) {
	repo := repoForTest(b)
	p := benchPayload(b, repo)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := decodePayload(p); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGitReadOnly is line 1 with the ahead/behind cache warm: open the
// repository, resolve HEAD, read one commit.
func BenchmarkGitReadOnly(b *testing.B) {
	repo := repoForTest(b)
	if _, err := readGitForBench(repo); err != nil {
		b.Skipf("repo unavailable: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = readGitForBench(repo)
	}
}

// BenchmarkAheadBehindUncached is the reason gitcache.go exists: two full
// ancestry walks, reading every object along the way. Skips unless the branch has
// an upstream to compare against.
func BenchmarkAheadBehindUncached(b *testing.B) {
	repo := repoForTest(b)
	b.Setenv("CLAUDE_CONFIG_DIR", b.TempDir()) // never touch the real cache

	r, err := git.PlainOpenWithOptions(repo, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		b.Skipf("cannot open %s: %v", repo, err)
	}
	head, err := r.Head()
	if err != nil || !head.Name().IsBranch() {
		b.Skipf("no branch HEAD: %v", err)
	}
	branch := head.Name().Short()

	ctx := context.Background()
	if _, _, err := aheadBehind(ctx, r, branch, head.Hash()); err != nil {
		b.Skipf("no upstream to compare against: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		os.Remove(gitCachePath()) // force the walk, which is the point
		b.StartTimer()

		if _, _, err := aheadBehind(ctx, r, branch, head.Hash()); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCostScanSteadyState is the common case: nothing has changed since the
// last render, so every transcript is dismissed on a stat.
func BenchmarkCostScanSteadyState(b *testing.B) {
	benchFixture(b)

	in := &StatusLineInput{}
	v := 0.42
	in.Cost.TotalCostUSD = &v

	_ = buildCostReport(in, time.Now()) // prime the state file

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildCostReport(in, time.Now())
	}
}

// BenchmarkCostScanCold is the first render after an install, or after the state
// file is discarded: every transcript inside the horizon is parsed in full.
func BenchmarkCostScanCold(b *testing.B) {
	benchFixture(b)

	in := &StatusLineInput{}
	v := 0.42
	in.Cost.TotalCostUSD = &v

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		os.Remove(costStatePath())
		b.StartTimer()

		_ = buildCostReport(in, time.Now())
	}
}

// BenchmarkLimitsSegment reads and parses the account-wide record. Note that
// ~/.claude.json is the host's real file — its size varies per machine, so this
// row is comparable across runs on one box, not across boxes.
func BenchmarkLimitsSegment(b *testing.B) {
	in := &StatusLineInput{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = limitsSegment(in)
	}
}

// BenchmarkCavemanSegment is two small stat-and-read calls against the fixture.
func BenchmarkCavemanSegment(b *testing.B) {
	dir := b.TempDir()
	b.Setenv("CLAUDE_CONFIG_DIR", dir)
	if err := os.WriteFile(cavemanFlagPath(), []byte("full"), 0o600); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(cavemanSuffixPath(), []byte("· 42% saved"), 0o600); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cavemanSegment()
	}
}
