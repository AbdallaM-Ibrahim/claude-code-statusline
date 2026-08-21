package main

import (
	"strings"
	"unicode/utf8"
)

// Untrusted text reaches the terminal from three directions: git metadata in a
// repository we did not create, the Claude Code payload, and small state files
// under ~/.claude. A terminal executes escape sequences rather than printing
// them, and the status line re-renders every few seconds — so one hostile commit
// subject repaints the screen for as long as that directory stays open, with no
// obvious culprit. The caveman flag was already hardened against exactly this;
// git strings were not.
//
// safeTerminal is the single choke point. Everything that reaches either line
// passes through it before it is coloured or truncated — sanitising first also
// keeps truncate's rune budget honest and stops a cut landing mid-sequence.

// safeTerminal drops every rune a terminal reads as a command rather than as
// text:
//
//	C0 0x00-0x1F except tab       ESC introduces CSI and OSC — the injection vector
//	DEL 0x7F                      erases already-rendered output on some terminals
//	C1 0x80-0x9F                  0x9B is single-byte CSI, 0x9D single-byte OSC
//	U+202A-U+202E, U+2066-U+2069  bidi overrides: text that renders as something else
//
// Everything else survives, so a commit subject keeps its emoji and its accents.
// Invalid UTF-8 is replaced rather than passed through: a lone 0x9B byte is not
// valid UTF-8, and forwarding it raw would hand the terminal a CSI.
func safeTerminal(s string) string {
	if !needsSanitising(s) {
		return s // by far the common case: no copy, no allocation
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToValidUTF8(s, "�") {
		if unsafeRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// needsSanitising decides whether safeTerminal has to build a new string.
// Invalid encoding counts: it is how a raw C1 byte would otherwise slip past.
func needsSanitising(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if size == 1 && r == utf8.RuneError {
			return true // invalid byte, not a real U+FFFD
		}
		if unsafeRune(r) {
			return true
		}
		i += size
	}
	return false
}

func unsafeRune(r rune) bool {
	switch {
	case r == '\t':
		return false
	case r < 0x20 || r == 0x7F:
		return true
	case r >= 0x80 && r <= 0x9F:
		return true
	case r >= 0x202A && r <= 0x202E: // LRE RLE PDF LRO RLO
		return true
	case r >= 0x2066 && r <= 0x2069: // LRI RLI FSI PDI
		return true
	default:
		return false
	}
}
