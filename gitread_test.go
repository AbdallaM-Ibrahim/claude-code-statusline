package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// The repository the status line is normally pointed at. Skipped when absent so
// the suite still runs on another machine.
const testRepo = "C:/Users/abdo/workspace/software-engineer-website"

func skipWithoutRepo(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(testRepo + "/.git"); err != nil {
		t.Skipf("test repo not present: %v", err)
	}
}

func TestReadGitRealRepo(t *testing.T) {
	skipWithoutRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	g, err := readGit(ctx, testRepo)
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
	skipWithoutRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g, err := readGit(ctx, testRepo)
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
