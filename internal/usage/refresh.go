// Package usage keeps the account-wide usage record fresh without /usage.
//
// Claude Code writes ~/.claude.json's cachedUsageUtilization only from its own
// /usage fetch, so on a machine where that panel is not opened the record — and
// with it the per-model weekly window this status line renders as "Fable 30%" —
// goes stale for weeks. With STATUSLINE_USAGE_REFRESH set, this package asks the
// same endpoint Claude Code asks, with the OAuth access token Claude Code already
// holds, at most once per interval per machine, and caches the answer next to the
// other state files. Off by default; nothing here runs unless the switch is set.
package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
)

const (
	endpointDefault = "https://api.anthropic.com/api/oauth/usage"
	// betaHeader is what Claude Code sends alongside a bearer OAuth token.
	betaHeader = "oauth-2025-04-20"
	userAgent  = "claude-code-statusline"
	// fetchTimeout keeps one slow fetch inside the render's own two-second
	// deadline with room for the rest of the session line.
	fetchTimeout = 1200 * time.Millisecond
	// maxBodyBytes bounds the response read. The real body is under 4 KB.
	maxBodyBytes = 1 << 20
)

// endpoint and client are variables only so tests can point them at an httptest
// server. Nothing reads them from the environment: the switch can turn the
// fetch on, never redirect it.
var (
	endpoint = endpointDefault
	client   = &http.Client{Timeout: fetchTimeout, CheckRedirect: refuseRedirect}
)

// refuseRedirect stops the client following anywhere. A redirect is returned as
// its 3xx status and rejected below; the token goes to one host only.
func refuseRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// Refresh fetches a fresh usage record when the switch is on and the newest
// record anyone has (newestFetchedAtMs, 0 for none) is older than cfg.Interval.
//
// It returns the new record — same shape as ~/.claude.json — or nil when nothing
// was fetched, for any reason: off, fresh enough, no usable token, another
// session already fetching or recently failed, network, status or shape failure.
// Failure is silent by design: the segment falls back to what it has.
func Refresh(ctx context.Context, cfg Config, newestFetchedAtMs int64, now time.Time) []byte {
	if !cfg.Enabled {
		return nil
	}
	if newestFetchedAtMs > 0 && now.Sub(time.UnixMilli(newestFetchedAtMs)) < cfg.Interval {
		return nil
	}
	tok := token(now)
	if tok == "" {
		return nil
	}
	lock := paths.UsageLock()
	if !acquire(lock, now) {
		return nil
	}
	body, ok := fetch(ctx, tok)
	if !ok {
		return nil // the lock stays: see lockTTL
	}
	var rec record
	rec.CachedUsageUtilization.FetchedAtMs = now.UnixMilli()
	rec.CachedUsageUtilization.Utilization = body
	data, err := json.Marshal(rec)
	if err != nil {
		return nil
	}
	if !writeCache(paths.UsageCache(), data) {
		return nil
	}
	release(lock)
	return data
}

// fetch performs the request and returns the validated body.
func fetch(ctx context.Context, tok string) (json.RawMessage, bool) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("anthropic-beta", betaHeader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil || len(body) > maxBodyBytes {
		return nil, false
	}
	return validate(body)
}

// validate accepts only a JSON object carrying a non-null "limits" list — the
// part of the response the limits segment is built from. An error envelope, an
// HTML page or a future shape writes nothing rather than a record that would
// render as no windows.
func validate(body []byte) (json.RawMessage, bool) {
	var probe responseProbe
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, false
	}
	if len(probe.Limits) == 0 || string(probe.Limits) == "null" {
		return nil, false
	}
	return json.RawMessage(body), true
}
