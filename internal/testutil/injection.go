package testutil

import (
	"regexp"
	"strings"
	"testing"
)

// ourColours matches exactly what term.paint emits — an SGR sequence and nothing
// else. Stripping it leaves the escapes we did not write, which is the only way
// to assert on injection without tripping over our own formatting. Note what it
// deliberately does not match: CSI sequences ending in anything but 'm' (a
// screen-clearing "\x1b[2J") and OSC sequences, both of which stay visible to
// the assertion.
var ourColours = regexp.MustCompile("\x1b\\[[0-9;]*m")

// StripANSI removes the SGR colour codes this program writes itself.
func StripANSI(s string) string { return ourColours.ReplaceAllString(s, "") }

// AssertNoInjection fails the test if rendered carries any terminal command
// outside the program's own colour codes.
func AssertNoInjection(t *testing.T, rendered string) {
	t.Helper()
	payload := StripANSI(rendered)
	for _, bad := range []rune{0x1b, 0x07, 0x9b, 0x9d, 0x202e} {
		if strings.ContainsRune(payload, bad) {
			t.Errorf("rendered line carries %U outside our own colour codes: %q", bad, rendered)
		}
	}
}
