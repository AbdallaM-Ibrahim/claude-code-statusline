# Plan: opt-in automatic usage refresh (per-model "Fable N%" without /usage)

Status: **implemented 2026-09-09** on `feat/usage-refresh` (stacked on
`fix/stale-scoped-week`) as `internal/usage`, wired on the owner's machine.

**Decision.** The owner chose option 2 — a direct HTTPS fetch with the OAuth token
Claude Code already holds — over this document's recommendation to spawn the `claude`
CLI. The measured gap decided it: 50 ms CPU and 14 MB per refresh against 1.4 s and
340 MB (see "Harness SDK evaluation"). The token-handling concerns that made option 2
the runner-up here are answered in code and in SECURITY.md: the token is read into a
single typed field, checked for the OAuth prefix and expiry, sent to a compile-time
endpoint with redirects refused, and never written, logged or printed; the switch is
off by default, so the program's original posture holds unless the user opts in.
The sections below are kept as the record of what was considered; where they
describe the CLI-spawning design ("helper mode", detached spawn, `--refresh-usage`),
that is the road not taken.

## Problem

The per-model weekly window ("Current week (Fable)") exists only in
`~/.claude.json` → `cachedUsageUtilization`. Claude Code v2.1.266 writes that record
only from its own `/usage` fetch (5-minute write throttle; its own reader discards it
after 1 hour). The status line stdin payload carries `five_hour`, `seven_day`,
`spend_limit` only — no per-model window (verified against a live captured payload and
the docs). On this machine the record sat at 2026-08-28 for 12 days; the segment first
lied ("Fable 0%", fixed in `fix/stale-scoped-week`) and now simply disappears until
`/usage` is opened.

## What was verified (all on Claude Code 2.1.266, Windows 11, Git Bash present)

| Fact | Evidence |
|---|---|
| Claude Code's stream-json control protocol answers `{"type":"control_request","request":{"subtype":"get_usage"}}` with the full usage object, **no model call**, no tokens spent | `claude -p --input-format stream-json --output-format stream-json < req.jsonl` returned `rate_limits_available`, `subscription_type`, `rate_limits` incl. `limits[]` (weekly_scoped Fable 57–58%) and a projected `rate_limits.model_scoped[]` (`display_name`, `utilization`, `resets_at`). Siblings in the same handler: `get_context_usage`, `list_models`, `get_session_cost`, `mcp_message`. **Undocumented** — the Agent SDK docs list no usage method. |
| The same request refreshes `~/.claude.json` as a side effect | `fetchedAtMs` moved from 2026-08-28 to 2026-09-09T13:37:54Z after the first run; later runs within 5 min returned fresh data but did not rewrite the file (Claude's `yxo = 300000` ms write throttle). |
| The existing Go reader then works unchanged | `statusline.exe` rendered `⏳ 5h 90% ·1m · 7d 30% ·1m · Fable 57% ·1m` from the captured payload. |
| `--safe-mode` is the right probe shape | 2.1 s wall clock, **0 hook events**, 1 output line, auth intact (`--bare` would disable OAuth). Full flag set: `claude -p --safe-mode --input-format stream-json --output-format stream-json --verbose --no-session-persistence --strict-mcp-config --mcp-config '{"mcpServers":{}}' --tools ""`. Without `--safe-mode`: 2.7 s and 4 SessionStart hooks fire. |
| No transcript pollution | `--no-session-persistence`; zero new `.jsonl` under `~/.claude/projects` for the probe cwd after three runs (also zero without the flag). |
| The status line command runs under **Git Bash** here | Env probe: parent process `bash.exe`; `MSYSTEM=MINGW64`; env carries `CLAUDECODE=1`, `CLAUDE_CODE_ENTRYPOINT=cli`, `CLAUDE_PID`, `CLAUDE_PROJECT_DIR`, `COLUMNS`, `LINES`. So `"command": "VAR=1 C:/…/statusline.exe"` is valid on this machine. Docs: Git Bash when installed, otherwise PowerShell. |
| `settings.json` has no `env` block today, so its propagation to the status line is untested | Keys listed; documented behaviour is silent on inheritance. Not needed for the chosen wiring. |
| The repo is cross-platform by design | Prebuilt binaries for windows/amd64, darwin/arm64+amd64, linux/amd64+arm64; pure Go, no cgo; `internal/paths` uses `filepath.Join`/`os.UserHomeDir`. Any design must work on all three. |

## Options considered

1. **Spawn the `claude` CLI and ask it (`get_usage` control request).** Recommended.
   Claude Code does the OAuth (token refresh included, Keychain on macOS, file on
   Windows/Linux), the HTTPS call and the `~/.claude.json` write. Our binary never
   touches a credential and gains no network code. Cost: one `claude` process every
   ≥5 minutes (~2 s CPU, runs detached, never on the render path). Dependency: an
   undocumented control subtype — version-detect and degrade to "no segment".
2. **Fetch `GET https://api.anthropic.com/api/oauth/usage` ourselves** with
   `Authorization: Bearer <accessToken>` + `anthropic-beta: oauth-2025-04-20`.
   Rejected: reads `~/.claude/.credentials.json` (macOS: Keychain → needs `security`
   subprocess anyway), cannot refresh an expired token without racing Claude Code,
   adds TLS + token handling to a program whose SECURITY.md promises neither, and a
   10-second status line is a bad place to hold a bearer token.
3. **Claude Agent SDK (TypeScript or Python).** Evaluated hands-on, see "Harness SDK
   evaluation" below. Both SDKs are wrappers that spawn the same `claude` CLI and
   speak the same one-line control request; the TypeScript one has a public
   (experimental) `usage_EXPERIMENTAL_MAY_CHANGE_DO_NOT_RELY_ON_THIS_API_YET()`, the
   Python one has no public usage call and bundles a second 220 MB `claude.exe`.
   Adding a Node or Python runtime in front of the child buys no capability the Go
   binary cannot get by writing the request itself. Their `SDKRateLimitEvent` is
   header-derived, per session, only after a model call — not a usage source. No SDK
   facility simulates the status line payload; the captured `statusline-input.json`
   remains the test fixture.
4. **Wait for `model_scoped` in the status line payload.** Claude Code already
   projects it for `get_usage`; forwarding it to the payload would make all of this
   unnecessary. File the feature request regardless; do not block on it.

## Design (option 1)

### Switch

`STATUSLINE_USAGE_REFRESH` — unset/empty/`0`/`false` → feature off (today's
behaviour, zero new code paths executed). `1`/`true` → on with a 5-minute interval.
A Go duration (`10m`, `1h`) → on with that interval; floor 5m (Claude does not
persist more often anyway); unparsable → off, never a crash.

`STATUSLINE_CLAUDE_BIN` — optional absolute path to the `claude` executable;
default `exec.LookPath("claude")` (handles `.exe`/`.cmd` on Windows).

### Render path (unchanged deadline, no added latency)

In `limits.Segment`, after reading the record: if the switch is on **and** the
newest record is older than the interval **and** no refresh lock is held → spawn
`statusline --refresh-usage` **detached** and do not wait. Everything else renders
exactly as now. The decision costs one `os.Stat` on the lock file.

### Helper mode `statusline --refresh-usage`

1. Take `~/.claude/statusline-usage.lock` with `O_CREATE|O_EXCL`; a lock older than
   60 s is stale and replaced. One refresh per machine, however many sessions render.
2. Run the `--safe-mode` probe command above with stdin = the `get_usage` request,
   `cwd = paths.ClaudeDir()`, stdout captured, stderr discarded, 30 s timeout.
3. Take the first line whose `type == "control_response"` and
   `response.subtype == "success"`; require `response.response.rate_limits` to be an
   object with `limits` → otherwise write nothing (covers a future CLI that drops
   the subtype or changes the shape).
4. Write `~/.claude/statusline-usage.json` as `{fetchedAtMs, accountUuid?,
   utilization: <rate_limits object>}` — the same shape `parseGlobal` already reads —
   via the existing tmp + `os.Rename`, mode `0600`.
5. Release the lock. Exit code 0 on success, 1 otherwise; print nothing.

### Reading side

`readGlobal` loads both `~/.claude.json` and `statusline-usage.json`, parses each with
the existing `parseGlobal`, and keeps the record with the newer `fetchedAtMs`. Claude's
own write from the probe usually makes them agree; ours is the guarantee when Claude's
throttle skips the write. The age label (`·1m`) keeps telling the truth.

### Cross-platform pieces

- Detached spawn: `proc_windows.go` (`CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS |
  CREATE_NO_WINDOW`) and `proc_unix.go` (`Setsid: true`); stdio to `os.DevNull`.
  Must be verified to survive Claude Code cancelling the in-flight status line
  (300 ms debounce cancels the previous run).
- The `claude` binary is resolved the same way on all three OSes; `--safe-mode`
  keeps OAuth from Keychain (macOS) or the credentials file (Windows/Linux).

### Files

| Path | Change |
|---|---|
| `internal/usage/refresh.go` (+ `proc_*.go`) | switch parsing, staleness decision, lock, detached spawn, helper run + response → record |
| `internal/usage/refresh_test.go` | interval parsing, floor, stale-lock replacement, response fixture (today's real response, numbers kept), shape rejection, merge-newer |
| `internal/limits/limits.go` | `readGlobal` merges the two records; calls `usage.MaybeRefresh` |
| `internal/paths/paths.go` | `UsageCache()`, `UsageLock()` |
| `cmd/statusline/main.go` | `--refresh-usage` dispatch (still ignores unknown flags) |
| `README.md` | "Automatic usage refresh (opt-in)" section + per-OS `settings.json` wiring |
| `SECURITY.md` | opt-in spawns one subprocess pair (self + `claude`) at most once per interval; still no network code, never reads credentials; new 0600 cache; undocumented-protocol caveat |

### settings.json wiring (done by the agent at the end, after a backup)

```jsonc
// macOS / Linux, and Windows with Git Bash installed (this machine)
"command": "STATUSLINE_USAGE_REFRESH=1 C:/Users/abdal/.claude/statusline.exe"
```

Windows without Git Bash (PowerShell host) cannot take the prefix form; README
documents a two-line `.cmd` wrapper for that case.

### Acceptance

- With the switch off: byte-identical output to today, `go test -bench` unchanged.
- With the switch on and a record older than 5 m: within one render a single `claude`
  process appears (across 4 concurrent sessions), finishes in ≈2 s, both cache files
  update, and the next render shows `Fable N% ·<1m`.
- Kill the probe mid-flight → no cache written, lock expires, retry on the next
  stale render.
- `claude` missing, logged out, or a CLI that no longer answers `get_usage` → segment
  simply absent, render time unaffected.
- gofmt, vet, test, race (CI), govulncheck, build on all five targets.

### Risks

- Undocumented control subtype; pin the observed shape in a fixture and fail closed.
- A future Claude Code may treat a `--safe-mode -p` session as a "session" for other
  bookkeeping; `--no-session-persistence` covers transcripts today.
- ~1.4 s of CPU every 5 minutes per machine while any session is open.

## Harness SDK evaluation (2026-09-09)

Question asked: can the Claude Agent SDK (the harness SDK) do the fetch, possibly
packaged as an executable, and what does each route cost?

### What the SDKs expose

| | TypeScript `@anthropic-ai/claude-agent-sdk` 0.3.266 | Python `claude-agent-sdk` 0.2.152 |
|---|---|---|
| Usage API | `query.usage_EXPERIMENTAL_MAY_CHANGE_DO_NOT_RELY_ON_THIS_API_YET({skipBehaviors})` → `SDKControlGetUsageResponse`; doc comment names "a usage meter" as the intended caller | none public; `ClaudeSDKClient` has `get_context_usage`, `get_server_info`, … — reached `get_usage` only via the private `client._query._send_control_request()` |
| Wire message | `{"type":"control_request","request":{"subtype":"get_usage","skip_behaviors":true}}` — identical to what the Go helper would write | same |
| CLI it runs | the installed `claude` (`pathToClaudeCodeExecutable`, no bundled binary) | its own bundled `_bundled/claude.exe` (219.7 MB) unless `cli_path` is set |
| Runtime added | Node.js + 239 MB `node_modules` | Python + 256 MB venv; SDK import alone 1.0–1.1 s warm |
| Model call needed | no | no |
| Result | Fable 59% ✓ | Fable 59% ✓ |

### Measured cost per refresh (this machine, 3–5 runs each, whole process tree)

`claude` was launched with `--safe-mode --no-session-persistence --strict-mcp-config
--mcp-config '{"mcpServers":{}}' --tools ""` in every spawning route.

| Route | wall ms | tree CPU ms | tree peak MB | extra on disk |
|---|---|---|---|---|
| A · status line render today (baseline, runs every 10 s) | 160 | 31 | 10 | — |
| B · Go in-process HTTPS GET (no-auth stand-in for option 2) | 191 | 50 | 14 | — |
| C · Go spawns installed `claude -p … get_usage` | 1 591–2 134 | 1 365 | 340 | — |
| D · Node + TS SDK → installed `claude` | 1 217 | 1 130 | 510 | 239 MB + Node |
| E · Python + SDK → installed `claude` | 3 391 | 2 344 | 415 | 256 MB + Python |
| F · Python + SDK → bundled `claude` | 3 162 | 2 698 | 435 | 256 MB + Python |

Amortised at one refresh per 5 minutes: C ≈ 16 s CPU/hour, D ≈ 14 s, E/F ≈ 28–32 s,
B ≈ 0.6 s; the status line's own 360 renders/hour cost ≈ 11 s. Route C therefore
roughly doubles the status line's CPU budget; the Python routes triple it.

Observations that matter for the implementation:

- D beat C on wall time only because the SDK reads the `control_response` and exits
  at once, while `-p` waits for the CLI to wind down after stdin EOF (~0.5–1 s). The
  Go helper should do the same: read the first `control_response`, then terminate
  the child. Expect ≈1.2 s wall, ≈1.2 s CPU.
- Even in `--safe-mode` the CLI spawns helpers (`conhost.exe`, `where.exe`,
  `reg.exe`, sometimes `git.exe`); peak tree memory swings 300–510 MB.
- The Python SDK is the worst route on every axis: twice the CPU and wall of C,
  256 MB on disk, a second copy of Claude Code to keep updated, and a private API.
  Packaging it as an executable (PyInstaller) cannot remove the `claude` child; the
  one-file form adds an extraction step per run on top.
- The TypeScript SDK is the only SDK with a public usage call, but for a Go binary it
  adds Node and 239 MB to send a one-line JSON message the binary can send itself.

### Verdict

Use the harness, not the SDK: spawn the installed `claude` directly from Go (route C,
with the early-exit refinement). Keep the SDK's `SDKControlGetUsageResponse` type as
the documentation of the response shape, and its `skip_behaviors` flag (it skips a
scan of every transcript touched in seven days).
