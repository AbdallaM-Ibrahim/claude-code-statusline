// Package term holds the rendering primitives both status lines share: ANSI
// colours, the terminal-safety filter every untrusted string passes through,
// and the compact formats for ages, clocks and truncated text.
//
// It knows nothing about git, payloads or money. Anything that turns a value
// into bytes destined for the terminal lives here so the rules are applied
// identically on both lines.
package term

// Colour codes are reproduced exactly from the TypeScript version this replaces:
// line 1 must stay byte-for-byte identical so the two can be diffed during
// rollout.

const reset = "\x1b[0m"

func paint(code, s string) string { return "\x1b[" + code + "m" + s + reset }

// Dim and the colour helpers wrap s in one SGR sequence and a reset.
func Dim(s string) string     { return paint("2", s) }
func Cyan(s string) string    { return paint("1;36", s) }
func Magenta(s string) string { return paint("35", s) }
func Yellow(s string) string  { return paint("33", s) }
func Green(s string) string   { return paint("32", s) }
func Red(s string) string     { return paint("31", s) }
func Blue(s string) string    { return paint("34", s) }
func Orange(s string) string  { return paint("38;5;172", s) }

// Heat is the shared threshold for every percentage on the line: context usage
// and both rate-limit windows.
func Heat(pct int, s string) string {
	switch {
	case pct >= 75:
		return Red(s)
	case pct >= 50:
		return Yellow(s)
	default:
		return Green(s)
	}
}
