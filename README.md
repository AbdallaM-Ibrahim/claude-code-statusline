# claude-code-statusline

[![ci](https://github.com/AbdallaM-Ibrahim/claude-code-statusline/actions/workflows/ci.yml/badge.svg)](https://github.com/AbdallaM-Ibrahim/claude-code-statusline/actions/workflows/ci.yml)

A [Claude Code](https://claude.com/claude-code) status line as one compiled Go
binary. No interpreter, no subprocesses, no network — it reads its payload on
stdin and prints two lines:

```
software-engineer-website ⟨main ↓72⟩ be66d0f 2w ago · first commit
🤖 Opus 5 xhigh 💭 | 🧠 34% | 💰 $1.42 session / $8.90 today | 🔥 $2.10/hr 🟢 | ⏳ 5h 42% resets 3:15pm · 7d 18% · Fable 30%
```

Line 1 is **where you are**: directory, branch, ahead/behind, HEAD, commit age
and subject. Line 2 is **what the session costs**: model, effort, context
pressure, money, burn rate and rate-limit windows.

Every segment is optional. A missing payload field, an unreadable file or a
failing subsystem drops **that segment only** — it never blanks the line, and
unparseable stdin degrades to a bare `🤖 Claude`.

---

## Why a binary

The status line runs on a timer — every 10 seconds by default — and the version
this replaces spawned four processes per render: `bun`, `git status`, `git log`,
`ccusage`.

Process creation, not the work, was the cost. Measured on a 4-core laptop under
sustained load, bare `bun -e ''` startup inflated **9.8×** (86 ms → 842 ms) while
`git log`, doing real I/O, inflated only **2.1×**. So this version spawns nothing:
git objects are read in-process with [go-git](https://github.com/go-git/go-git),
and cost is computed from the transcripts directly instead of shelling out to
`ccusage`.

See [Benchmarks](#benchmarks) for what that bought.

---

## Install

### Prebuilt binary

One file, no runtime. `~/.claude/` is the conventional home, but anywhere works.

```sh
# macOS, Apple silicon
curl -Lo ~/.claude/statusline https://github.com/AbdallaM-Ibrahim/claude-code-statusline/releases/latest/download/statusline-darwin-arm64
chmod +x ~/.claude/statusline

# macOS, Intel
curl -Lo ~/.claude/statusline https://github.com/AbdallaM-Ibrahim/claude-code-statusline/releases/latest/download/statusline-darwin-amd64
chmod +x ~/.claude/statusline

# Linux x64  (arm64: swap amd64 for arm64)
curl -Lo ~/.claude/statusline https://github.com/AbdallaM-Ibrahim/claude-code-statusline/releases/latest/download/statusline-linux-amd64
chmod +x ~/.claude/statusline
```

Windows, PowerShell:

```powershell
curl.exe -Lo $HOME\.claude\statusline.exe https://github.com/AbdallaM-Ibrahim/claude-code-statusline/releases/latest/download/statusline-windows-amd64.exe
```

Those URLs always resolve to the newest release. Pin a version by swapping
`latest/download` for `download/v1.0.0`.

| Platform | Asset | Size |
|---|---|---|
| Windows x64 | `statusline-windows-amd64.exe` | 7.1 MB |
| macOS Apple silicon | `statusline-darwin-arm64` | 6.4 MB |
| macOS Intel | `statusline-darwin-amd64` | 7.0 MB |
| Linux x64 | `statusline-linux-amd64` | 6.8 MB |
| Linux arm64 | `statusline-linux-arm64` | 6.3 MB |

Every release ships a `SHA256SUMS`, and the assets are built by CI from the
tagged commit rather than uploaded from anyone's machine:

```sh
curl -LO https://github.com/AbdallaM-Ibrahim/claude-code-statusline/releases/latest/download/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
```

Ask a binary what it is with `statusline --version`. A build from a working tree
answers `dev`; a released one answers its tag.

### With Go

```sh
go install github.com/AbdallaM-Ibrahim/claude-code-statusline/cmd/statusline@latest
```

That lands in `$(go env GOPATH)/bin/statusline`.

### From source

```sh
git clone https://github.com/AbdallaM-Ibrahim/claude-code-statusline
cd claude-code-statusline

go build -trimpath -ldflags "-s -w" -o ~/.claude/statusline     ./cmd/statusline  # macOS / Linux
go build -trimpath -ldflags "-s -w" -o ~/.claude/statusline.exe ./cmd/statusline  # Windows
```

Or, with make: `make install` builds the host binary into `~/.claude`, and
`make dist` cross-compiles every target into `dist/` with checksums. Pure Go, no
cgo. On Windows without make, `./build.ps1` does both in one step.

Requires Go **1.26.6 or newer** — that floor is a security requirement, not a
preference. See [SECURITY.md](SECURITY.md#dependency-scanning).

---

## Wiring it up

Add this to `~/.claude/settings.json`:

```jsonc
{
  "statusLine": {
    "type": "command",
    "command": "/Users/you/.claude/statusline",   // macOS / Linux
    "padding": 0,
    "refreshInterval": 10
  }
}
```

On Windows use a forward-slashed absolute path:

```jsonc
"command": "C:/Users/you/.claude/statusline.exe"
```

`padding: 0` lets line 1 start at the left edge. `refreshInterval` is in seconds;
the render is cheap enough that 10 is comfortable, and there is a hard 2-second
deadline on the whole thing regardless.

Claude Code passes no arguments — the payload arrives on stdin. The one argument
this understands is `--version`, so a downloaded binary can identify itself;
anything else is ignored rather than refused, because a status line that will not
render over an unrecognised flag is worse than one that ignores it.

---

## What each segment means

### Line 1 — place

| Segment | Example | Hidden when |
|---|---|---|
| Directory | `software-engineer-website` | never |
| Branch | `⟨main⟩` | not a repository |
| Detached HEAD | `⟨detached⟩` | on a branch |
| Ahead / behind | `⟨main ↑2 ↓72⟩` | no upstream, or the commit walk could not finish |
| Short HEAD | `be66d0f` | no commits yet |
| Commit age | `2w ago` | commit unreadable |
| Subject | `· first commit` | commit unreadable; cut at 40 runes |
| Worktree | `⑂ feature-x` | payload has no worktree |
| Pull request | `PR #12` | payload has no PR |
| Agent | `@explore` | not running as a subagent |

Ahead/behind is as fresh as your last `fetch` — it compares against the
remote-tracking ref on disk, exactly as `git status --branch` does, and never
touches the network.

### Line 2 — session

| Segment | Example | Notes |
|---|---|---|
| Model | `🤖 Opus 5` | falls back to the model id, then to `Claude` |
| Fast mode | `⚡` | only when enabled |
| Effort | `xhigh` | from the payload |
| Thinking | `💭` | only when enabled |
| Context | `🧠 34%` | pre-computed percentage, else derived from the token breakdown |
| Money | `💰 $1.42 session / $8.90 today / $4.10 block (3h 42m left)` | session is exact (Claude Code sends it); the rest is computed |
| Burn rate | `🔥 $2.10/hr 🟢` | `🟢` under $5/hr, `⚠️` from $5, `🔴` from $15 |
| Rate limits | `⏳ 5h 42% resets 3:15pm · 7d 18%` | payload window reconciled against the account record |
| Per-model week | `Fable 30%` | a model-scoped weekly cap (the "Current week (Fable)" line in `/usage`); only while the usage record carries one whose reset is still ahead — open `/usage`, or turn on [automatic refresh](#automatic-usage-refresh-opt-in) |
| Caveman | `[CAVEMAN]` | only with the [caveman plugin](https://github.com/JuliusBrussee/caveman) active |

Percentages share one colour scale: green under 50%, yellow from 50%, red from
75%.

The block estimate is dropped when the payload already carries a rate-limit
window — two competing "how much is left" readings on one line is one too many.

---

## Configuration

| Variable | Effect |
|---|---|
| `CLAUDE_CONFIG_DIR` | Where Claude Code state lives. Honoured everywhere, so the whole thing can be pointed at a scratch directory. |
| `CAVEMAN_STATUSLINE_SAVINGS=0` | Suppresses the savings suffix after `[CAVEMAN]`. |
| `STATUSLINE_TEST_REPO` | Repository the test suite and git benchmarks read. Defaults to your checkout. |

### Pricing

Transcripts record token counts and a model name, never a cost. `today`, `block`
and the burn rate are therefore priced from `pricing.json`, embedded at build
time with `go:embed` (25 models at present).

To override without rebuilding, drop a `pricing.json` next to the binary; its
entries win. That is the escape hatch for a model released after your build —
and a reason to keep the binary somewhere only you can write
(see [SECURITY.md](SECURITY.md)).

An **unknown model contributes nothing** and marks the affected figures with a
`~` (`💰 ~$8.90 today`). Guessing a rate renders confidently wrong money, which
reads exactly like correct money; visibly incomplete is better.

Session cost is never computed — Claude Code passes it in the payload.

---

## How it works

```
stdin (JSON payload)
        │
        ├── goroutine 1 ──▶ go-git: HEAD, branch, one commit, ahead/behind    (400ms deadline)
        │                     └── ahead/behind cached on the (HEAD, upstream) hash pair
        │
        └── goroutine 2 ──▶ rate-limit windows: payload + ~/.claude.json
                            cost: incremental transcript scan ─▶ hour buckets ─▶ today / block / burn
                                                                      (2s deadline over everything)
```

Things worth knowing:

- **Deadlines, not hopes.** The whole render is bounded at 2 s and the git read at
  400 ms. On expiry whatever has landed is printed; partial beats blank.
- **Panics are contained.** Each goroutine recovers on its own, so one broken
  subsystem costs its own segments and nothing else.
- **The ahead/behind cache is exact, not stale-tolerant.** The counts are a pure
  function of the HEAD and upstream hashes, so the cache is keyed on both. If
  neither moved, the answer cannot have changed.
- **A truncated commit walk reports nothing.** Set-differencing two partial
  ancestries produces authoritative-looking wrong numbers — an early revision
  showed `↑1 ↓64` where git said `+0 -72`. The walk is bounded at 20 000 commits
  and refuses to guess.
- **The transcript scan is incremental.** Files are keyed by path with the size
  and mtime last consumed, so an unchanged file is dismissed on a stat and a
  grown one is read from its previous offset. Only complete, newline-terminated
  lines advance the cursor, so a record caught mid-write is not lost.
- **Responses are deduplicated.** Transcripts repeat entries — in one install
  2044 assistant lines collapsed to 959 unique responses, so counting naively
  roughly doubles every figure.
- **Nothing older than 48 hours is remembered**, which is what keeps the state
  file and the dedup set bounded.

### Package map

The code is one binary under `cmd/` and a set of packages under `internal/`,
which the Go toolchain keeps private to this module. Dependencies point one
way: `cmd` → `statusline` → the subsystems → the leaf packages. No subsystem
imports another, so each can be read, tested and benchmarked on its own.

```
cmd/statusline/         the process boundary: --version, stdin in, two lines out
internal/
  statusline/           fan-out, deadlines, panic isolation; composes line 1 and line 2
  gitinfo/              native git reads, ahead/behind and its hash-pair cache
  cost/                 incremental transcript scan, hour buckets, block maths, pricing.json
  limits/               rate-limit windows and reconciliation
  caveman/              caveman plugin indicator
  payload/              the stdin contract
  term/                 colours, terminal-safety filter, ages, clocks, truncation
  paths/                every file under ~/.claude this program touches
  testutil/             fixtures shared by more than one package's tests
```

| Package | Responsibility |
|---|---|
| `cmd/statusline` | `main`: argument handling, stdin/stdout, the version stamp |
| `internal/statusline` | `Render`: goroutine fan-out, the 2 s and 400 ms deadlines, panic recovery, the `🤖 Claude` fallback; `place.go` and `session.go` compose the two lines |
| `internal/gitinfo` | `Read` opens the repository with go-git and returns a `State`; `aheadbehind.go` walks both ancestries; `cache.go` keys the counts on the `(HEAD, upstream)` hash pair |
| `internal/cost` | `Build` scans transcripts incrementally (`state.go`), prices them (`pricing.go`, embedded `pricing.json`) and `Segments` renders 💰 and 🔥 |
| `internal/limits` | `Segment` reconciles the payload's windows against `~/.claude.json` |
| `internal/caveman` | `Segment` reads the plugin flag with symlink and size guards |
| `internal/payload` | `Input`, `Decode`, and the derived context percentage |
| `internal/term` | `Sanitize`, the colour helpers, `Heat`, `CompactAge`, `Clock`, `Truncate`, `Basename` |
| `internal/paths` | `ClaudeDir` and every path derived from it, honouring `CLAUDE_CONFIG_DIR` |
| `internal/testutil` | the test repository lookup, synthetic transcripts, the injection assertion |
| `bench/e2e.ps1` | wall-clock harness including process creation |
| `parity.ts` | diffs this against the bun version it replaces |

Where to look when changing something: a new segment on line 2 is a new
package under `internal/` plus one line in `internal/statusline/session.go`; a
new path under `~/.claude` goes in `internal/paths`; anything that puts
untrusted text on screen must pass through `term.Sanitize` first.

---

## Benchmarks

All numbers below were measured on the machine that runs this status line:

> **Intel Core i5-3320M @ 2.60 GHz**, 4 logical cores, Windows 10, Go 1.26.6.
> A 2012 dual-core laptop — deliberately the slow end. The box was **not
> quiescent**: a Claude Code session was running throughout, which is realistic
> but noisy, so `min` is the cleanest estimate and `median` the honest one.
> Reproduce with the commands under each table.

### End to end: what a status line tick actually costs

This is the number that matters, because it includes process creation — the thing
the rewrite was for. Identical payload on stdin, both arms interleaved iteration
by iteration so any load spike hits them equally, 30 iterations each, against the
real `~/.claude` with a 1.3 MB transcript named in the payload.

**Idle:**

| arm | min | median | p90 | max |
|---|---|---|---|---|
| **this binary** | **36.3 ms** | **45.4 ms** | 71.4 ms | 84.9 ms |
| the bun version it replaces | 192 ms | 251.4 ms | 383.9 ms | 456.3 ms |
| `bun -e ''` (calibration) | 20.2 ms | 23.5 ms | 36.7 ms | 43.1 ms |

**All four cores saturated:**

| arm | min | median | p90 | max |
|---|---|---|---|---|
| **this binary** | **32.2 ms** | **49.1 ms** | 103.7 ms | 231.1 ms |
| the bun version it replaces | 231.6 ms | 290 ms | 477.1 ms | 657.3 ms |
| `bun -e ''` (calibration) | 19.9 ms | 27.9 ms | 59.1 ms | 73.9 ms |

**5.5× faster idle, 5.9× under full load.** Note where the old version's floor
comes from: bare interpreter startup alone (23.5 ms) is half of this binary's
entire render, before a single byte of git or cost work.

Two things to keep the comparison honest:

- The `bun -e ''` row is why the ratio holds up under load: interpreter startup is
  what inflates, and the old version paid it before doing any work at all.
- The old arm needs `transcript_path` in the payload or it skips its `ccusage`
  subprocess entirely and looks 1.5× better than it is. The harness supplies one.
  The Go binary ignores that field — it scans the projects directory — so the
  field changes only the arm being compared against.

```powershell
pwsh bench/e2e.ps1                     # idle
pwsh bench/e2e.ps1 -Load               # every core saturated
pwsh bench/e2e.ps1 -Isolate            # against a scratch CLAUDE_CONFIG_DIR
```

Windows PowerShell 5.1 works too — the script is deliberately ASCII and avoids
.NET Core-only APIs.

### In-process: where the time goes

`min / median` of 5 runs, `-benchmem`:

| benchmark | min | median | B/op | allocs/op | what it covers |
|---|---|---|---|---|---|
| `RenderFull` | 8.4 ms | 9.3 ms | 285 KB | 1 870 | both lines, both goroutines, warm caches |
| `DecodePayload` | 14.3 µs | 28.7 µs | 696 B | 14 | the stdin parse |
| `GitReadOnly` | 3.6 ms | 6.7 ms | 100 KB | 582 | line 1 on this repo (10 commits) |
| `GitReadOnly` | 26.1 ms | 44.3 ms | 341 KB | 2 354 | line 1 on a 73-commit repo |
| `AheadBehindUncached` | 50.0 ms | 94.4 ms | 2.0 MB | 16 261 | the double ancestry walk, cache bypassed |
| `CostScanSteadyState` | 5.9 ms | 12.7 ms | 71 KB | 1 028 | 1 000 entries across 4 transcripts, nothing changed |
| `CostScanCold` | 22.5 ms | 38.3 ms | 1.72 MB | 19 802 | the same fixture parsed from scratch |
| `LimitsSegment` | 1.28 ms | 1.39 ms | 67 KB | 43 | read + parse the 62 KB `~/.claude.json` |
| `CavemanSegment` | 0.59 ms | 0.91 ms | 4.6 KB | 35 | two guarded small-file reads |

Read that table as a set of design decisions rather than trivia:

- **`RenderFull` (8.4 ms) is less than its parts summed** — line 1 and line 2 are
  gathered concurrently, so the git read hides behind the transcript scan.
- **`AheadBehindUncached` at 50–94 ms is why `internal/gitinfo/cache.go` exists**, and why it is
  keyed on the `(HEAD, upstream)` hash pair rather than a timer: at that price you
  want to pay it only when an answer could actually have changed. `GitReadOnly`,
  which hits that cache, is 7–14× cheaper on the same repository.
- **Cold vs steady-state cost scan (38.3 ms → 12.7 ms)** is the incremental cursor
  earning its keep. In normal operation almost every transcript is dismissed on a
  stat.
- **`LimitsSegment` is a fixed ~1.3 ms tax** for parsing the account record. It is
  the reason that file is decoded into a narrow struct rather than a generic map.

```sh
go test ./... -run '^$' -bench . -benchmem -count=5

# the git rows want a repository with history and an upstream
STATUSLINE_TEST_REPO=~/some/repo go test ./internal/gitinfo -run '^$' -bench . -benchmem -count=5
```

### Binary size

| target | asset | size |
|---|---|---|
| Windows x64 | `statusline-windows-amd64.exe` | 7.1 MB |
| macOS Apple silicon | `statusline-darwin-arm64` | 6.4 MB |
| macOS Intel | `statusline-darwin-amd64` | 7.0 MB |
| Linux x64 | `statusline-linux-amd64` | 6.8 MB |
| Linux arm64 | `statusline-linux-arm64` | 6.3 MB |

Pure Go, no cgo, `-trimpath -ldflags "-s -w"`. Most of it is go-git and its
crypto dependencies — the price of not shelling out to `git`.

### Historical note

The `9.8×` interpreter-inflation figure quoted above comes from a manual A/B run
on 2026-08-07 under sustained 100% CPU on the same 4-core box, recorded while the
old version was still in service: `bun -e ''` median went 86 ms → 842 ms while
`git log` via `spawnSync` went 115 ms → 243 ms. Process creation, not the work,
was the bottleneck — which is what made a compiled binary worth writing rather
than optimising the TypeScript.

---

## Development

```sh
go test ./...                  # suite
go test -race ./...            # the render fans out; the detector earns its keep
go vet ./...
gofmt -l .                     # must print nothing
go build ./cmd/statusline

go test ./... -run '^$' -bench . -benchmem -count=5   # micro-benchmarks
```

The same commands are Make targets: `make check` runs gofmt, vet, test and
build as CI does, and `make race`, `make bench`, `make vulncheck`, `make dist`
and `make install` cover the rest. `make help` lists them.

The git tests and benchmarks resolve their repository in this order:
`STATUSLINE_TEST_REPO`, then the checkout you are in, then skip. Point the
variable at a repository with an upstream and a long history to make the
ahead/behind benchmark say something:

```sh
STATUSLINE_TEST_REPO=~/work/some-big-repo go test ./internal/gitinfo -run '^$' -bench AheadBehind -count=5
```

Working from a `git worktree` checkout? go-git cannot resolve HEAD through the
`.git` file a worktree gets, so the real-repository git tests fail there. Point
`STATUSLINE_TEST_REPO` at the main checkout instead.

CI runs the suite on Linux, macOS and Windows, plus `go vet`, `gofmt`,
`go test -race` and `govulncheck`.

### Parity against the old version

`parity.ts` diffs this binary against the bun/TypeScript status line it replaced,
across the payload shapes that actually vary. It needs bun and a copy of the old
`statusline.js`, so it is only useful on a machine that ran that version:

```sh
STATUSLINE_JS=~/.claude/statusline.js bun parity.ts
```

Line 1 was verified byte-for-byte identical across every payload shape tested.
The deliberate line 2 differences:

| Difference | Why |
|---|---|
| `today` / `block` / burn are **higher** | ccusage's offline pricing table has no `claude-opus-5` entry and priced it at $0 — its own daily row for 2026-08-07 carried 44.9M tokens at `totalCost: 0`. This table prices it. |
| Burn-rate emoji thresholds | Ours is a plain documented rate threshold. ccusage's marker did not track the rate monotonically in observed output, so it keys off something not derivable from a transcript. |
| No `session` figure when the payload omits `cost` | ccusage derives one from the transcript; this only ever passes the payload's value through. Claude Code always sends it, so this shows up in synthetic payloads only. |

The cost maths was validated at token level rather than against ccusage's
dollars: deduplicated totals since local midnight matched ccusage's own per-day
token attribution exactly — `input 576`, `output 232 533`,
`cacheRead 44 271 478`, `cacheCreation 402 901`.

---

## Security

No network, no subprocesses, no secrets read or logged. The interesting surface
is that **a repository you did not write supplies text this program prints to
your terminal on a timer** — so every untrusted string is filtered through
`term.Sanitize` before it is coloured or truncated, stripping escape introducers,
DEL, single-byte C1 CSI/OSC, bidi overrides and invalid UTF-8.

[SECURITY.md](SECURITY.md) has the threat model, the findings fixed in the first
public release, and how to report anything new.

---

## Known limitations

- **Concurrent renders can undercount, briefly.** Several sessions rendering at
  once read-modify-write one cost-state file; the last writer wins. The next
  render re-reads from each file's stored offset, so it self-corrects, but a tick
  can report slightly low.
- **Ahead/behind is as stale as your last fetch.** By design — the render never
  touches the network.
- **A model missing from the pricing table contributes $0** and marks the figures
  `~`. Add it to `pricing.json`.
- **`~/.claude.json` is read on every render** for the account-wide rate-limit
  record. It is a ~50 KB parse, which the benchmarks below account for.
- **Per-model weekly caps come only from a usage record, and Claude Code refreshes
  its own only when `/usage` is opened** (verified against v2.1.266: the write sits
  behind the `/usage` fetch, throttled to once per 5 minutes, and Claude's own
  reader discards it after an hour). The stdin payload has no scoped window, so
  `Fable 30%` is as fresh as your last `/usage` — or as the last
  [automatic refresh](#automatic-usage-refresh-opt-in), if you turned it on — and
  carries the same age label as any other global reading. Once the row's reset
  passes it is dropped rather than shown as 0% — nothing live can confirm the new
  week. If the segment is missing, open `/usage` once; it returns on the next
  render.

## Automatic usage refresh (opt-in)

Off by default. With it off, the program has no network code path and never reads
a credential — exactly as before.

Claude Code writes the usage record in `~/.claude.json` only when you open
`/usage`. On a machine where you never do, the per-model week (`Fable 30%`) goes
stale, then disappears, and the account-wide 5h/7d numbers you see while a session
is idle can lag too. Set one environment variable on the status line command and
this program keeps its own record fresh instead:

```jsonc
// macOS / Linux, and Windows with Git Bash installed (Claude Code runs the
// command through Git Bash when it is present, PowerShell otherwise)
"command": "STATUSLINE_USAGE_REFRESH=1 /Users/you/.claude/statusline"
"command": "STATUSLINE_USAGE_REFRESH=1 C:/Users/you/.claude/statusline.exe"
```

On Windows without Git Bash, put the two lines in a `statusline.cmd` next to the
binary and point `command` at that:

```bat
@set STATUSLINE_USAGE_REFRESH=1
@"C:\Users\you\.claude\statusline.exe"
```

| Value | Effect |
|---|---|
| unset, empty, `0`, `false`, `off`, `no` | off |
| `1`, `true`, `on`, `yes` | on, refresh when the newest record is older than **5 minutes** |
| a Go duration, e.g. `10m`, `1h` | on, at that interval; anything under `2m` is raised to `2m` |
| anything else | off — a typo must never turn into a network call |

What happens, once per interval, per machine:

1. The render finds both records — Claude's `~/.claude.json` and this program's
   `~/.claude/statusline-usage.json` — older than the interval.
2. It reads the OAuth access token Claude Code already holds:
   `~/.claude/.credentials.json` on Windows and Linux, the login Keychain on macOS
   (`security find-generic-password -s "Claude Code-credentials" -w`, the only
   subprocess this program ever spawns, macOS only). A token that is not an OAuth
   access token (`sk-ant-oat…`), or is within 30 s of expiry, means no request;
   Claude Code refreshes its token on its own next call and this program never
   does.
3. It takes `statusline-usage.lock` so concurrent sessions make one request, not
   four, re-checks the cache under the lock in case another session just fetched,
   and sends `GET https://api.anthropic.com/api/oauth/usage` — the same request
   `/usage` makes — with a 1.2 s budget inside the render's 2 s deadline.
   Redirects are refused; the token goes to that host and nowhere else.
4. A `200` whose body carries a `limits` list is written, verbatim, to
   `statusline-usage.json` (`0600`) in the same shape as Claude's record. Any
   other outcome writes nothing and leaves the lock in place, which backs the next
   attempt off for a full interval; the segment renders whatever it already had.

The hard ceiling is therefore **one request per interval per machine**, however
many sessions are open and however often they render — and the same ceiling holds
while the request is failing.

The render then reads whichever record was fetched last. Cost on the machine this
was measured on: about 50 ms of CPU and 14 MB for the one render in every interval
that fetches; the other renders pay one extra `stat`. Claude Code's own record is
never written.

## Troubleshooting

| Symptom | Cause |
|---|---|
| A bare `🤖 Claude` | stdin did not parse as JSON. Check the command in `settings.json` actually points at the binary. |
| No money segments | No transcripts inside the 48-hour horizon, or `CLAUDE_CONFIG_DIR` points somewhere empty. |
| Money marked `~` | A model in your transcripts is missing from the pricing table. |
| No `↑`/`↓` | The branch has no upstream, or the commit walk hit its bound and refused to guess. |
| Line 1 is only a directory name | Not a git repository, or the git read exceeded its 400 ms deadline. |

---

## License

[MIT](LICENSE) © 2026 Abdalla Mostafa
