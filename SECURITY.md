# Security

## What this program is exposed to

It renders a status line every few seconds from three untrusted sources:

1. **Git metadata** — branch name and commit subject from whatever repository the
   shell happens to be in. Cloning a repository is enough to hand an attacker
   this input, and they never need code execution.
2. **The Claude Code payload** on stdin — model name, effort level, worktree and
   agent names.
3. **Local state files** — `~/.claude/statusline-cost-state.json`,
   `statusline-git-cache.json`, the caveman flag and its savings suffix, and every
   transcript under `~/.claude/projects`.

It has no network code path of its own, spawns no subprocesses, and writes
nothing outside `~/.claude` (or `$CLAUDE_CONFIG_DIR`).

## The main class of bug: terminal escape injection

Everything on both lines is written to a terminal, which *executes* escape
sequences rather than printing them. A commit subject carrying `ESC[2J` clears
the screen; an OSC 8 sequence turns text into a hyperlink to somewhere else; a
bidi override renders a branch name as something it is not. Because the status
line repaints on a timer, one hostile commit repeats the effect indefinitely and
gives no hint where it came from.

Every untrusted string therefore passes through `term.Sanitize`
(`internal/term/sanitize.go`) before it is coloured or truncated. It removes:

| Removed | Why |
|---|---|
| C0 `0x00–0x1F` except tab | `ESC` introduces CSI and OSC |
| `0x7F` DEL | erases already-rendered output on some terminals |
| C1 `0x80–0x9F` | `0x9B` is a single-byte CSI, `0x9D` a single-byte OSC |
| U+202A–U+202E, U+2066–U+2069 | bidi overrides — text that renders as something else |
| invalid UTF-8 | a lone `0x9B` byte is not valid UTF-8, and passing it through hands the terminal a CSI |

Sanitising happens **before** truncation, so a 40-rune cut cannot land inside a
sequence and reassemble one.

`internal/term/sanitize_test.go` is the regression suite for the filter itself.
The injection tests in `internal/statusline` and `internal/caveman` check the
rendered lines, using the distinction that matters when asserting: the only
escapes allowed in the output are the SGR colour codes this program writes
itself (`testutil.AssertNoInjection`).

## Fixed in the initial public release

| Finding | Severity | Fix |
|---|---|---|
| Git branch name and commit subject were written to the terminal unfiltered; a hostile repository could inject escape sequences on every render | High | `term.Sanitize` applied to all git-derived strings, plus the cwd basename and payload strings |
| One transcript line was read with an unbounded `ReadString`, so a corrupt or hostile line could be buffered whole | Moderate | `maxLineBytes` (4 MiB) cap in `readLine` (`internal/cost/state.go`); an over-long line is stepped over, never buffered or parsed |
| Cost-state and git-cache files were written `0644`, though they list every project path on the machine and recent API response ids | Low | `0600` |
| The caveman flag read did `Lstat` then a separate `ReadFile`, so the path could be swapped between the two checks | Low | one `os.Open`, checks against the handle, and `os.SameFile` confirming it is the file `Lstat` approved (`internal/caveman`) |

## Dependency scanning

`go run golang.org/x/vuln/cmd/govulncheck@latest ./...` runs in CI on every push.

The `go` directive in `go.mod` is a **security floor, not a preference**: it was
raised to 1.26.6 because 1.26.5 carried two advisories govulncheck found
reachable from this code (`GO-2026-6090` in `crypto/tls`, `GO-2026-5972` in
`encoding/asn1`, both reached through go-git). Do not lower it.

One advisory is knowingly carried: `GO-2026-5932` in `golang.org/x/crypto`
v0.53.0, which has no published fix. It is reachable only through go-git's SSH
transport, and this program never opens a network connection — it reads refs and
objects from disk.

## Residual risk worth knowing about

`pricing.json` is loaded from the directory **next to the binary**, overriding the
embedded table with no signature check. That is deliberate — it is the escape
hatch for a newly released model — but it means anyone who can write next to the
binary controls the money shown on your status line. Install it somewhere only
you can write, such as `~/.claude`.

## Reporting

Open a GitHub security advisory on this repository, or a normal issue if the
problem is not sensitive.
