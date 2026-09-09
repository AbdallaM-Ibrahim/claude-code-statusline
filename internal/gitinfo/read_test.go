package gitinfo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

func TestReadRealRepo(t *testing.T) {
	repo := testutil.Repo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	g, err := Read(ctx, repo)
	if err != nil {
		t.Fatalf("Read: %v", err)
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
	if g.When.IsZero() {
		t.Error("expected a commit time")
	}
	t.Logf("branch=%q ahead=%d behind=%d hash=%s when=%s subject=%q",
		g.Branch, g.Ahead, g.Behind, g.Hash, g.When, g.Subject)
}

func TestReadNonRepoFails(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := Read(ctx, dir); err == nil {
		t.Error("expected an error for a directory that is not a repository")
	}
}

// A cancelled context must not hang or panic; it degrades to partial data.
func TestReadRespectsCancelledContext(t *testing.T) {
	repo := testutil.Repo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g, err := Read(ctx, repo)
	if err != nil {
		t.Skipf("repo could not be opened at all: %v", err)
	}
	if g.Hash == "" {
		t.Error("hash should still be available even with a dead context")
	}
}

// The cache maps local repository paths to commit hashes. Nothing else on the
// box needs to read it.
func TestCacheFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a synthetic mode; ACLs are not what Go writes here")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	loadCache().save()

	fi, err := os.Stat(paths.GitCache())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("%s mode = %04o, want 0600", filepath.Base(paths.GitCache()), perm)
	}
}
