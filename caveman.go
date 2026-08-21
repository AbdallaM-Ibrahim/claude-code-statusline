package main

import (
	"io"
	"os"
	"strings"
)

// cavemanModes is the whitelist. Anything else renders nothing at all.
var cavemanModes = map[string]bool{
	"off": true, "lite": true, "full": true, "ultra": true,
	"wenyan-lite": true, "wenyan": true, "wenyan-full": true, "wenyan-ultra": true,
	"commit": true, "review": true, "compress": true,
}

// cavemanMaxBytes caps both state files. These bytes are written to the terminal
// on every render, so the size limit is a security control, not tidiness.
const cavemanMaxBytes = 64

// readCavemanFile returns the contents of a small caveman state file, or "" and
// false when it is absent, a symlink or reparse point, not a regular file, or
// over the size cap.
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
func readCavemanFile(path string) (string, bool) {
	li, err := os.Lstat(path)
	if err != nil {
		return "", false
	}
	if li.Mode()&os.ModeSymlink != 0 || !li.Mode().IsRegular() || li.Size() > cavemanMaxBytes {
		return "", false
	}

	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > cavemanMaxBytes || !os.SameFile(li, fi) {
		return "", false
	}

	data, err := io.ReadAll(io.LimitReader(f, cavemanMaxBytes))
	if err != nil {
		return "", false
	}
	// PowerShell's `Set-Content -Encoding utf8` writes a BOM, which would render
	// as a stray glyph.
	return strings.TrimPrefix(string(data), "\ufeff"), true
}

// keepCavemanChars strips everything outside [a-z0-9-], which blocks terminal
// escape and OSC hyperlink injection through the flag's contents.
func keepCavemanChars(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cavemanSegment renders "[CAVEMAN]" or "[CAVEMAN:LITE]", gated on the plugin's
// activation flag. Returns "" when the plugin is not active.
func cavemanSegment() string {
	raw, ok := readCavemanFile(cavemanFlagPath())
	if !ok {
		return ""
	}

	// First line only, then normalise and whitelist.
	first := raw
	if i := strings.IndexAny(first, "\r\n"); i >= 0 {
		first = first[:i]
	}
	mode := keepCavemanChars(strings.ToLower(strings.TrimSpace(first)))
	if !cavemanModes[mode] {
		return ""
	}

	out := "[CAVEMAN]"
	if mode != "full" {
		out = "[CAVEMAN:" + strings.ToUpper(mode) + "]"
	}

	// Savings suffix, written by /caveman-stats. Absent until that has run once.
	if os.Getenv("CAVEMAN_STATUSLINE_SAVINGS") != "0" {
		if suffixRaw, ok := readCavemanFile(cavemanSuffixPath()); ok {
			if suffix := strings.TrimSpace(safeTerminal(suffixRaw)); suffix != "" {
				out += " " + suffix
			}
		}
	}

	return orange(out)
}
