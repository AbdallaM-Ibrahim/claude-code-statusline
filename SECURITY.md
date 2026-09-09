# Security

## What this program is exposed to

It renders a status line every few seconds from three untrusted sources:

1. **Git metadata** — branch name and commit subject from whatever repository the
   shell happens to be in. Cloning a repository is enough to hand an attacker
   this input, and they never need code execution.
2. **The Claude Code payload** on stdin — model name, effort level, worktree and
   agent names.
3. **Local state files** — `~/.claude/statusline-cost-state.json`,
   `statusline-git-cache.json`, `statusline-usage.json`, the caveman flag and its
   savings suffix, and every transcript under `~/.claude/projects`.
4. **Only with `STATUSLINE_USAGE_REFRESH` set** — Claude Code's credential store
   and one HTTPS response from `api.anthropic.com`. See
   [the opt-in network path](#the-opt-in-network-path-statusline_usage_refresh).

By default it has no network code path of its own, spawns no subprocesses, and
writes nothing outside `~/.claude` (or `$CLAUDE_CONFIG_DIR`). The one opt-in
exception is described below.

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

## The opt-in network path (`STATUSLINE_USAGE_REFRESH`)

`internal/usage` is the only code that touches a credential or a socket, and none
of it runs unless the environment variable is set on the status line command. With
it set, at most once per interval (default 5 minutes, floor 2) per machine:

| Step | Guarantee |
|---|---|
| Read the OAuth access token from `~/.claude/.credentials.json` (macOS: the login Keychain via `/usr/bin/security`, the program's only subprocess, darwin only) | Only `claudeAiOauth.accessToken` and `expiresAt` are decoded; the refresh token is never read into a typed field. The file is read through `Lstat` with a 64 KiB cap; a symlink or an oversized file reads as absent. |
| Decide whether to send | The token must start with `sk-ant-oat` — an API key (`sk-ant-api…`) is never sent as a bearer token — and must not be within 30 s of `expiresAt`. This program never refreshes a token. |
| `GET https://api.anthropic.com/api/oauth/usage` with `Authorization: Bearer …` and `anthropic-beta: oauth-2025-04-20` | The endpoint is a compile-time constant; no environment variable or file can redirect it. `CheckRedirect` refuses every redirect, so the token reaches one host. 1.2 s timeout, 1 MiB body cap. |
| Cache the response | Only a `200` whose body is a JSON object with a non-null `limits` list is written, to `statusline-usage.json`, `0600`, via temp file and rename. The token is never written anywhere, never logged, never printed. |
| Serialise across sessions | `statusline-usage.lock` (`O_EXCL`); a failed attempt leaves it in place as a 60 s back-off, so a revoked token costs one request a minute, not one a render. |

What this changes in the threat model: a process that can already read the user's
`~/.claude` could already read the credential file — this program adds no new
access. It does add `net/http` and `crypto/tls` to the binary's reachable code, so
govulncheck findings in those packages now matter when the switch is on. It never
writes Claude Code's own `~/.claude.json`.

## Dependency scanning

`go run golang.org/x/vuln/cmd/govulncheck@latest ./...` runs in CI on every push.

The `go` directive in `go.mod` is a **security floor, not a preference**: it was
raised to 1.26.6 because 1.26.5 carried two advisories govulncheck found
reachable from this code (`GO-2026-6090` in `crypto/tls`, `GO-2026-5972` in
`encoding/asn1`, both reached through go-git). Do not lower it.

One advisory is knowingly carried: `GO-2026-5932` in `golang.org/x/crypto`
v0.53.0, which has no published fix. It is reachable only through go-git's SSH
transport, which this program never uses — git refs and objects are read from
disk, and the opt-in usage fetch above goes through `net/http`, not go-git.

## Residual risk worth knowing about

`pricing.json` is loaded from the directory **next to the binary**, overriding the
embedded table with no signature check. That is deliberate — it is the escape
hatch for a newly released model — but it means anyone who can write next to the
binary controls the money shown on your status line. Install it somewhere only
you can write, such as `~/.claude`.

## Reporting

Open a GitHub security advisory on this repository, or a normal issue if the
problem is not sensitive.
