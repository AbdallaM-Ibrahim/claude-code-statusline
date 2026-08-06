package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withCavemanDir points CLAUDE_CONFIG_DIR at a scratch directory so these tests
// never touch the real flag.
func withCavemanDir(t *testing.T) string {
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

func TestCavemanAbsentFlagRendersNothing(t *testing.T) {
	withCavemanDir(t)
	if got := cavemanSegment(); got != "" {
		t.Errorf("no flag should render nothing, got %q", got)
	}
}

func TestCavemanModes(t *testing.T) {
	cases := map[string]string{
		"full":   "[CAVEMAN]",
		"lite":   "[CAVEMAN:LITE]",
		"ultra":  "[CAVEMAN:ULTRA]",
		"wenyan": "[CAVEMAN:WENYAN]",
		"off":    "[CAVEMAN:OFF]",
	}
	for mode, want := range cases {
		dir := withCavemanDir(t)
		writeFlag(t, dir, ".caveman-active", mode)
		if got := cavemanSegment(); got != orange(want) {
			t.Errorf("mode %q rendered %q, want %q", mode, got, orange(want))
		}
	}
}

func TestCavemanRejectsUnknownMode(t *testing.T) {
	dir := withCavemanDir(t)
	writeFlag(t, dir, ".caveman-active", "bogus")
	if got := cavemanSegment(); got != "" {
		t.Errorf("unknown mode should render nothing, got %q", got)
	}
}

// The flag's bytes reach the terminal, so injected escape sequences must be
// stripped before the whitelist check — which then rejects the whole thing.
func TestCavemanStripsEscapeInjection(t *testing.T) {
	dir := withCavemanDir(t)
	writeFlag(t, dir, ".caveman-active", "full\x1b[31mEVIL")
	got := cavemanSegment()
	if strings.Contains(got, "EVIL") || strings.Contains(got, "\x1b[31m") {
		t.Errorf("escape injection survived: %q", got)
	}
	// "full\x1b[31mEVIL" normalises to "fullmevil", which is not whitelisted.
	if got != "" {
		t.Errorf("expected rejection, got %q", got)
	}
}

func TestCavemanRejectsOversizeFile(t *testing.T) {
	dir := withCavemanDir(t)
	writeFlag(t, dir, ".caveman-active", strings.Repeat("a", 65))
	if got := cavemanSegment(); got != "" {
		t.Errorf("oversize flag should render nothing, got %q", got)
	}
}

func TestCavemanRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Creating a symlink on Windows needs elevation or developer mode.
		t.Skip("symlink creation requires privilege on Windows")
	}
	dir := withCavemanDir(t)
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("full"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, ".caveman-active")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if got := cavemanSegment(); got != "" {
		t.Errorf("symlinked flag should render nothing, got %q", got)
	}
}

func TestCavemanStripsBOM(t *testing.T) {
	dir := withCavemanDir(t)
	writeFlag(t, dir, ".caveman-active", "\ufefffull")
	if got := cavemanSegment(); got != orange("[CAVEMAN]") {
		t.Errorf("BOM-prefixed flag rendered %q", got)
	}
}

func TestCavemanSavingsSuffix(t *testing.T) {
	dir := withCavemanDir(t)
	writeFlag(t, dir, ".caveman-active", "full")
	writeFlag(t, dir, ".caveman-statusline-suffix", "\ufeffsaved 12k")

	if got := cavemanSegment(); got != orange("[CAVEMAN] saved 12k") {
		t.Errorf("suffix rendered %q, want %q", got, orange("[CAVEMAN] saved 12k"))
	}

	t.Setenv("CAVEMAN_STATUSLINE_SAVINGS", "0")
	if got := cavemanSegment(); got != orange("[CAVEMAN]") {
		t.Errorf("opt-out ignored, got %q", got)
	}
}

func TestCavemanUsesFirstLineOnly(t *testing.T) {
	dir := withCavemanDir(t)
	writeFlag(t, dir, ".caveman-active", "lite\nignored")
	if got := cavemanSegment(); got != orange("[CAVEMAN:LITE]") {
		t.Errorf("multi-line flag rendered %q", got)
	}
}
