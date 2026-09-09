package cost

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

//go:embed pricing.json
var embeddedPricing []byte

// ModelRate is USD per MILLION tokens.
//
// Cache writes are split because Anthropic charges them differently: a 5-minute
// ephemeral write is 1.25x base input, a 1-hour write is 2x. Transcripts report
// the split in usage.cache_creation, so collapsing them into one rate would
// mis-price every long-lived cache — which is most of this session's traffic.
type ModelRate struct {
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	CacheRead    float64 `json:"cacheRead"`
	CacheWrite5m float64 `json:"cacheWrite5m"`
	CacheWrite1h float64 `json:"cacheWrite1h"`
}

type pricingDoc struct {
	Models map[string]ModelRate `json:"models"`
}

var (
	pricingOnce  sync.Once
	pricingTable map[string]ModelRate
)

// loadPricing reads the embedded table, then lets a pricing.json sitting next to
// the binary override it entirely. That override is the escape hatch for a newly
// released model: edit a file, no rebuild, no toolchain.
func loadPricing() map[string]ModelRate {
	pricingOnce.Do(func() {
		pricingTable = map[string]ModelRate{}

		var doc pricingDoc
		if err := json.Unmarshal(embeddedPricing, &doc); err == nil {
			for k, v := range doc.Models {
				pricingTable[k] = v
			}
		}

		if exe, err := os.Executable(); err == nil {
			override := filepath.Join(filepath.Dir(exe), "pricing.json")
			if data, err := os.ReadFile(override); err == nil {
				var od pricingDoc
				if err := json.Unmarshal(data, &od); err == nil && len(od.Models) > 0 {
					for k, v := range od.Models {
						pricingTable[k] = v
					}
				}
			}
		}
	})
	return pricingTable
}

// TokenCounts is one API response's billable usage.
type TokenCounts struct {
	Input        int64 `json:"in"`
	Output       int64 `json:"out"`
	CacheRead    int64 `json:"cr"`
	CacheWrite5m int64 `json:"cw5"`
	CacheWrite1h int64 `json:"cw1"`
}

func (t *TokenCounts) add(o TokenCounts) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite5m += o.CacheWrite5m
	t.CacheWrite1h += o.CacheWrite1h
}

// costOf prices one model's tokens. ok is false when the model is not in the
// table, in which case the caller must flag the total as approximate rather than
// quietly reporting a smaller number. Guessing a rate would render confidently
// wrong money, which reads as correct — worse than visibly incomplete money.
func costOf(model string, t TokenCounts) (usd float64, ok bool) {
	rate, found := loadPricing()[model]
	if !found {
		return 0, false
	}
	const perMillion = 1_000_000.0
	usd = float64(t.Input)/perMillion*rate.Input +
		float64(t.Output)/perMillion*rate.Output +
		float64(t.CacheRead)/perMillion*rate.CacheRead +
		float64(t.CacheWrite5m)/perMillion*rate.CacheWrite5m +
		float64(t.CacheWrite1h)/perMillion*rate.CacheWrite1h
	return usd, true
}
