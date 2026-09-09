// Package gitinfo reads what line 1 needs from a repository without spawning
// git: HEAD, the branch, one commit, and the ahead/behind counts against the
// configured upstream.
//
// Everything is read in-process with go-git. The ahead/behind walk is the one
// expensive operation, so it is cached on the (HEAD, upstream) hash pair — see
// cache.go.
package gitinfo

import (
	"context"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// State is everything line 1 renders. Note what is absent: there is no dirty
// or modified-file count. The previous implementation ran a full
// `git status --porcelain=v2` on every render to compute one, and nothing ever
// displayed it. Reading refs and one commit is all this line actually needs, and
// it avoids walking the working tree entirely.
type State struct {
	Branch   string // empty when detached
	Ahead    int
	Behind   int
	Hash     string    // short, 7 chars
	When     time.Time // committer time of HEAD; zero when the commit was unreadable
	Subject  string
	Detached bool
}

// Read opens the repository containing dir and collects line 1's data.
//
// ctx carries the deadline: a pathological repository drops line 1's git
// segments for that tick rather than delaying the whole render, which is the
// same bargain the 400ms subprocess timeout struck before.
func Read(ctx context.Context, dir string) (*State, error) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}

	head, err := repo.Head()
	if err != nil {
		// A repository with no commits yet: nothing to render, not an error
		// worth surfacing.
		return nil, err
	}

	st := &State{Hash: shortHash(head.Hash())}

	if head.Name().IsBranch() {
		st.Branch = head.Name().Short()
	} else {
		st.Detached = true
	}

	if err := ctx.Err(); err != nil {
		return st, nil // deadline hit: return what we have
	}

	if commit, err := repo.CommitObject(head.Hash()); err == nil {
		st.When = commit.Committer.When
		st.Subject = subjectOf(commit)
	}

	if err := ctx.Err(); err != nil {
		return st, nil
	}

	// Ahead/behind is best-effort: no upstream configured is the normal case for
	// a local-only branch, not a failure.
	if st.Branch != "" {
		if ahead, behind, err := aheadBehind(ctx, repo, st.Branch, head.Hash()); err == nil {
			st.Ahead, st.Behind = ahead, behind
		}
	}

	return st, nil
}

func shortHash(h plumbing.Hash) string {
	s := h.String()
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// subjectOf takes the first line of the commit message, matching git's %s.
func subjectOf(c *object.Commit) string {
	msg := c.Message
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return strings.TrimSpace(msg)
}
