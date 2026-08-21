package main

import (
	"context"
	"fmt"
	"strings"
)

// dirLabel is the directory name as it is safe to print. A path is as
// attacker-controllable as anything else on the line: a repository can be cloned
// into a directory whose name carries escape bytes.
func dirLabel(cwd string) string { return safeTerminal(basename(cwd)) }

// subjectMax is where a commit subject gets cut, matching the original.
const subjectMax = 40

// buildPlaceLine renders line 1: where you are.
//
//	software-engineer-website ⟨main ↓72⟩ be66d0f 2w ago · first commit
//
// Every part after the directory name is optional. An unreadable repository
// leaves just the directory name rather than blanking the line.
func buildPlaceLine(ctx context.Context, in *StatusLineInput, cwd string) string {
	g, err := readGit(ctx, cwd)
	if err != nil {
		g = nil
	}
	return placeLine(g, in, cwd)
}

// placeLine composes line 1 from state already gathered.
//
// Split from the read so the composition can be tested against input a real
// repository cannot easily be made to hold: a branch name carrying an escape
// byte is not a legal filename on Windows, yet it can still arrive through
// packed-refs or a crafted HEAD.
func placeLine(g *GitState, in *StatusLineInput, cwd string) string {
	parts := []string{cyan(dirLabel(cwd))}

	if g != nil {
		var inner []string

		// Detached HEAD still shows the hash separately below, so the marker
		// only has to flag the state.
		if g.Branch != "" {
			inner = append(inner, magenta(safeTerminal(g.Branch)))
		} else {
			inner = append(inner, magenta("detached"))
		}
		if g.Ahead > 0 {
			inner = append(inner, green(fmt.Sprintf("↑%d", g.Ahead)))
		}
		if g.Behind > 0 {
			inner = append(inner, red(fmt.Sprintf("↓%d", g.Behind)))
		}
		if len(inner) > 0 {
			parts = append(parts, dim("⟨")+strings.Join(inner, " ")+dim("⟩"))
		}

		if g.Hash != "" {
			parts = append(parts, yellow(g.Hash))
		}
		if g.Age != "" {
			parts = append(parts, dim(g.Age))
		}
		if g.Subject != "" {
			parts = append(parts, dim("· "+truncate(safeTerminal(g.Subject), subjectMax)))
		}
	}

	if in.Worktree.Name != "" {
		parts = append(parts, blue("⑂ "+safeTerminal(in.Worktree.Name)))
	}
	if in.PR.Number != 0 {
		parts = append(parts, blue(fmt.Sprintf("PR #%d", in.PR.Number)))
	}
	if in.Agent.Name != "" {
		parts = append(parts, blue("@"+safeTerminal(in.Agent.Name)))
	}

	return strings.Join(parts, " ")
}
