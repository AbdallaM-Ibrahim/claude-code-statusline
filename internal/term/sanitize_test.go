package term

import (
	"strings"
	"testing"
)

// The threat: a repository we did not author supplies the branch name and the
// commit subject, and the status line repaints them every few seconds. An escape
// sequence there is executed by the terminal, not printed, and the user has no
// obvious way to tell where it came from.

func TestSanitizeRemovesTerminalCommands(t *testing.T) {
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
			got := Sanitize(c.in)
			if got != c.want {
				t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
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
func TestSanitizeReplacesInvalidUTF8(t *testing.T) {
	in := string([]byte{'a', 0x9b, '2', 'J', 'b'})
	got := Sanitize(in)

	if strings.Contains(got, string([]byte{0x9b})) {
		t.Errorf("raw C1 byte survived: %q", got)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Errorf("surrounding text was lost: %q", got)
	}
}

// Sanitising has to happen before truncation: strip second and a 40-rune cut can
// land inside an escape sequence, which is how a filter gets bypassed.
func TestSanitizeBeforeTruncateLeavesNoEscape(t *testing.T) {
	const max = 40
	long := "\x1b[31m" + strings.Repeat("a", max+10)
	got := Truncate(Sanitize(long), max)

	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("escape survived truncation: %q", got)
	}
	if runes := len([]rune(got)); runes > max {
		t.Errorf("truncated subject is %d runes, want <= %d", runes, max)
	}
}
