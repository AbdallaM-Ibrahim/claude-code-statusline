package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testRepoPath resolves the repository the git tests and benchmarks read:
//
//	STATUSLINE_TEST_REPO   an explicit choice — point it at a large history to
//	                       make the ahead/behind benchmark say something
//	this module's own repo  every clone of this project is a git repository, so a
//	                       fresh checkout needs no setup at all
//	""                     nothing usable, and the caller skips
//
// This used to be a const naming a directory on the author's machine, which
// meant every git test silently skipped for everybody else.
func testRepoPath() string {
	if p := os.Getenv("STATUSLINE_TEST_REPO"); p != "" {
		if isGitRepo(p) {
			return p
		}
		return ""
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := wd; ; {
		if isGitRepo(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isGitRepo accepts both a .git directory and the .git file a worktree gets.
func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// repoForTest is testRepoPath with the skip attached, for tests and benchmarks
// alike.
func repoForTest(tb testing.TB) string {
	tb.Helper()
	p := testRepoPath()
	if p == "" {
		tb.Skip("no git repository found; set STATUSLINE_TEST_REPO to one")
	}
	return p
}

func TestReadGitRealRepo(t *testing.T) {
	repo := repoForTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	g, err := readGit(ctx, repo)
	if err != nil {
		t.Fatalf("readGit: %v", err)
	}
	if g.Branch == "" && !g.Detached {
		t.Error("expected a branch or a detached flag")
	}
	if len(g.Hash) != 7 {
		t.Errorf("short hash = %q, want 7 chars", g.Hash)
	}
	if g.Subject == "" {
		t.Error("expected a commit subject")
	}
	if g.Age == "" {
		t.Error("expected a formatted age")
	}
	t.Logf("branch=%q ahead=%d behind=%d hash=%s age=%s subject=%q",
		g.Branch, g.Ahead, g.Behind, g.Hash, g.Age, g.Subject)
}

func TestReadGitNonRepoFails(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := readGit(ctx, dir); err == nil {
		t.Error("expected an error for a directory that is not a repository")
	}
}

// A cancelled context must not hang or panic; it degrades to partial data.
func TestReadGitRespectsCancelledContext(t *testing.T) {
	repo := repoForTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g, err := readGit(ctx, repo)
	if err != nil {
		t.Skipf("repo could not be opened at all: %v", err)
	}
	if g.Hash == "" {
		t.Error("hash should still be available even with a dead context")
	}
}

func TestBuildPlaceLineNonRepoShowsDirectoryOnly(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	in := &StatusLineInput{}

	got := buildPlaceLine(ctx, in, dir)
	want := cyan(basename(dir))
	if got != want {
		t.Errorf("non-repo place line = %q, want %q", got, want)
	}
}

func TestBuildPlaceLineAppendsPayloadBadges(t *testing.T) {
	dir := t.TempDir()
	in := &StatusLineInput{}
	in.Worktree.Name = "wt"
	in.PR.Number = 12
	in.Agent.Name = "explore"

	got := buildPlaceLine(context.Background(), in, dir)
	for _, want := range []string{blue("⑂ wt"), blue("PR #12"), blue("@explore")} {
		if !contains(got, want) {
			t.Errorf("place line %q missing %q", got, want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		})()
}
