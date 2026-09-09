package statusline

import (
	"context"
	"fmt"
	"strings"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/gitinfo"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/term"
)

// dirLabel is the directory name as it is safe to print. A path is as
// attacker-controllable as anything else on the line: a repository can be cloned
// into a directory whose name carries escape bytes.
func dirLabel(cwd string) string { return term.Sanitize(term.Basename(cwd)) }

// subjectMax is where a commit subject gets cut, matching the original.
const subjectMax = 40

// buildPlaceLine renders line 1: where you are.
//
//	software-engineer-website ⟨main ↓72⟩ be66d0f 2w ago · first commit
//
// Every part after the directory name is optional. An unreadable repository
// leaves just the directory name rather than blanking the line.
func buildPlaceLine(ctx context.Context, in *payload.Input, cwd string) string {
	g, err := gitinfo.Read(ctx, cwd)
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
func placeLine(g *gitinfo.State, in *payload.Input, cwd string) string {
	parts := []string{term.Cyan(dirLabel(cwd))}

	if g != nil {
		var inner []string

		// Detached HEAD still shows the hash separately below, so the marker
		// only has to flag the state.
		if g.Branch != "" {
			inner = append(inner, term.Magenta(term.Sanitize(g.Branch)))
		} else {
			inner = append(inner, term.Magenta("detached"))
		}
		if g.Ahead > 0 {
			inner = append(inner, term.Green(fmt.Sprintf("↑%d", g.Ahead)))
		}
		if g.Behind > 0 {
			inner = append(inner, term.Red(fmt.Sprintf("↓%d", g.Behind)))
		}
		if len(inner) > 0 {
			parts = append(parts, term.Dim("⟨")+strings.Join(inner, " ")+term.Dim("⟩"))
		}

		if g.Hash != "" {
			parts = append(parts, term.Yellow(g.Hash))
		}
		if !g.When.IsZero() {
			parts = append(parts, term.Dim(term.CompactAge(g.When.Unix())))
		}
		if g.Subject != "" {
			parts = append(parts, term.Dim("· "+term.Truncate(term.Sanitize(g.Subject), subjectMax)))
		}
	}

	if in.Worktree.Name != "" {
		parts = append(parts, term.Blue("⑂ "+term.Sanitize(in.Worktree.Name)))
	}
	if in.PR.Number != 0 {
		parts = append(parts, term.Blue(fmt.Sprintf("PR #%d", in.PR.Number)))
	}
	if in.Agent.Name != "" {
		parts = append(parts, term.Blue("@"+term.Sanitize(in.Agent.Name)))
	}

	return strings.Join(parts, " ")
}
