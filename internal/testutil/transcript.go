package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TranscriptEntry is one assistant line in the shape Claude Code writes to a
// .jsonl transcript, with the usage fields the cost scan reads.
func TranscriptEntry(id, reqID, model string, ts time.Time, in, out, cr, cw5, cw1 int64) string {
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

// WriteTranscript writes entries as newline-terminated lines, creating parent
// directories as needed. Writing a transcript and scanning it is the closest
// thing to an end-to-end test of the money path.
func WriteTranscript(tb testing.TB, path string, entries []string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(entries, "\n")+"\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
}

// The benchmark fixture size: enough entries to make the parse measurable, few
// enough that the fixture builds instantly.
const (
	benchFiles           = 4
	benchEntriesPerFile  = 250
	benchDuplicateEvery  = 5 // transcripts really do repeat responses
	benchFixtureHourSpan = 6
)

// BenchTranscripts points CLAUDE_CONFIG_DIR at a fresh directory holding a
// synthetic transcript tree, and returns it.
//
// Everything that touches ~/.claude is pointed at this fixture instead:
// otherwise benchmark numbers describe one machine's accumulated transcript
// history and cannot be compared with anybody else's — and the benchmark would
// mutate real state while it runs.
func BenchTranscripts(b *testing.B) string {
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
			lines = append(lines, TranscriptEntry(id, req, model, ts, 120, 840, 48_000, 2_000, 400))
		}
		path := filepath.Join(dir, "projects", fmt.Sprintf("proj-%d", f), "session.jsonl")
		WriteTranscript(b, path, lines)
	}
	return dir
}
