package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The threat: a repository we did not author supplies the branch name and the
// commit subject, and the status line repaints them every few seconds. An escape
// sequence there is executed by the terminal, not printed, and the user has no
// obvious way to tell where it came from.

func TestSafeTerminalRemovesTerminalCommands(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"csi clear screen", "fix\x1b[2Jthing", "fix[2Jthing"},
		{"osc hyperlink", "fix\x1b]8;;http://evil\x07click", "fix]8;;http://evilclick"},
		{"del", "fi\x7fx", "fix"},
		{"c1 csi as utf-8", "fix\u009b2Jthing", "fix2Jthing"},
		{"c1 osc as utf-8", "fix\u009d0;title\u009c", "fix0;title"},
		{"bidi override", "fix \u202eyranib/nur", "fix yranib/nur"},
		{"bidi isolate", "fix \u2066a\u2069b", "fix ab"},
		{"newline cannot split the line", "subject\nfake second line", "subjectfake second line"},
		{"carriage return cannot rewind", "real\rfake", "realfake"},
		{"tab survives", "a\tb", "a\tb"},
		{"text survives", "feat: café — 🚀 ship it", "feat: café — 🚀 ship it"},
		{"empty", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := safeTerminal(c.in)
			if got != c.want {
				t.Errorf("safeTerminal(%q) = %q, want %q", c.in, got, c.want)
			}
			for _, r := range got {
				if unsafeRune(r) {
					t.Errorf("output %q still carries %U", got, r)
				}
			}
		})
	}
}

// A lone 0x9B byte is not valid UTF-8, so ranging over the string yields
// RuneError and a naive filter would pass the raw byte straight through — where
// a UTF-8 terminal reads it as CSI.
func TestSafeTerminalReplacesInvalidUTF8(t *testing.T) {
	in := string([]byte{'a', 0x9b, '2', 'J', 'b'})
	got := safeTerminal(in)

	if strings.Contains(got, string([]byte{0x9b})) {
		t.Errorf("raw C1 byte survived: %q", got)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Errorf("surrounding text was lost: %q", got)
	}
}

// Sanitising has to happen before truncation: strip second and a 40-rune cut can
// land inside an escape sequence, which is how a filter gets bypassed.
func TestSubjectIsSanitisedBeforeTruncation(t *testing.T) {
	long := "\x1b[31m" + strings.Repeat("a", subjectMax+10)
	got := truncate(safeTerminal(long), subjectMax)

	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("escape survived truncation: %q", got)
	}
	if runes := len([]rune(got)); runes > subjectMax {
		t.Errorf("truncated subject is %d runes, want <= %d", runes, subjectMax)
	}
}

// ourColours matches exactly what paint() emits — an SGR sequence and nothing
// else. Stripping it leaves the escapes we did not write, which is the only way
// to assert on injection without tripping over our own formatting. Note what it
// deliberately does not match: CSI sequences ending in anything but 'm' (a
// screen-clearing "\x1b[2J") and OSC sequences, both of which stay visible to
// the assertions below.
var ourColours = regexp.MustCompile("\x1b\\[[0-9;]*m")

func assertNoInjection(t *testing.T, rendered string) {
	t.Helper()
	payload := ourColours.ReplaceAllString(rendered, "")
	for _, bad := range []rune{0x1b, 0x07, 0x9b, 0x9d, 0x202e} {
		if strings.ContainsRune(payload, bad) {
			t.Errorf("rendered line carries %U outside our own colour codes: %q", bad, rendered)
		}
	}
}

func TestPlaceLineNeutralisesHostileGitMetadata(t *testing.T) {
	g := &GitState{
		Branch:  "ma\x1b[2Jin",
		Hash:    "be66d0f",
		Age:     "2w ago",
		Subject: "docs: tidy\x1b]8;;http://evil\x07 up\u202e",
	}
	in := &StatusLineInput{}
	in.Worktree.Name = "wt\x1b[31m"
	in.Agent.Name = "explore\u009b0m"

	got := placeLine(g, in, "/tmp/re\x1bpo")

	assertNoInjection(t, got)

	// Sanitising is not censoring: the printable text stays, only the control
	// runes go. A branch of "ma\x1b[2Jin" is therefore shown as "ma[2Jin".
	for _, want := range []string{"ma[2Jin", "be66d0f", "docs: tidy", "wt[31m", "@explore0m"} {
		if !strings.Contains(got, want) {
			t.Errorf("place line %q lost %q", got, want)
		}
	}
}

func TestSessionLineNeutralisesHostilePayloadStrings(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir()) // do not read the real caveman flag

	in := &StatusLineInput{}
	in.Model.DisplayName = "Opus 5\x1b[2J"
	in.Effort.Level = "xhigh\u009b0m"

	got := buildSessionLine(in, "", nil)

	assertNoInjection(t, got)
	if !strings.Contains(got, "Opus 5") || !strings.Contains(got, "xhigh") {
		t.Errorf("session line %q lost its text", got)
	}
}

// The caveman flag is read from disk every render, so its whitelist has to hold
// even when the file is hostile rather than merely stale.
func TestCavemanFlagRejectsInjection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	if err := os.WriteFile(cavemanFlagPath(), []byte("full\x1b[2J"), 0o600); err != nil {
		t.Fatal(err)
	}
	// "full" plus injected bytes is not a whitelisted mode, so nothing renders.
	if got := cavemanSegment(); got != "" {
		t.Errorf("cavemanSegment = %q, want empty for a non-whitelisted mode", got)
	}

	if err := os.WriteFile(cavemanFlagPath(), []byte("full"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cavemanSuffixPath(), []byte("· 42%\x1b[2J"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := cavemanSegment()
	assertNoInjection(t, got)
	if !strings.Contains(got, "42%") {
		t.Errorf("legitimate suffix text was dropped: %q", got)
	}
}

// A symlink is refused outright: the size cap alone would still put 64 bytes of
// somebody else's file on screen.
func TestCavemanFlagRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("full"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, cavemanFlagPath()); err != nil {
		t.Skipf("symlinks unavailable (Windows needs privilege): %v", err)
	}

	if got := cavemanSegment(); got != "" {
		t.Errorf("cavemanSegment = %q, want empty — a symlinked flag must be refused", got)
	}
}

// A single line larger than the cap must not be buffered, must not be parsed,
// and must still be stepped over — otherwise the scan reads it again forever.
func TestConsumeSkipsOversizeLineAndKeepsGoing(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	path := filepath.Join(root, "p", "a.jsonl")

	writeTranscript(t, path, []string{
		strings.Repeat("x", maxLineBytes+1024),
		entry("m1", "r1", "claude-opus-5", now.Add(-10*time.Minute), 1_000_000, 0, 0, 0, 0),
	})

	st := newCostState()
	st.scan(root, now)

	rep := &CostReport{}
	aggregate(st, rep, now)
	if rep.TodayUSD != 5.0 {
		t.Errorf("today = %v, want 5.00 — the entry after the oversize line was lost", rep.TodayUSD)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if cur := st.Files[path].Cursor; cur != fi.Size() {
		t.Errorf("cursor = %d, want %d: an oversize line must be stepped over, not re-read",
			cur, fi.Size())
	}
}

// These files list every project path on the machine and the id of every API
// response seen recently. Nothing else on the box needs to read them.
func TestStateFilesAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a synthetic mode; ACLs are not what Go writes here")
	}
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	newCostState().save(costStatePath())
	loadAheadBehindCache().save()

	for _, path := range []string{costStatePath(), gitCachePath()} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s mode = %04o, want 0600", filepath.Base(path), perm)
		}
	}
}
