package gitinfo

import (
	"context"
	"errors"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

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

	// The counts are a pure function of these two hashes, so a cache keyed on
	// both is exact rather than merely fresh — see cache.go.
	root := ""
	if wt, err := repo.Worktree(); err == nil {
		root = wt.Filesystem.Root()
	}
	key := cacheKey(root, headHash.String(), ref.Hash().String())
	cache := loadCache()
	if hit, ok := cache.Entries[key]; ok {
		hit.At = time.Now().Unix()
		cache.Entries[key] = hit
		cache.save()
		return hit.Ahead, hit.Behind, nil
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

	cache.Entries[key] = cacheEntry{Ahead: ahead, Behind: behind, At: time.Now().Unix()}
	cache.save()

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
