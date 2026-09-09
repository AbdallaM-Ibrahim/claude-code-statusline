package gitinfo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

func readForBench(repo string) (*State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return Read(ctx, repo)
}

// BenchmarkGitReadOnly is line 1 with the ahead/behind cache warm: open the
// repository, resolve HEAD, read one commit.
func BenchmarkGitReadOnly(b *testing.B) {
	repo := testutil.Repo(b)
	if _, err := readForBench(repo); err != nil {
		b.Skipf("repo unavailable: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = readForBench(repo)
	}
}

// BenchmarkAheadBehindUncached is the reason cache.go exists: two full ancestry
// walks, reading every object along the way. Skips unless the branch has an
// upstream to compare against.
func BenchmarkAheadBehindUncached(b *testing.B) {
	repo := testutil.Repo(b)
	b.Setenv("CLAUDE_CONFIG_DIR", b.TempDir()) // never touch the real cache

	r, err := git.PlainOpenWithOptions(repo, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		b.Skipf("cannot open %s: %v", repo, err)
	}
	head, err := r.Head()
	if err != nil || !head.Name().IsBranch() {
		b.Skipf("no branch HEAD: %v", err)
	}
	branch := head.Name().Short()

	ctx := context.Background()
	if _, _, err := aheadBehind(ctx, r, branch, head.Hash()); err != nil {
		b.Skipf("no upstream to compare against: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		os.Remove(paths.GitCache()) // force the walk, which is the point
		b.StartTimer()

		if _, _, err := aheadBehind(ctx, r, branch, head.Hash()); err != nil {
			b.Fatal(err)
		}
	}
}
