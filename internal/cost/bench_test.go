package cost

import (
	"os"
	"testing"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

func benchInput() *payload.Input {
	in := &payload.Input{}
	v := 0.42
	in.Cost.TotalCostUSD = &v
	return in
}

// BenchmarkCostScanSteadyState is the common case: nothing has changed since the
// last render, so every transcript is dismissed on a stat.
func BenchmarkCostScanSteadyState(b *testing.B) {
	testutil.BenchTranscripts(b)
	in := benchInput()

	_ = Build(in, time.Now()) // prime the state file

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Build(in, time.Now())
	}
}

// BenchmarkCostScanCold is the first render after an install, or after the state
// file is discarded: every transcript inside the horizon is parsed in full.
func BenchmarkCostScanCold(b *testing.B) {
	testutil.BenchTranscripts(b)
	in := benchInput()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		os.Remove(paths.CostState())
		b.StartTimer()

		_ = Build(in, time.Now())
	}
}
