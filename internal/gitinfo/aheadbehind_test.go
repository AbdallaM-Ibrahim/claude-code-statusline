package gitinfo

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

// The counts must agree with git itself. An earlier revision derived them from a
// deadline-truncated commit walk and reported "↑1 ↓64" where git reported
// "+0 -72", so this pins the behaviour.
func TestAheadBehindMatchesGit(t *testing.T) {
	repo := testutil.Repo(t)

	out, err := exec.Command("git", "-C", repo, "status", "--porcelain=v2", "--branch", "--untracked-files=no").Output()
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	var wantAhead, wantBehind int
	found := false
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "# branch.ab ") {
			if _, err := fmtSscan(strings.TrimPrefix(line, "# branch.ab "), &wantAhead, &wantBehind); err == nil {
				found = true
			}
		}
	}
	if !found {
		t.Skip("branch has no upstream to compare against")
	}

	// A generous deadline: this test is about correctness, not the timeout path.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	g, err := Read(ctx, repo)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if g.Ahead != wantAhead || g.Behind != wantBehind {
		t.Errorf("ahead/behind = %d/%d, git says %d/%d", g.Ahead, g.Behind, wantAhead, wantBehind)
	}
}

// With an already-dead context the walk cannot complete, so no counts may be
// reported — zeroes here mean "not shown", not "in sync".
func TestAheadBehindOmittedWhenWalkTruncated(t *testing.T) {
	repo := testutil.Repo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g, err := Read(ctx, repo)
	if err != nil {
		t.Skipf("repo could not be opened: %v", err)
	}
	if g.Ahead != 0 || g.Behind != 0 {
		t.Errorf("a truncated walk must not report counts, got ↑%d ↓%d", g.Ahead, g.Behind)
	}
}

// fmtSscan parses "+N -M" without pulling fmt's scanning into the hot path.
func fmtSscan(s string, ahead, behind *int) (int, error) {
	var a, b int
	n, err := sscanAheadBehind(s, &a, &b)
	if err == nil {
		*ahead, *behind = a, b
	}
	return n, err
}

func sscanAheadBehind(s string, a, b *int) (int, error) {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0, errWalkTruncated
	}
	var err error
	if *a, err = atoiSigned(fields[0]); err != nil {
		return 0, err
	}
	if *b, err = atoiSigned(fields[1]); err != nil {
		return 0, err
	}
	return 2, nil
}

func atoiSigned(s string) (int, error) {
	neg := false
	switch {
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	case strings.HasPrefix(s, "-"):
		s, neg = s[1:], true
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errWalkTruncated
		}
		n = n*10 + int(r-'0')
	}
	_ = neg // git reports behind as "-N"; the magnitude is what we compare
	return n, nil
}
