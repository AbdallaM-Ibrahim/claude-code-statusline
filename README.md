# claude-statusline

Claude Code status line as a single compiled binary. Replaces the bun/TypeScript
version at `~/.claude/statusline.js`.

Two lines:

```
software-engineer-website ⟨main ↓72⟩ be66d0f 2w ago · first commit
🤖 Opus 5 xhigh 💭 | 🧠 34% | 💰 $1.42 session | 🔥 $2.10/hr | ⏳ 5h 42% resets 3:15pm | [CAVEMAN]
```

## Why a binary

The status line runs every 10s and the previous version spawned four processes
per render — bun, `git status`, `git log`, `ccusage`. On a 4-core machine under
load, bare bun interpreter startup inflated 9.8× (86ms → 842ms) while git, doing
real I/O, inflated only 2.1×. Process creation was the bottleneck, so this
version spawns nothing: git is read natively, cost is computed in-process.

## Build

```sh
go build -o ~/.claude/statusline.exe .    # Windows
go build -o ~/.claude/statusline .        # macOS / Linux
```

There is no auto-rebuild. Edit, build, done.

Cross-compile every target into `dist/` with `./build.ps1`. Pure Go, no cgo.

## Wiring

`~/.claude/settings.json`:

```json
"statusLine": {
  "type": "command",
  "command": "C:/Users/abdo/.claude/statusline.exe",
  "padding": 0,
  "refreshInterval": 10
}
```

## Pricing

Transcripts record token counts and a model name, never a cost. `today`, `block`
and the burn rate are therefore computed from `pricing.json` (embedded at build
time via `go:embed`).

Drop a `pricing.json` next to the binary to override without rebuilding.

An **unknown model contributes nothing** and forces a `~` prefix on the affected
figures (`💰 ~$1.42 today`). Guessing a rate would render confidently wrong
money, which is worse than visibly incomplete money. When a new model ships,
add it to the table.

Session cost is not computed — Claude Code passes it in the payload.

## Known differences from the ccusage-based version

Verified with `bun parity.ts`. Line 1 is byte-for-byte identical across every
payload shape tested; these are the deliberate line 2 differences:

| Difference | Why |
|---|---|
| `today` / `block` / burn rate are **higher** | ccusage's offline pricing table has no `claude-opus-5` entry and prices it at $0. Its own daily row for 2026-08-07 carries 44.9M tokens at `totalCost: 0`. Our table prices it. |
| Burn-rate emoji thresholds | Ours is a plain documented rate threshold. ccusage's marker did not track the rate monotonically in observed output, so it keys off something not derivable from a transcript. |
| No `session` figure when the payload omits `cost` | ccusage falls back to deriving session cost from the transcript. We only ever pass the payload's value through. Claude Code always sends it, so this shows up in synthetic payloads only. |

The cost maths was validated at token level rather than against ccusage's
dollars: our deduplicated totals since local midnight matched ccusage's own
per-day token attribution exactly — `input 576`, `output 232533`,
`cacheRead 44,271,478`, `cacheCreation 402,901`.

Note when comparing by hand: ccusage caches its own status line output for one
second by default, so consecutive invocations can report identical money figures
regardless of the payload.

## Design contract

Every segment is optional. A missing payload field, an unreadable file, or a
failing subsystem drops **that segment only** — it never blanks the line, and
malformed stdin falls back to a single `🤖 Claude`.
