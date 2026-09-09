package statusline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

func TestRenderMalformedStdinFallsBack(t *testing.T) {
	got := Render([]byte("not json"))
	if got != Fallback {
		t.Errorf("Render = %q, want the single-line fallback", got)
	}
	if strings.Contains(got, "\n") {
		t.Error("the fallback must be a single line")
	}
}

// Truncated JSON cannot yield a model name: the only route to the fallback is a
// parse failure, so any retry would fail the same way.
func TestRenderTruncatedJSONFallsBack(t *testing.T) {
	got := Render([]byte(`{"model":{"display_name":"Opus 5"},`))
	if got != Fallback {
		t.Errorf("Render = %q, want the bare fallback", got)
	}
}

// A valid JSON object always decodes, because unknown fields are ignored — so a
// well-formed payload never reaches the fallback no matter what it contains.
func TestRenderUnknownFieldsStillDecode(t *testing.T) {
	got := Render([]byte(`{"totally":"unexpected","cwd":"."}`))
	if got == Fallback {
		t.Error("a valid object with unknown fields should still render fully")
	}
}

func TestRenderEmptyStdinDoesNotPanic(t *testing.T) {
	if got := Render(nil); got != Fallback {
		t.Errorf("Render(nil) = %q", got)
	}
}

// Anything piping the payload through a Windows shell can prepend a BOM, which
// Go's JSON decoder rejects outright.
func TestRenderToleratesUTF8BOM(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"model":          map[string]string{"display_name": "Opus 5"},
		"cwd":            t.TempDir(),
		"context_window": map[string]any{"used_percentage": 34},
	})
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, raw...)

	got := Render(withBOM)
	if got == Fallback {
		t.Fatal("a BOM-prefixed payload fell through to the fallback")
	}
	if !strings.Contains(got, "🧠") {
		t.Errorf("expected a full render, got %q", got)
	}
}

func TestRenderAlwaysProducesTwoLines(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"model": map[string]string{"display_name": "Opus 5"},
		"cwd":   t.TempDir(), // not a repository
	})
	got := Render(raw)
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("expected exactly two lines, got %d newlines: %q", n, got)
	}
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
//
// The wall-clock a user sees also includes process creation, which is not
// something this program can influence — separating the two is the only way to
// know which half to attack. bench/e2e.ps1 measures the other half.
func BenchmarkRenderFull(b *testing.B) {
	repo := testutil.Repo(b)
	testutil.BenchTranscripts(b)
	p := benchPayload(b, repo)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Render(p)
	}
}
