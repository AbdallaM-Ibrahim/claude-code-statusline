package term

import "strings"

// Basename splits on both separators so a Windows path and a POSIX path behave
// the same.
func Basename(p string) string {
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return p
	}
	return parts[len(parts)-1]
}

// Truncate counts runes, not bytes, so a multi-byte commit subject cannot be cut
// mid-character.
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
