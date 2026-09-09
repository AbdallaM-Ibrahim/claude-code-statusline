// Package statusline composes the two lines from the subsystem packages.
//
// It reads the status line JSON payload and produces:
//
//	line 1 — place:   repo dir, branch, ahead/behind, HEAD hash, commit age, subject
//	line 2 — session: model, effort, context %, cost, rate-limit resets, caveman mode
//
// Nothing here spawns a subprocess. The version this replaces ran bun, two git
// commands and ccusage on every render; on a 4-core machine under load, bare
// interpreter startup inflated 9.8x while git inflated 2.1x, so process
// creation — not the work itself — was the cost.
//
// Every segment is optional. A missing payload field, an unreadable file or a
// failing subsystem drops that segment alone; it never blanks the line.
//
// This package owns the fan-out, the deadlines and the panic isolation. What
// each segment says is decided in the package that gathers it: gitinfo, cost,
// limits and caveman.
package statusline

import (
	"context"
	"os"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/cost"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/limits"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/term"
)

// renderDeadline bounds the whole render. The status line is re-run on a timer,
// so a tick that cannot finish promptly is better abandoned than queued behind.
const renderDeadline = 2 * time.Second

// gitDeadline is tighter: line 1 is the least important thing on screen and the
// most able to stall on a pathological repository.
const gitDeadline = 400 * time.Millisecond

// Fallback is the last resort when stdin will not parse.
//
// The version this replaces tried to recover the model name here with a second
// JSON parse of the same bytes. That could never succeed: the only way to reach
// this path is a parse failure, so the retry failed identically every time. The
// payload struct ignores unknown fields, so no valid JSON object reaches here
// either. A bare name is all this can honestly report.
const Fallback = "🤖 Claude"

// Render turns the raw stdin payload into the two-line status line.
func Render(raw []byte) string {
	in, err := payload.Decode(raw)
	if err != nil {
		return Fallback
	}

	ctx, cancel := context.WithTimeout(context.Background(), renderDeadline)
	defer cancel()

	cwd := in.Dir(mustGetwd())

	// Line 1's git read and line 2's transcript scan touch nothing in common, so
	// they run together. Both are in-process now, so this is goroutine
	// scheduling rather than the process spawning that used to dominate.
	//
	// Results come back over channels rather than through shared variables: on
	// a deadline we stop waiting but the goroutines keep running, and reading
	// variables they might still be writing would be a data race.
	placeCh := make(chan string, 1)
	sessionCh := make(chan sessionParts, 1)

	go func() {
		// A panic in one subsystem must not take the render down. A status line
		// that loses its git segments is a nuisance; one that crashes leaves the
		// user staring at nothing.
		defer func() {
			if recover() != nil {
				placeCh <- ""
			}
		}()
		gitCtx, gitCancel := context.WithTimeout(ctx, gitDeadline)
		defer gitCancel()
		placeCh <- buildPlaceLine(gitCtx, in, cwd)
	}()

	go func() {
		defer func() {
			if recover() != nil {
				sessionCh <- sessionParts{}
			}
		}()
		// The limits segment decides whether the block estimate is redundant, so
		// it has to be resolved before the cost segments are rendered.
		lim := limits.Segment(in)
		rep := cost.Build(in, time.Now())
		sessionCh <- sessionParts{limits: lim, cost: cost.Segments(rep, lim != "")}
	}()

	var (
		place   string
		session sessionParts
	)

	for pending := 2; pending > 0; pending-- {
		select {
		case p := <-placeCh:
			place = p
		case s := <-sessionCh:
			session = s
		case <-ctx.Done():
			// Deadline hit: render whatever landed. Partial beats blank.
			pending = 1
		}
	}

	if place == "" {
		place = term.Cyan(dirLabel(cwd))
	}
	return place + "\n" + buildSessionLine(in, session.limits, session.cost)
}

// sessionParts is line 2's independently gathered pieces.
type sessionParts struct {
	limits string
	cost   []string
}

func mustGetwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
