package statusline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/gitinfo"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/payload"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/term"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

func TestBuildPlaceLineNonRepoShowsDirectoryOnly(t *testing.T) {
	dir := t.TempDir()
	in := &payload.Input{}

	got := buildPlaceLine(context.Background(), in, dir)
	want := term.Cyan(term.Basename(dir))
	if got != want {
		t.Errorf("non-repo place line = %q, want %q", got, want)
	}
}

func TestBuildPlaceLineAppendsPayloadBadges(t *testing.T) {
	dir := t.TempDir()
	in := &payload.Input{}
	in.Worktree.Name = "wt"
	in.PR.Number = 12
	in.Agent.Name = "explore"

	got := buildPlaceLine(context.Background(), in, dir)
	for _, want := range []string{term.Blue("⑂ wt"), term.Blue("PR #12"), term.Blue("@explore")} {
		if !strings.Contains(got, want) {
			t.Errorf("place line %q missing %q", got, want)
		}
	}
}

func TestPlaceLineRendersEveryGitSegment(t *testing.T) {
	g := &gitinfo.State{
		Branch:  "main",
		Ahead:   2,
		Behind:  72,
		Hash:    "be66d0f",
		When:    time.Now().Add(-14 * 24 * time.Hour),
		Subject: "first commit",
	}
	got := testutil.StripANSI(placeLine(g, &payload.Input{}, "/work/site"))
	want := "site ⟨main ↑2 ↓72⟩ be66d0f 2w ago · first commit"
	if got != want {
		t.Errorf("place line = %q, want %q", got, want)
	}
}

func TestPlaceLineDetachedHead(t *testing.T) {
	g := &gitinfo.State{Hash: "be66d0f", Detached: true}
	got := testutil.StripANSI(placeLine(g, &payload.Input{}, "/work/site"))
	if got != "site ⟨detached⟩ be66d0f" {
		t.Errorf("detached place line = %q", got)
	}
}

// A repository we did not author supplies the branch name and the commit
// subject, and the status line repaints them every few seconds.
func TestPlaceLineNeutralisesHostileGitMetadata(t *testing.T) {
	g := &gitinfo.State{
		Branch:  "ma\x1b[2Jin",
		Hash:    "be66d0f",
		When:    time.Now().Add(-14 * 24 * time.Hour),
		Subject: "docs: tidy\x1b]8;;http://evil\x07 up\u202e",
	}
	in := &payload.Input{}
	in.Worktree.Name = "wt\x1b[31m"
	in.Agent.Name = "explore\u009b0m"

	got := placeLine(g, in, "/tmp/re\x1bpo")

	testutil.AssertNoInjection(t, got)

	// Sanitising is not censoring: the printable text stays, only the control
	// runes go. A branch of "ma\x1b[2Jin" is therefore shown as "ma[2Jin".
	for _, want := range []string{"ma[2Jin", "be66d0f", "docs: tidy", "wt[31m", "@explore0m"} {
		if !strings.Contains(got, want) {
			t.Errorf("place line %q lost %q", got, want)
		}
	}
}

// Sanitising has to happen before truncation: strip second and a 40-rune cut can
// land inside an escape sequence, which is how a filter gets bypassed.
func TestSubjectIsSanitisedBeforeTruncation(t *testing.T) {
	g := &gitinfo.State{Subject: "\x1b[31m" + strings.Repeat("a", subjectMax+10)}
	got := testutil.StripANSI(placeLine(g, &payload.Input{}, "/work/site"))

	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("escape survived truncation: %q", got)
	}
	subject := got[strings.Index(got, "· ")+len("· "):]
	if runes := len([]rune(subject)); runes > subjectMax {
		t.Errorf("truncated subject is %d runes, want <= %d", runes, subjectMax)
	}
}
