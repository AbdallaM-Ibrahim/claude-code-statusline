// Package caveman renders the indicator for the caveman plugin
// (https://github.com/JuliusBrussee/caveman), gated on the plugin's activation
// flag under the Claude config directory.
//
// The flag's bytes reach the terminal on every render, so the reads are
// deliberately paranoid: size-capped, symlink-refusing, whitelisted.
package caveman

import (
	"io"
	"os"
	"strings"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/term"
)

// modes is the whitelist. Anything else renders nothing at all.
var modes = map[string]bool{
	"off": true, "lite": true, "full": true, "ultra": true,
	"wenyan-lite": true, "wenyan": true, "wenyan-full": true, "wenyan-ultra": true,
	"commit": true, "review": true, "compress": true,
}

// maxBytes caps both state files. These bytes are written to the terminal on
// every render, so the size limit is a security control, not tidiness.
const maxBytes = 64

// readFile returns the contents of a small caveman state file, or "" and false
// when it is absent, a symlink or reparse point, not a regular file, or over
// the size cap.
//
// The symlink and size guards matter: without them, swapping the flag for a link
// to something secret would render that file's bytes — including any escape
// sequences it contains — to the terminal on every keystroke. Ported from the
// plugin's own hook, which hardens the same way.
// The checks are deliberately split across a path and a handle. Lstat rejects a
// link without following it, which os.Open cannot do portably; SameFile then
// confirms the handle we opened is the file Lstat approved, so substituting a
// link for the flag between the two calls fails the second check instead of
// quietly reading the link's target.
func readFile(path string) (string, bool) {
	li, err := os.Lstat(path)
	if err != nil {
		return "", false
	}
	if li.Mode()&os.ModeSymlink != 0 || !li.Mode().IsRegular() || li.Size() > maxBytes {
		return "", false
	}

	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxBytes || !os.SameFile(li, fi) {
		return "", false
	}

	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return "", false
	}
	// PowerShell's `Set-Content -Encoding utf8` writes a BOM, which would render
	// as a stray glyph.
	return strings.TrimPrefix(string(data), "\ufeff"), true
}

// keepModeChars strips everything outside [a-z0-9-], which blocks terminal
// escape and OSC hyperlink injection through the flag's contents.
func keepModeChars(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Segment renders "[CAVEMAN]" or "[CAVEMAN:LITE]", gated on the plugin's
// activation flag. Returns "" when the plugin is not active.
func Segment() string {
	raw, ok := readFile(paths.CavemanFlag())
	if !ok {
		return ""
	}

	// First line only, then normalise and whitelist.
	first := raw
	if i := strings.IndexAny(first, "\r\n"); i >= 0 {
		first = first[:i]
	}
	mode := keepModeChars(strings.ToLower(strings.TrimSpace(first)))
	if !modes[mode] {
		return ""
	}

	out := "[CAVEMAN]"
	if mode != "full" {
		out = "[CAVEMAN:" + strings.ToUpper(mode) + "]"
	}

	// Savings suffix, written by /caveman-stats. Absent until that has run once.
	if os.Getenv("CAVEMAN_STATUSLINE_SAVINGS") != "0" {
		if suffixRaw, ok := readFile(paths.CavemanSuffix()); ok {
			if suffix := strings.TrimSpace(term.Sanitize(suffixRaw)); suffix != "" {
				out += " " + suffix
			}
		}
	}

	return term.Orange(out)
}
