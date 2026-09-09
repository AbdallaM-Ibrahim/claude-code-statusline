package caveman

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/term"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/testutil"
)

// withConfigDir points CLAUDE_CONFIG_DIR at a scratch directory so these tests
// never touch the real flag.
func withConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	return dir
}

func writeFlag(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAbsentFlagRendersNothing(t *testing.T) {
	withConfigDir(t)
	if got := Segment(); got != "" {
		t.Errorf("no flag should render nothing, got %q", got)
	}
}

func TestModes(t *testing.T) {
	cases := map[string]string{
		"full":   "[CAVEMAN]",
		"lite":   "[CAVEMAN:LITE]",
		"ultra":  "[CAVEMAN:ULTRA]",
		"wenyan": "[CAVEMAN:WENYAN]",
		"off":    "[CAVEMAN:OFF]",
	}
	for mode, want := range cases {
		dir := withConfigDir(t)
		writeFlag(t, dir, ".caveman-active", mode)
		if got := Segment(); got != term.Orange(want) {
			t.Errorf("mode %q rendered %q, want %q", mode, got, term.Orange(want))
		}
	}
}

func TestRejectsUnknownMode(t *testing.T) {
	dir := withConfigDir(t)
	writeFlag(t, dir, ".caveman-active", "bogus")
	if got := Segment(); got != "" {
		t.Errorf("unknown mode should render nothing, got %q", got)
	}
}

// The flag's bytes reach the terminal, so injected escape sequences must be
// stripped before the whitelist check — which then rejects the whole thing.
func TestStripsEscapeInjection(t *testing.T) {
	dir := withConfigDir(t)
	writeFlag(t, dir, ".caveman-active", "full\x1b[31mEVIL")
	got := Segment()
	if strings.Contains(got, "EVIL") || strings.Contains(got, "\x1b[31m") {
		t.Errorf("escape injection survived: %q", got)
	}
	// "full\x1b[31mEVIL" normalises to "fullmevil", which is not whitelisted.
	if got != "" {
		t.Errorf("expected rejection, got %q", got)
	}
}

// The flag is read from disk every render, so its whitelist has to hold even
// when the file is hostile rather than merely stale — and the suffix, which is
// free text, must come out sanitised.
func TestSuffixIsSanitised(t *testing.T) {
	dir := withConfigDir(t)

	writeFlag(t, dir, ".caveman-active", "full\x1b[2J")
	// "full" plus injected bytes is not a whitelisted mode, so nothing renders.
	if got := Segment(); got != "" {
		t.Errorf("Segment = %q, want empty for a non-whitelisted mode", got)
	}

	writeFlag(t, dir, ".caveman-active", "full")
	writeFlag(t, dir, ".caveman-statusline-suffix", "· 42%\x1b[2J")
	got := Segment()
	testutil.AssertNoInjection(t, got)
	if !strings.Contains(got, "42%") {
		t.Errorf("legitimate suffix text was dropped: %q", got)
	}
}

func TestRejectsOversizeFile(t *testing.T) {
	dir := withConfigDir(t)
	writeFlag(t, dir, ".caveman-active", strings.Repeat("a", 65))
	if got := Segment(); got != "" {
		t.Errorf("oversize flag should render nothing, got %q", got)
	}
}

// A symlink is refused outright: the size cap alone would still put 64 bytes of
// somebody else's file on screen.
func TestRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Creating a symlink on Windows needs elevation or developer mode.
		t.Skip("symlink creation requires privilege on Windows")
	}
	dir := withConfigDir(t)
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("full"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, paths.CavemanFlag()); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if got := Segment(); got != "" {
		t.Errorf("symlinked flag should render nothing, got %q", got)
	}
}

func TestStripsBOM(t *testing.T) {
	dir := withConfigDir(t)
	writeFlag(t, dir, ".caveman-active", "\ufefffull")
	if got := Segment(); got != term.Orange("[CAVEMAN]") {
		t.Errorf("BOM-prefixed flag rendered %q", got)
	}
}

func TestSavingsSuffix(t *testing.T) {
	dir := withConfigDir(t)
	writeFlag(t, dir, ".caveman-active", "full")
	writeFlag(t, dir, ".caveman-statusline-suffix", "\ufeffsaved 12k")

	if got := Segment(); got != term.Orange("[CAVEMAN] saved 12k") {
		t.Errorf("suffix rendered %q, want %q", got, term.Orange("[CAVEMAN] saved 12k"))
	}

	t.Setenv("CAVEMAN_STATUSLINE_SAVINGS", "0")
	if got := Segment(); got != term.Orange("[CAVEMAN]") {
		t.Errorf("opt-out ignored, got %q", got)
	}
}

func TestUsesFirstLineOnly(t *testing.T) {
	dir := withConfigDir(t)
	writeFlag(t, dir, ".caveman-active", "lite\nignored")
	if got := Segment(); got != term.Orange("[CAVEMAN:LITE]") {
		t.Errorf("multi-line flag rendered %q", got)
	}
}

// BenchmarkCavemanSegment is two small stat-and-read calls against a fixture.
func BenchmarkCavemanSegment(b *testing.B) {
	dir := b.TempDir()
	b.Setenv("CLAUDE_CONFIG_DIR", dir)
	if err := os.WriteFile(paths.CavemanFlag(), []byte("full"), 0o600); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(paths.CavemanSuffix(), []byte("· 42% saved"), 0o600); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Segment()
	}
}
