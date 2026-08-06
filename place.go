package main

import (
	"context"
	"fmt"
	"strings"
)

// subjectMax is where a commit subject gets cut, matching the original.
const subjectMax = 40

// buildPlaceLine renders line 1: where you are.
//
//	software-engineer-website ⟨main ↓72⟩ be66d0f 2w ago · first commit
//
// Every part after the directory name is optional. An unreadable repository
// leaves just the directory name rather than blanking the line.
func buildPlaceLine(ctx context.Context, in *StatusLineInput, cwd string) string {
	parts := []string{cyan(basename(cwd))}

	if g, err := readGit(ctx, cwd); err == nil && g != nil {
		var inner []string

		// Detached HEAD still shows the hash separately below, so the marker
		// only has to flag the state.
		if g.Branch != "" {
			inner = append(inner, magenta(g.Branch))
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
			parts = append(parts, dim("· "+truncate(g.Subject, subjectMax)))
		}
	}

	if in.Worktree.Name != "" {
		parts = append(parts, blue("⑂ "+in.Worktree.Name))
	}
	if in.PR.Number != 0 {
		parts = append(parts, blue(fmt.Sprintf("PR #%d", in.PR.Number)))
	}
	if in.Agent.Name != "" {
		parts = append(parts, blue("@"+in.Agent.Name))
	}

	return strings.Join(parts, " ")
}
