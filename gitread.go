package main

import (
	"context"
	"errors"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitState is everything line 1 renders. Note what is absent: there is no dirty
// or modified-file count. The previous implementation ran a full
// `git status --porcelain=v2` on every render to compute one, and nothing ever
// displayed it. Reading refs and one commit is all this line actually needs, and
// it avoids walking the working tree entirely.
type GitState struct {
	Branch   string // empty when detached
	Ahead    int
	Behind   int
	Hash     string // short, 7 chars
	Age      string // already formatted, e.g. "2w ago"
	Subject  string
	Detached bool
}

// readGit opens the repository containing dir and collects line 1's data.
//
// ctx carries the deadline: a pathological repository drops line 1's git
// segments for that tick rather than delaying the whole render, which is the
// same bargain the 400ms subprocess timeout struck before.
func readGit(ctx context.Context, dir string) (*GitState, error) {
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

	st := &GitState{Hash: shortHash(head.Hash())}

	if head.Name().IsBranch() {
		st.Branch = head.Name().Short()
	} else {
		st.Detached = true
	}

	if err := ctx.Err(); err != nil {
		return st, nil // deadline hit: return what we have
	}

	if commit, err := repo.CommitObject(head.Hash()); err == nil {
		st.Age = compactAge(commit.Committer.When.Unix())
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

var errNoUpstream = errors.New("no upstream configured")

// aheadBehind counts commits on each side of the fork point between the local
// branch and its configured upstream, the same numbers `git status --branch`
// reports as +N/-N.
func aheadBehind(ctx context.Context, repo *git.Repository, branch string, headHash plumbing.Hash) (int, int, error) {
	cfg, err := repo.Config()
	if err != nil {
		return 0, 0, err
	}
	br, ok := cfg.Branches[branch]
	if !ok || br.Remote == "" {
		return 0, 0, errNoUpstream
	}

	// The upstream's remote-tracking ref. This is local data, last refreshed by
	// whenever the user last fetched — the counts are as stale as that fetch,
	// exactly as they were with the git subprocess.
	upstreamName := branch
	if br.Merge != "" {
		upstreamName = br.Merge.Short()
	}
	ref, err := repo.Reference(
		plumbing.NewRemoteReferenceName(br.Remote, upstreamName), true)
	if err != nil {
		return 0, 0, err
	}

	// A truncated walk must not produce counts. Set-differencing two partial
	// ancestries yields numbers that look authoritative and are wrong — an
	// observed run reported "↑1 ↓64" where git reported "+0 -72". Showing
	// nothing is the honest outcome.
	local, truncated, err := ancestry(ctx, repo, headHash)
	if err != nil || truncated {
		return 0, 0, errWalkTruncated
	}
	remote, truncated, err := ancestry(ctx, repo, ref.Hash())
	if err != nil || truncated {
		return 0, 0, errWalkTruncated
	}

	ahead := 0
	for h := range local {
		if _, shared := remote[h]; !shared {
			ahead++
		}
	}
	behind := 0
	for h := range remote {
		if _, shared := local[h]; !shared {
			behind++
		}
	}
	return ahead, behind, nil
}

// ancestryLimit bounds the walk. An unbounded walk on a large history is exactly
// the stall the deadline exists to prevent — but hitting either bound makes the
// resulting counts meaningless, so truncation is reported rather than hidden.
const ancestryLimit = 20000

// ancestry collects every commit reachable from `from`. truncated is true when
// the walk stopped early, in which case the set is incomplete and callers must
// not derive counts from it.
func ancestry(ctx context.Context, repo *git.Repository, from plumbing.Hash) (seen map[plumbing.Hash]struct{}, truncated bool, err error) {
	seen = make(map[plumbing.Hash]struct{}, 64)
	iter, err := repo.Log(&git.LogOptions{From: from})
	if err != nil {
		return nil, false, err
	}
	defer iter.Close()

	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if ctx.Err() != nil {
			truncated = true
			return storerStop
		}
		seen[c.Hash] = struct{}{}
		count++
		if count >= ancestryLimit {
			truncated = true
			return storerStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, storerStop) {
		return nil, false, err
	}
	return seen, truncated, nil
}

// storerStop ends a ForEach early without being treated as a failure.
var storerStop = errors.New("stop")

// errWalkTruncated means the commit walk did not finish, so ahead/behind cannot
// be computed honestly.
var errWalkTruncated = errors.New("commit walk truncated")
