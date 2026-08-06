package main

import (
	"encoding/json"
	"testing"
)

// These measure in-process render cost only. The wall-clock a user sees also
// includes Windows process creation, which is not something this program can
// influence — separating the two is the only way to know which half to attack.

func benchPayload(b *testing.B) []byte {
	b.Helper()
	p, err := json.Marshal(map[string]any{
		"session_id": "741a0639-b2eb-42f0-8f7e-5827c5932a1f",
		"cwd":        testRepo,
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

func BenchmarkRenderFull(b *testing.B) {
	p := benchPayload(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = render(p)
	}
}

func BenchmarkGitReadOnly(b *testing.B) {
	if _, err := readGitForBench(); err != nil {
		b.Skipf("repo unavailable: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = readGitForBench()
	}
}

func BenchmarkCostScanSteadyState(b *testing.B) {
	in := &StatusLineInput{}
	v := 0.42
	in.Cost.TotalCostUSD = &v
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildCostReport(in, nowForBench())
	}
}

func BenchmarkLimitsSegment(b *testing.B) {
	in := &StatusLineInput{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = limitsSegment(in)
	}
}

func BenchmarkCavemanSegment(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = cavemanSegment()
	}
}
