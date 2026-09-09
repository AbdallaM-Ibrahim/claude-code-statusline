package payload

import (
	"encoding/json"
	"testing"
)

func TestDecodeToleratesUTF8BOM(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"model": map[string]string{"display_name": "Opus 5"},
	})
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, raw...)

	in, err := Decode(withBOM)
	if err != nil {
		t.Fatalf("Decode rejected a BOM-prefixed payload: %v", err)
	}
	if in.Model.DisplayName != "Opus 5" {
		t.Errorf("model = %q, want Opus 5", in.Model.DisplayName)
	}
}

// A valid JSON object always decodes, because unknown fields are ignored — so a
// well-formed payload never reaches the fallback no matter what it contains.
func TestDecodeIgnoresUnknownFields(t *testing.T) {
	if _, err := Decode([]byte(`{"totally":"unexpected","cwd":"."}`)); err != nil {
		t.Errorf("unknown fields should be ignored, got %v", err)
	}
}

func TestDecodeRejectsMalformedInput(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("not json"), []byte(`{"model":{"display_name":"Opus 5"},`)} {
		if _, err := Decode(raw); err == nil {
			t.Errorf("Decode(%q) should fail", raw)
		}
	}
}

func TestDirPrefersWorkspaceOverCWD(t *testing.T) {
	in := &Input{}
	if got := in.Dir("/fallback"); got != "/fallback" {
		t.Errorf("empty payload dir = %q, want the fallback", got)
	}
	in.CWD = "/cwd"
	if got := in.Dir("/fallback"); got != "/cwd" {
		t.Errorf("dir = %q, want cwd", got)
	}
	in.Workspace.CurrentDir = "/workspace"
	if got := in.Dir("/fallback"); got != "/workspace" {
		t.Errorf("dir = %q, want the workspace dir", got)
	}
}

func BenchmarkDecodePayload(b *testing.B) {
	raw, err := json.Marshal(map[string]any{
		"session_id": "741a0639-b2eb-42f0-8f7e-5827c5932a1f",
		"cwd":        "/work/site",
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func TestContextPercentFallback(t *testing.T) {
	in := &Input{}
	if _, ok := in.ContextPercent(); ok {
		t.Error("empty context window should report nothing")
	}

	in.ContextWindow.ContextWindowSize = 200000
	in.ContextWindow.CurrentUsage = &ContextUsage{
		InputTokens: 1000, CacheCreationInputTokens: 2000, CacheReadInputTokens: 47000,
	}
	got, ok := in.ContextPercent()
	if !ok || got != 25 {
		t.Errorf("derived percent = %d (ok=%v), want 25", got, ok)
	}

	// An explicit percentage always wins over the derived one.
	p := 34.4
	in.ContextWindow.UsedPercentage = &p
	if got, _ := in.ContextPercent(); got != 34 {
		t.Errorf("explicit percent = %d, want 34", got)
	}
}
