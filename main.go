// Command statusline renders the Claude Code status line.
//
// It reads the status line JSON payload on stdin and writes two lines:
//
//	line 1 — place:   repo dir, branch, ahead/behind, HEAD hash, commit age, subject
//	line 2 — session: model, effort, context %, cost, rate-limit resets, caveman mode
//
// It spawns no subprocesses. The version this replaces ran bun, two git commands
// and ccusage on every render; on a 4-core machine under load, bare interpreter
// startup inflated 9.8x while git inflated 2.1x, so process creation — not the
// work itself — was the cost.
//
// Every segment is optional. A missing payload field, an unreadable file or a
// failing subsystem drops that segment alone; it never blanks the line.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// renderDeadline bounds the whole render. The status line is re-run on a timer,
// so a tick that cannot finish promptly is better abandoned than queued behind.
const renderDeadline = 2 * time.Second

// gitDeadline is tighter: line 1 is the least important thing on screen and the
// most able to stall on a pathological repository.
const gitDeadline = 400 * time.Millisecond

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Println("🤖 Claude")
		return
	}

	out := render(raw)

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	fmt.Fprintln(w, out)
}

func render(raw []byte) string {
	in, err := decodePayload(raw)
	if err != nil {
		return fallbackLine(raw)
	}

	ctx, cancel := context.WithTimeout(context.Background(), renderDeadline)
	defer cancel()

	cwd := in.dir(mustGetwd())

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
		limits := limitsSegment(in)
		rep := buildCostReport(in, time.Now())
		sessionCh <- sessionParts{limits: limits, cost: costSegments(rep, limits != "")}
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
		place = cyan(basename(cwd))
	}
	return place + "\n" + buildSessionLine(in, session.limits, session.cost)
}

// sessionParts is line 2's independently gathered pieces.
type sessionParts struct {
	limits string
	cost   []string
}

// fallbackLine is the last resort when stdin will not parse.
//
// The version this replaces tried to recover the model name here with a second
// JSON parse of the same bytes. That could never succeed: the only way to reach
// this path is a parse failure, so the retry failed identically every time. The
// payload struct ignores unknown fields, so no valid JSON object reaches here
// either. A bare name is all this can honestly report.
func fallbackLine([]byte) string {
	return "🤖 Claude"
}

func mustGetwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
